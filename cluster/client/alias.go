package client

import (
	"bytes"
	"net/http"
	"net/url"

	"github.com/datarhei/core/v16/encoding/json"
)

func (c *APIClient) AliasAdd(origin string, r AddAliasRequest) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}

	_, err = c.call(http.MethodPost, "/v1/alias", "application/json", bytes.NewReader(data), origin)

	return err
}

func (c *APIClient) AliasUpdate(origin, pattern string, r UpdateAliasRequest) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}

	_, err = c.call(http.MethodPut, "/v1/alias/rule?pattern="+url.QueryEscape(pattern), "application/json", bytes.NewReader(data), origin)

	return err
}

func (c *APIClient) AliasRemove(origin, pattern string) error {
	_, err := c.call(http.MethodDelete, "/v1/alias/rule?pattern="+url.QueryEscape(pattern), "application/json", nil, origin)

	return err
}

func (c *APIClient) AliasesSet(origin string, r SetAliasesRequest) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}

	_, err = c.call(http.MethodPut, "/v1/alias", "application/json", bytes.NewReader(data), origin)

	return err
}
