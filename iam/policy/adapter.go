package policy

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/datarhei/core/v16/io/fs"
	"github.com/datarhei/core/v16/log"
)

type policyadapter struct {
	fs       fs.Filesystem
	filePath string
	logger   log.Logger
	domains  []Domain
	lock     sync.Mutex
}

type Adapter interface {
	AddPolicy(policy Policy) error
	LoadPolicy(model Model) error
	RemovePolicy(policy Policy) error
	SavePolicy(model Model) error
	AddPolicies(policies []Policy) error
	RemovePolicies(policies []Policy) error

	AllDomains() []string
	HasDomain(string) bool
}

func NewJSONAdapter(fs fs.Filesystem, filePath string, logger log.Logger) (Adapter, error) {
	a := &policyadapter{
		fs:       fs,
		filePath: filePath,
		logger:   logger,
	}

	if a.fs == nil {
		return nil, fmt.Errorf("a filesystem has to be provided")
	}

	if len(a.filePath) == 0 {
		return nil, fmt.Errorf("invalid file path, file path cannot be empty")
	}

	if a.logger == nil {
		a.logger = log.New("")
	}

	return a, nil
}

// Adapter
func (a *policyadapter) LoadPolicy(model Model) error {
	a.lock.Lock()
	defer a.lock.Unlock()

	return a.loadPolicyFile(model)
}

func (a *policyadapter) loadPolicyFile(model Model) error {
	if _, err := a.fs.Stat(a.filePath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			a.domains = []Domain{}
			return nil
		}

		return err
	}

	data, err := a.fs.ReadFile(a.filePath)
	if err != nil {
		return err
	}

	domains := []Domain{}

	err = json.Unmarshal(data, &domains)
	if err != nil {
		return err
	}

	for _, domain := range domains {
		for _, policy := range domain.Policies {
			rtypes, resource := DecodeResource(policy.Resource)
			p := normalizePolicy(Policy{
				Name:     policy.Username,
				Domain:   domain.Name,
				Types:    rtypes,
				Resource: resource,
				Actions:  DecodeActions(policy.Actions),
			})

			if err := a.importPolicy(model, p); err != nil {
				return err
			}
		}
	}

	a.domains = domains

	return nil
}

func (a *policyadapter) importPolicy(model Model, policy Policy) error {
	a.logger.Debug().WithFields(log.Fields{
		"subject":  policy.Name,
		"domain":   policy.Domain,
		"types":    policy.Types,
		"resource": policy.Resource,
		"actions":  policy.Actions,
	}).Log("Imported policy")

	model.AddPolicy(policy)

	return nil
}

// Adapter
func (a *policyadapter) SavePolicy(model Model) error {
	a.lock.Lock()
	defer a.lock.Unlock()

	return a.savePolicyFile()
}

func (a *policyadapter) savePolicyFile() error {
	jsondata, err := json.MarshalIndent(a.domains, "", "    ")
	if err != nil {
		return err
	}

	_, _, err = a.fs.WriteFileSafe(a.filePath, jsondata)

	return err
}

// Adapter (auto-save)
func (a *policyadapter) AddPolicy(policy Policy) error {
	a.lock.Lock()
	defer a.lock.Unlock()

	err := a.addPolicy(policy)
	if err != nil {
		return err
	}

	return a.savePolicyFile()
}

// BatchAdapter (auto-save)
func (a *policyadapter) AddPolicies(policies []Policy) error {
	a.lock.Lock()
	defer a.lock.Unlock()

	for _, policy := range policies {
		err := a.addPolicy(policy)
		if err != nil {
			return err
		}
	}

	return a.savePolicyFile()
}

func (a *policyadapter) addPolicy(policy Policy) error {
	ok, err := a.hasPolicy(policy)
	if err != nil {
		return err
	}

	if ok {
		// the policy is already there, nothing to add
		return nil
	}

	policy = normalizePolicy(policy)

	username := policy.Name
	domain := policy.Domain
	resource := EncodeResource(policy.Types, policy.Resource)
	actions := EncodeActions(policy.Actions)

	a.logger.Debug().WithFields(log.Fields{
		"subject":  username,
		"domain":   domain,
		"resource": resource,
		"actions":  actions,
	}).Log("Adding policy")

	var dom *Domain = nil
	for i := range a.domains {
		if a.domains[i].Name == domain {
			dom = &a.domains[i]
			break
		}
	}

	if dom == nil {
		g := Domain{
			Name:     domain,
			Policies: []DomainPolicy{},
		}

		a.domains = append(a.domains, g)
		dom = &a.domains[len(a.domains)-1]
	}

	dom.Policies = append(dom.Policies, DomainPolicy{
		Username: username,
		Resource: resource,
		Actions:  actions,
	})

	return nil
}

func (a *policyadapter) hasPolicy(policy Policy) (bool, error) {
	policy = normalizePolicy(policy)

	username := policy.Name
	domain := policy.Domain
	resource := EncodeResource(policy.Types, policy.Resource)
	actions := EncodeActions(policy.Actions)

	var dom *Domain = nil
	for i := range a.domains {
		if a.domains[i].Name == domain {
			dom = &a.domains[i]
			break
		}
	}

	if dom == nil {
		// if we can't find any domain with that name, then the policy doesn't exist
		return false, nil
	}

	for _, p := range dom.Policies {
		if p.Username == username && p.Resource == resource && p.Actions == actions {
			return true, nil
		}
	}

	return false, nil
}

// Adapter (auto-save)
func (a *policyadapter) RemovePolicy(policy Policy) error {
	a.lock.Lock()
	defer a.lock.Unlock()

	err := a.removePolicy(policy)
	if err != nil {
		return err
	}

	return a.savePolicyFile()
}

// BatchAdapter (auto-save)
func (a *policyadapter) RemovePolicies(policies []Policy) error {
	a.lock.Lock()
	defer a.lock.Unlock()

	for _, policy := range policies {
		err := a.removePolicy(policy)
		if err != nil {
			return err
		}
	}

	return a.savePolicyFile()
}

func (a *policyadapter) removePolicy(policy Policy) error {
	ok, err := a.hasPolicy(policy)
	if err != nil {
		return err
	}

	if !ok {
		// the policy is not there, nothing to remove
		return nil
	}

	policy = normalizePolicy(policy)

	username := policy.Name
	domain := policy.Domain
	resource := EncodeResource(policy.Types, policy.Resource)
	actions := EncodeActions(policy.Actions)

	a.logger.Debug().WithFields(log.Fields{
		"subject":  username,
		"domain":   domain,
		"resource": resource,
		"actions":  actions,
	}).Log("Removing policy")

	var dom *Domain = nil
	for i := range a.domains {
		if a.domains[i].Name == domain {
			dom = &a.domains[i]
			break
		}
	}

	policies := []DomainPolicy{}

	for _, p := range dom.Policies {
		if p.Username == username && p.Resource == resource && p.Actions == actions {
			continue
		}

		policies = append(policies, p)
	}

	dom.Policies = policies

	// Remove the group if there are no rules and policies
	if len(dom.Policies) == 0 {
		groups := []Domain{}

		for _, g := range a.domains {
			if g.Name == dom.Name {
				continue
			}

			groups = append(groups, g)
		}

		a.domains = groups
	}

	return nil
}

// Adapter
func (a *policyadapter) RemoveFilteredPolicy(sec string, ptype string, fieldIndex int, fieldValues ...string) error {
	return fmt.Errorf("not implemented")
}

func (a *policyadapter) AllDomains() []string {
	a.lock.Lock()
	defer a.lock.Unlock()

	names := []string{}

	for _, domain := range a.domains {
		if domain.Name[0] == '$' {
			continue
		}

		names = append(names, domain.Name)
	}

	return names
}

func (a *policyadapter) HasDomain(name string) bool {
	a.lock.Lock()
	defer a.lock.Unlock()

	for _, domain := range a.domains {
		if domain.Name[0] == '$' {
			continue
		}

		if domain.Name == name {
			return true
		}
	}

	return false
}

type Domain struct {
	Name     string         `json:"name"`
	Policies []DomainPolicy `json:"policies"`
}

type DomainPolicy struct {
	Username string `json:"username"`
	Resource string `json:"resource"`
	Actions  string `json:"actions"`
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
