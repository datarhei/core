package iam

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/datarhei/core/v16/io/fs"
	"github.com/datarhei/core/v16/log"

	"github.com/casbin/casbin/v2/model"
)

// Adapter is the file adapter for Casbin.
// It can load policy from file or save policy to file.
type adapter struct {
	fs       fs.Filesystem
	filePath string
	logger   log.Logger
	groups   []Group
	lock     sync.Mutex
}

func newAdapter(fs fs.Filesystem, filePath string, logger log.Logger) (*adapter, error) {
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
	if _, err := a.fs.Stat(a.filePath); os.IsNotExist(err) {
		a.groups = []Group{}
		return nil
	}

	data, err := a.fs.ReadFile(a.filePath)
	if err != nil {
		return err
	}

	groups := []Group{}

	err = json.Unmarshal(data, &groups)
	if err != nil {
		return err
	}

	rule := [5]string{}
	for _, group := range groups {
		rule[0] = "p"
		rule[2] = group.Name
		for name, roles := range group.Roles {
			rule[1] = "role:" + name
			for _, role := range roles {
				rule[3] = role.Resource
				rule[4] = formatActions(role.Actions)

				if err := a.importPolicy(model, rule[0:5]); err != nil {
					return err
				}
			}
		}

		for _, policy := range group.Policies {
			rule[1] = policy.Username
			rule[3] = policy.Resource
			rule[4] = formatActions(policy.Actions)

			if err := a.importPolicy(model, rule[0:5]); err != nil {
				return err
			}
		}

		rule[0] = "g"
		rule[3] = group.Name

		for _, ug := range group.UserRoles {
			rule[1] = ug.Username
			rule[2] = "role:" + ug.Role

			if err := a.importPolicy(model, rule[0:4]); err != nil {
				return err
			}
		}
	}

	a.groups = groups

	return nil
}

func (a *adapter) importPolicy(model model.Model, rule []string) error {
	copiedRule := make([]string, len(rule))
	copy(copiedRule, rule)

	a.logger.Debug().WithFields(log.Fields{
		"username": copiedRule[1],
		"domain":   copiedRule[2],
		"resource": copiedRule[3],
		"actions":  copiedRule[4],
	}).Log("imported policy")

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
	jsondata, err := json.MarshalIndent(a.groups, "", "    ")
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
		actions = formatActions(rule[3])

		a.logger.Debug().WithFields(log.Fields{
			"username": username,
			"domain":   domain,
			"resource": resource,
			"actions":  actions,
		}).Log("adding policy")
	} else if ptype == "g" {
		username = rule[0]
		role = rule[1]
		domain = rule[2]

		a.logger.Debug().WithFields(log.Fields{
			"username": username,
			"role":     role,
			"domain":   domain,
		}).Log("adding role mapping")
	} else {
		return fmt.Errorf("unknown ptype: %s", ptype)
	}

	var group *Group = nil
	for i := range a.groups {
		if a.groups[i].Name == domain {
			group = &a.groups[i]
			break
		}
	}

	if group == nil {
		g := Group{
			Name:      domain,
			Roles:     map[string][]Role{},
			UserRoles: []MapUserRole{},
			Policies:  []GroupPolicy{},
		}

		a.groups = append(a.groups, g)
		group = &a.groups[len(a.groups)-1]
	}

	if ptype == "p" {
		if strings.HasPrefix(username, "role:") {
			if group.Roles == nil {
				group.Roles = make(map[string][]Role)
			}

			role := strings.TrimPrefix(username, "role:")
			group.Roles[role] = append(group.Roles[role], Role{
				Resource: resource,
				Actions:  actions,
			})
		} else {
			group.Policies = append(group.Policies, GroupPolicy{
				Username: username,
				Role: Role{
					Resource: resource,
					Actions:  actions,
				},
			})
		}
	} else {
		group.UserRoles = append(group.UserRoles, MapUserRole{
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
		if len(rule) != 4 {
			return false, fmt.Errorf("invalid rule length. must be 'user/role, domain, resource, actions'")
		}

		username = rule[0]
		domain = rule[1]
		resource = rule[2]
		actions = formatActions(rule[3])
	} else if ptype == "g" {
		username = rule[0]
		role = rule[1]
		domain = rule[2]
	} else {
		return false, fmt.Errorf("unknown ptype: %s", ptype)
	}

	var group *Group = nil
	for i := range a.groups {
		if a.groups[i].Name == domain {
			group = &a.groups[i]
			break
		}
	}

	if group == nil {
		// if we can't find any group with that name, then the policy doesn't exist
		return false, nil
	}

	if ptype == "p" {
		isRole := false
		if strings.HasPrefix(username, "role:") {
			isRole = true
			username = strings.TrimPrefix(username, "role:")
		}

		if isRole {
			roles, ok := group.Roles[username]
			if !ok {
				// unknown role, policy doesn't exist
				return false, nil
			}

			for _, role := range roles {
				if role.Resource == resource && formatActions(role.Actions) == actions {
					return true, nil
				}
			}
		} else {
			for _, p := range group.Policies {
				if p.Username == username && p.Resource == resource && formatActions(p.Actions) == actions {
					return true, nil
				}
			}
		}
	} else {
		role = strings.TrimPrefix(role, "role:")
		for _, user := range group.UserRoles {
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
		actions = formatActions(rule[3])

		a.logger.Debug().WithFields(log.Fields{
			"username": username,
			"domain":   domain,
			"resource": resource,
			"actions":  actions,
		}).Log("removing policy")
	} else if ptype == "g" {
		username = rule[0]
		role = rule[1]
		domain = rule[2]

		a.logger.Debug().WithFields(log.Fields{
			"username": username,
			"role":     role,
			"domain":   domain,
		}).Log("adding role mapping")
	} else {
		return fmt.Errorf("unknown ptype: %s", ptype)
	}

	var group *Group = nil
	for i := range a.groups {
		if a.groups[i].Name == domain {
			group = &a.groups[i]
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
			roles := group.Roles[username]

			newRoles := []Role{}

			for _, role := range roles {
				if role.Resource == resource && formatActions(role.Actions) == actions {
					continue
				}

				newRoles = append(newRoles, role)
			}

			group.Roles[username] = newRoles
		} else {
			policies := []GroupPolicy{}

			for _, p := range group.Policies {
				if p.Username == username && p.Resource == resource && formatActions(p.Actions) == actions {
					continue
				}

				policies = append(policies, p)
			}

			group.Policies = policies
		}
	} else {
		role = strings.TrimPrefix(role, "role:")

		users := []MapUserRole{}

		for _, user := range group.UserRoles {
			if user.Username == username && user.Role == role {
				continue
			}

			users = append(users, user)
		}

		group.UserRoles = users
	}

	// Remove the group if there are no rules and policies
	if len(group.Roles) == 0 && len(group.UserRoles) == 0 && len(group.Policies) == 0 {
		groups := []Group{}

		for _, g := range a.groups {
			if g.Name == group.Name {
				continue
			}

			groups = append(groups, g)
		}

		a.groups = groups
	}

	return nil
}

// Adapter
func (a *adapter) RemoveFilteredPolicy(sec string, ptype string, fieldIndex int, fieldValues ...string) error {
	return fmt.Errorf("not implemented")
}

func (a *adapter) getAllGroups() []string {
	names := []string{}

	for _, group := range a.groups {
		if group.Name[0] == '$' {
			continue
		}

		names = append(names, group.Name)
	}

	return names
}

type Group struct {
	Name      string            `json:"name"`
	Roles     map[string][]Role `json:"roles"`
	UserRoles []MapUserRole     `json:"userroles"`
	Policies  []GroupPolicy     `json:"policies"`
}

type Role struct {
	Resource string `json:"resource"`
	Actions  string `json:"actions"`
}

type MapUserRole struct {
	Username string `json:"username"`
	Role     string `json:"role"`
}

type GroupPolicy struct {
	Username string `json:"username"`
	Role
}

func formatActions(actions string) string {
	a := strings.Split(actions, "|")

	sort.Strings(a)

	return strings.Join(a, "|")
}
