package access

import (
	"strings"

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

type Enforcer interface {
	Enforce(name, domain, resource, action string) (bool, string)

	HasDomain(name string) bool
	ListDomains() []string
}

type Manager interface {
	Enforcer

	HasPolicy(name, domain, resource string, actions []string) bool
	AddPolicy(name, domain, resource string, actions []string) bool
	RemovePolicy(name, domain, resource string, actions []string) bool
	ListPolicies(name, domain, resource string, actions []string) []Policy
	ReloadPolicies() error
}

type access struct {
	logger log.Logger

	adapter  Adapter
	model    model.Model
	enforcer *casbin.Enforcer
}

type Config struct {
	Adapter Adapter
	Logger  log.Logger
}

func New(config Config) (Manager, error) {
	am := &access{
		adapter: config.Adapter,
		logger:  config.Logger,
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

	e, err := casbin.NewEnforcer(m, am.adapter)
	if err != nil {
		return nil, err
	}

	e.AddFunction("ResourceMatch", resourceMatchFunc)
	e.AddFunction("ActionMatch", actionMatchFunc)

	am.enforcer = e
	am.model = m

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

func (am *access) ReloadPolicies() error {
	am.model.ClearPolicy()

	return am.adapter.LoadPolicy(am.model)
}

func (am *access) HasDomain(name string) bool {
	return am.adapter.HasDomain(name)
}

func (am *access) ListDomains() []string {
	return am.adapter.AllDomains()
}

func (am *access) Enforce(name, domain, resource, action string) (bool, string) {
	ok, rule, _ := am.enforcer.EnforceEx(name, domain, resource, action)

	return ok, strings.Join(rule, ", ")
}
