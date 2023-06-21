package fs

import (
	"fmt"
	"io"
	gofs "io/fs"
	"time"

	"github.com/datarhei/core/v16/cluster/proxy"
	"github.com/datarhei/core/v16/io/fs"
)

type Filesystem interface {
	fs.Filesystem
}

type filesystem struct {
	fs.Filesystem

	name  string
	proxy proxy.ProxyReader
}

func NewClusterFS(name string, fs fs.Filesystem, proxy proxy.ProxyReader) Filesystem {
	if proxy == nil {
		return fs
	}

	f := &filesystem{
		Filesystem: fs,
		name:       name,
		proxy:      proxy,
	}

	return f
}

func (fs *filesystem) Open(path string) fs.File {
	// Check if the file is locally available
	if file := fs.Filesystem.Open(path); file != nil {
		return file
	}

	// Check if the file is available in the cluster
	size, lastModified, err := fs.proxy.GetFileInfo(fs.name, path)
	if err != nil {
		return nil
	}

	file := &file{
		getFile: func(offset int64) (io.ReadCloser, error) {
			return fs.proxy.GetFile(fs.name, path, offset)
		},
		name:          path,
		size:          size,
		lastModiefied: lastModified,
	}

	return file
}

type file struct {
	io.ReadCloser

	getFile       func(offset int64) (io.ReadCloser, error)
	name          string
	size          int64
	lastModiefied time.Time
}

func (f *file) Read(p []byte) (int, error) {
	if f.ReadCloser == nil {
		data, err := f.getFile(0)
		if err != nil {
			return 0, err
		}

		f.ReadCloser = data
	}

	return f.ReadCloser.Read(p)
}

func (f *file) Close() error {
	if f.ReadCloser == nil {
		return nil
	}

	return f.ReadCloser.Close()
}

func (f *file) Seek(offset int64, whence int) (int64, error) {
	if whence != io.SeekStart {
		return 0, fmt.Errorf("not implemented")
	}

	if f.ReadCloser != nil {
		f.ReadCloser.Close()
		f.ReadCloser = nil
	}

	if f.ReadCloser == nil {
		data, err := f.getFile(offset)
		if err != nil {
			return 0, err
		}

		f.ReadCloser = data
	}

	return offset, nil
}

func (f *file) Name() string {
	return f.name
}

func (f *file) Stat() (fs.FileInfo, error) {
	return f, nil
}

func (f *file) Mode() gofs.FileMode {
	return gofs.FileMode(gofs.ModePerm)
}

func (f *file) Size() int64 {
	return f.size
}

func (f *file) ModTime() time.Time {
	return f.lastModiefied
}

func (f *file) IsLink() (string, bool) {
	return "", false
}

func (f *file) IsDir() bool {
	return false
}
