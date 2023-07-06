package coreclient

import (
	"encoding/json"
	"io"

	"github.com/datarhei/core-client-go/v16/api"
)

func (r *restclient) Cluster() (api.ClusterAbout, error) {
	var about api.ClusterAbout

	data, err := r.call("GET", "/v3/cluster", nil, nil, "", nil)
	if err != nil {
		return about, err
	}

	err = json.Unmarshal(data, &about)

	return about, err
}

func (r *restclient) ClusterHealthy() (bool, error) {
	var healthy bool

	data, err := r.call("GET", "/v3/cluster/healthy", nil, nil, "", nil)
	if err != nil {
		return healthy, err
	}

	err = json.Unmarshal(data, &healthy)

	return healthy, err
}

func (r *restclient) ClusterSnapshot() (io.ReadCloser, error) {
	return r.stream("GET", "/v3/cluster/snapshot", nil, nil, "", nil)
}

func (r *restclient) ClusterLeave() error {
	_, err := r.call("PUT", "/v3/cluster/leave", nil, nil, "", nil)

	return err
}
