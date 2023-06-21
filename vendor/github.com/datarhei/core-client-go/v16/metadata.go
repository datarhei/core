package coreclient

import (
	"bytes"
	"encoding/json"
	"net/url"

	"github.com/datarhei/core-client-go/v16/api"
)

func (r *restclient) Metadata(key string) (api.Metadata, error) {
	var m api.Metadata

	path := "/v3/metadata"
	if len(key) != 0 {
		path += "/" + url.PathEscape(key)
	}

	data, err := r.call("GET", path, nil, nil, "", nil)
	if err != nil {
		return m, err
	}

	err = json.Unmarshal(data, &m)

	return m, err
}

func (r *restclient) MetadataSet(key string, metadata api.Metadata) error {
	var buf bytes.Buffer

	e := json.NewEncoder(&buf)
	e.Encode(metadata)

	path := "/v3/metadata"
	if len(key) != 0 {
		path += "/" + url.PathEscape(key)
	}

	_, err := r.call("PUT", path, nil, nil, "application/json", &buf)
	if err != nil {
		return err
	}

	return nil
}
