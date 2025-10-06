package event

import (
	"context"
	"fmt"
	"sync"

	"github.com/lithammer/shortuuid/v4"
)

type Event interface {
	Clone() Event
}

type CancelFunc func()

type EventSource interface {
	Events() (<-chan Event, CancelFunc, error)
}

type PubSub struct {
	publisher       chan Event
	publisherClosed bool
	publisherLock   sync.Mutex

	ctx    context.Context
	cancel context.CancelFunc

	subscriber     map[string]chan Event
	subscriberLock sync.Mutex
}

func NewPubSub() *PubSub {
	w := &PubSub{
		publisher:       make(chan Event, 1024),
		publisherClosed: false,
		subscriber:      make(map[string]chan Event),
	}

	w.ctx, w.cancel = context.WithCancel(context.Background())

	go w.broadcast()

	return w
}

func (w *PubSub) Publish(e Event) error {
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
	w.cancel()

	w.publisherLock.Lock()
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
	l := make(chan Event, 1024)

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
				pp := e.Clone()

				select {
				case c <- pp:
				default:
				}
			}
			w.subscriberLock.Unlock()
		}
	}
}
