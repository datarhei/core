package cluster

import (
	"time"

	"github.com/datarhei/core/v16/cluster/kvs"
	"github.com/datarhei/core/v16/cluster/store"
	"github.com/datarhei/core/v16/log"
)

func (c *cluster) CreateLock(origin string, name string, validUntil time.Time) (*kvs.Lock, error) {
	if ok, _ := c.IsClusterDegraded(); ok {
		return nil, ErrDegraded
	}

	if !c.IsRaftLeader() {
		err := c.forwarder.CreateLock(origin, name, validUntil)
		if err != nil {
			return nil, err
		}

		l := &kvs.Lock{
			ValidUntil: validUntil,
		}

		return l, nil
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

func (c *cluster) DeleteLock(origin string, name string) error {
	if ok, _ := c.IsClusterDegraded(); ok {
		return ErrDegraded
	}

	if !c.IsRaftLeader() {
		return c.forwarder.DeleteLock(origin, name)
	}

	cmd := &store.Command{
		Operation: store.OpDeleteLock,
		Data: &store.CommandDeleteLock{
			Name: name,
		},
	}

	return c.applyCommand(cmd)
}

func (c *cluster) ListLocks() map[string]time.Time {
	return c.store.ListLocks()
}

func (c *cluster) SetKV(origin, key, value string) error {
	if ok, _ := c.IsClusterDegraded(); ok {
		return ErrDegraded
	}

	if !c.IsRaftLeader() {
		return c.forwarder.SetKV(origin, key, value)
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

func (c *cluster) UnsetKV(origin, key string) error {
	if ok, _ := c.IsClusterDegraded(); ok {
		return ErrDegraded
	}

	if !c.IsRaftLeader() {
		return c.forwarder.UnsetKV(origin, key)
	}

	cmd := &store.Command{
		Operation: store.OpUnsetKV,
		Data: &store.CommandUnsetKV{
			Key: key,
		},
	}

	return c.applyCommand(cmd)
}

func (c *cluster) GetKV(origin, key string, stale bool) (string, time.Time, error) {
	if !stale {
		if ok, _ := c.IsClusterDegraded(); ok {
			return "", time.Time{}, ErrDegraded
		}

		if !c.IsRaftLeader() {
			return c.forwarder.GetKV(origin, key)
		}
	}

	value, err := c.store.GetFromKVS(key)
	if err != nil {
		return "", time.Time{}, err
	}

	return value.Value, value.UpdatedAt, nil
}

func (c *cluster) ListKV(prefix string) map[string]store.Value {
	storeValues := c.store.ListKVS(prefix)

	return storeValues
}

type ClusterKVS interface {
	kvs.KVS

	AllowStaleKeys(allow bool)
}

type clusterKVS struct {
	cluster    Cluster
	allowStale bool
	logger     log.Logger
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
	s.allowStale = allow
}

func (s *clusterKVS) CreateLock(name string, validUntil time.Time) (*kvs.Lock, error) {
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
	s.logger.Debug().WithFields(log.Fields{
		"key":   key,
		"stale": s.allowStale,
	}).Log("Get KV")
	return s.cluster.GetKV("", key, s.allowStale)
}

func (s *clusterKVS) ListKV(prefix string) map[string]store.Value {
	s.logger.Debug().Log("List KV")
	return s.cluster.ListKV(prefix)
}
