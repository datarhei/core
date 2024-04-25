package coreclient

import (
	"bytes"
	"net/url"

	"github.com/goccy/go-json"

	"github.com/datarhei/core-client-go/v16/api"
)

func (r *restclient) identitiesList(where string) ([]api.IAMUser, error) {
	var users []api.IAMUser

	path := "/v3/iam/user"
	if where == "cluster" {
		path = "/v3/cluster/iam/user"
	}

	data, err := r.call("GET", path, nil, nil, "", nil)
	if err != nil {
		return users, err
	}

	err = json.Unmarshal(data, &users)

	return users, err
}

func (r *restclient) identity(where, name string) (api.IAMUser, error) {
	var user api.IAMUser

	path := "/v3/iam/user/" + url.PathEscape(name)
	if where == "cluster" {
		path = "/v3/cluster/iam/user/" + url.PathEscape(name)
	}

	data, err := r.call("GET", path, nil, nil, "", nil)
	if err != nil {
		return user, err
	}

	err = json.Unmarshal(data, &user)

	return user, err
}

func (r *restclient) identityAdd(where string, u api.IAMUser) error {
	var buf bytes.Buffer

	path := "/v3/iam/user"
	if where == "cluster" {
		path = "/v3/cluster/iam/user"
	}

	e := json.NewEncoder(&buf)
	e.Encode(u)

	_, err := r.call("POST", path, nil, nil, "application/json", &buf)
	if err != nil {
		return err
	}

	return nil
}

func (r *restclient) identityUpdate(where, name string, u api.IAMUser) error {
	var buf bytes.Buffer

	path := "/v3/iam/user/" + url.PathEscape(name)
	if where == "cluster" {
		path = "/v3/cluster/iam/user/" + url.PathEscape(name)
	}

	e := json.NewEncoder(&buf)
	e.Encode(u)

	_, err := r.call("PUT", path, nil, nil, "application/json", &buf)
	if err != nil {
		return err
	}

	return nil
}

func (r *restclient) identitySetPolicies(where, name string, p []api.IAMPolicy) error {
	var buf bytes.Buffer

	path := "/v3/iam/user/" + url.PathEscape(name) + "/policy"
	if where == "cluster" {
		path = "/v3/cluster/iam/user/" + url.PathEscape(name) + "/policy"
	}

	e := json.NewEncoder(&buf)
	e.Encode(p)

	_, err := r.call("PUT", path, nil, nil, "application/json", &buf)
	if err != nil {
		return err
	}

	return nil
}

func (r *restclient) identityDelete(where, name string) error {
	path := "/v3/iam/user/" + url.PathEscape(name)
	if where == "cluster" {
		path = "/v3/cluster/iam/user/" + url.PathEscape(name)
	}

	_, err := r.call("DELETE", path, nil, nil, "", nil)

	return err
}

func (r *restclient) IdentitiesList() ([]api.IAMUser, error) {
	return r.identitiesList("")
}

func (r *restclient) Identity(name string) (api.IAMUser, error) {
	return r.identity("", name)
}

func (r *restclient) IdentityAdd(u api.IAMUser) error {
	return r.identityAdd("", u)
}

func (r *restclient) IdentityUpdate(name string, u api.IAMUser) error {
	return r.identityUpdate("", name, u)
}

func (r *restclient) IdentitySetPolicies(name string, p []api.IAMPolicy) error {
	return r.identitySetPolicies("", name, p)
}

func (r *restclient) IdentityDelete(name string) error {
	return r.identityDelete("", name)
}
