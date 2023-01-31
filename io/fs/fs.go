// Package fs provides a simple interface for a filesystem
package fs

import (
	"io"
	"time"
)

// FileInfo describes a file and is returned by Stat.
type FileInfo interface {
	// Name returns the full name of the file
	Name() string

	// Size reports the size of the file in bytes
	Size() int64

	// ModTime returns the time of last modification
	ModTime() time.Time

	// IsLink returns the path this file is linking to and true. Otherwise an empty string and false.
	IsLink() (string, bool)

	// IsDir returns whether the file represents a directory
	IsDir() bool
}

// File provides access to a single file.
type File interface {
	io.ReadCloser

	// Name returns the Name of the file
	Name() string

	// Stat returns the FileInfo to this file. In case of an error
	// FileInfo is nil and the error is non-nil.
	Stat() (FileInfo, error)
}

// Filesystem is an interface that provides access to a filesystem.
type Filesystem interface {
	// Name returns the name of this filesystem
	Name() string

	// Base returns the base path of this filesystem
	Base() string

	// Rebase sets a new base path for this filesystem
	Rebase(string) error

	// Type returns the type of this filesystem
	Type() string

	// Size returns the consumed size and capacity of the filesystem in bytes. The
	// capacity is negative if the filesystem can consume as much space as it can.
	Size() (int64, int64)

	// Resize resizes the filesystem to the new size. Files may need to be deleted.
	Resize(size int64)

	// Files returns the current number of files in the filesystem.
	Files() int64

	// Symlink creates newname as a symbolic link to oldname.
	Symlink(oldname, newname string) error

	// Open returns the file stored at the given path. It returns nil if the
	// file doesn't exist.
	Open(path string) File

	// Store adds a file to the filesystem. Returns the size of the data that has been
	// stored in bytes and whether the file is new. The size is negative if there was
	// an error adding the file and error is not nil.
	Store(path string, r io.Reader) (int64, bool, error)

	// Delete removes a file at the given path from the filesystem. Returns the size of
	// the removed file in bytes. The size is negative if the file doesn't exist.
	Delete(path string) int64

	// DeleteAll removes all files from the filesystem. Returns the size of the
	// removed files in bytes.
	DeleteAll() int64

	// List lists all files that are currently on the filesystem.
	List(pattern string) []FileInfo
}
