package client

import (
	"bytes"
	"encoding/json"

	"github.com/datarhei/core/v16/http/api"
)

func (r *restclient) Metrics(query api.MetricsQuery) (api.MetricsResponse, error) {
	var m api.MetricsResponse
	var buf bytes.Buffer

	e := json.NewEncoder(&buf)
	e.Encode(query)

	data, err := r.call("POST", "/metrics", "application/json", &buf)
	if err != nil {
		return m, err
	}

	err = json.Unmarshal(data, &m)

	return m, err
}
