package coreclient

import (
	"encoding/json"
	"net/url"

	"github.com/datarhei/core-client-go/v16/api"
)

func (r *restclient) PlayoutStatus(id ProcessID, inputID string) (api.PlayoutStatus, error) {
	var status api.PlayoutStatus

	path := "/v3/process/" + url.PathEscape(id.ID) + "/playout/" + url.PathEscape(inputID) + "/status"

	values := &url.Values{}
	values.Set("domain", id.Domain)

	data, err := r.call("GET", path, values, nil, "", nil)
	if err != nil {
		return status, err
	}

	err = json.Unmarshal(data, &status)

	return status, err
}
