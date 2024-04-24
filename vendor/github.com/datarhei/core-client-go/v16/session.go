package coreclient

import (
	"bytes"
	"net/url"
	"strings"

	"github.com/goccy/go-json"

	"github.com/datarhei/core-client-go/v16/api"
)

func (r *restclient) Sessions(collectors []string) (api.SessionsSummary, error) {
	var sessions api.SessionsSummary

	query := &url.Values{}
	query.Set("collectors", strings.Join(collectors, ","))

	data, err := r.call("GET", "/v3/session", query, nil, "", nil)
	if err != nil {
		return sessions, err
	}

	err = json.Unmarshal(data, &sessions)

	return sessions, err
}

func (r *restclient) SessionsActive(collectors []string) (api.SessionsActive, error) {
	var sessions api.SessionsActive

	query := &url.Values{}
	query.Set("collectors", strings.Join(collectors, ","))

	data, err := r.call("GET", "/v3/session/active", query, nil, "", nil)
	if err != nil {
		return sessions, err
	}

	err = json.Unmarshal(data, &sessions)

	return sessions, err
}

func (r *restclient) SessionToken(name string, req []api.SessionTokenRequest) ([]api.SessionTokenRequest, error) {
	var tokens []api.SessionTokenRequest
	var buf bytes.Buffer

	e := json.NewEncoder(&buf)
	e.Encode(req)

	data, err := r.call("PUT", "/v3/session/token/"+url.PathEscape(name), nil, nil, "application/json", &buf)
	if err != nil {
		return tokens, err
	}

	err = json.Unmarshal(data, &tokens)

	return tokens, err
}
