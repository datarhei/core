package forwarder

import (
	apiclient "github.com/datarhei/core/v16/cluster/client"
	iamaccess "github.com/datarhei/core/v16/iam/access"
	iamidentity "github.com/datarhei/core/v16/iam/identity"
)

func (f *Forwarder) AddIdentity(origin string, identity iamidentity.User) error {
	if origin == "" {
		origin = f.ID
	}

	r := apiclient.AddIdentityRequest{
		Identity: identity,
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.AddIdentity(origin, r)
}

func (f *Forwarder) UpdateIdentity(origin, name string, identity iamidentity.User) error {
	if origin == "" {
		origin = f.ID
	}

	r := apiclient.UpdateIdentityRequest{
		Identity: identity,
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.UpdateIdentity(origin, name, r)
}

func (f *Forwarder) SetPolicies(origin, name string, policies []iamaccess.Policy) error {
	if origin == "" {
		origin = f.ID
	}

	r := apiclient.SetPoliciesRequest{
		Policies: policies,
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.SetPolicies(origin, name, r)
}

func (f *Forwarder) RemoveIdentity(origin string, name string) error {
	if origin == "" {
		origin = f.ID
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.RemoveIdentity(origin, name)
}
