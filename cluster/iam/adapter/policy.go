package adapter

import (
	"strings"
	"sync"

	"github.com/datarhei/core/v16/cluster/store"
	iamaccess "github.com/datarhei/core/v16/iam/access"

	"github.com/casbin/casbin/v2/model"
)

type policyAdapter struct {
	store   store.Store
	domains map[string]struct{}
	lock    sync.RWMutex
}

func NewPolicyAdapter(store store.Store) (iamaccess.Adapter, error) {
	a := &policyAdapter{
		store:   store,
		domains: map[string]struct{}{},
	}

	return a, nil
}

func (a *policyAdapter) LoadPolicy(model model.Model) error {
	policies := a.store.ListPolicies()

	rules := [][]string{}
	domains := map[string]struct{}{}

	for _, p := range policies.Policies {
		if len(p.Domain) == 0 {
			p.Domain = "$none"
		}

		rule := []string{
			p.Name,
			p.Domain,
			p.Resource,
			strings.Join(p.Actions, "|"),
		}

		domains[p.Domain] = struct{}{}

		rules = append(rules, rule)
	}

	model.AddPolicies("p", "p", rules)

	a.lock.Lock()
	a.domains = domains
	a.lock.Unlock()

	return nil
}

func (a *policyAdapter) SavePolicy(model model.Model) error {
	return nil
}

func (a *policyAdapter) AddPolicy(sec, ptype string, rule []string) error {
	return nil
}

func (a *policyAdapter) AddPolicies(sec string, ptype string, rules [][]string) error {
	return nil
}

func (a *policyAdapter) RemovePolicy(sec string, ptype string, rule []string) error {
	return nil
}

func (a *policyAdapter) RemovePolicies(sec string, ptype string, rules [][]string) error {
	return nil
}

func (a *policyAdapter) RemoveFilteredPolicy(sec string, ptype string, fieldIndex int, fieldValues ...string) error {
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
