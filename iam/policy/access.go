package policy

import (
	"fmt"
	"strings"
)

type Policy struct {
	Name     string
	Domain   string
	Types    []string
	Resource string
	Actions  []string
}

func (p Policy) String() string {
	return fmt.Sprintf("%s@%s (%s):%s %s", p.Name, p.Domain, strings.Join(p.Types, "|"), p.Resource, strings.Join(p.Actions, "|"))
}

type Enforcer interface {
	Enforce(name, domain, rtype, resource, action string) (bool, Policy)

	HasDomain(name string) bool
	ListDomains() []string
}

type Manager interface {
	Enforcer

	HasPolicy(name, domain string, types []string, resource string, actions []string) bool
	AddPolicy(name, domain string, types []string, resource string, actions []string) error
	RemovePolicy(name, domain string) error
	ListPolicies(name, domain string) []Policy
	ReloadPolicies() error
}
