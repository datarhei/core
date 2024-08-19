package policy

import (
	"fmt"

	"github.com/datarhei/core/v16/log"
)

type policyaccess struct {
	logger log.Logger

	adapter  Adapter
	model    Model
	enforcer *PolicyEnforcer
}

type Config struct {
	Adapter   Adapter
	Logger    log.Logger
	Superuser string
}

func New(config Config) (Manager, error) {
	am := &policyaccess{
		adapter: config.Adapter,
		logger:  config.Logger,
	}

	if am.adapter == nil {
		return nil, fmt.Errorf("missing adapter")
	}

	if am.logger == nil {
		am.logger = log.New("")
	}

	m := NewModel(config.Superuser)
	e := NewEnforcer(m, am.adapter)

	am.enforcer = e
	am.model = m

	return am, nil
}

func (am *policyaccess) Enforce(name, domain, rtype, resource, action string) (bool, Policy) {
	return am.enforcer.Enforce(name, domain, rtype, resource, action)
}

func (am *policyaccess) HasPolicy(name, domain string, types []string, resource string, actions []string) bool {
	return am.enforcer.HasPolicy(Policy{
		Name:     name,
		Domain:   domain,
		Types:    types,
		Resource: resource,
		Actions:  actions,
	})
}

func (am *policyaccess) AddPolicy(name, domain string, types []string, resource string, actions []string) error {
	policy := Policy{
		Name:     name,
		Domain:   domain,
		Types:    types,
		Resource: resource,
		Actions:  actions,
	}

	return am.enforcer.AddPolicy(policy)
}

func (am *policyaccess) RemovePolicy(name, domain string) error {
	policies := am.enforcer.GetFilteredPolicy(name, domain)

	return am.enforcer.RemovePolicies(policies)
}

func (am *policyaccess) ListPolicies(name, domain string) []Policy {
	return am.enforcer.GetFilteredPolicy(name, domain)
}

func (am *policyaccess) ReloadPolicies() error {
	return am.enforcer.ReloadPolicies()
}

func (am *policyaccess) HasDomain(name string) bool {
	return am.adapter.HasDomain(name)
}

func (am *policyaccess) ListDomains() []string {
	return am.adapter.AllDomains()
}
