package client

import (
	"net/url"

	"github.com/datarhei/core/v16/encoding/json"
	"github.com/datarhei/core/v16/http/api"
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
