package coreclient

import (
	"io"

	"github.com/datarhei/core-client-go/v16/api"
)

func (r *restclient) MemFSList(sort, order string) ([]api.FileInfo, error) {
	return r.FilesystemList("mem", "", sort, order)
}

func (r *restclient) MemFSHasFile(path string) bool {
	return r.FilesystemHasFile("mem", path)
}

func (r *restclient) MemFSGetFile(path string) (io.ReadCloser, error) {
	return r.FilesystemGetFile("mem", path)
}

func (r *restclient) MemFSDeleteFile(path string) error {
	return r.FilesystemDeleteFile("mem", path)
}

func (r *restclient) MemFSAddFile(path string, data io.Reader) error {
	return r.FilesystemAddFile("mem", path, data)
}
