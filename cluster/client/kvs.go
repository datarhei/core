package client

import (
	"bytes"
	"io/fs"
	"net/http"
	"net/url"
	"time"

	"github.com/datarhei/core/v16/encoding/json"
)

func (c *APIClient) KVSet(origin string, r SetKVRequest) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}

	_, err = c.call(http.MethodPost, "/v1/kv", "application/json", bytes.NewReader(data), origin)

	return err
}

func (c *APIClient) KVUnset(origin string, key string) error {
	_, err := c.call(http.MethodDelete, "/v1/kv/"+url.PathEscape(key), "application/json", nil, origin)
	if err != nil {
		e, ok := err.(Error)
		if ok && e.Code == 404 {
			return fs.ErrNotExist
		}
	}

	return err
}

func (c *APIClient) KVGet(origin string, key string) (string, time.Time, error) {
	data, err := c.call(http.MethodGet, "/v1/kv/"+url.PathEscape(key), "application/json", nil, origin)
	if err != nil {
		e, ok := err.(Error)
		if ok && e.Code == 404 {
			return "", time.Time{}, fs.ErrNotExist
		}

		return "", time.Time{}, err
	}

	res := GetKVResponse{}
	err = json.Unmarshal(data, &res)
	if err != nil {
		return "", time.Time{}, err
	}

	return res.Value, res.UpdatedAt, nil
}
