package client

import (
	"bytes"
	"net/http"
	"net/url"

	"github.com/datarhei/core/v16/cluster/store"
	"github.com/datarhei/core/v16/encoding/json"
	"github.com/datarhei/core/v16/restream/app"
)

func (c *APIClient) ProcessAdd(origin string, r AddProcessRequest) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}

	_, err = c.call(http.MethodPost, "/v1/process", "application/json", bytes.NewReader(data), origin)

	return err
}

func (c APIClient) ProcessGet(origin string, id app.ProcessID) (store.Process, string, error) {
	res := GetProcessResponse{}

	data, err := c.call(http.MethodGet, "/v1/process/"+url.PathEscape(id.ID)+"?domain="+url.QueryEscape(id.Domain), "application/json", nil, origin)
	if err != nil {
		return store.Process{}, "", err
	}

	err = json.Unmarshal(data, &res)
	if err != nil {
		return store.Process{}, "", err
	}

	return res.Process, res.NodeID, nil
}

func (c *APIClient) ProcessRemove(origin string, id app.ProcessID) error {
	_, err := c.call(http.MethodDelete, "/v1/process/"+url.PathEscape(id.ID)+"?domain="+url.QueryEscape(id.Domain), "application/json", nil, origin)

	return err
}

func (c *APIClient) ProcessUpdate(origin string, id app.ProcessID, r UpdateProcessRequest) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}

	_, err = c.call(http.MethodPut, "/v1/process/"+url.PathEscape(id.ID)+"?domain="+url.QueryEscape(id.Domain), "application/json", bytes.NewReader(data), origin)

	return err
}

func (c *APIClient) ProcessSetCommand(origin string, id app.ProcessID, r SetProcessCommandRequest) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}

	_, err = c.call(http.MethodPut, "/v1/process/"+url.PathEscape(id.ID)+"/command?domain="+url.QueryEscape(id.Domain), "application/json", bytes.NewReader(data), origin)

	return err
}

func (c *APIClient) ProcessSetMetadata(origin string, id app.ProcessID, key string, r SetProcessMetadataRequest) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}

	_, err = c.call(http.MethodPut, "/v1/process/"+url.PathEscape(id.ID)+"/metadata/"+url.PathEscape(key)+"?domain="+url.QueryEscape(id.Domain), "application/json", bytes.NewReader(data), origin)

	return err
}

func (c *APIClient) ProcessesRelocate(origin string, r RelocateProcessesRequest) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}

	_, err = c.call(http.MethodPut, "/v1/relocate", "application/json", bytes.NewReader(data), origin)

	return err
}
