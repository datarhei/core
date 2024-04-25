package coreclient

import (
	"github.com/goccy/go-json"

	"github.com/datarhei/core-client-go/v16/api"
)

func (r *restclient) RTMPChannels() ([]api.RTMPChannel, error) {
	var m []api.RTMPChannel

	data, err := r.call("GET", "/v3/rtmp", nil, nil, "", nil)
	if err != nil {
		return m, err
	}

	err = json.Unmarshal(data, &m)

	return m, err
}
