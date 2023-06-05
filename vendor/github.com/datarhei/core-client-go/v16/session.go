package coreclient

import (
	"encoding/json"
	"net/url"
	"strings"

	"github.com/datarhei/core-client-go/v16/api"
)

func (r *restclient) Sessions(collectors []string) (api.SessionsSummary, error) {
	var sessions api.SessionsSummary

	query := &url.Values{}
	query.Set("collectors", strings.Join(collectors, ","))

	data, err := r.call("GET", "/v3/sessions", query, "", nil)
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

	data, err := r.call("GET", "/v3/sessions/active", query, "", nil)
	if err != nil {
		return sessions, err
	}

	err = json.Unmarshal(data, &sessions)

	return sessions, err
}
