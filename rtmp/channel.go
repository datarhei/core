package rtmp

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/datarhei/core/v16/session"
	"github.com/datarhei/joy4/av"
	"github.com/datarhei/joy4/av/pubsub"
)

type client struct {
	conn      connection
	id        string
	createdAt time.Time

	txbytes uint64
	rxbytes uint64

	collector session.Collector

	cancel context.CancelFunc
}

func newClient(conn connection, id string, collector session.Collector) *client {
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
			txbytes := c.conn.TxBytes()
			rxbytes := c.conn.RxBytes()

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
	c.collector.Close(c.id)
}

// channel represents a stream that is sent to the server
type channel struct {
	// The packet queue for the stream
	queue *pubsub.DurationQueue

	// The metadata of the stream
	streams []av.CodecData

	// Whether the stream has an audio track
	hasAudio bool

	// Whether the stream has a video track
	hasVideo bool

	collector session.Collector
	path      string
	reference string

	publisher  *client
	subscriber map[string]*client
	lock       sync.RWMutex

	isProxy bool
}

func newChannel(conn connection, playPath, reference string, remote net.Addr, streams []av.CodecData, isProxy bool, identity string, collector session.Collector) *channel {
	ch := &channel{
		path:       playPath,
		reference:  reference,
		publisher:  newClient(conn, playPath, collector),
		subscriber: make(map[string]*client),
		collector:  collector,
		streams:    streams,
		queue:      pubsub.NewDurationQueue(),
		isProxy:    isProxy,
	}

	ch.queue.WriteHeader(streams)

	addr := remote.String()
	ip, _, _ := net.SplitHostPort(addr)

	if collector.IsCollectableIP(ip) {
		extra := map[string]interface{}{
			"name":   identity,
			"method": "publish",
		}
		collector.RegisterAndActivate(ch.path, ch.reference, ch.path, addr)
		collector.Extra(ch.path, extra)
	}

	return ch
}

func (ch *channel) Close() {
	if ch.publisher == nil {
		return
	}

	ch.publisher.Close()
	ch.publisher = nil

	ch.queue.Close()
}

func (ch *channel) AddSubscriber(conn connection, addr string, playPath, identity string) string {
	ip, _, _ := net.SplitHostPort(addr)

	client := newClient(conn, addr, ch.collector)

	if ch.collector.IsCollectableIP(ip) {
		extra := map[string]interface{}{
			"name":   identity,
			"method": "play",
		}
		ch.collector.RegisterAndActivate(addr, ch.reference, playPath, addr)
		ch.collector.Extra(addr, extra)
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
