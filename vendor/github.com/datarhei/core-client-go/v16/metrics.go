package coreclient

import (
	"bytes"
	"encoding/json"

	"github.com/datarhei/core-client-go/v16/api"
)

func (r *restclient) MetricsList() ([]api.MetricsDescription, error) {
	descriptions := []api.MetricsDescription{}

	data, err := r.call("GET", "/v3/metrics", "application/json", nil)
	if err != nil {
		return descriptions, err
	}

	err = json.Unmarshal(data, &descriptions)

	return descriptions, err
}

func (r *restclient) Metrics(query api.MetricsQuery) (api.MetricsResponse, error) {
	var m api.MetricsResponse
	var buf bytes.Buffer

	e := json.NewEncoder(&buf)
	e.Encode(query)

	data, err := r.call("POST", "/v3/metrics", "application/json", &buf)
	if err != nil {
		return m, err
	}

	err = json.Unmarshal(data, &m)

	return m, err
}
