package access

import (
	"sort"
	"strings"

	"github.com/datarhei/core/v16/log"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
)

type Policy struct {
	Name     string
	Domain   string
	Types    []string
	Resource string
	Actions  []string
}

type Enforcer interface {
	Enforce(name, domain, rtype, resource, action string) (bool, string)

	HasDomain(name string) bool
	ListDomains() []string
}

type Manager interface {
	Enforcer

	HasPolicy(name, domain string, types []string, resource string, actions []string) bool
	AddPolicy(name, domain string, types []string, resource string, actions []string) error
	RemovePolicy(name, domain string, types []string, resource string, actions []string) error
	ListPolicies(name, domain string, types []string, resource string, actions []string) []Policy
	ReloadPolicies() error
}

type access struct {
	logger log.Logger

	adapter  Adapter
	model    model.Model
	enforcer *casbin.SyncedEnforcer
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
	m.AddDef("m", "m", `g(r.sub, p.sub, r.dom) && r.dom == p.dom && ResourceMatch(r.obj, p.obj) && ActionMatch(r.act, p.act) || r.sub == "$superuser"`)

	e, err := casbin.NewSyncedEnforcer(m, am.adapter)
	if err != nil {
		return nil, err
	}

	e.AddFunction("ResourceMatch", resourceMatchFunc)
	e.AddFunction("ActionMatch", actionMatchFunc)

	am.enforcer = e
	am.model = m

	return am, nil
}

func (am *access) HasPolicy(name, domain string, types []string, resource string, actions []string) bool {
	policy := []string{name, domain, EncodeResource(types, resource), EncodeActions(actions)}

	hasPolicy, _ := am.enforcer.HasPolicy(policy)

	return hasPolicy
}

func (am *access) AddPolicy(name, domain string, types []string, resource string, actions []string) error {
	policy := []string{name, domain, EncodeResource(types, resource), EncodeActions(actions)}

	if hasPolicy, _ := am.enforcer.HasPolicy(policy); hasPolicy {
		return nil
	}

	_, err := am.enforcer.AddPolicy(policy)

	return err
}

func (am *access) RemovePolicy(name, domain string, types []string, resource string, actions []string) error {
	policies, err := am.enforcer.GetFilteredPolicy(0, name, domain, EncodeResource(types, resource), EncodeActions(actions))
	if err != nil {
		return err
	}

	_, err = am.enforcer.RemovePolicies(policies)

	return err
}

func (am *access) ListPolicies(name, domain string, types []string, resource string, actions []string) []Policy {
	policies := []Policy{}

	ps, err := am.enforcer.GetFilteredPolicy(0, name, domain, EncodeResource(types, resource), EncodeActions(actions))
	if err != nil {
		return policies
	}

	for _, p := range ps {
		types, resource := DecodeResource(p[2])
		policies = append(policies, Policy{
			Name:     p[0],
			Domain:   p[1],
			Types:    types,
			Resource: resource,
			Actions:  DecodeActions(p[3]),
		})
	}

	return policies
}

func (am *access) ReloadPolicies() error {
	am.enforcer.ClearPolicy()

	return am.enforcer.LoadPolicy()
}

func (am *access) HasDomain(name string) bool {
	return am.adapter.HasDomain(name)
}

func (am *access) ListDomains() []string {
	return am.adapter.AllDomains()
}

func (am *access) Enforce(name, domain, rtype, resource, action string) (bool, string) {
	resource = rtype + ":" + resource

	ok, rule, _ := am.enforcer.EnforceEx(name, domain, resource, action)

	return ok, strings.Join(rule, ", ")
}

func EncodeActions(actions []string) string {
	return strings.Join(actions, "|")
}

func DecodeActions(actions string) []string {
	return strings.Split(actions, "|")
}

func EncodeResource(types []string, resource string) string {
	if len(types) == 0 {
		return resource
	}

	sort.Strings(types)

	return strings.Join(types, "|") + ":" + resource
}

func DecodeResource(resource string) ([]string, string) {
	before, after, found := strings.Cut(resource, ":")
	if !found {
		return []string{"$none"}, resource
	}

	return strings.Split(before, "|"), after
}
