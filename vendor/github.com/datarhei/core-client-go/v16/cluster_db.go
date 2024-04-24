package coreclient

import (
	"net/url"

	"github.com/goccy/go-json"

	"github.com/datarhei/core-client-go/v16/api"
)

func (r *restclient) ClusterDBProcessList() ([]api.Process, error) {
	var processes []api.Process

	data, err := r.call("GET", "/v3/cluster/db/process", nil, nil, "", nil)
	if err != nil {
		return processes, err
	}

	err = json.Unmarshal(data, &processes)

	return processes, err
}

func (r *restclient) ClusterDBProcess(id ProcessID) (api.Process, error) {
	var info api.Process

	values := &url.Values{}
	values.Set("domain", id.Domain)

	data, err := r.call("GET", "/v3/cluster/db/process/"+url.PathEscape(id.ID), values, nil, "", nil)
	if err != nil {
		return info, err
	}

	err = json.Unmarshal(data, &info)

	return info, err
}

func (r *restclient) ClusterDBUserList() ([]api.IAMUser, error) {
	var users []api.IAMUser

	data, err := r.call("GET", "/v3/cluster/db/user", nil, nil, "", nil)
	if err != nil {
		return users, err
	}

	err = json.Unmarshal(data, &users)

	return users, err
}

func (r *restclient) ClusterDBUser(name string) (api.IAMUser, error) {
	var user api.IAMUser

	data, err := r.call("GET", "/v3/cluster/db/user/"+url.PathEscape(name), nil, nil, "", nil)
	if err != nil {
		return user, err
	}

	err = json.Unmarshal(data, &user)

	return user, err
}

func (r *restclient) ClusterDBPolicies() ([]api.IAMPolicy, error) {
	var policies []api.IAMPolicy

	data, err := r.call("GET", "/v3/cluster/db/policies", nil, nil, "", nil)
	if err != nil {
		return policies, err
	}

	err = json.Unmarshal(data, &policies)

	return policies, err
}

func (r *restclient) ClusterDBLocks() ([]api.ClusterLock, error) {
	var locks []api.ClusterLock

	data, err := r.call("GET", "/v3/cluster/db/locks", nil, nil, "", nil)
	if err != nil {
		return locks, err
	}

	err = json.Unmarshal(data, &locks)

	return locks, err
}

func (r *restclient) ClusterDBKeyValues() (api.ClusterKVS, error) {
	var kvs api.ClusterKVS

	data, err := r.call("GET", "/v3/cluster/db/kv", nil, nil, "", nil)
	if err != nil {
		return kvs, err
	}

	err = json.Unmarshal(data, &kvs)

	return kvs, err
}

func (r *restclient) ClusterDBProcessMap() (api.ClusterProcessMap, error) {
	var m api.ClusterProcessMap

	data, err := r.call("GET", "/v3/cluster/db/map/process", nil, nil, "", nil)
	if err != nil {
		return m, err
	}

	err = json.Unmarshal(data, &m)

	return m, err
}
