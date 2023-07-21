package kvs

import (
	"time"

	"github.com/datarhei/core/v16/cluster/store"
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
