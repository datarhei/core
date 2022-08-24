package fs

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"sync"
	"time"

	"github.com/datarhei/core/v16/glob"
	"github.com/datarhei/core/v16/log"
)

// MemConfig is the config that is required for creating
// a new memory filesystem.
type MemConfig struct {
	// Namee is the name of the filesystem
	Name string

	// Base is the base path to be reported for this filesystem
	Base string

	// Size is the capacity of the filesystem in bytes
	Size int64

	// Set true to automatically delete the oldest files until there's
	// enough space to store a new file
	Purge bool

	// For logging, optional
	Logger log.Logger
}

type memFileInfo struct {
	name    string
	size    int64
	lastMod time.Time
	linkTo  string
}

func (f *memFileInfo) Name() string {
	return f.name
}

func (f *memFileInfo) Size() int64 {
	return f.size
}

func (f *memFileInfo) ModTime() time.Time {
	return f.lastMod
}

func (f *memFileInfo) IsLink() (string, bool) {
	return f.linkTo, len(f.linkTo) != 0
}

func (f *memFileInfo) IsDir() bool {
	return false
}

type memFile struct {
	// Name of the file
	name string

	// Size of the file in bytes
	size int64

	// Last modification of the file as a UNIX timestamp
	lastMod time.Time

	// Contents of the file
	data *bytes.Buffer

	// Link to another file
	linkTo string
}

func (f *memFile) Name() string {
	return f.name
}

func (f *memFile) Stat() (FileInfo, error) {
	info := &memFileInfo{
		name:    f.name,
		size:    f.size,
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

func (f *memFile) Close() error {
	if f.data == nil {
		return io.EOF
	}

	f.data = nil

	return nil
}

type memFilesystem struct {
	name string
	base string

	// Mapping of path to file
	files map[string]*memFile

	// Mutex for the files map
	filesLock sync.RWMutex

	// Pool for the storage of the contents of files
	dataPool sync.Pool

	// Max. size of the filesystem in bytes as
	// given by the config
	maxSize int64

	// Current size of the filesystem in bytes
	currentSize int64

	// Purge setting from the config
	purge bool

	// Logger from the config
	logger log.Logger
}

// NewMemFilesystem creates a new filesystem in memory that implements
// the Filesystem interface.
func NewMemFilesystem(config MemConfig) Filesystem {
	fs := &memFilesystem{
		name:    config.Name,
		base:    config.Base,
		maxSize: config.Size,
		purge:   config.Purge,
		logger:  config.Logger,
	}

	if fs.logger == nil {
		fs.logger = log.New("")
	}

	fs.logger = fs.logger.WithField("type", "mem")

	fs.files = make(map[string]*memFile)

	fs.dataPool = sync.Pool{
		New: func() interface{} {
			return new(bytes.Buffer)
		},
	}

	fs.logger.WithFields(log.Fields{
		"size_bytes": fs.maxSize,
		"purge":      fs.purge,
	}).Debug().Log("Created")

	return fs
}

func (fs *memFilesystem) Name() string {
	return fs.name
}

func (fs *memFilesystem) Base() string {
	return fs.base
}

func (fs *memFilesystem) Rebase(base string) error {
	fs.base = base

	return nil
}

func (fs *memFilesystem) Type() string {
	return "memfs"
}

func (fs *memFilesystem) Size() (int64, int64) {
	fs.filesLock.RLock()
	defer fs.filesLock.RUnlock()

	return fs.currentSize, fs.maxSize
}

func (fs *memFilesystem) Resize(size int64) {
	fs.filesLock.Lock()
	defer fs.filesLock.Unlock()

	diffSize := fs.maxSize - size

	if diffSize == 0 {
		return
	}

	if diffSize > 0 {
		fs.free(diffSize)
	}

	fs.logger.WithFields(log.Fields{
		"from_bytes": fs.maxSize,
		"to_bytes":   size,
	}).Debug().Log("Resizing")

	fs.maxSize = size
}

func (fs *memFilesystem) Files() int64 {
	fs.filesLock.RLock()
	defer fs.filesLock.RUnlock()

	return int64(len(fs.files))
}

func (fs *memFilesystem) Open(path string) File {
	fs.filesLock.RLock()
	file, ok := fs.files[path]
	fs.filesLock.RUnlock()

	if !ok {
		return nil
	}

	newFile := &memFile{
		name:    file.name,
		size:    file.size,
		lastMod: file.lastMod,
		linkTo:  file.linkTo,
	}

	if file.data != nil {
		newFile.data = bytes.NewBuffer(file.data.Bytes())
	}

	return newFile
}

func (fs *memFilesystem) Symlink(oldname, newname string) error {
	fs.filesLock.Lock()
	defer fs.filesLock.Unlock()

	if _, ok := fs.files[newname]; ok {
		return fmt.Errorf("%s already exist", newname)
	}

	if oldname[0] != '/' {
		oldname = "/" + oldname
	}

	if file, ok := fs.files[oldname]; ok {
		if len(file.linkTo) != 0 {
			return fmt.Errorf("%s can't link to another link (%s)", newname, oldname)
		}
	}

	newFile := &memFile{
		name:    newname,
		size:    0,
		lastMod: time.Now(),
		data:    nil,
		linkTo:  oldname,
	}

	fs.files[newname] = newFile

	return nil
}

func (fs *memFilesystem) Store(path string, r io.Reader) (int64, bool, error) {
	newFile := &memFile{
		name:    path,
		size:    0,
		lastMod: time.Now(),
		data:    nil,
	}

	data := fs.dataPool.Get().(*bytes.Buffer)
	data.Reset()

	size, err := data.ReadFrom(r)
	if err != nil {
		fs.logger.WithFields(log.Fields{
			"path":           path,
			"filesize_bytes": size,
			"error":          err,
		}).Warn().Log("Incomplete file")
	}
	newFile.size = size
	newFile.data = data

	// reject if the new file is larger than the available space
	if fs.maxSize > 0 && newFile.size > fs.maxSize {
		fs.dataPool.Put(data)
		return -1, false, fmt.Errorf("File is too big")
	}

	fs.filesLock.Lock()
	defer fs.filesLock.Unlock()

	// calculate the new size of the filesystem
	newSize := fs.currentSize + newFile.size

	file, replace := fs.files[path]
	if replace {
		newSize -= file.size
	}

	if fs.maxSize > 0 {
		if newSize > fs.maxSize {
			if !fs.purge {
				fs.dataPool.Put(data)
				return -1, false, fmt.Errorf("not enough space on device")
			}

			if replace {
				delete(fs.files, path)
				fs.currentSize -= file.size

				fs.dataPool.Put(file.data)
				file.data = nil
			}

			newSize -= fs.free(fs.currentSize + newFile.size - fs.maxSize)
		}
	} else {
		if replace {
			delete(fs.files, path)

			fs.dataPool.Put(file.data)
			file.data = nil
		}
	}

	fs.currentSize = newSize
	fs.files[path] = newFile

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

func (fs *memFilesystem) free(size int64) int64 {
	files := []*memFile{}

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

func (fs *memFilesystem) Delete(path string) int64 {
	fs.filesLock.Lock()
	defer fs.filesLock.Unlock()

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

func (fs *memFilesystem) DeleteAll() int64 {
	fs.filesLock.Lock()
	defer fs.filesLock.Unlock()

	size := fs.currentSize

	fs.files = make(map[string]*memFile)
	fs.currentSize = 0

	return size
}

func (fs *memFilesystem) List(pattern string) []FileInfo {
	files := []FileInfo{}

	fs.filesLock.RLock()
	defer fs.filesLock.RUnlock()

	for _, file := range fs.files {
		if len(pattern) != 0 {
			if ok, _ := glob.Match(pattern, file.name, '/'); !ok {
				continue
			}
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
