package policy

import (
	"github.com/puzpuzpuz/xsync/v3"
)

type PolicyEnforcer struct {
	model   Model
	adapter Adapter
	lock    *xsync.RBMutex
}

func NewEnforcer(model Model, adapter Adapter) *PolicyEnforcer {
	e := &PolicyEnforcer{
		model:   model,
		adapter: adapter,
		lock:    xsync.NewRBMutex(),
	}

	e.ReloadPolicies()

	return e
}

func (e *PolicyEnforcer) HasPolicy(policy Policy) bool {
	token := e.lock.RLock()
	defer e.lock.RUnlock(token)

	return e.model.HasPolicy(policy)
}

func (e *PolicyEnforcer) AddPolicy(policy Policy) error {
	token := e.lock.RLock()
	defer e.lock.RUnlock(token)

	err := e.model.AddPolicy(policy)
	if err != nil {
		return err
	}

	if e.adapter != nil {
		e.adapter.AddPolicy(policy)
	}

	return nil
}

func (e *PolicyEnforcer) RemovePolicies(policies []Policy) error {
	token := e.lock.RLock()
	defer e.lock.RUnlock(token)

	err := e.model.RemovePolicies(policies)
	if err != nil {
		return err
	}

	if e.adapter != nil {
		e.adapter.RemovePolicies(policies)
	}

	return nil
}

func (e *PolicyEnforcer) GetFilteredPolicy(name, domain string) []Policy {
	token := e.lock.RLock()
	defer e.lock.RUnlock(token)

	return e.model.GetFilteredPolicy(name, domain)
}

func (e *PolicyEnforcer) ReloadPolicies() error {
	e.lock.Lock()
	defer e.lock.Unlock()

	e.model.ClearPolicy()

	return e.adapter.LoadPolicy(e.model)
}

func (e *PolicyEnforcer) Enforce(name, domain, rtype, resource, action string) (bool, Policy) {
	token := e.lock.RLock()
	defer e.lock.RUnlock(token)

	return e.model.Enforce(name, domain, rtype, resource, action)
}

func (e *PolicyEnforcer) HasDomain(name string) bool {
	return e.adapter.HasDomain(name)
}

func (e *PolicyEnforcer) ListDomains() []string {
	return e.adapter.AllDomains()
}
