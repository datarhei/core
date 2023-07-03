package cluster

import (
	"time"

	"github.com/datarhei/core/v16/cluster/kvs"
	"github.com/datarhei/core/v16/cluster/store"
	"github.com/datarhei/core/v16/log"
)

type clusterKVS struct {
	cluster Cluster
	logger  log.Logger
}

func NewClusterKVS(cluster Cluster, logger log.Logger) (kvs.KVS, error) {
	s := &clusterKVS{
		cluster: cluster,
		logger:  logger,
	}

	if s.logger == nil {
		s.logger = log.New("")
	}

	return s, nil
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
	s.logger.Debug().WithField("key", key).Log("Get KV")
	return s.cluster.GetKV("", key)
}

func (s *clusterKVS) ListKV(prefix string) map[string]store.Value {
	s.logger.Debug().Log("List KV")
	return s.cluster.ListKV(prefix)
}
