package fs

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/datarhei/core/v16/glob"
	"github.com/datarhei/core/v16/log"
)

// MemConfig is the config that is required for creating
// a new memory filesystem.
type MemConfig struct {
	Logger log.Logger // For logging, optional
}

type memFileInfo struct {
	name    string    // Full name of the file (including path)
	size    int64     // The size of the file in bytes
	dir     bool      // Whether this file represents a directory
	lastMod time.Time // The time of the last modification of the file
	linkTo  string    // Where the file links to, empty if it's not a link
}

func (f *memFileInfo) Name() string {
	return f.name
}

func (f *memFileInfo) Size() int64 {
	return f.size
}

func (f *memFileInfo) Mode() fs.FileMode {
	mode := fs.FileMode(fs.ModePerm)

	if f.dir {
		mode |= fs.ModeDir
	}

	if len(f.linkTo) != 0 {
		mode |= fs.ModeSymlink
	}

	return mode
}

func (f *memFileInfo) ModTime() time.Time {
	return f.lastMod
}

func (f *memFileInfo) IsLink() (string, bool) {
	return f.linkTo, len(f.linkTo) != 0
}

func (f *memFileInfo) IsDir() bool {
	return f.dir
}

type internalMemFile struct {
	memFileInfo
	data *bytes.Buffer // Contents of the file
}

type memFile struct {
	memFileInfo
	data *bytes.Reader
}

func (f *memFile) Name() string {
	return f.name
}

func (f *memFile) Stat() (FileInfo, error) {
	info := &memFileInfo{
		name:    f.name,
		size:    f.size,
		dir:     f.dir,
		lastMod: f.lastMod,
		linkTo:  f.linkTo,
	}

	return info, nil
}

func (f *memFile) Read(p []byte) (int, error) {
	if f.data == nil {
		return 0, io.EOF
	}

	return f.data.Read(p)
}

func (f *memFile) Seek(offset int64, whence int) (int64, error) {
	if f.data == nil {
		return 0, io.EOF
	}

	return f.data.Seek(offset, whence)
}

func (f *memFile) Close() error {
	if f.data == nil {
		return io.EOF
	}

	f.data = nil

	return nil
}

type memFilesystem struct {
	metadata map[string]string
	metaLock sync.RWMutex

	// Mapping of path to file
	files map[string]*internalMemFile

	// Mutex for the files map
	filesLock sync.RWMutex

	// Pool for the storage of the contents of files
	dataPool sync.Pool

	// Current size of the filesystem in bytes
	currentSize int64

	// Logger from the config
	logger log.Logger
}

// NewMemFilesystem creates a new filesystem in memory that implements
// the Filesystem interface.
func NewMemFilesystem(config MemConfig) (Filesystem, error) {
	fs := &memFilesystem{
		metadata: make(map[string]string),
		logger:   config.Logger,
	}

	if fs.logger == nil {
		fs.logger = log.New("")
	}

	fs.logger = fs.logger.WithField("type", "mem")

	fs.files = make(map[string]*internalMemFile)

	fs.dataPool = sync.Pool{
		New: func() interface{} {
			return new(bytes.Buffer)
		},
	}

	fs.logger.Debug().Log("Created")

	return fs, nil
}

func NewMemFilesystemFromDir(dir string, config MemConfig) (Filesystem, error) {
	mem, err := NewMemFilesystem(config)
	if err != nil {
		return nil, err
	}

	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			return nil
		}

		mode := info.Mode()
		if !mode.IsRegular() {
			return nil
		}

		if mode&os.ModeSymlink != 0 {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return nil
		}

		defer file.Close()

		_, _, err = mem.WriteFileReader(path, file)
		if err != nil {
			return fmt.Errorf("can't copy %s", path)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return mem, nil
}

func (fs *memFilesystem) Name() string {
	return "mem"
}

func (fs *memFilesystem) Type() string {
	return "mem"
}

func (fs *memFilesystem) Metadata(key string) string {
	fs.metaLock.RLock()
	defer fs.metaLock.RUnlock()

	return fs.metadata[key]
}

func (fs *memFilesystem) SetMetadata(key, data string) {
	fs.metaLock.Lock()
	defer fs.metaLock.Unlock()

	fs.metadata[key] = data
}

func (fs *memFilesystem) Size() (int64, int64) {
	fs.filesLock.RLock()
	defer fs.filesLock.RUnlock()

	return fs.currentSize, -1
}

func (fs *memFilesystem) Files() int64 {
	fs.filesLock.RLock()
	defer fs.filesLock.RUnlock()

	nfiles := int64(0)

	for _, f := range fs.files {
		if f.dir {
			continue
		}

		nfiles++
	}

	return nfiles
}

func (fs *memFilesystem) Open(path string) File {
	path = fs.cleanPath(path)

	fs.filesLock.RLock()
	file, ok := fs.files[path]
	fs.filesLock.RUnlock()

	if !ok {
		return nil
	}

	newFile := &memFile{
		memFileInfo: memFileInfo{
			name:    file.name,
			size:    file.size,
			lastMod: file.lastMod,
			linkTo:  file.linkTo,
		},
	}

	if len(file.linkTo) != 0 {
		file, ok = fs.files[file.linkTo]
		if !ok {
			return nil
		}
	}

	if file.data != nil {
		newFile.lastMod = file.lastMod
		newFile.data = bytes.NewReader(file.data.Bytes())
		newFile.size = int64(newFile.data.Len())
	}

	return newFile
}

func (fs *memFilesystem) ReadFile(path string) ([]byte, error) {
	path = fs.cleanPath(path)

	fs.filesLock.RLock()
	file, ok := fs.files[path]
	fs.filesLock.RUnlock()

	if !ok {
		return nil, os.ErrNotExist
	}

	if len(file.linkTo) != 0 {
		file, ok = fs.files[file.linkTo]
		if !ok {
			return nil, os.ErrNotExist
		}
	}

	if file.data != nil {
		return file.data.Bytes(), nil
	}

	return nil, nil
}

func (fs *memFilesystem) Symlink(oldname, newname string) error {
	oldname = fs.cleanPath(oldname)
	newname = fs.cleanPath(newname)

	fs.filesLock.Lock()
	defer fs.filesLock.Unlock()

	if _, ok := fs.files[oldname]; !ok {
		return os.ErrNotExist
	}

	if _, ok := fs.files[newname]; ok {
		return os.ErrExist
	}

	if file, ok := fs.files[oldname]; ok {
		if len(file.linkTo) != 0 {
			return fmt.Errorf("%s can't link to another link (%s)", newname, oldname)
		}
	}

	newFile := &internalMemFile{
		memFileInfo: memFileInfo{
			name:    newname,
			dir:     false,
			size:    0,
			lastMod: time.Now(),
			linkTo:  oldname,
		},
		data: nil,
	}

	fs.files[newname] = newFile

	return nil
}

func (fs *memFilesystem) WriteFileReader(path string, r io.Reader) (int64, bool, error) {
	path = fs.cleanPath(path)

	fs.filesLock.Lock()
	isdir := fs.isDir(path)
	fs.filesLock.Unlock()

	if isdir {
		return -1, false, fmt.Errorf("path not writeable")
	}

	newFile := &internalMemFile{
		memFileInfo: memFileInfo{
			name:    path,
			dir:     false,
			size:    0,
			lastMod: time.Now(),
		},
		data: fs.dataPool.Get().(*bytes.Buffer),
	}

	newFile.data.Reset()
	size, err := newFile.data.ReadFrom(r)
	if err != nil {
		fs.logger.WithFields(log.Fields{
			"path":           path,
			"filesize_bytes": size,
			"error":          err,
		}).Warn().Log("Incomplete file")

		return -1, false, fmt.Errorf("incomplete file")
	}

	newFile.size = size

	fs.filesLock.Lock()
	defer fs.filesLock.Unlock()

	file, replace := fs.files[path]
	if replace {
		delete(fs.files, path)

		fs.currentSize -= file.size

		fs.dataPool.Put(file.data)
		file.data = nil
	}

	fs.files[path] = newFile

	fs.currentSize += newFile.size

	logger := fs.logger.WithFields(log.Fields{
		"path":           newFile.name,
		"filesize_bytes": newFile.size,
		"size_bytes":     fs.currentSize,
	})

	if replace {
		logger.Debug().Log("Replaced file")
	} else {
		logger.Debug().Log("Added file")
	}

	return newFile.size, !replace, nil
}

func (fs *memFilesystem) WriteFile(path string, data []byte) (int64, bool, error) {
	return fs.WriteFileReader(path, bytes.NewReader(data))
}

func (fs *memFilesystem) WriteFileSafe(path string, data []byte) (int64, bool, error) {
	return fs.WriteFileReader(path, bytes.NewReader(data))
}

func (fs *memFilesystem) Purge(size int64) int64 {
	fs.filesLock.Lock()
	defer fs.filesLock.Unlock()

	files := []*internalMemFile{}

	for _, f := range fs.files {
		files = append(files, f)
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].lastMod.Before(files[j].lastMod)
	})

	var freed int64 = 0

	for _, f := range files {
		delete(fs.files, f.name)
		size -= f.size
		freed += f.size
		fs.currentSize -= f.size

		fs.dataPool.Put(f.data)
		f.data = nil

		fs.logger.WithFields(log.Fields{
			"path":           f.name,
			"filesize_bytes": f.size,
			"size_bytes":     fs.currentSize,
		}).Debug().Log("Purged file")

		if size <= 0 {
			break
		}
	}

	files = nil

	return freed
}

func (fs *memFilesystem) MkdirAll(path string, perm os.FileMode) error {
	path = fs.cleanPath(path)

	fs.filesLock.Lock()
	defer fs.filesLock.Unlock()

	info, err := fs.stat(path)
	if err == nil {
		if info.IsDir() {
			return nil
		}

		return os.ErrExist
	}

	f := &internalMemFile{
		memFileInfo: memFileInfo{
			name:    path,
			size:    0,
			dir:     true,
			lastMod: time.Now(),
		},
		data: nil,
	}

	fs.files[path] = f

	return nil
}

func (fs *memFilesystem) Rename(src, dst string) error {
	src = filepath.Join("/", filepath.Clean(src))
	dst = filepath.Join("/", filepath.Clean(dst))

	if src == dst {
		return nil
	}

	fs.filesLock.Lock()
	defer fs.filesLock.Unlock()

	srcFile, ok := fs.files[src]
	if !ok {
		return os.ErrNotExist
	}

	dstFile, ok := fs.files[dst]
	if ok {
		fs.currentSize -= dstFile.size

		fs.dataPool.Put(dstFile.data)
		dstFile.data = nil
	}

	fs.files[dst] = srcFile
	delete(fs.files, src)

	return nil
}

func (fs *memFilesystem) Copy(src, dst string) error {
	src = filepath.Join("/", filepath.Clean(src))
	dst = filepath.Join("/", filepath.Clean(dst))

	if src == dst {
		return nil
	}

	fs.filesLock.Lock()
	defer fs.filesLock.Unlock()

	srcFile, ok := fs.files[src]
	if !ok {
		return os.ErrNotExist
	}

	if srcFile.dir {
		return os.ErrNotExist
	}

	if fs.isDir(dst) {
		return os.ErrInvalid
	}

	dstFile, ok := fs.files[dst]
	if ok {
		fs.currentSize -= dstFile.size
	} else {
		dstFile = &internalMemFile{
			memFileInfo: memFileInfo{
				name:    dst,
				dir:     false,
				size:    srcFile.size,
				lastMod: time.Now(),
			},
			data: fs.dataPool.Get().(*bytes.Buffer),
		}
	}

	dstFile.data.Reset()
	dstFile.data.Write(srcFile.data.Bytes())

	fs.currentSize += dstFile.size

	fs.files[dst] = dstFile

	return nil
}

func (fs *memFilesystem) Stat(path string) (FileInfo, error) {
	path = fs.cleanPath(path)

	fs.filesLock.RLock()
	defer fs.filesLock.RUnlock()

	return fs.stat(path)
}

func (fs *memFilesystem) stat(path string) (FileInfo, error) {
	file, ok := fs.files[path]
	if ok {
		f := &memFileInfo{
			name:    file.name,
			size:    file.size,
			dir:     file.dir,
			lastMod: file.lastMod,
			linkTo:  file.linkTo,
		}

		if len(f.linkTo) != 0 {
			file, ok := fs.files[f.linkTo]
			if !ok {
				return nil, os.ErrNotExist
			}

			f.lastMod = file.lastMod
			f.size = file.size
		}

		return f, nil
	}

	// Check for directories
	if !fs.isDir(path) {
		return nil, os.ErrNotExist
	}

	f := &memFileInfo{
		name:    path,
		size:    0,
		dir:     true,
		lastMod: time.Now(),
		linkTo:  "",
	}

	return f, nil
}

func (fs *memFilesystem) isDir(path string) bool {
	file, ok := fs.files[path]
	if ok {
		return file.dir
	}

	if !strings.HasSuffix(path, "/") {
		path = path + "/"
	}

	if path == "/" {
		return true
	}

	for k := range fs.files {
		if strings.HasPrefix(k, path) {
			return true
		}
	}

	return false
}

func (fs *memFilesystem) Remove(path string) int64 {
	path = fs.cleanPath(path)

	fs.filesLock.Lock()
	defer fs.filesLock.Unlock()

	return fs.remove(path)
}

func (fs *memFilesystem) remove(path string) int64 {
	file, ok := fs.files[path]
	if ok {
		delete(fs.files, path)
		fs.currentSize -= file.size

		fs.dataPool.Put(file.data)
		file.data = nil
	} else {
		return -1
	}

	fs.logger.WithFields(log.Fields{
		"path":           file.name,
		"filesize_bytes": file.size,
		"size_bytes":     fs.currentSize,
	}).Debug().Log("Removed file")

	return file.size
}

func (fs *memFilesystem) RemoveList(path string, options ListOptions) ([]string, int64) {
	path = fs.cleanPath(path)

	fs.filesLock.Lock()
	defer fs.filesLock.Unlock()

	var size int64 = 0
	files := []string{}

	for _, file := range fs.files {
		if !strings.HasPrefix(file.name, path) {
			continue
		}

		if len(options.Pattern) != 0 {
			if ok, _ := glob.Match(options.Pattern, file.name, '/'); !ok {
				continue
			}
		}

		if options.ModifiedStart != nil {
			if file.lastMod.Before(*options.ModifiedStart) {
				continue
			}
		}

		if options.ModifiedEnd != nil {
			if file.lastMod.After(*options.ModifiedEnd) {
				continue
			}
		}

		if options.SizeMin > 0 {
			if file.size < options.SizeMin {
				continue
			}
		}

		if options.SizeMax > 0 {
			if file.size > options.SizeMax {
				continue
			}
		}

		if file.dir {
			continue
		}

		size += fs.remove(file.name)

		files = append(files, file.name)
	}

	return files, size
}

func (fs *memFilesystem) List(path string, options ListOptions) []FileInfo {
	path = fs.cleanPath(path)
	files := []FileInfo{}

	fs.filesLock.RLock()
	defer fs.filesLock.RUnlock()

	for _, file := range fs.files {
		if !strings.HasPrefix(file.name, path) {
			continue
		}

		if len(options.Pattern) != 0 {
			if ok, _ := glob.Match(options.Pattern, file.name, '/'); !ok {
				continue
			}
		}

		if options.ModifiedStart != nil {
			if file.lastMod.Before(*options.ModifiedStart) {
				continue
			}
		}

		if options.ModifiedEnd != nil {
			if file.lastMod.After(*options.ModifiedEnd) {
				continue
			}
		}

		if options.SizeMin > 0 {
			if file.size < options.SizeMin {
				continue
			}
		}

		if options.SizeMax > 0 {
			if file.size > options.SizeMax {
				continue
			}
		}

		if file.dir {
			continue
		}

		files = append(files, &memFileInfo{
			name:    file.name,
			size:    file.size,
			lastMod: file.lastMod,
			linkTo:  file.linkTo,
		})
	}

	return files
}

func (fs *memFilesystem) LookPath(file string) (string, error) {
	if strings.Contains(file, "/") {
		file = fs.cleanPath(file)
		info, err := fs.Stat(file)
		if err == nil {
			if !info.Mode().IsRegular() {
				return file, os.ErrNotExist
			}
			return file, nil
		}
		return "", os.ErrNotExist
	}
	path := os.Getenv("PATH")
	for _, dir := range filepath.SplitList(path) {
		if dir == "" {
			// Unix shell semantics: path element "" means "."
			dir = "."
		}
		path := filepath.Join(dir, file)
		path = fs.cleanPath(path)
		if info, err := fs.Stat(path); err == nil {
			if !filepath.IsAbs(path) {
				return path, os.ErrNotExist
			}
			if !info.Mode().IsRegular() {
				return path, os.ErrNotExist
			}
			return path, nil
		}
	}
	return "", os.ErrNotExist
}

func (fs *memFilesystem) cleanPath(path string) string {
	if !filepath.IsAbs(path) {
		path = filepath.Join("/", path)
	}

	return filepath.Join("/", filepath.Clean(path))
}
