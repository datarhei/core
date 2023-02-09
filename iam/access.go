package iam

import (
	"strings"

	"github.com/datarhei/core/v16/io/fs"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
)

type AccessEnforcer interface {
	Enforce(name, domain, resource, action string) (bool, string)
}

type AccessManager interface {
	AccessEnforcer

	AddPolicy()
}

type access struct {
	fs fs.Filesystem

	enforcer *casbin.Enforcer
}

func NewAccessManager(fs fs.Filesystem) (AccessManager, error) {
	am := &access{
		fs: fs,
	}

	m := model.NewModel()
	m.AddDef("r", "r", "sub, dom, obj, act")
	m.AddDef("p", "p", "sub, dom, obj, act")
	m.AddDef("g", "g", "_, _, _")
	m.AddDef("e", "e", "some(where (p.eft == allow))")
	m.AddDef("m", "m", `g(r.sub, p.sub, r.dom) && r.dom == p.dom && ResourceMatch(r.obj, r.dom, p.obj) && ActionMatch(r.act, p.act) || r.sub == "$superuser"`)

	a := newAdapter(fs, "./policy.json")

	e, err := casbin.NewEnforcer(m, a)
	if err != nil {
		return nil, err
	}

	e.AddFunction("ResourceMatch", resourceMatchFunc)
	e.AddFunction("ActionMatch", actionMatchFunc)

	am.enforcer = e

	return am, nil
}

func (am *access) AddPolicy() {}
func (am *access) Enforce(name, domain, resource, action string) (bool, string) {
	ok, rule, _ := am.enforcer.EnforceEx(name, domain, resource, action)

	return ok, strings.Join(rule, ", ")
}
