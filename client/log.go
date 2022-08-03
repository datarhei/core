package client

import (
	"encoding/json"

	"github.com/datarhei/core/v16/http/api"
)

func (r *restclient) Log() ([]api.LogEvent, error) {
	var log []api.LogEvent

	data, err := r.call("GET", "/log", "", nil)
	if err != nil {
		return log, err
	}

	err = json.Unmarshal(data, &log)

	return log, err
}
