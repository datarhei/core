package coreclient

import (
	"net/url"

	"github.com/goccy/go-json"

	"github.com/datarhei/core-client-go/v16/api"
)

func (r *restclient) Log() ([]api.LogEvent, error) {
	var log []api.LogEvent

	query := &url.Values{}
	query.Set("format", "raw")

	data, err := r.call("GET", "/v3/log", query, nil, "", nil)
	if err != nil {
		return log, err
	}

	err = json.Unmarshal(data, &log)

	return log, err
}
