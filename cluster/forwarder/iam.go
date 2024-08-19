package forwarder

import (
	apiclient "github.com/datarhei/core/v16/cluster/client"
	iamidentity "github.com/datarhei/core/v16/iam/identity"
	iampolicy "github.com/datarhei/core/v16/iam/policy"
)

func (f *Forwarder) IAMIdentityAdd(origin string, identity iamidentity.User) error {
	if origin == "" {
		origin = f.ID
	}

	r := apiclient.AddIdentityRequest{
		Identity: identity,
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return reconstructError(client.IAMIdentityAdd(origin, r))
}

func (f *Forwarder) IAMIdentityUpdate(origin, name string, identity iamidentity.User) error {
	if origin == "" {
		origin = f.ID
	}

	r := apiclient.UpdateIdentityRequest{
		Identity: identity,
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return reconstructError(client.IAMIdentityUpdate(origin, name, r))
}

func (f *Forwarder) IAMPoliciesSet(origin, name string, policies []iampolicy.Policy) error {
	if origin == "" {
		origin = f.ID
	}

	r := apiclient.SetPoliciesRequest{
		Policies: policies,
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return reconstructError(client.IAMPoliciesSet(origin, name, r))
}

func (f *Forwarder) IAMIdentityRemove(origin string, name string) error {
	if origin == "" {
		origin = f.ID
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return reconstructError(client.IAMIdentityRemove(origin, name))
}
