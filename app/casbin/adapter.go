package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/casbin/casbin/v2/model"
	"github.com/casbin/casbin/v2/persist"
)

// Adapter is the file adapter for Casbin.
// It can load policy from file or save policy to file.
type adapter struct {
	filePath string
	groups   []Group
	lock     sync.Mutex
}

func NewAdapter(filePath string) persist.Adapter {
	return &adapter{filePath: filePath}
}

// Adapter
func (a *adapter) LoadPolicy(model model.Model) error {
	a.lock.Lock()
	defer a.lock.Unlock()

	if a.filePath == "" {
		return fmt.Errorf("invalid file path, file path cannot be empty")
	}

	/*
		logger := &log.DefaultLogger{}
		logger.EnableLog(true)

		model.SetLogger(logger)
	*/

	return a.loadPolicyFile(model)
}

func (a *adapter) loadPolicyFile(model model.Model) error {
	if _, err := os.Stat(a.filePath); os.IsNotExist(err) {
		a.groups = []Group{}
		return nil
	}

	data, err := os.ReadFile(a.filePath)
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
				rule[4] = role.Actions

				if err := a.importPolicy(model, rule[0:5]); err != nil {
					return err
				}
			}
		}

		for _, policy := range group.Policies {
			rule[1] = policy.Username
			rule[3] = policy.Resource
			rule[4] = policy.Actions

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
	if a.filePath == "" {
		return fmt.Errorf("invalid file path, file path cannot be empty")
	}

	jsondata, err := json.MarshalIndent(a.groups, "", "    ")
	if err != nil {
		return err
	}

	dir, filename := filepath.Split(a.filePath)

	tmpfile, err := os.CreateTemp(dir, filename)
	if err != nil {
		return err
	}

	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write(jsondata); err != nil {
		return err
	}

	if err := tmpfile.Close(); err != nil {
		return err
	}

	if err := os.Rename(tmpfile.Name(), a.filePath); err != nil {
		return err
	}

	return nil
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
		actions = rule[3]
	} else if ptype == "g" {
		username = rule[0]
		role = rule[1]
		domain = rule[2]
	} else {
		return fmt.Errorf("unknown ptype: %s", ptype)
	}

	var group *Group = nil
	for i := range a.groups {
		if a.groups[i].Name == domain {
			group = &a.groups[i]
		}
	}

	if group == nil {
		g := Group{
			Name: domain,
		}

		a.groups = append(a.groups, g)
		group = &g
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
			group.Policies = append(group.Policies, Policy{
				Username: rule[0],
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
		actions = rule[3]
	} else if ptype == "g" {
		username = rule[0]
		role = rule[1]
		domain = rule[2]
	} else {
		return false, fmt.Errorf("unknown ptype: %s", ptype)
	}

	var group *Group = nil
	for _, g := range a.groups {
		if g.Name == domain {
			group = &g
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
				if role.Resource == resource && role.Actions == actions {
					return true, nil
				}
			}
		} else {
			for _, p := range group.Policies {
				if p.Username == username && p.Resource == resource && p.Actions == actions {
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
		actions = rule[3]
	} else if ptype == "g" {
		username = rule[0]
		role = rule[1]
		domain = rule[2]
	} else {
		return fmt.Errorf("unknown ptype: %s", ptype)
	}

	var group *Group = nil
	for i := range a.groups {
		if a.groups[i].Name == domain {
			group = &a.groups[i]
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
				if role.Resource == resource && role.Actions == actions {
					continue
				}

				newRoles = append(newRoles, role)
			}

			group.Roles[username] = newRoles
		} else {
			policies := []Policy{}

			for _, p := range group.Policies {
				if p.Username == username && p.Resource == resource && p.Actions == actions {
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

	return nil
}

// Adapter
func (a *adapter) RemoveFilteredPolicy(sec string, ptype string, fieldIndex int, fieldValues ...string) error {
	return fmt.Errorf("not implemented")
}

func (a *adapter) GetAllGroupNames() []string {
	a.lock.Lock()
	defer a.lock.Unlock()

	groups := []string{}

	for _, group := range a.groups {
		groups = append(groups, group.Name)
	}

	return groups
}

type Group struct {
	Name      string            `json:"name"`
	Roles     map[string][]Role `json:"roles"`
	UserRoles []MapUserRole     `json:"userroles"`
	Policies  []Policy          `json:"policies"`
}

type Role struct {
	Resource string `json:"resource"`
	Actions  string `json:"actions"`
}

type MapUserRole struct {
	Username string `json:"username"`
	Role     string `json:"role"`
}

type Policy struct {
	Username string `json:"username"`
	Role
}
