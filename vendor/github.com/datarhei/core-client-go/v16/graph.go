package coreclient

import (
	"bytes"
	"encoding/json"

	"github.com/datarhei/core-client-go/v16/api"
)

func (r *restclient) Graph(query api.GraphQuery) (api.GraphResponse, error) {
	var resp api.GraphResponse
	var buf bytes.Buffer

	e := json.NewEncoder(&buf)
	e.Encode(query)

	data, err := r.call("PUT", "/v3/graph", "application/json", &buf)
	if err != nil {
		return resp, err
	}

	err = json.Unmarshal(data, &resp)

	return resp, err
}
