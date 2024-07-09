package client

import (
	"bytes"
	"net/http"
	"net/url"

	"github.com/datarhei/core/v16/encoding/json"
)

func (c *APIClient) IAMIdentityAdd(origin string, r AddIdentityRequest) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}

	_, err = c.call(http.MethodPost, "/v1/iam/user", "application/json", bytes.NewReader(data), origin)

	return err
}

func (c *APIClient) IAMIdentityUpdate(origin, name string, r UpdateIdentityRequest) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}

	_, err = c.call(http.MethodPut, "/v1/iam/user/"+url.PathEscape(name), "application/json", bytes.NewReader(data), origin)

	return err
}

func (c *APIClient) IAMPoliciesSet(origin, name string, r SetPoliciesRequest) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}

	_, err = c.call(http.MethodPut, "/v1/iam/user/"+url.PathEscape(name)+"/policies", "application/json", bytes.NewReader(data), origin)

	return err
}

func (c *APIClient) IAMIdentityRemove(origin string, name string) error {
	_, err := c.call(http.MethodDelete, "/v1/iam/user/"+url.PathEscape(name), "application/json", nil, origin)

	return err
}
