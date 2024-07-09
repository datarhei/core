package store

import (
	"fmt"
	"time"
)

func (s *store) createLock(cmd CommandCreateLock) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	validUntil, ok := s.data.Locks[cmd.Name]

	if ok {
		if time.Now().Before(validUntil) {
			return fmt.Errorf("the lock with the ID '%s' already exists%w", cmd.Name, ErrBadRequest)
		}
	}

	s.data.Locks[cmd.Name] = cmd.ValidUntil

	return nil
}

func (s *store) deleteLock(cmd CommandDeleteLock) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if _, ok := s.data.Locks[cmd.Name]; !ok {
		return nil
	}

	delete(s.data.Locks, cmd.Name)

	return nil
}

func (s *store) clearLocks(_ CommandClearLocks) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	for name, validUntil := range s.data.Locks {
		if time.Now().Before(validUntil) {
			// Lock is still valid
			continue
		}

		delete(s.data.Locks, name)
	}

	return nil
}

func (s *store) LockHasLock(name string) bool {
	s.lock.RLock()
	defer s.lock.RUnlock()

	_, ok := s.data.Locks[name]

	return ok
}

func (s *store) LockList() map[string]time.Time {
	s.lock.RLock()
	defer s.lock.RUnlock()

	m := map[string]time.Time{}

	for key, value := range s.data.Locks {
		m[key] = value
	}

	return m
}
