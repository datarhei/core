// Package fs provides a simple interface for a filesystem
package fs

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"time"
)

var ErrExist = errors.New("file or directory already exists")
var ErrNotExist = errors.New("file or directory does not exist")

// FileInfo describes a file and is returned by Stat.
type FileInfo interface {
	// Name returns the full name of the file.
	Name() string

	// Size reports the size of the file in bytes.
	Size() int64

	// Mode returns the file mode.
	Mode() fs.FileMode

	// ModTime returns the time of last modification.
	ModTime() time.Time

	// IsLink returns the path this file is linking to and true. Otherwise an empty string and false.
	IsLink() (string, bool)

	// IsDir returns whether the file represents a directory.
	IsDir() bool
}

// File provides access to a single file.
type File interface {
	io.ReadSeekCloser

	// Name returns the Name of the file.
	Name() string

	// Stat returns the FileInfo to this file. In case of an error FileInfo is nil
	// and the error is non-nil. If the file is a symlink, the info reports the name and mode
	// of the link itself, but the modification time and size of the linked file.
	Stat() (FileInfo, error)
}

type ListOptions struct {
	Pattern       string
	ModifiedStart *time.Time
	ModifiedEnd   *time.Time
	SizeMin       int64
	SizeMax       int64
}

type ReadFilesystem interface {
	// Size returns the consumed size and capacity of the filesystem in bytes. The
	// capacity is zero or negative if the filesystem can consume as much space as it wants.
	Size() (int64, int64)

	// Files returns the current number of files in the filesystem.
	Files() int64

	// Open returns the file stored at the given path. It returns nil if the
	// file doesn't exist. If the file is a symlink, the name is the name of
	// the link, but it will read the contents of the linked file.
	Open(path string) File

	// ReadFile reads the content of the file at the given path into the writer. Returns
	// the number of bytes read or an error.
	ReadFile(path string) ([]byte, error)

	// Stat returns info about the file at path. If the file doesn't exist, the error
	// ErrNotExist will be returned. If the file is a symlink, the info reports the name
	// and mode of the link itself, but the modification time and size are of the linked file.
	Stat(path string) (FileInfo, error)

	// List lists all files that are currently on the filesystem.
	List(path string, options ListOptions) []FileInfo

	// LookPath searches for an executable named file in the directories named by the PATH environment
	// variable. If file contains a slash, it is tried directly and the PATH is not consulted. Otherwise,
	// on success, the result is an absolute path. On non-disk filesystems. Only the mere existence
	// of that file is verfied. In case the file is not found, the error ErrNotExist will be returned.
	LookPath(file string) (string, error)
}

type WriteFilesystem interface {
	// Symlink creates newname as a symbolic link to oldname. Return ErrNotExist if oldname doesn't exist.
	// If newname already exists, ErrExits will be returned.
	Symlink(oldname, newname string) error

	// WriteFileReader adds a file to the filesystem. Returns the size of the data that has been
	// stored in bytes and whether the file is new. The size is negative if there was
	// an error adding the file and error is not nil. The size parameter is to suggest a size
	// for the file to write. Use a negative value if the size is unknown.
	WriteFileReader(path string, r io.Reader, size int) (int64, bool, error)

	// WriteFile adds a file to the filesystem. Returns the size of the data that has been
	// stored in bytes and whether the file is new. The size is negative if there was
	// an error adding the file and error is not nil.
	WriteFile(path string, data []byte) (int64, bool, error)

	// WriteFileSafe adds a file to the filesystem by first writing it to a tempfile and then
	// renaming it to the actual path. Returns the size of the data that has been
	// stored in bytes and whether the file is new. The size is negative if there was
	// an error adding the file and error is not nil.
	WriteFileSafe(path string, data []byte) (int64, bool, error)

	// AppendFileReader appends the contents from reader to the file at path. If the file doesn't
	// exist, it will be created. The number of written bytes will be returned, -1 otherwise.
	AppendFileReader(path string, r io.Reader, size int) (int64, error)

	// MkdirAll creates a directory named path, along with any necessary parents, and returns nil,
	// or else returns an error. The permission bits perm (before umask) are used for all directories
	// that MkdirAll creates. If path is already a directory, MkdirAll does nothing and returns nil.
	// If the path already exists and is a regular file, ErrExists will be returned.
	MkdirAll(path string, perm os.FileMode) error

	// Rename renames the file from src to dst. If src and dst can't be renamed
	// regularly, the data is copied from src to dst. dst will be overwritten
	// if it already exists. src will be removed after all data has been copied
	// successfully. Both files exist during copying.
	Rename(src, dst string) error

	// Copy copies a file from src to dst.
	Copy(src, dst string) error

	// Remove removes a file at the given path from the filesystem. Returns the size of
	// the removed file in bytes. The size is negative if the file doesn't exist.
	Remove(path string) int64

	// RemoveList removes all files from the filesystem. Returns a list of the names of
	// the removed file and the total size of all removed files in bytes.
	RemoveList(path string, options ListOptions) ([]string, int64)
}

// Filesystem is an interface that provides access to a filesystem.
type Filesystem interface {
	ReadFilesystem
	WriteFilesystem

	// Name returns the name of the filesystem.
	Name() string

	// Type returns the type of the filesystem, e.g. disk, mem, s3
	Type() string

	Metadata(key string) string
	SetMetadata(key string, data string)
}
