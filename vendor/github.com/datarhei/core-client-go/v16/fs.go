package coreclient

import (
	"encoding/json"
	"io"
	"net/url"
	"path/filepath"

	"github.com/datarhei/core-client-go/v16/api"
)

const (
	SORT_DEFAULT  = "none"
	SORT_NONE     = "none"
	SORT_NAME     = "name"
	SORT_SIZE     = "size"
	SORT_LASTMOD  = "lastmod"
	ORDER_DEFAULT = "asc"
	ORDER_ASC     = "asc"
	ORDER_DESC    = "desc"
)

func (r *restclient) FilesystemList(name, pattern, sort, order string) ([]api.FileInfo, error) {
	var files []api.FileInfo

	query := &url.Values{}
	query.Set("glob", pattern)
	query.Set("sort", sort)
	query.Set("order", order)

	data, err := r.call("GET", "/v3/fs/"+url.PathEscape(name), query, "", nil)
	if err != nil {
		return files, err
	}

	err = json.Unmarshal(data, &files)

	return files, err
}

func (r *restclient) FilesystemHasFile(name, path string) bool {
	if !filepath.IsAbs(path) {
		path = "/" + path
	}

	_, err := r.call("HEAD", "/v3/fs/"+url.PathEscape(name)+path, nil, "", nil)

	return err == nil
}

func (r *restclient) FilesystemGetFile(name, path string) (io.ReadCloser, error) {
	if !filepath.IsAbs(path) {
		path = "/" + path
	}

	return r.stream("GET", "/v3/fs/"+url.PathEscape(name)+path, nil, "", nil)
}

func (r *restclient) FilesystemDeleteFile(name, path string) error {
	if !filepath.IsAbs(path) {
		path = "/" + path
	}

	_, err := r.call("DELETE", "/v3/fs/"+url.PathEscape(name)+path, nil, "", nil)

	return err
}

func (r *restclient) FilesystemAddFile(name, path string, data io.Reader) error {
	if !filepath.IsAbs(path) {
		path = "/" + path
	}

	_, err := r.call("PUT", "/v3/fs/"+url.PathEscape(name)+path, nil, "application/data", data)

	return err
}
