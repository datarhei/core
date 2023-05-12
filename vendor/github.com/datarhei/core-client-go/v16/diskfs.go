package coreclient

import (
	"io"

	"github.com/datarhei/core-client-go/v16/api"
)

func (r *restclient) DiskFSList(sort, order string) ([]api.FileInfo, error) {
	return r.FilesystemList("disk", "", sort, order)
}

func (r *restclient) DiskFSHasFile(path string) bool {
	return r.FilesystemHasFile("disk", path)
}

func (r *restclient) DiskFSGetFile(path string) (io.ReadCloser, error) {
	return r.FilesystemGetFile("disk", path)
}

func (r *restclient) DiskFSDeleteFile(path string) error {
	return r.FilesystemDeleteFile("disk", path)
}

func (r *restclient) DiskFSAddFile(path string, data io.Reader) error {
	return r.FilesystemAddFile("disk", path, data)
}
