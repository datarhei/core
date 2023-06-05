package coreclient

import (
	"bytes"
	"encoding/json"

	"github.com/datarhei/core-client-go/v16/api"
)

type configVersion struct {
	Config struct {
		Version int64 `json:"version"`
	} `json:"config"`
}

func (r *restclient) Config() (int64, api.Config, error) {
	version := configVersion{}

	data, err := r.call("GET", "/v3/config", nil, "", nil)
	if err != nil {
		return 0, api.Config{}, err
	}

	if err := json.Unmarshal(data, &version); err != nil {
		return 0, api.Config{}, err
	}

	config := api.Config{}

	if err := json.Unmarshal(data, &config); err != nil {
		return 0, api.Config{}, err
	}

	configdata, err := json.Marshal(config.Config)
	if err != nil {
		return 0, api.Config{}, err
	}

	switch version.Config.Version {
	case 1:
		cfg := api.ConfigV1{}
		err := json.Unmarshal(configdata, &cfg)
		if err != nil {
			return 0, api.Config{}, err
		}
		config.Config = cfg
	case 2:
		cfg := api.ConfigV2{}
		err := json.Unmarshal(configdata, &cfg)
		if err != nil {
			return 0, api.Config{}, err
		}
		config.Config = cfg
	case 3:
		cfg := api.ConfigV3{}
		err := json.Unmarshal(configdata, &cfg)
		if err != nil {
			return 0, api.Config{}, err
		}
		config.Config = cfg
	}

	return version.Config.Version, config, nil
}

func (r *restclient) ConfigSet(config interface{}) error {
	var buf bytes.Buffer

	e := json.NewEncoder(&buf)
	e.Encode(config)

	_, err := r.call("PUT", "/v3/config", nil, "application/json", &buf)

	if e, ok := err.(api.Error); ok {
		if e.Code == 409 {
			ce := api.ConfigError{}
			if err := json.Unmarshal(e.Body, &ce); err != nil {
				return e
			}
			return ce
		}
	}

	return err
}

func (r *restclient) ConfigReload() error {
	_, err := r.call("GET", "/v3/config/reload", nil, "", nil)

	return err
}
