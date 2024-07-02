package client

import (
	"bytes"
	"net/http"
	"net/url"

	"github.com/datarhei/core/v16/encoding/json"
)

func (c *APIClient) AddIdentity(origin string, r AddIdentityRequest) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}

	_, err = c.call(http.MethodPost, "/v1/iam/user", "application/json", bytes.NewReader(data), origin)

	return err
}

func (c *APIClient) UpdateIdentity(origin, name string, r UpdateIdentityRequest) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}

	_, err = c.call(http.MethodPut, "/v1/iam/user/"+url.PathEscape(name), "application/json", bytes.NewReader(data), origin)

	return err
}

func (c *APIClient) SetPolicies(origin, name string, r SetPoliciesRequest) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}

	_, err = c.call(http.MethodPut, "/v1/iam/user/"+url.PathEscape(name)+"/policies", "application/json", bytes.NewReader(data), origin)

	return err
}

func (c *APIClient) RemoveIdentity(origin string, name string) error {
	_, err := c.call(http.MethodDelete, "/v1/iam/user/"+url.PathEscape(name), "application/json", nil, origin)

	return err
}
