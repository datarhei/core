package adapter

import (
	"sync"

	"github.com/datarhei/core/v16/cluster/store"
	"github.com/datarhei/core/v16/iam/policy"
)

type policyAdapter struct {
	store   store.Store
	domains map[string]struct{}
	lock    sync.RWMutex
}

func NewPolicyAdapter(store store.Store) (policy.Adapter, error) {
	a := &policyAdapter{
		store:   store,
		domains: map[string]struct{}{},
	}

	return a, nil
}

func (a *policyAdapter) LoadPolicy(model policy.Model) error {
	storePolicies := a.store.IAMPolicyList()

	policies := []policy.Policy{}
	domains := map[string]struct{}{}

	for _, p := range storePolicies.Policies {
		policy := p.Clone()

		if len(policy.Domain) == 0 {
			policy.Domain = "$none"
		}

		if len(policy.Types) == 0 {
			policy.Types = []string{"$none"}
		}

		domains[policy.Domain] = struct{}{}

		policies = append(policies, policy)
	}

	model.AddPolicies(policies)

	a.lock.Lock()
	a.domains = domains
	a.lock.Unlock()

	return nil
}

func (a *policyAdapter) SavePolicy(_ policy.Model) error {
	return nil
}

func (a *policyAdapter) AddPolicy(_ policy.Policy) error {
	return nil
}

func (a *policyAdapter) AddPolicies(_ []policy.Policy) error {
	return nil
}

func (a *policyAdapter) RemovePolicy(_ policy.Policy) error {
	return nil
}

func (a *policyAdapter) RemovePolicies(_ []policy.Policy) error {
	return nil
}

func (a *policyAdapter) AllDomains() []string {
	a.lock.RLock()
	defer a.lock.RUnlock()

	n := len(a.domains)
	domains := make([]string, n)

	for domain := range a.domains {
		domains[n-1] = domain
		n--
	}

	return domains
}

func (a *policyAdapter) HasDomain(name string) bool {
	a.lock.RLock()
	defer a.lock.RUnlock()

	_, ok := a.domains[name]

	return ok
}
