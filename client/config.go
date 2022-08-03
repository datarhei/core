package client

import (
	"bytes"
	"encoding/json"

	"github.com/datarhei/core/v16/http/api"
)

func (r *restclient) Config() (api.Config, error) {
	var config api.Config

	data, err := r.call("GET", "/config", "", nil)
	if err != nil {
		return config, err
	}

	err = json.Unmarshal(data, &config)

	return config, err
}

func (r *restclient) ConfigSet(config api.ConfigData) error {
	var buf bytes.Buffer

	e := json.NewEncoder(&buf)
	e.Encode(config)

	_, err := r.call("PUT", "/config", "application/json", &buf)

	return err
}

func (r *restclient) ConfigReload() error {
	_, err := r.call("GET", "/config/reload", "", nil)

	return err
}
