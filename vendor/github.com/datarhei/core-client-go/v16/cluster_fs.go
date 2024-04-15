package coreclient

import (
	"encoding/json"
	"net/url"

	"github.com/datarhei/core-client-go/v16/api"
)

func (r *restclient) ClusterFilesystemList(storage, pattern, sort, order string) ([]api.FileInfo, error) {
	var files []api.FileInfo

	query := &url.Values{}
	query.Set("glob", pattern)
	query.Set("sort", sort)
	query.Set("order", order)

	data, err := r.call("GET", "/v3/cluster/fs/"+url.PathEscape(storage), query, nil, "", nil)
	if err != nil {
		return files, err
	}

	err = json.Unmarshal(data, &files)

	return files, err
}
