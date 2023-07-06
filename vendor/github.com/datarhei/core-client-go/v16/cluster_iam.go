package coreclient

import "github.com/datarhei/core-client-go/v16/api"

func (r *restclient) ClusterIdentitiesList() ([]api.IAMUser, error) {
	return r.identitiesList("cluster")
}

func (r *restclient) ClusterIdentity(name string) (api.IAMUser, error) {
	return r.identity("cluster", name)
}

func (r *restclient) ClusterIdentityAdd(u api.IAMUser) error {
	return r.identityAdd("cluster", u)
}

func (r *restclient) ClusterIdentityUpdate(name string, u api.IAMUser) error {
	return r.identityUpdate("cluster", name, u)
}

func (r *restclient) ClusterIdentitySetPolicies(name string, p []api.IAMPolicy) error {
	return r.identitySetPolicies("cluster", name, p)
}

func (r *restclient) ClusterIdentityDelete(name string) error {
	return r.identityDelete("cluster", name)
}

func (r *restclient) ClusterIAMReload() error {
	_, err := r.call("PUT", "/v3/cluster/iam/reload", nil, nil, "", nil)

	return err
}
