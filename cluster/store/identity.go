package store

import (
	"fmt"
	"time"
)

func (s *store) addIdentity(cmd CommandAddIdentity) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	err := s.data.Users.userlist.Add(cmd.Identity)
	if err != nil {
		return fmt.Errorf("the identity with the name '%s' already exists%w", cmd.Identity.Name, ErrBadRequest)
	}

	now := time.Now()

	s.data.Users.UpdatedAt = now

	cmd.Identity.CreatedAt = now
	cmd.Identity.UpdatedAt = now
	s.data.Users.Users[cmd.Identity.Name] = cmd.Identity

	return nil
}

func (s *store) updateIdentity(cmd CommandUpdateIdentity) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if cmd.Name == "$anon" {
		return fmt.Errorf("the identity with the name '%s' can't be updated%w", cmd.Name, ErrBadRequest)
	}

	oldUser, err := s.data.Users.userlist.Get(cmd.Name)
	if err != nil {
		return fmt.Errorf("the identity with the name '%s' doesn't exist%w", cmd.Name, ErrNotFound)
	}

	o, ok := s.data.Users.Users[oldUser.Name]
	if !ok {
		return fmt.Errorf("the identity with the name '%s' doesn't exist%w", cmd.Name, ErrNotFound)
	}

	err = s.data.Users.userlist.Update(cmd.Name, cmd.Identity)
	if err != nil {
		return err
	}

	user, err := s.data.Users.userlist.Get(cmd.Identity.Name)
	if err != nil {
		return fmt.Errorf("the identity with the name '%s' doesn't exist%w", cmd.Identity.Name, ErrNotFound)
	}

	now := time.Now()

	user.CreatedAt = o.CreatedAt
	user.UpdatedAt = now

	s.data.Users.UpdatedAt = now
	delete(s.data.Users.Users, oldUser.Name)
	s.data.Users.Users[user.Name] = user

	s.data.Policies.UpdatedAt = now
	policies := s.data.Policies.Policies[oldUser.Name]
	delete(s.data.Policies.Policies, oldUser.Name)
	s.data.Policies.Policies[user.Name] = policies

	return nil
}

func (s *store) removeIdentity(cmd CommandRemoveIdentity) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	user, err := s.data.Users.userlist.Get(cmd.Name)
	if err != nil {
		return nil
	}

	s.data.Users.userlist.Delete(user.Name)

	delete(s.data.Users.Users, user.Name)
	s.data.Users.UpdatedAt = time.Now()
	delete(s.data.Policies.Policies, user.Name)
	s.data.Policies.UpdatedAt = time.Now()

	return nil
}

func (s *store) IAMIdentityList() Users {
	s.lock.RLock()
	defer s.lock.RUnlock()

	u := Users{
		UpdatedAt: s.data.Users.UpdatedAt,
	}

	for _, user := range s.data.Users.Users {
		u.Users = append(u.Users, user)
	}

	return u
}

func (s *store) IAMIdentityGet(name string) Users {
	s.lock.RLock()
	defer s.lock.RUnlock()

	u := Users{
		UpdatedAt: s.data.Users.UpdatedAt,
	}

	user, err := s.data.Users.userlist.Get(name)
	if err != nil {
		return u
	}

	u.UpdatedAt = user.UpdatedAt

	if user, ok := s.data.Users.Users[user.Name]; ok {
		u.Users = append(u.Users, user)
	}

	return u
}
