package kvs

import (
	"fmt"
	"io/fs"
	"strings"
	"sync"
	"time"

	"github.com/datarhei/core/v16/cluster/store"
)

type memKVS struct {
	lock sync.Mutex

	locks  map[string]*Lock
	values map[string]store.Value
}

func NewMemoryKVS() (KVS, error) {
	return &memKVS{
		locks:  map[string]*Lock{},
		values: map[string]store.Value{},
	}, nil
}

func (s *memKVS) CreateLock(name string, validUntil time.Time) (*Lock, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	l, ok := s.locks[name]

	if ok {
		if time.Now().Before(l.ValidUntil) {
			return nil, fmt.Errorf("the lock with the name '%s' already exists", name)
		}
	}

	l = &Lock{
		ValidUntil: validUntil,
	}

	s.locks[name] = l

	return l, nil
}

func (s *memKVS) DeleteLock(name string) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	lock, ok := s.locks[name]
	if !ok {
		return fmt.Errorf("the lock with the name '%s' doesn't exist", name)
	}

	lock.Unlock()

	delete(s.locks, name)

	return nil
}

func (s *memKVS) ListLocks() map[string]time.Time {
	s.lock.Lock()
	defer s.lock.Unlock()

	m := map[string]time.Time{}

	for key, lock := range s.locks {
		m[key] = lock.ValidUntil
	}

	return m
}

func (s *memKVS) SetKV(key, value string) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	v := s.values[key]

	v.Value = value
	v.UpdatedAt = time.Now()

	s.values[key] = v

	return nil
}

func (s *memKVS) UnsetKV(key string) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if _, ok := s.values[key]; !ok {
		return fs.ErrNotExist
	}

	delete(s.values, key)

	return nil
}

func (s *memKVS) GetKV(key string) (string, time.Time, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	v, ok := s.values[key]
	if !ok {
		return "", time.Time{}, fs.ErrNotExist
	}

	return v.Value, v.UpdatedAt, nil
}

func (s *memKVS) ListKV(prefix string) map[string]store.Value {
	s.lock.Lock()
	defer s.lock.Unlock()

	m := map[string]store.Value{}

	for key, value := range s.values {
		if !strings.HasPrefix(key, prefix) {
			continue
		}

		m[key] = value
	}

	return m
}
