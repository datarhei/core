package client

import (
	"bytes"
	"net/http"
	"net/url"

	"github.com/datarhei/core/v16/encoding/json"
)

func (c *APIClient) NodeSetState(origin string, nodeid string, r SetNodeStateRequest) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}

	_, err = c.call(http.MethodPut, "/v1/node/"+url.PathEscape(nodeid)+"/state", "application/json", bytes.NewReader(data), origin)

	return err
}
