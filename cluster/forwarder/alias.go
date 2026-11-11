package forwarder

import (
	"github.com/datarhei/core/v16/alias"
	apiclient "github.com/datarhei/core/v16/cluster/client"
)

func (f *Forwarder) AliasAdd(origin string, rule alias.Rule) error {
	if origin == "" {
		origin = f.ID
	}

	r := apiclient.AddAliasRequest{
		Rule: rule,
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return reconstructError(client.AliasAdd(origin, r))
}

func (f *Forwarder) AliasUpdate(origin, pattern string, rule alias.Rule) error {
	if origin == "" {
		origin = f.ID
	}

	r := apiclient.UpdateAliasRequest{
		Rule: rule,
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return reconstructError(client.AliasUpdate(origin, pattern, r))
}

func (f *Forwarder) AliasRemove(origin, pattern string) error {
	if origin == "" {
		origin = f.ID
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return reconstructError(client.AliasRemove(origin, pattern))
}

func (f *Forwarder) AliasesSet(origin string, rules []alias.Rule) error {
	if origin == "" {
		origin = f.ID
	}

	r := apiclient.SetAliasesRequest{
		Rules: rules,
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return reconstructError(client.AliasesSet(origin, r))
}
