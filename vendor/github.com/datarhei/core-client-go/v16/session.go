package coreclient

import (
	"encoding/json"
	"net/url"
	"strings"

	"github.com/datarhei/core-client-go/v16/api"
)

func (r *restclient) Sessions(collectors []string) (api.SessionsSummary, error) {
	var sessions api.SessionsSummary

	values := url.Values{}
	values.Set("collectors", strings.Join(collectors, ","))

	data, err := r.call("GET", "/v3/sessions?"+values.Encode(), "", nil)
	if err != nil {
		return sessions, err
	}

	err = json.Unmarshal(data, &sessions)

	return sessions, err
}

func (r *restclient) SessionsActive(collectors []string) (api.SessionsActive, error) {
	var sessions api.SessionsActive

	values := url.Values{}
	values.Set("collectors", strings.Join(collectors, ","))

	data, err := r.call("GET", "/v3/sessions/active?"+values.Encode(), "", nil)
	if err != nil {
		return sessions, err
	}

	err = json.Unmarshal(data, &sessions)

	return sessions, err
}
