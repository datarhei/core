package cluster

import (
	"fmt"
	"sync"
	"time"

	"github.com/datarhei/core/v16/cluster/kvs"
	"github.com/datarhei/core/v16/cluster/store"
	"github.com/datarhei/core/v16/log"
)

func (c *cluster) LockCreate(origin string, name string, validUntil time.Time) (*kvs.Lock, error) {
	if !c.IsRaftLeader() {
		err := c.forwarder.LockCreate(origin, name, validUntil)
		if err != nil {
			return nil, err
		}

		l := &kvs.Lock{
			ValidUntil: validUntil,
		}

		return l, nil
	}

	if c.store.LockHasLock(name) {
		return nil, fmt.Errorf("the lock '%s' already exists", name)
	}

	cmd := &store.Command{
		Operation: store.OpCreateLock,
		Data: &store.CommandCreateLock{
			Name:       name,
			ValidUntil: validUntil,
		},
	}

	err := c.applyCommand(cmd)
	if err != nil {
		return nil, err
	}

	l := &kvs.Lock{
		ValidUntil: validUntil,
	}

	return l, nil
}

func (c *cluster) LockDelete(origin string, name string) error {
	if !c.IsRaftLeader() {
		return c.forwarder.LockDelete(origin, name)
	}

	cmd := &store.Command{
		Operation: store.OpDeleteLock,
		Data: &store.CommandDeleteLock{
			Name: name,
		},
	}

	return c.applyCommand(cmd)
}

func (c *cluster) KVSet(origin, key, value string) error {
	if !c.IsRaftLeader() {
		return c.forwarder.KVSet(origin, key, value)
	}

	cmd := &store.Command{
		Operation: store.OpSetKV,
		Data: &store.CommandSetKV{
			Key:   key,
			Value: value,
		},
	}

	return c.applyCommand(cmd)
}

func (c *cluster) KVUnset(origin, key string) error {
	if !c.IsRaftLeader() {
		return c.forwarder.KVUnset(origin, key)
	}

	cmd := &store.Command{
		Operation: store.OpUnsetKV,
		Data: &store.CommandUnsetKV{
			Key: key,
		},
	}

	return c.applyCommand(cmd)
}

func (c *cluster) KVGet(origin, key string, stale bool) (string, time.Time, error) {
	if !stale {
		if !c.IsRaftLeader() {
			return c.forwarder.KVGet(origin, key)
		}
	}

	value, err := c.store.KVSGetValue(key)
	if err != nil {
		return "", time.Time{}, err
	}

	return value.Value, value.UpdatedAt, nil
}

type ClusterKVS interface {
	kvs.KVS

	AllowStaleKeys(allow bool)
}

type clusterKVS struct {
	cluster Cluster
	logger  log.Logger

	allowStale bool
	staleLock  sync.Mutex
}

func NewClusterKVS(cluster Cluster, logger log.Logger) (ClusterKVS, error) {
	s := &clusterKVS{
		cluster: cluster,
		logger:  logger,
	}

	if s.logger == nil {
		s.logger = log.New("")
	}

	return s, nil
}

func (s *clusterKVS) AllowStaleKeys(allow bool) {
	s.staleLock.Lock()
	defer s.staleLock.Unlock()

	s.allowStale = allow
}

func (s *clusterKVS) CreateLock(name string, validUntil time.Time) (*kvs.Lock, error) {
	s.logger.Debug().WithFields(log.Fields{
		"name":        name,
		"valid_until": validUntil,
	}).Log("Create lock")
	return s.cluster.LockCreate("", name, validUntil)
}

func (s *clusterKVS) DeleteLock(name string) error {
	s.logger.Debug().WithField("name", name).Log("Delete lock")
	return s.cluster.LockDelete("", name)
}

func (s *clusterKVS) ListLocks() map[string]time.Time {
	s.logger.Debug().Log("List locks")
	return s.cluster.Store().LockList()
}

func (s *clusterKVS) SetKV(key, value string) error {
	s.logger.Debug().WithFields(log.Fields{
		"key":   key,
		"value": value,
	}).Log("Set KV")
	return s.cluster.KVSet("", key, value)
}

func (s *clusterKVS) UnsetKV(key string) error {
	s.logger.Debug().WithField("key", key).Log("Unset KV")
	return s.cluster.KVUnset("", key)
}

func (s *clusterKVS) GetKV(key string) (string, time.Time, error) {
	s.staleLock.Lock()
	stale := s.allowStale
	s.staleLock.Unlock()

	s.logger.Debug().WithFields(log.Fields{
		"key":   key,
		"stale": stale,
	}).Log("Get KV")
	return s.cluster.KVGet("", key, stale)
}

func (s *clusterKVS) ListKV(prefix string) map[string]store.Value {
	s.logger.Debug().Log("List KV")
	return s.cluster.Store().KVSList(prefix)
}
