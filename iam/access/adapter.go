package access

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/datarhei/core/v16/io/fs"
	"github.com/datarhei/core/v16/log"

	"github.com/casbin/casbin/v2/model"
	"github.com/casbin/casbin/v2/persist"
)

// Adapter is the file adapter for Casbin.
// It can load policy from file or save policy to file.
type adapter struct {
	fs       fs.Filesystem
	filePath string
	logger   log.Logger
	domains  []Domain
	lock     sync.Mutex
}

type Adapter interface {
	persist.BatchAdapter

	AllDomains() []string
	HasDomain(string) bool
}

func NewJSONAdapter(fs fs.Filesystem, filePath string, logger log.Logger) (Adapter, error) {
	a := &adapter{
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
func (a *adapter) LoadPolicy(model model.Model) error {
	a.lock.Lock()
	defer a.lock.Unlock()

	return a.loadPolicyFile(model)
}

func (a *adapter) loadPolicyFile(model model.Model) error {
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

	rule := [5]string{}
	for _, domain := range domains {
		rule[0] = "p"
		rule[2] = domain.Name
		for name, roles := range domain.Roles {
			rule[1] = "role:" + name
			for _, role := range roles {
				rule[3] = role.Resource
				rule[4] = formatList(role.Actions)

				if err := a.importPolicy(model, rule[0:5]); err != nil {
					return err
				}
			}
		}

		for _, policy := range domain.Policies {
			rule[1] = policy.Username
			rule[3] = policy.Resource
			rule[4] = formatList(policy.Actions)

			if err := a.importPolicy(model, rule[0:5]); err != nil {
				return err
			}
		}

		rule[0] = "g"
		rule[3] = domain.Name

		for _, ug := range domain.UserRoles {
			rule[1] = ug.Username
			rule[2] = "role:" + ug.Role

			if err := a.importPolicy(model, rule[0:4]); err != nil {
				return err
			}
		}
	}

	a.domains = domains

	return nil
}

func (a *adapter) importPolicy(model model.Model, rule []string) error {
	copiedRule := make([]string, len(rule))
	copy(copiedRule, rule)

	a.logger.Debug().WithFields(log.Fields{
		"subject":  copiedRule[1],
		"domain":   copiedRule[2],
		"resource": copiedRule[3],
		"actions":  copiedRule[4],
	}).Log("Imported policy")

	ok, err := model.HasPolicyEx(copiedRule[0], copiedRule[0], copiedRule[1:])
	if err != nil {
		return err
	}
	if ok {
		return nil // skip duplicated policy
	}

	model.AddPolicy(copiedRule[0], copiedRule[0], copiedRule[1:])

	return nil
}

// Adapter
func (a *adapter) SavePolicy(model model.Model) error {
	a.lock.Lock()
	defer a.lock.Unlock()

	return a.savePolicyFile()
}

func (a *adapter) savePolicyFile() error {
	jsondata, err := json.MarshalIndent(a.domains, "", "    ")
	if err != nil {
		return err
	}

	_, _, err = a.fs.WriteFileSafe(a.filePath, jsondata)

	return err
}

// Adapter (auto-save)
func (a *adapter) AddPolicy(sec, ptype string, rule []string) error {
	a.lock.Lock()
	defer a.lock.Unlock()

	err := a.addPolicy(ptype, rule)
	if err != nil {
		return err
	}

	return a.savePolicyFile()
}

// BatchAdapter (auto-save)
func (a *adapter) AddPolicies(sec string, ptype string, rules [][]string) error {
	a.lock.Lock()
	defer a.lock.Unlock()

	for _, rule := range rules {
		err := a.addPolicy(ptype, rule)
		if err != nil {
			return err
		}
	}

	return a.savePolicyFile()
}

func (a *adapter) addPolicy(ptype string, rule []string) error {
	ok, err := a.hasPolicy(ptype, rule)
	if err != nil {
		return err
	}

	if ok {
		// the policy is already there, nothing to add
		return nil
	}

	username := ""
	role := ""
	domain := ""
	resource := ""
	actions := ""

	if ptype == "p" {
		username = rule[0]
		domain = rule[1]
		resource = rule[2]
		actions = formatList(rule[3])

		a.logger.Debug().WithFields(log.Fields{
			"subject":  username,
			"domain":   domain,
			"resource": resource,
			"actions":  actions,
		}).Log("Adding policy")
	} else if ptype == "g" {
		username = rule[0]
		role = rule[1]
		domain = rule[2]

		a.logger.Debug().WithFields(log.Fields{
			"subject": username,
			"role":    role,
			"domain":  domain,
		}).Log("Adding role mapping")
	} else {
		return fmt.Errorf("unknown ptype: %s", ptype)
	}

	var dom *Domain = nil
	for i := range a.domains {
		if a.domains[i].Name == domain {
			dom = &a.domains[i]
			break
		}
	}

	if dom == nil {
		g := Domain{
			Name:      domain,
			Roles:     map[string][]Role{},
			UserRoles: []MapUserRole{},
			Policies:  []DomainPolicy{},
		}

		a.domains = append(a.domains, g)
		dom = &a.domains[len(a.domains)-1]
	}

	if ptype == "p" {
		if strings.HasPrefix(username, "role:") {
			if dom.Roles == nil {
				dom.Roles = make(map[string][]Role)
			}

			role := strings.TrimPrefix(username, "role:")
			dom.Roles[role] = append(dom.Roles[role], Role{
				Resource: resource,
				Actions:  actions,
			})
		} else {
			dom.Policies = append(dom.Policies, DomainPolicy{
				Username: username,
				Role: Role{
					Resource: resource,
					Actions:  actions,
				},
			})
		}
	} else {
		dom.UserRoles = append(dom.UserRoles, MapUserRole{
			Username: username,
			Role:     strings.TrimPrefix(role, "role:"),
		})
	}

	return nil
}

func (a *adapter) hasPolicy(ptype string, rule []string) (bool, error) {
	var username string
	var role string
	var domain string
	var resource string
	var actions string

	if ptype == "p" {
		if len(rule) < 4 {
			return false, fmt.Errorf("invalid rule length. must be 'user/role, domain, resource, actions'")
		}

		username = rule[0]
		domain = rule[1]
		resource = rule[2]
		actions = formatList(rule[3])
	} else if ptype == "g" {
		if len(rule) < 3 {
			return false, fmt.Errorf("invalid rule length. must be 'user, role, domain'")
		}

		username = rule[0]
		role = rule[1]
		domain = rule[2]
	} else {
		return false, fmt.Errorf("unknown ptype: %s", ptype)
	}

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

	if ptype == "p" {
		isRole := false
		if strings.HasPrefix(username, "role:") {
			isRole = true
			username = strings.TrimPrefix(username, "role:")
		}

		if isRole {
			roles, ok := dom.Roles[username]
			if !ok {
				// unknown role, policy doesn't exist
				return false, nil
			}

			for _, role := range roles {
				if role.Resource == resource && formatList(role.Actions) == actions {
					return true, nil
				}
			}
		} else {
			for _, p := range dom.Policies {
				if p.Username == username && p.Resource == resource && formatList(p.Actions) == actions {
					return true, nil
				}
			}
		}
	} else {
		role = strings.TrimPrefix(role, "role:")
		for _, user := range dom.UserRoles {
			if user.Username == username && user.Role == role {
				return true, nil
			}
		}
	}

	return false, nil
}

// Adapter (auto-save)
func (a *adapter) RemovePolicy(sec string, ptype string, rule []string) error {
	a.lock.Lock()
	defer a.lock.Unlock()

	err := a.removePolicy(ptype, rule)
	if err != nil {
		return err
	}

	return a.savePolicyFile()
}

// BatchAdapter (auto-save)
func (a *adapter) RemovePolicies(sec string, ptype string, rules [][]string) error {
	a.lock.Lock()
	defer a.lock.Unlock()

	for _, rule := range rules {
		err := a.removePolicy(ptype, rule)
		if err != nil {
			return err
		}
	}

	return a.savePolicyFile()
}

func (a *adapter) removePolicy(ptype string, rule []string) error {
	ok, err := a.hasPolicy(ptype, rule)
	if err != nil {
		return err
	}

	if !ok {
		// the policy is not there, nothing to remove
		return nil
	}

	username := ""
	role := ""
	domain := ""
	resource := ""
	actions := ""

	if ptype == "p" {
		username = rule[0]
		domain = rule[1]
		resource = rule[2]
		actions = formatList(rule[3])

		a.logger.Debug().WithFields(log.Fields{
			"subject":  username,
			"domain":   domain,
			"resource": resource,
			"actions":  actions,
		}).Log("Removing policy")
	} else if ptype == "g" {
		username = rule[0]
		role = rule[1]
		domain = rule[2]

		a.logger.Debug().WithFields(log.Fields{
			"subject": username,
			"role":    role,
			"domain":  domain,
		}).Log("Removing role mapping")
	} else {
		return fmt.Errorf("unknown ptype: %s", ptype)
	}

	var dom *Domain = nil
	for i := range a.domains {
		if a.domains[i].Name == domain {
			dom = &a.domains[i]
			break
		}
	}

	if ptype == "p" {
		isRole := false
		if strings.HasPrefix(username, "role:") {
			isRole = true
			username = strings.TrimPrefix(username, "role:")
		}

		if isRole {
			roles := dom.Roles[username]

			newRoles := []Role{}

			for _, role := range roles {
				if role.Resource == resource && formatList(role.Actions) == actions {
					continue
				}

				newRoles = append(newRoles, role)
			}

			dom.Roles[username] = newRoles
		} else {
			policies := []DomainPolicy{}

			for _, p := range dom.Policies {
				if p.Username == username && p.Resource == resource && formatList(p.Actions) == actions {
					continue
				}

				policies = append(policies, p)
			}

			dom.Policies = policies
		}
	} else {
		role = strings.TrimPrefix(role, "role:")

		users := []MapUserRole{}

		for _, user := range dom.UserRoles {
			if user.Username == username && user.Role == role {
				continue
			}

			users = append(users, user)
		}

		dom.UserRoles = users
	}

	// Remove the group if there are no rules and policies
	if len(dom.Roles) == 0 && len(dom.UserRoles) == 0 && len(dom.Policies) == 0 {
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
func (a *adapter) RemoveFilteredPolicy(sec string, ptype string, fieldIndex int, fieldValues ...string) error {
	return fmt.Errorf("not implemented")
}

func (a *adapter) AllDomains() []string {
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

func (a *adapter) HasDomain(name string) bool {
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
	Name      string            `json:"name"`
	Roles     map[string][]Role `json:"roles"`
	UserRoles []MapUserRole     `json:"userroles"`
	Policies  []DomainPolicy    `json:"policies"`
}

type Role struct {
	Resource string `json:"resource"`
	Actions  string `json:"actions"`
}

type MapUserRole struct {
	Username string `json:"username"`
	Role     string `json:"role"`
}

type DomainPolicy struct {
	Username string `json:"username"`
	Role
}

func formatList(list string) string {
	a := strings.Split(list, "|")

	sort.Strings(a)

	return strings.Join(a, "|")
}
