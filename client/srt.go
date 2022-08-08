package client

import (
	"encoding/json"

	"github.com/datarhei/core/v16/http/api"
)

func (r *restclient) SRTChannels() ([]api.SRTChannel, error) {
	var m []api.SRTChannel

	data, err := r.call("GET", "/srt", "", nil)
	if err != nil {
		return m, err
	}

	err = json.Unmarshal(data, &m)

	return m, err
}
