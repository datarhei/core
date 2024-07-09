package store

import (
	"fmt"
	"time"

	"github.com/datarhei/core/v16/iam/access"
)

func (s *store) setPolicies(cmd CommandSetPolicies) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	now := time.Now()

	if cmd.Name != "$anon" {
		user, err := s.data.Users.userlist.Get(cmd.Name)
		if err != nil {
			return fmt.Errorf("unknown identity %s%w", cmd.Name, ErrNotFound)
		}

		u, ok := s.data.Users.Users[user.Name]
		if !ok {
			return fmt.Errorf("unknown identity %s%w", cmd.Name, ErrNotFound)
		}

		u.UpdatedAt = now
		s.data.Users.Users[user.Name] = u
	}

	for i, p := range cmd.Policies {
		p = s.updatePolicy(p)

		if len(p.Domain) == 0 {
			p.Domain = "$none"
		}

		cmd.Policies[i] = p
	}

	delete(s.data.Policies.Policies, cmd.Name)
	s.data.Policies.Policies[cmd.Name] = cmd.Policies
	s.data.Policies.UpdatedAt = now

	return nil
}

func (s *store) IAMPolicyList() Policies {
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

func (s *store) IAMIdentityPolicyList(name string) Policies {
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

// updatePolicy updates a policy such that the resource type is split off the resource
func (s *store) updatePolicy(p access.Policy) access.Policy {
	if len(p.Types) == 0 {
		p.Types, p.Resource = access.DecodeResource(p.Resource)
	}

	return p
}
