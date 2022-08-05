package fs

import (
	"io"
	"net/http"
	"time"

	"github.com/datarhei/core/v16/cluster"
	"github.com/datarhei/core/v16/io/fs"
)

type Filesystem interface {
	fs.Filesystem
}

type filesystem struct {
	fs.Filesystem

	what    string
	cluster cluster.Cluster
}

func NewClusterFS(what string, fs fs.Filesystem, cluster cluster.Cluster) Filesystem {
	f := &filesystem{
		Filesystem: fs,
		what:       what,
		cluster:    cluster,
	}

	return f
}

func (fs *filesystem) Open(path string) fs.File {
	// Check if the file is locally available
	if file := fs.Filesystem.Open(path); file != nil {
		return file
	}

	// Check if the file is available in the cluster
	url, err := fs.cluster.GetURL(fs.what + ":" + path)
	if err != nil {
		return nil
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil
	}

	client := &http.Client{
		Timeout: 15 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil
	}

	file := &file{
		ReadCloser: resp.Body,
		name:       path,
	}

	return file
}

type file struct {
	io.ReadCloser

	name string
}

func (f *file) Name() string {
	return f.name
}

func (f *file) Stat() (fs.FileInfo, error) {
	return f, nil
}

func (f *file) Size() int64 {
	return 0
}

func (f *file) ModTime() time.Time {
	return time.Now()
}

func (f *file) IsLink() (string, bool) {
	return "", false
}

func (f *file) IsDir() bool {
	return false
}
