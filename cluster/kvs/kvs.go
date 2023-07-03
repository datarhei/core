package kvs

import (
	"context"
	"sync"
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
