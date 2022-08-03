package client

import (
	"bytes"
	"encoding/json"

	"github.com/datarhei/core/v16/http/api"
)

func (r *restclient) Metadata(id, key string) (api.Metadata, error) {
	var m api.Metadata

	path := "/process/" + id + "/metadata"
	if len(key) != 0 {
		path += "/" + key
	}

	data, err := r.call("GET", path, "", nil)
	if err != nil {
		return m, err
	}

	err = json.Unmarshal(data, &m)

	return m, err
}

func (r *restclient) MetadataSet(id, key string, metadata api.Metadata) error {
	var buf bytes.Buffer

	e := json.NewEncoder(&buf)
	e.Encode(metadata)

	_, err := r.call("PUT", "/process/"+id+"/metadata/"+key, "application/json", &buf)
	if err != nil {
		return err
	}

	return nil
}
