package coreclient

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/goccy/go-json"

	"github.com/datarhei/core-client-go/v16/api"
)

func (r *restclient) Cluster() (*api.ClusterAboutV1, *api.ClusterAboutV2, error) {
	data, err := r.call("GET", "/v3/cluster", nil, nil, "", nil)
	if err != nil {
		return nil, nil, err
	}

	var aboutV1 *api.ClusterAboutV1
	var aboutV2 *api.ClusterAboutV2

	type version struct {
		Version string `json:"version"`
	}

	v := version{}

	err = json.Unmarshal(data, &v)
	if err != nil {
		return nil, nil, err
	}

	if strings.HasPrefix(v.Version, "1.") {
		aboutV1 = &api.ClusterAboutV1{}

		err = json.Unmarshal(data, aboutV1)
		if err != nil {
			return nil, nil, err
		}
	} else if strings.HasPrefix(v.Version, "2.") {
		aboutV2 = &api.ClusterAboutV2{}

		err = json.Unmarshal(data, aboutV2)
		if err != nil {
			return nil, nil, err
		}
	} else {
		err = fmt.Errorf("unsupported version (%s)", v.Version)
	}

	return aboutV1, aboutV2, err
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
	return r.stream(context.Background(), "GET", "/v3/cluster/snapshot", nil, nil, "", nil)
}

func (r *restclient) ClusterLeave() error {
	_, err := r.call("PUT", "/v3/cluster/leave", nil, nil, "", nil)

	return err
}

func (r *restclient) ClusterTransferLeadership(id string) error {
	_, err := r.call("PUT", "/v3/cluster/transfer/"+url.PathEscape(id), nil, nil, "", nil)

	return err
}
