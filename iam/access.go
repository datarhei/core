package iam

import (
	"fmt"
	"strings"

	"github.com/datarhei/core/v16/io/fs"
	"github.com/datarhei/core/v16/log"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
)

type Policy struct {
	Name     string
	Domain   string
	Resource string
	Actions  []string
}

type AccessEnforcer interface {
	Enforce(name, domain, resource, action string) (bool, string)

	HasDomain(name string) bool
	ListDomains() []string
}

type AccessManager interface {
	AccessEnforcer

	HasPolicy(name, domain, resource string, actions []string) bool
	AddPolicy(name, domain, resource string, actions []string) bool
	RemovePolicy(name, domain, resource string, actions []string) bool
	ListPolicies(name, domain, resource string, actions []string) []Policy
}

type access struct {
	fs     fs.Filesystem
	logger log.Logger

	adapter  *adapter
	enforcer *casbin.Enforcer
}

type AccessConfig struct {
	FS     fs.Filesystem
	Logger log.Logger
}

func NewAccessManager(config AccessConfig) (AccessManager, error) {
	am := &access{
		fs:     config.FS,
		logger: config.Logger,
	}

	if am.fs == nil {
		return nil, fmt.Errorf("a filesystem has to be provided")
	}

	if am.logger == nil {
		am.logger = log.New("")
	}

	m := model.NewModel()
	m.AddDef("r", "r", "sub, dom, obj, act")
	m.AddDef("p", "p", "sub, dom, obj, act")
	m.AddDef("g", "g", "_, _, _")
	m.AddDef("e", "e", "some(where (p.eft == allow))")
	m.AddDef("m", "m", `g(r.sub, p.sub, r.dom) && r.dom == p.dom && ResourceMatch(r.obj, r.dom, p.obj) && ActionMatch(r.act, p.act) || r.sub == "$superuser"`)

	a, err := newAdapter(am.fs, "./policy.json", am.logger)
	if err != nil {
		return nil, err
	}

	e, err := casbin.NewEnforcer(m, a)
	if err != nil {
		return nil, err
	}

	e.AddFunction("ResourceMatch", resourceMatchFunc)
	e.AddFunction("ActionMatch", actionMatchFunc)

	am.enforcer = e
	am.adapter = a

	return am, nil
}

func (am *access) HasPolicy(name, domain, resource string, actions []string) bool {
	policy := []string{name, domain, resource, strings.Join(actions, "|")}

	return am.enforcer.HasPolicy(policy)
}

func (am *access) AddPolicy(name, domain, resource string, actions []string) bool {
	policy := []string{name, domain, resource, strings.Join(actions, "|")}

	if am.enforcer.HasPolicy(policy) {
		return true
	}

	ok, _ := am.enforcer.AddPolicy(policy)

	return ok
}

func (am *access) RemovePolicy(name, domain, resource string, actions []string) bool {
	policies := am.enforcer.GetFilteredPolicy(0, name, domain, resource, strings.Join(actions, "|"))
	am.enforcer.RemovePolicies(policies)

	return true
}

func (am *access) ListPolicies(name, domain, resource string, actions []string) []Policy {
	policies := []Policy{}

	ps := am.enforcer.GetFilteredPolicy(0, name, domain, resource, strings.Join(actions, "|"))

	for _, p := range ps {
		policies = append(policies, Policy{
			Name:     p[0],
			Domain:   p[1],
			Resource: p[2],
			Actions:  strings.Split(p[3], "|"),
		})
	}

	return policies
}

func (am *access) HasDomain(name string) bool {
	groups := am.adapter.getAllGroups()

	for _, g := range groups {
		if g == name {
			return true
		}
	}

	return false
}

func (am *access) ListDomains() []string {
	return am.adapter.getAllGroups()
}

func (am *access) Enforce(name, domain, resource, action string) (bool, string) {
	ok, rule, _ := am.enforcer.EnforceEx(name, domain, resource, action)

	return ok, strings.Join(rule, ", ")
}
