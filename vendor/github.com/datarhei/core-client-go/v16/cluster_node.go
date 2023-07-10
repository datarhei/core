package coreclient

import (
	"encoding/json"
	"net/url"

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
