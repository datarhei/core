package fs

import (
	"bytes"
	"errors"
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
	Logger  log.Logger // For logging, optional
	Storage string
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

type memFile struct {
	memFileInfo
	data *bytes.Buffer // Contents of the file
	r    *bytes.Reader
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
	if f.r == nil {
		return 0, io.EOF
	}

	return f.r.Read(p)
}

func (f *memFile) Seek(offset int64, whence int) (int64, error) {
	if f.r == nil {
		return 0, io.EOF
	}

	return f.r.Seek(offset, whence)
}

func (f *memFile) Close() error {
	if f.data == nil {
		return io.EOF
	}

	f.r = nil
	f.data = nil

	return nil
}

type memFilesystem struct {
	metadata map[string]string
	metaLock sync.RWMutex

	// Current size of the filesystem in bytes and its mutex
	currentSize int64
	sizeLock    sync.RWMutex

	// Logger from the config
	logger log.Logger

	// Storage backend
	storage memStorage
	dirs    *dirStorage
}

type dirStorage struct {
	dirs map[string]uint64
	lock sync.RWMutex
}

func newDirStorage() *dirStorage {
	s := &dirStorage{
		dirs: map[string]uint64{},
	}

	s.dirs["/"] = 1

	return s
}

func (s *dirStorage) Has(path string) bool {
	s.lock.RLock()
	defer s.lock.RUnlock()

	_, hasDir := s.dirs[path]

	return hasDir
}

func (s *dirStorage) Add(path string) {
	dir := filepath.Dir(path)
	elements := strings.Split(dir, "/")

	s.lock.Lock()
	defer s.lock.Unlock()

	p := "/"
	for _, e := range elements {
		p = filepath.Join(p, e)
		n := s.dirs[p]
		n++
		s.dirs[p] = n
	}
}

func (s *dirStorage) Remove(path string) {
	dir := filepath.Dir(path)
	elements := strings.Split(dir, "/")

	s.lock.Lock()
	defer s.lock.Unlock()

	p := "/"
	for _, e := range elements {
		p = filepath.Join(p, e)
		n := s.dirs[p]
		n--
		if n == 0 {
			delete(s.dirs, p)
		} else {
			s.dirs[p] = n
		}
	}
}

// NewMemFilesystem creates a new filesystem in memory that implements
// the Filesystem interface.
func NewMemFilesystem(config MemConfig) (Filesystem, error) {
	fs := &memFilesystem{
		metadata: map[string]string{},
		logger:   config.Logger,
		dirs:     newDirStorage(),
	}

	if fs.logger == nil {
		fs.logger = log.New("")
	}

	fs.logger = fs.logger.WithField("type", "mem")

	if config.Storage == "map" {
		fs.storage = newMapStorage()
	} else if config.Storage == "swiss" {
		fs.storage = newSwissMapStorage()
	} else {
		fs.storage = newMapOfStorage()
	}

	fs.logger.Debug().Log("Created")

	return fs, nil
}

func NewMemFilesystemFromDir(dir string, config MemConfig) (Filesystem, error) {
	mem, err := NewMemFilesystem(config)
	if err != nil {
		return nil, err
	}

	dir = filepath.Clean(dir)

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

		_, _, err = mem.WriteFileReader(strings.TrimPrefix(path, dir), file, int(info.Size()))
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
	fs.sizeLock.RLock()
	defer fs.sizeLock.RUnlock()

	return fs.currentSize, -1
}

func (fs *memFilesystem) Files() int64 {
	nfiles := int64(0)

	fs.storage.Range(func(key string, f *memFile) bool {
		if f.dir {
			return true
		}

		nfiles++

		return true
	})

	return nfiles
}

func (fs *memFilesystem) Open(path string) File {
	path = fs.cleanPath(path)

	file, ok := fs.storage.LoadAndCopy(path)
	if !ok {
		return nil
	}

	newFile := &memFile{
		memFileInfo: memFileInfo{
			name:    file.name,
			lastMod: file.lastMod,
			linkTo:  file.linkTo,
		},
	}

	if len(file.linkTo) != 0 {
		file.Close()

		file, ok = fs.storage.LoadAndCopy(file.linkTo)
		if !ok {
			return nil
		}
	}

	newFile.lastMod = file.lastMod
	newFile.data = file.data
	newFile.size = file.size
	newFile.r = bytes.NewReader(file.data.Bytes())

	return newFile
}

func (fs *memFilesystem) ReadFile(path string) ([]byte, error) {
	path = fs.cleanPath(path)

	file, ok := fs.storage.LoadAndCopy(path)
	if !ok {
		return nil, ErrNotExist
	}

	if len(file.linkTo) != 0 {
		file.Close()

		file, ok = fs.storage.LoadAndCopy(file.linkTo)
		if !ok {
			return nil, ErrNotExist
		}
	}

	defer file.Close()
	return file.data.Bytes(), nil
}

func (fs *memFilesystem) Symlink(oldname, newname string) error {
	oldname = fs.cleanPath(oldname)
	newname = fs.cleanPath(newname)

	if fs.storage.Has(newname) {
		return ErrExist
	}

	oldFile, ok := fs.storage.Load(oldname)
	if !ok {
		return ErrNotExist
	}

	if len(oldFile.linkTo) != 0 {
		return fmt.Errorf("%s can't link to another link (%s)", newname, oldname)
	}

	newFile := &memFile{
		memFileInfo: memFileInfo{
			name:    newname,
			dir:     false,
			size:    0,
			lastMod: time.Now(),
			linkTo:  oldname,
		},
	}

	oldFile, replaced := fs.storage.Store(newname, newFile)

	if !replaced {
		fs.dirs.Add(newname)
	}

	fs.sizeLock.Lock()
	defer fs.sizeLock.Unlock()

	if replaced {
		oldFile.Close()
		fs.currentSize -= oldFile.size
	}

	fs.currentSize += newFile.size

	return nil
}

func copyToBufferFromReader(buf *bytes.Buffer, r io.Reader, _ int) (int64, error) {
	chunkData := [128 * 1024]byte{}
	chunk := chunkData[0:]

	size := int64(0)

	for {
		n, err := r.Read(chunk)
		if n != 0 {
			buf.Write(chunk[:n])
			size += int64(n)
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				return size, nil
			}

			return size, err
		}

		if n == 0 {
			break
		}
	}

	return size, nil
}

func (fs *memFilesystem) WriteFileReader(path string, r io.Reader, sizeHint int) (int64, bool, error) {
	path = fs.cleanPath(path)

	isdir := fs.isDir(path)
	if isdir {
		return -1, false, fmt.Errorf("path not writeable")
	}

	newFile := &memFile{
		memFileInfo: memFileInfo{
			name:    path,
			dir:     false,
			size:    0,
			lastMod: time.Now(),
		},
		data: &bytes.Buffer{},
	}

	if sizeHint > 0 && sizeHint < 5*1024*1024 {
		newFile.data.Grow(sizeHint)
	}

	size, err := copyToBufferFromReader(newFile.data, r, 8*1024)
	if err != nil {
		fs.logger.WithFields(log.Fields{
			"path":           path,
			"filesize_bytes": size,
			"error":          err,
		}).Warn().Log("Incomplete file")

		newFile.Close()

		return -1, false, fmt.Errorf("incomplete file")
	}

	newFile.size = size

	oldFile, replace := fs.storage.Store(path, newFile)

	if !replace {
		fs.dirs.Add(path)
	}

	fs.sizeLock.Lock()
	defer fs.sizeLock.Unlock()

	if replace {
		oldFile.Close()

		fs.currentSize -= oldFile.size
	}

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
	return fs.WriteFileReader(path, bytes.NewReader(data), len(data))
}

func (fs *memFilesystem) WriteFileSafe(path string, data []byte) (int64, bool, error) {
	return fs.WriteFileReader(path, bytes.NewReader(data), len(data))
}

func (fs *memFilesystem) AppendFileReader(path string, r io.Reader, sizeHint int) (int64, error) {
	path = fs.cleanPath(path)

	file, hasFile := fs.storage.LoadAndCopy(path)
	if !hasFile {
		size, _, err := fs.WriteFileReader(path, r, sizeHint)
		return size, err
	}

	size, err := copyToBufferFromReader(file.data, r, 8*1024)
	if err != nil {
		fs.logger.WithFields(log.Fields{
			"path":           path,
			"filesize_bytes": size,
			"error":          err,
		}).Warn().Log("Incomplete file")

		file.Close()

		return -1, fmt.Errorf("incomplete file")
	}

	file.size += size

	fs.storage.Store(path, file)

	fs.sizeLock.Lock()
	defer fs.sizeLock.Unlock()

	fs.currentSize += size

	fs.logger.Debug().WithFields(log.Fields{
		"path":           file.name,
		"filesize_bytes": file.size,
		"size_bytes":     fs.currentSize,
	}).Log("Appended to file")

	return size, nil
}

func (fs *memFilesystem) Purge(size int64) int64 {
	files := []*memFile{}

	fs.storage.Range(func(_ string, file *memFile) bool {
		files = append(files, file)
		return true
	})

	sort.Slice(files, func(i, j int) bool {
		return files[i].lastMod.Before(files[j].lastMod)
	})

	var freed int64 = 0

	for _, f := range files {
		fs.storage.Delete(f.name)
		size -= f.size
		freed += f.size

		fs.dirs.Remove(f.name)

		fs.sizeLock.Lock()
		fs.currentSize -= f.size
		fs.sizeLock.Unlock()

		f.Close()

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

	info, err := fs.stat(path)
	if err == nil {
		if info.IsDir() {
			return nil
		}

		return ErrExist
	}

	fs.dirs.Add(filepath.Join(path, "x"))

	return nil
}

func (fs *memFilesystem) Rename(src, dst string) error {
	src = filepath.Join("/", filepath.Clean(src))
	dst = filepath.Join("/", filepath.Clean(dst))

	if src == dst {
		return nil
	}

	srcFile, ok := fs.storage.Load(src)
	if !ok {
		return ErrNotExist
	}

	dstFile, replace := fs.storage.Store(dst, srcFile)
	fs.storage.Delete(src)

	fs.dirs.Remove(src)
	if !replace {
		fs.dirs.Add(dst)
	}

	fs.sizeLock.Lock()
	defer fs.sizeLock.Unlock()

	if replace {
		dstFile.Close()

		fs.currentSize -= dstFile.size
	}

	return nil
}

func (fs *memFilesystem) Copy(src, dst string) error {
	src = filepath.Join("/", filepath.Clean(src))
	dst = filepath.Join("/", filepath.Clean(dst))

	if src == dst {
		return nil
	}

	if fs.isDir(dst) {
		return os.ErrInvalid
	}

	srcFile, ok := fs.storage.LoadAndCopy(src)
	if !ok {
		return ErrNotExist
	}

	if srcFile.dir {
		srcFile.Close()
		return ErrNotExist
	}

	dstFile := &memFile{
		memFileInfo: memFileInfo{
			name:    dst,
			dir:     false,
			size:    srcFile.size,
			lastMod: time.Now(),
		},
		data: srcFile.data,
	}

	f, replace := fs.storage.Store(dst, dstFile)

	if !replace {
		fs.dirs.Add(dst)
	}

	fs.sizeLock.Lock()
	defer fs.sizeLock.Unlock()

	if replace {
		f.Close()
		fs.currentSize -= f.size
	}

	fs.currentSize += dstFile.size

	return nil
}

func (fs *memFilesystem) Stat(path string) (FileInfo, error) {
	path = fs.cleanPath(path)

	return fs.stat(path)
}

func (fs *memFilesystem) stat(path string) (FileInfo, error) {
	file, ok := fs.storage.Load(path)
	if ok {
		f := &memFileInfo{
			name:    file.name,
			size:    file.size,
			dir:     file.dir,
			lastMod: file.lastMod,
			linkTo:  file.linkTo,
		}

		if len(f.linkTo) != 0 {
			file, ok := fs.storage.Load(f.linkTo)
			if !ok {
				return nil, ErrNotExist
			}

			f.lastMod = file.lastMod
			f.size = file.size
		}

		return f, nil
	}

	// Check for directories
	if !fs.isDir(path) {
		return nil, ErrNotExist
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
	return fs.dirs.Has(path)
}

func (fs *memFilesystem) Remove(path string) int64 {
	path = fs.cleanPath(path)

	return fs.remove(path)
}

func (fs *memFilesystem) remove(path string) int64 {
	file, ok := fs.storage.Delete(path)
	if ok {
		file.Close()

		fs.dirs.Remove(path)

		fs.sizeLock.Lock()
		defer fs.sizeLock.Unlock()

		fs.currentSize -= file.size
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

	var compiledPattern glob.Glob
	var err error

	if len(options.Pattern) != 0 {
		compiledPattern, err = glob.Compile(options.Pattern, '/')
		if err != nil {
			return nil, 0
		}
	}

	files := []*memFile{}

	fs.storage.Range(func(key string, file *memFile) bool {
		if file.dir {
			return true
		}

		if !strings.HasPrefix(file.name, path) {
			return true
		}

		if compiledPattern != nil {
			if !compiledPattern.Match(file.name) {
				return true
			}
		}

		if options.ModifiedStart != nil {
			if file.lastMod.Before(*options.ModifiedStart) {
				return true
			}
		}

		if options.ModifiedEnd != nil {
			if file.lastMod.After(*options.ModifiedEnd) {
				return true
			}
		}

		if options.SizeMin > 0 {
			if file.size < options.SizeMin {
				return true
			}
		}

		if options.SizeMax > 0 {
			if file.size > options.SizeMax {
				return true
			}
		}

		files = append(files, file)

		return true
	})

	var size int64 = 0
	names := make([]string, 0, len(files))

	for _, file := range files {
		fs.storage.Delete(file.name)
		size += file.size
		names = append(names, file.name)

		fs.dirs.Remove(file.name)

		file.Close()
	}

	fs.sizeLock.Lock()
	defer fs.sizeLock.Unlock()

	fs.currentSize -= size

	return names, size
}

func (fs *memFilesystem) List(path string, options ListOptions) []FileInfo {
	path = fs.cleanPath(path)
	files := []FileInfo{}

	var compiledPattern glob.Glob
	var err error

	if len(options.Pattern) != 0 {
		compiledPattern, err = glob.Compile(options.Pattern, '/')
		if err != nil {
			return nil
		}
	}

	fs.storage.Range(func(key string, file *memFile) bool {
		if file.dir {
			return true
		}

		if !strings.HasPrefix(file.name, path) {
			return true
		}

		if compiledPattern != nil {
			if !compiledPattern.Match(file.name) {
				return true
			}
		}

		if options.ModifiedStart != nil {
			if file.lastMod.Before(*options.ModifiedStart) {
				return true
			}
		}

		if options.ModifiedEnd != nil {
			if file.lastMod.After(*options.ModifiedEnd) {
				return true
			}
		}

		if options.SizeMin > 0 {
			if file.size < options.SizeMin {
				return true
			}
		}

		if options.SizeMax > 0 {
			if file.size > options.SizeMax {
				return true
			}
		}

		files = append(files, &memFileInfo{
			name:    file.name,
			size:    file.size,
			lastMod: file.lastMod,
			linkTo:  file.linkTo,
		})

		return true
	})

	return files
}

func (fs *memFilesystem) LookPath(file string) (string, error) {
	if strings.Contains(file, "/") {
		file = fs.cleanPath(file)
		info, err := fs.Stat(file)
		if err == nil {
			if !info.Mode().IsRegular() {
				return file, ErrNotExist
			}
			return file, nil
		}
		return "", ErrNotExist
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
				return path, ErrNotExist
			}
			if !info.Mode().IsRegular() {
				return path, ErrNotExist
			}
			return path, nil
		}
	}
	return "", ErrNotExist
}

func (fs *memFilesystem) cleanPath(path string) string {
	if !filepath.IsAbs(path) {
		path = filepath.Join("/", path)
	}

	return filepath.Join("/", filepath.Clean(path))
}
