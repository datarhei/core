package policy

import (
	"slices"
	"strings"

	"github.com/puzpuzpuz/xsync/v3"
)

type Model interface {
	Enforce(name, domain, rtype, resource, action string) (bool, Policy)
	HasPolicy(policy Policy) bool
	AddPolicy(policy Policy) error
	AddPolicies(policies []Policy) error
	RemovePolicy(policy Policy) error
	RemovePolicies(policies []Policy) error
	GetFilteredPolicy(name, domain string) []Policy
	ClearPolicy()
}

type model struct {
	superuser string
	policies  map[string][]Policy // user@domain
	lock      *xsync.RBMutex
}

func NewModel(superuser string) Model {
	m := &model{
		superuser: superuser,
		policies:  map[string][]Policy{},
		lock:      xsync.NewRBMutex(),
	}

	return m
}

func (m *model) HasPolicy(policy Policy) bool {
	token := m.lock.RLock()
	defer m.lock.RUnlock(token)

	return m.hasPolicy(policy)
}

func (m *model) hasPolicy(policy Policy) bool {
	key := policy.Name + "@" + policy.Domain

	policies, hasKey := m.policies[key]
	if !hasKey {
		return false
	}

	policy = normalizePolicy(policy)

	for _, p := range policies {
		if slices.Equal(p.Types, policy.Types) && p.Resource == policy.Resource && slices.Equal(p.Actions, policy.Actions) {
			return true
		}
	}

	return false
}

func (m *model) AddPolicy(policy Policy) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	return m.addPolicy(policy)
}

func (m *model) AddPolicies(policies []Policy) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	for _, policy := range policies {
		m.addPolicy(policy)
	}

	return nil
}

func (m *model) addPolicy(policy Policy) error {
	if m.hasPolicy(policy) {
		return nil
	}

	policy = normalizePolicy(policy)
	key := policy.Name + "@" + policy.Domain

	policies, hasKey := m.policies[key]
	if !hasKey {
		policies = []Policy{}
	}

	policies = append(policies, policy)
	m.policies[key] = policies

	return nil
}

func (m *model) RemovePolicy(policy Policy) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	return m.removePolicy(policy)
}

func (m *model) RemovePolicies(policies []Policy) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	for _, policy := range policies {
		m.removePolicy(policy)
	}

	return nil
}

func (m *model) removePolicy(policy Policy) error {
	if !m.hasPolicy(policy) {
		return nil
	}

	policy = normalizePolicy(policy)
	key := policy.Name + "@" + policy.Domain

	policies := m.policies[key]

	newPolicies := []Policy{}

	for _, p := range policies {
		if slices.Equal(p.Types, policy.Types) && p.Resource == policy.Resource && slices.Equal(p.Actions, policy.Actions) {
			continue
		}

		newPolicies = append(newPolicies, p)
	}

	if len(newPolicies) != 0 {
		m.policies[key] = newPolicies
	} else {
		delete(m.policies, key)
	}

	return nil
}

func (m *model) GetFilteredPolicy(name, domain string) []Policy {
	token := m.lock.RLock()
	defer m.lock.RUnlock(token)

	filteredPolicies := []Policy{}

	if len(name) == 0 && len(domain) == 0 {
		for _, policies := range m.policies {
			filteredPolicies = append(filteredPolicies, policies...)
		}
	} else if len(name) != 0 && len(domain) == 0 {
		for key, policies := range m.policies {
			if !strings.HasPrefix(key, name+"@") {
				continue
			}

			filteredPolicies = append(filteredPolicies, policies...)
		}
	} else if len(name) == 0 && len(domain) != 0 {
		for key, policies := range m.policies {
			if !strings.HasSuffix(key, "@"+domain) {
				continue
			}

			filteredPolicies = append(filteredPolicies, policies...)
		}
	} else {
		for key, policies := range m.policies {
			before, after, _ := strings.Cut(key, "@")

			if name != before || domain != after {
				continue
			}

			filteredPolicies = append(filteredPolicies, policies...)
		}
	}

	return filteredPolicies
}

func (m *model) ClearPolicy() {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.policies = map[string][]Policy{}
}

func (m *model) Enforce(name, domain, rtype, resource, action string) (bool, Policy) {
	token := m.lock.RLock()
	defer m.lock.RUnlock(token)

	if name == m.superuser {
		return true, Policy{
			Name: m.superuser,
		}
	}

	key := name + "@" + domain

	policies, hasKey := m.policies[key]
	if !hasKey {
		return false, Policy{}
	}

	rtype = strings.ToLower(rtype)
	action = strings.ToLower(action)

	for _, p := range policies {
		if resourceMatch(rtype, resource, p.Types, p.Resource) && actionMatch(action, p.Actions, "any") {
			return true, p
		}
	}

	return false, Policy{}
}

func normalizePolicy(p Policy) Policy {
	for i, t := range p.Types {
		p.Types[i] = strings.ToLower(t)
	}

	slices.Sort(p.Types)

	for i, a := range p.Actions {
		p.Actions[i] = strings.ToLower(a)
	}

	slices.Sort(p.Actions)

	return p
}
