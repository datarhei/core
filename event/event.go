package event

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/lithammer/shortuuid/v4"
	"golang.org/x/time/rate"
)

type Event interface {
	Clone() Event
}

type CancelFunc func()

type EventSource interface {
	Events() (<-chan Event, CancelFunc, error)
}

type eventSource struct {
	pubsub *PubSub
}

func (s *eventSource) Events() (<-chan Event, CancelFunc, error) {
	ch, cancel := s.pubsub.Subscribe()

	return ch, cancel, nil
}

type PubSub struct {
	publisher       chan Event
	publisherClosed bool
	publisherLock   sync.Mutex

	ctx    context.Context
	cancel context.CancelFunc

	subscriber     map[string]chan Event
	subscriberLock sync.Mutex

	limiter *rate.Limiter
}

func NewPubSub() *PubSub {
	return NewPubSubWithRateLimit(math.MaxFloat64)
}

func NewPubSubWithRateLimit(limit float64) *PubSub {
	w := &PubSub{
		publisher:       make(chan Event, 1024),
		publisherClosed: false,
		subscriber:      make(map[string]chan Event),
	}

	if limit != math.MaxFloat64 {
		limiter := rate.NewLimiter(rate.Limit(limit), 10000)
		limiter.AllowN(time.Now(), 10000)
	}

	w.ctx, w.cancel = context.WithCancel(context.Background())

	go w.broadcast()

	return w
}

func (w *PubSub) EventSource() EventSource {
	return &eventSource{pubsub: w}
}

func (w *PubSub) Consume(s EventSource, rewrite func(e Event) Event) {
	ch, cancel, err := s.Events()
	if err != nil {
		return
	}

	if rewrite == nil {
		rewrite = func(e Event) Event { return e }
	}

	go func(ch <-chan Event, cancel CancelFunc) {
		for {
			select {
			case <-w.ctx.Done():
				cancel()
				return
			case e, ok := <-ch:
				if !ok {
					return
				}
				w.Publish(rewrite(e))
			}
		}
	}(ch, cancel)
}

func (w *PubSub) Publish(e Event) error {
	if w.limiter != nil {
		// Drop events that exceed the rate limit
		if !w.limiter.Allow() {
			return fmt.Errorf("publishing limit reached")
		}
	}

	event := e.Clone()

	w.publisherLock.Lock()
	defer w.publisherLock.Unlock()

	if w.publisherClosed {
		return fmt.Errorf("writer is closed")
	}

	select {
	case w.publisher <- event:
	default:
		return fmt.Errorf("publisher queue full")
	}

	return nil
}

func (w *PubSub) Close() {
	w.publisherLock.Lock()
	if w.publisherClosed {
		w.publisherLock.Unlock()
		return
	}

	w.cancel()

	close(w.publisher)
	w.publisherClosed = true
	w.publisherLock.Unlock()

	w.subscriberLock.Lock()
	for _, c := range w.subscriber {
		close(c)
	}
	w.subscriber = make(map[string]chan Event)
	w.subscriberLock.Unlock()
}

func (w *PubSub) Subscribe() (<-chan Event, CancelFunc) {
	l := make(chan Event, 128)

	var id string = ""

	w.subscriberLock.Lock()
	for {
		id = shortuuid.New()
		if _, ok := w.subscriber[id]; !ok {
			w.subscriber[id] = l
			break
		}
	}
	w.subscriberLock.Unlock()

	unsubscribe := func() {
		w.subscriberLock.Lock()
		delete(w.subscriber, id)
		w.subscriberLock.Unlock()

		close(l)
	}

	return l, unsubscribe
}

func (w *PubSub) broadcast() {
	for {
		select {
		case <-w.ctx.Done():
			return
		case e := <-w.publisher:
			w.subscriberLock.Lock()
			for _, c := range w.subscriber {
				select {
				case c <- e.Clone():
				default:
				}
			}
			w.subscriberLock.Unlock()
		}
	}
}
