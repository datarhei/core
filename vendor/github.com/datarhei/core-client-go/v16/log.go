package coreclient

import (
	"encoding/json"

	"github.com/datarhei/core-client-go/v16/api"
)

func (r *restclient) Log() ([]api.LogEvent, error) {
	var log []api.LogEvent

	data, err := r.call("GET", "/v3/log?format=raw", "", nil)
	if err != nil {
		return log, err
	}

	err = json.Unmarshal(data, &log)

	return log, err
}
