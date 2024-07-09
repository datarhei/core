package client

import (
	"bytes"
	"net/http"
	"net/url"

	"github.com/datarhei/core/v16/encoding/json"
)

func (c *APIClient) LockCreate(origin string, r LockRequest) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}

	_, err = c.call(http.MethodPost, "/v1/lock", "application/json", bytes.NewReader(data), origin)

	return err
}

func (c *APIClient) LockDelete(origin string, name string) error {
	_, err := c.call(http.MethodDelete, "/v1/lock/"+url.PathEscape(name), "application/json", nil, origin)

	return err
}
