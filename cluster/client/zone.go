package client

import (
	"bytes"
	"net/http"
	"net/url"

	"github.com/datarhei/core/v16/encoding/json"
)

func (c *APIClient) ZoneAdd(origin string, r AddZoneRequest) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}

	_, err = c.call(http.MethodPost, "/v1/zone", "application/json", bytes.NewReader(data), origin)

	return err
}

func (c *APIClient) ZoneUpdate(origin, id string, r UpdateZoneRequest) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}

	_, err = c.call(http.MethodPut, "/v1/zone/"+url.PathEscape(id), "application/json", bytes.NewReader(data), origin)

	return err
}

func (c *APIClient) ZoneRemove(origin, id string) error {
	_, err := c.call(http.MethodDelete, "/v1/zone/"+url.PathEscape(id), "application/json", nil, origin)

	return err
}
