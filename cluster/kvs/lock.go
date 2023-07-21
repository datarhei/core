package kvs

import (
	"context"
	"sync"
	"time"
)

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
