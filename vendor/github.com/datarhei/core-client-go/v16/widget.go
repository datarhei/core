package coreclient

import (
	"encoding/json"
	"net/url"

	"github.com/datarhei/core-client-go/v16/api"
)

func (r *restclient) WidgetProcess(id ProcessID) (api.WidgetProcess, error) {
	var w api.WidgetProcess

	query := &url.Values{}
	query.Set("domain", id.Domain)

	data, err := r.call("GET", "/v3/widget/process"+url.PathEscape(id.ID), query, nil, "", nil)
	if err != nil {
		return w, err
	}

	err = json.Unmarshal(data, &w)

	return w, err
}
