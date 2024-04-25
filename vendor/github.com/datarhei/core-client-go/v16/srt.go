package coreclient

import (
	"github.com/goccy/go-json"

	"github.com/datarhei/core-client-go/v16/api"
)

func (r *restclient) SRTChannels() ([]api.SRTChannel, error) {
	var m []api.SRTChannel

	data, err := r.SRTChannelsRaw()
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(data, &m)

	return m, err
}

func (r *restclient) SRTChannelsRaw() ([]byte, error) {
	return r.call("GET", "/v3/srt", nil, nil, "", nil)
}
