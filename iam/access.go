package iam

import (
	"fmt"
	"strings"

	"github.com/datarhei/core/v16/io/fs"
	"github.com/datarhei/core/v16/log"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
)

type AccessEnforcer interface {
	Enforce(name, domain, resource, action string) (bool, string)
	HasGroup(name string) bool
}

type AccessManager interface {
	AccessEnforcer

	AddPolicy(username, domain, resource, actions string) bool
	RemovePolicy(username, domain, resource, actions string) bool
	ListPolicies(username, domain, resource, actions string) [][]string
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

	a := newAdapter(am.fs, "./policy.json", am.logger)

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

func (am *access) AddPolicy(username, domain, resource, actions string) bool {
	policy := []string{username, domain, resource, actions}

	if am.enforcer.HasPolicy(policy) {
		return true
	}

	ok, _ := am.enforcer.AddPolicy(policy)

	return ok
}

func (am *access) RemovePolicy(username, domain, resource, actions string) bool {
	policies := am.enforcer.GetFilteredPolicy(0, username, domain, resource, actions)
	am.enforcer.RemovePolicies(policies)

	return true
}

func (am *access) ListPolicies(username, domain, resource, actions string) [][]string {
	return am.enforcer.GetFilteredPolicy(0, username, domain, resource, actions)
}

func (am *access) HasGroup(name string) bool {
	groups, err := am.enforcer.GetAllDomains()
	if err != nil {
		return false
	}

	for _, g := range groups {
		if g == name {
			return true
		}
	}

	return false
}

func (am *access) Enforce(name, domain, resource, action string) (bool, string) {
	ok, rule, _ := am.enforcer.EnforceEx(name, domain, resource, action)

	return ok, strings.Join(rule, ", ")
}
