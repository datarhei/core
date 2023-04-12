package srt

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/datarhei/core/v16/session"
	srt "github.com/datarhei/gosrt"
)

type client struct {
	conn      srt.Conn
	id        string
	createdAt time.Time

	txbytes uint64
	rxbytes uint64

	collector session.Collector

	cancel context.CancelFunc
}

func newClient(conn srt.Conn, id string, collector session.Collector) *client {
	c := &client{
		conn:      conn,
		id:        id,
		createdAt: time.Now(),

		collector: collector,
	}

	var ctx context.Context
	ctx, c.cancel = context.WithCancel(context.Background())

	go c.ticker(ctx)

	return c
}

func (c *client) ticker(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			stats := &srt.Statistics{}
			c.conn.Stats(stats)

			rxbytes := stats.Accumulated.ByteRecv
			txbytes := stats.Accumulated.ByteSent

			c.collector.Ingress(c.id, int64(rxbytes-c.rxbytes))
			c.collector.Egress(c.id, int64(txbytes-c.txbytes))

			c.txbytes = txbytes
			c.rxbytes = rxbytes
		}
	}
}

func (c *client) Close() {
	c.cancel()
	c.conn.Close()
}

// channel represents a stream that is sent to the server
type channel struct {
	pubsub    srt.PubSub
	collector session.Collector
	path      string

	publisher  *client
	subscriber map[string]*client
	lock       sync.RWMutex

	isProxy bool
}

func newChannel(conn srt.Conn, resource string, isProxy bool, collector session.Collector) *channel {
	ch := &channel{
		pubsub:     srt.NewPubSub(srt.PubSubConfig{}),
		path:       resource,
		publisher:  newClient(conn, resource, collector),
		subscriber: make(map[string]*client),
		collector:  collector,
		isProxy:    isProxy,
	}

	addr := conn.RemoteAddr().String()
	ip, _, _ := net.SplitHostPort(addr)

	if collector.IsCollectableIP(ip) {
		collector.RegisterAndActivate(resource, resource, "publish:"+resource, addr)
	}

	return ch
}

func (ch *channel) Close() {
	if ch.publisher == nil {
		return
	}

	ch.publisher.Close()
	ch.publisher = nil
}

func (ch *channel) AddSubscriber(conn srt.Conn, resource string) string {
	addr := conn.RemoteAddr().String()
	ip, _, _ := net.SplitHostPort(addr)

	client := newClient(conn, addr, ch.collector)

	if ch.collector.IsCollectableIP(ip) {
		ch.collector.RegisterAndActivate(addr, resource, "play:"+resource, addr)
	}

	ch.lock.Lock()
	ch.subscriber[addr] = client
	ch.lock.Unlock()

	return addr
}

func (ch *channel) RemoveSubscriber(id string) {
	ch.lock.Lock()
	defer ch.lock.Unlock()

	client := ch.subscriber[id]
	if client != nil {
		delete(ch.subscriber, id)
		client.Close()
	}

	// If this is a proxied channel and the last subscriber leaves,
	// close the channel.
	if len(ch.subscriber) == 0 && ch.isProxy {
		ch.Close()
	}
}
