package coreclient

import (
	"encoding/json"

	"github.com/datarhei/core-client-go/v16/api"
)

func (r *restclient) SRTChannels() ([]api.SRTChannel, error) {
	var m []api.SRTChannel

	data, err := r.call("GET", "/v3/srt", "", nil)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(data, &m)

	return m, err
}
