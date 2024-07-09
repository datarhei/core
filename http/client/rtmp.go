package client

import (
	"github.com/datarhei/core/v16/encoding/json"
	"github.com/datarhei/core/v16/http/api"
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
