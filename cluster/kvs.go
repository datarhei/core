package cluster

import (
	"context"
	"fmt"
	"io/fs"
	"strings"
	"sync"
	"time"

	"github.com/datarhei/core/v16/cluster/store"
	"github.com/datarhei/core/v16/log"
)

type KVS interface {
	CreateLock(name string, validUntil time.Time) (*Lock, error)
	DeleteLock(name string) error
	ListLocks() map[string]time.Time

	SetKV(key, value string) error
	UnsetKV(key string) error
	GetKV(key string) (string, time.Time, error)
	ListKV(prefix string) map[string]store.Value
}

type Lock struct {
	ValidUntil time.Time
	ctx        context.Context
	cancel     context.CancelFunc
	lock       sync.Mutex
}

func (l *Lock) Expired() <-chan struct{} {
	l.lock.Lock()
	defer l.lock.Unlock()

	if l.ctx == nil {
		l.ctx, l.cancel = context.WithDeadline(context.Background(), l.ValidUntil)

		go func(l *Lock) {
			<-l.ctx.Done()

			l.lock.Lock()
			defer l.lock.Unlock()
			if l.cancel != nil {
				l.cancel()
			}
		}(l)
	}

	return l.ctx.Done()
}

func (l *Lock) Unlock() {
	l.lock.Lock()
	defer l.lock.Unlock()

	if l.cancel != nil {
		l.ValidUntil = time.Now()
		l.cancel()
	}
}

type clusterKVS struct {
	cluster Cluster
	logger  log.Logger
}

func NewClusterKVS(cluster Cluster, logger log.Logger) (KVS, error) {
	s := &clusterKVS{
		cluster: cluster,
		logger:  logger,
	}

	if s.logger == nil {
		s.logger = log.New("")
	}

	return s, nil
}

func (s *clusterKVS) CreateLock(name string, validUntil time.Time) (*Lock, error) {
	s.logger.Debug().WithFields(log.Fields{
		"name":        name,
		"valid_until": validUntil,
	}).Log("Create lock")
	return s.cluster.CreateLock("", name, validUntil)
}

func (s *clusterKVS) DeleteLock(name string) error {
	s.logger.Debug().WithField("name", name).Log("Delete lock")
	return s.cluster.DeleteLock("", name)
}

func (s *clusterKVS) ListLocks() map[string]time.Time {
	s.logger.Debug().Log("List locks")
	return s.cluster.ListLocks()
}

func (s *clusterKVS) SetKV(key, value string) error {
	s.logger.Debug().WithFields(log.Fields{
		"key":   key,
		"value": value,
	}).Log("Set KV")
	return s.cluster.SetKV("", key, value)
}

func (s *clusterKVS) UnsetKV(key string) error {
	s.logger.Debug().WithField("key", key).Log("Unset KV")
	return s.cluster.UnsetKV("", key)
}

func (s *clusterKVS) GetKV(key string) (string, time.Time, error) {
	s.logger.Debug().WithField("key", key).Log("Get KV")
	return s.cluster.GetKV("", key)
}

func (s *clusterKVS) ListKV(prefix string) map[string]store.Value {
	s.logger.Debug().Log("List KV")
	return s.cluster.ListKV(prefix)
}

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
