package store

import (
	"fmt"
	"time"
)

func (s *store) setPolicies(cmd CommandSetPolicies) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	now := time.Now()

	if cmd.Name != "$anon" {
		user, err := s.data.Users.userlist.Get(cmd.Name)
		if err != nil {
			return fmt.Errorf("the identity with the name '%s' doesn't exist", cmd.Name)
		}

		u, ok := s.data.Users.Users[user.Name]
		if !ok {
			return fmt.Errorf("the identity with the name '%s' doesn't exist", cmd.Name)
		}

		u.UpdatedAt = now
		s.data.Users.Users[user.Name] = u
	}

	for i, p := range cmd.Policies {
		if len(p.Domain) != 0 {
			continue
		}

		p.Domain = "$none"
		cmd.Policies[i] = p
	}

	delete(s.data.Policies.Policies, cmd.Name)
	s.data.Policies.Policies[cmd.Name] = cmd.Policies
	s.data.Policies.UpdatedAt = now

	return nil
}

func (s *store) ListPolicies() Policies {
	s.lock.RLock()
	defer s.lock.RUnlock()

	p := Policies{
		UpdatedAt: s.data.Policies.UpdatedAt,
	}

	for _, policies := range s.data.Policies.Policies {
		p.Policies = append(p.Policies, policies...)
	}

	return p
}

func (s *store) ListUserPolicies(name string) Policies {
	s.lock.RLock()
	defer s.lock.RUnlock()

	p := Policies{
		UpdatedAt: s.data.Policies.UpdatedAt,
	}

	user, err := s.data.Users.userlist.Get(name)
	if err != nil {
		return p
	}

	p.UpdatedAt = user.UpdatedAt
	p.Policies = append(p.Policies, s.data.Policies.Policies[user.Name]...)

	return p
}
