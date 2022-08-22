package fs

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/datarhei/core/v16/glob"
	"github.com/datarhei/core/v16/log"
)

// DiskConfig is the config required to create a new disk
// filesystem.
type DiskConfig struct {
	// Dir is the path to the directory to observe
	Dir string

	// Size of the filesystem in bytes
	Size int64

	// For logging, optional
	Logger log.Logger
}

// diskFileInfo implements the FileInfo interface
type diskFileInfo struct {
	dir   string
	name  string
	finfo os.FileInfo
}

func (fi *diskFileInfo) Name() string {
	return fi.name
}

func (fi *diskFileInfo) Size() int64 {
	return fi.finfo.Size()
}

func (fi *diskFileInfo) ModTime() time.Time {
	return fi.finfo.ModTime()
}

func (fi *diskFileInfo) IsLink() (string, bool) {
	mode := fi.finfo.Mode()
	if mode&os.ModeSymlink == 0 {
		return fi.name, false
	}

	path, err := os.Readlink(filepath.Join(fi.dir, fi.name))
	if err != nil {
		return fi.name, false
	}

	path = filepath.Join(fi.dir, path)

	if !strings.HasPrefix(path, fi.dir) {
		return fi.name, false
	}

	name := strings.TrimPrefix(path, fi.dir)
	if name[0] != os.PathSeparator {
		name = string(os.PathSeparator) + name
	}

	return name, true
}

func (fi *diskFileInfo) IsDir() bool {
	return fi.finfo.IsDir()
}

// diskFile implements the File interface
type diskFile struct {
	dir  string
	name string
	file *os.File
}

func (f *diskFile) Name() string {
	return f.name
}

func (f *diskFile) Stat() (FileInfo, error) {
	finfo, err := f.file.Stat()
	if err != nil {
		return nil, err
	}

	dif := &diskFileInfo{
		dir:   f.dir,
		name:  f.name,
		finfo: finfo,
	}

	return dif, nil
}

func (f *diskFile) Close() error {
	return f.file.Close()
}

func (f *diskFile) Read(p []byte) (int, error) {
	return f.file.Read(p)
}

// diskFilesystem implements the Filesystem interface
type diskFilesystem struct {
	dir string

	// Max. size of the filesystem in bytes as
	// given by the config
	maxSize int64

	// Current size of the filesystem in bytes
	currentSize   int64
	lastSizeCheck time.Time

	// Logger from the config
	logger log.Logger
}

// NewDiskFilesystem returns a new filesystem that is backed by a disk
// that implements the Filesystem interface
func NewDiskFilesystem(config DiskConfig) (Filesystem, error) {
	fs := &diskFilesystem{
		maxSize: config.Size,
		logger:  config.Logger,
	}

	if fs.logger == nil {
		fs.logger = log.New("")
	}

	fs.logger = fs.logger.WithField("type", "disk")

	if err := fs.Rebase(config.Dir); err != nil {
		return nil, err
	}

	return fs, nil
}

func (fs *diskFilesystem) Base() string {
	return fs.dir
}

func (fs *diskFilesystem) Rebase(base string) error {
	if len(base) == 0 {
		return fmt.Errorf("invalid base path provided")
	}

	dir, err := filepath.Abs(base)
	if err != nil {
		return err
	}

	base = dir

	finfo, err := os.Stat(base)
	if err != nil {
		return fmt.Errorf("the provided base path '%s' doesn't exist", fs.dir)
	}

	if !finfo.IsDir() {
		return fmt.Errorf("the provided base path '%s' must be a directory", fs.dir)
	}

	fs.dir = base

	return nil
}

func (fs *diskFilesystem) Type() string {
	return "diskfs"
}

func (fs *diskFilesystem) Size() (int64, int64) {
	// This is to cache the size for some time in order not to
	// stress the underlying filesystem too much.
	if time.Since(fs.lastSizeCheck) >= 10*time.Second {
		var size int64 = 0

		fs.walk(func(path string, info os.FileInfo) {
			size += info.Size()
		})

		fs.currentSize = size

		fs.lastSizeCheck = time.Now()
	}

	return fs.currentSize, fs.maxSize
}

func (fs *diskFilesystem) Resize(size int64) {
	fs.maxSize = size
}

func (fs *diskFilesystem) Files() int64 {
	var nfiles int64 = 0

	fs.walk(func(path string, info os.FileInfo) {
		nfiles++
	})

	return nfiles
}

func (fs *diskFilesystem) Symlink(oldname, newname string) error {
	oldname = filepath.Join(fs.dir, filepath.Clean("/"+oldname))

	if !filepath.IsAbs(newname) {
		return nil
	}

	newname = filepath.Join(fs.dir, filepath.Clean("/"+newname))

	err := os.Symlink(oldname, newname)

	return err
}

func (fs *diskFilesystem) Open(path string) File {
	path = filepath.Join(fs.dir, filepath.Clean("/"+path))

	f, err := os.Open(path)
	if err != nil {
		return nil
	}

	df := &diskFile{
		dir:  fs.dir,
		name: path,
		file: f,
	}

	return df
}

func (fs *diskFilesystem) Store(path string, r io.Reader) (int64, bool, error) {
	path = filepath.Join(fs.dir, filepath.Clean("/"+path))

	replace := true

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return -1, false, fmt.Errorf("creating file failed: %w", err)
	}

	var f *os.File
	var err error

	f, err = os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		f, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			return -1, false, fmt.Errorf("creating file failed: %w", err)
		}

		replace = false
	}

	size, err := f.ReadFrom(r)
	if err != nil {
		return -1, false, fmt.Errorf("reading data failed: %w", err)
	}

	return size, !replace, nil
}

func (fs *diskFilesystem) Delete(path string) int64 {
	path = filepath.Join(fs.dir, filepath.Clean("/"+path))

	finfo, err := os.Stat(path)
	if err != nil {
		return -1
	}

	size := finfo.Size()

	if err := os.Remove(path); err != nil {
		return -1
	}

	return size
}

func (fs *diskFilesystem) DeleteAll() int64 {
	return 0
}

func (fs *diskFilesystem) List(pattern string) []FileInfo {
	files := []FileInfo{}

	fs.walk(func(path string, info os.FileInfo) {
		name := strings.TrimPrefix(path, fs.dir)
		if name[0] != os.PathSeparator {
			name = string(os.PathSeparator) + name
		}

		if len(pattern) != 0 {
			if ok, _ := glob.Match(pattern, name, '/'); !ok {
				return
			}
		}

		files = append(files, &diskFileInfo{
			dir:   fs.dir,
			name:  name,
			finfo: info,
		})
	})

	return files
}

func (fs *diskFilesystem) walk(walkfn func(path string, info os.FileInfo)) {
	filepath.Walk(fs.dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			return nil
		}

		mode := info.Mode()
		if !mode.IsRegular() && mode&os.ModeSymlink == 0 {
			return nil
		}

		walkfn(path, info)

		return nil
	})
}
