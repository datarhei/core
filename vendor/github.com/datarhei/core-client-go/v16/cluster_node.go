package coreclient

import (
	"context"
	"io"
	"net/url"
	"path/filepath"

	"github.com/goccy/go-json"

	"github.com/datarhei/core-client-go/v16/api"
)

func (r *restclient) ClusterNodeList() ([]api.ClusterNode, error) {
	var nodes []api.ClusterNode

	data, err := r.call("GET", "/v3/cluster/node", nil, nil, "", nil)
	if err != nil {
		return nodes, err
	}

	err = json.Unmarshal(data, &nodes)

	return nodes, err
}

func (r *restclient) ClusterNode(id string) (api.ClusterNode, error) {
	var node api.ClusterNode

	data, err := r.call("GET", "/v3/cluster/node/"+url.PathEscape(id), nil, nil, "", nil)
	if err != nil {
		return node, err
	}

	err = json.Unmarshal(data, &node)

	return node, err
}

func (r *restclient) ClusterNodeFiles(id string) (api.ClusterNodeFiles, error) {
	var files api.ClusterNodeFiles

	data, err := r.call("GET", "/v3/cluster/node/"+url.PathEscape(id)+"/files", nil, nil, "", nil)
	if err != nil {
		return files, err
	}

	err = json.Unmarshal(data, &files)

	return files, err
}

func (r *restclient) ClusterNodeFilesystemList(id, storage, pattern, sort, order string) ([]api.FileInfo, error) {
	var files []api.FileInfo

	query := &url.Values{}
	query.Set("glob", pattern)
	query.Set("sort", sort)
	query.Set("order", order)

	data, err := r.call("GET", "/v3/cluster/node/"+url.PathEscape(id)+"/fs/"+url.PathEscape(storage), query, nil, "", nil)
	if err != nil {
		return files, err
	}

	err = json.Unmarshal(data, &files)

	return files, err
}

func (r *restclient) ClusterNodeFilesystemPutFile(id, storage, path string, data io.Reader) error {
	if !filepath.IsAbs(path) {
		path = "/" + path
	}

	_, err := r.call("PUT", "/v3/cluster/node/"+url.PathEscape(id)+"/fs/"+url.PathEscape(storage)+path, nil, nil, "", data)

	return err
}

func (r *restclient) ClusterNodeFilesystemGetFile(id, storage, path string) (io.ReadCloser, error) {
	if !filepath.IsAbs(path) {
		path = "/" + path
	}

	return r.stream(context.Background(), "GET", "/v3/cluster/node/"+url.PathEscape(id)+"/fs/"+url.PathEscape(storage)+path, nil, nil, "", nil)
}

func (r *restclient) ClusterNodeFilesystemDeleteFile(id, storage, path string) error {
	if !filepath.IsAbs(path) {
		path = "/" + path
	}

	_, err := r.call("DELETE", "/v3/cluster/node/"+url.PathEscape(id)+"/fs/"+url.PathEscape(storage)+path, nil, nil, "", nil)

	return err
}

func (r *restclient) ClusterNodeProcessList(id string, opts ProcessListOptions) ([]api.Process, error) {
	var processes []api.Process

	data, err := r.call("GET", "/v3/cluster/node/"+url.PathEscape(id)+"/process", opts.Query(), nil, "", nil)
	if err != nil {
		return processes, err
	}

	err = json.Unmarshal(data, &processes)

	return processes, err
}

func (r *restclient) ClusterNodeVersion(id string) (api.Version, error) {
	var version api.Version

	data, err := r.call("GET", "/v3/cluster/node/"+url.PathEscape(id)+"/version", nil, nil, "", nil)
	if err != nil {
		return version, err
	}

	err = json.Unmarshal(data, &version)

	return version, err
}
