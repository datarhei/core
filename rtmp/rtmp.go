// Package rtmp provides an RTMP server
package rtmp

import (
	"crypto/tls"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/datarhei/core/log"
	"github.com/datarhei/core/session"

	"github.com/datarhei/joy4/av/avutil"
	"github.com/datarhei/joy4/av/pktque"
	"github.com/datarhei/joy4/av/pubsub"
	"github.com/datarhei/joy4/format"
	"github.com/datarhei/joy4/format/flv/flvio"
	"github.com/datarhei/joy4/format/rtmp"
)

// ErrServerClosed is returned by ListenAndServe (or ListenAndServeTLS) if the server
// has been closed regularly with the Close() function.
var ErrServerClosed = rtmp.ErrServerClosed

func init() {
	format.RegisterAll()
}

type client struct {
	conn      *rtmp.Conn
	id        string
	createdAt time.Time

	txbytes uint64
	rxbytes uint64

	collector session.Collector

	done chan struct{}
}

func newClient(conn *rtmp.Conn, id string, collector session.Collector) *client {
	c := &client{
		conn:      conn,
		id:        id,
		createdAt: time.Now(),

		collector: collector,

		done: make(chan struct{}),
	}

	go c.ticker()

	return c
}

func (c *client) ticker() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.done:
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
	close(c.done)
}

// channel represents a stream that is sent to the server
type channel struct {
	// The packet queue for the stream
	queue *pubsub.Queue

	// The metadata of the stream
	metadata flvio.AMFMap

	// Whether the stream has an audio track
	hasAudio bool

	// Whether the stream has a video track
	hasVideo bool

	collector session.Collector
	path      string

	publisher  *client
	subscriber map[string]*client
	lock       sync.RWMutex
}

func newChannel(conn *rtmp.Conn, collector session.Collector) *channel {
	ch := &channel{
		path:       conn.URL.Path,
		publisher:  newClient(conn, conn.URL.Path, collector),
		subscriber: make(map[string]*client),
		collector:  collector,
	}

	addr := conn.NetConn().RemoteAddr().String()
	ip, _, _ := net.SplitHostPort(addr)

	if collector.IsCollectableIP(ip) {
		reference := strings.TrimSuffix(filepath.Base(conn.URL.Path), filepath.Ext(conn.URL.Path))
		collector.RegisterAndActivate(conn.URL.Path, reference, "publish:"+conn.URL.Path, addr)
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

func (ch *channel) AddSubscriber(conn *rtmp.Conn) string {
	addr := conn.NetConn().RemoteAddr().String()
	ip, _, _ := net.SplitHostPort(addr)

	client := newClient(conn, addr, ch.collector)

	if ch.collector.IsCollectableIP(ip) {
		reference := strings.TrimSuffix(filepath.Base(conn.URL.Path), filepath.Ext(conn.URL.Path))
		ch.collector.RegisterAndActivate(addr, reference, "play:"+conn.URL.Path, addr)
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
}

// Config for a new RTMP server
type Config struct {
	// Logger. Optional.
	Logger log.Logger

	Collector session.Collector

	// The address the RTMP server should listen on, e.g. ":1935"
	Addr string

	// The app path for the streams, e.g. "/live". Optional. Defaults
	// to "/".
	App string

	// A token that needs to be added to the URL as query string
	// in order push or pull a stream. The key for the query
	// parameter is "token". Optional. By default no token is
	// required.
	Token string

	// TLSConfig optionally provides a TLS configuration for use
	// by ListenAndServe. Note that this value is cloned by
	// ListenAndServe, so it's not possible to modify the configuration
	// with methods like tls.Config.SetSessionTicketKeys.
	TLSConfig *tls.Config
}

// Server represents a RTMP server
type Server interface {
	// ListenAndServe starts the RTMP server
	ListenAndServe() error

	// ListenAndServe starts the RTMPS server
	ListenAndServeTLS(certFile, keyFile string) error

	// Close stops the RTMP server and closes all connections
	Close()

	// Channels return a list of currently publishing streams
	Channels() []string
}

// server is an implementation of the Server interface
type server struct {
	// Configuration parameter taken from the Config
	app       string
	token     string
	logger    log.Logger
	collector session.Collector

	// A joy4 RTMP server instance
	server *rtmp.Server

	// Map of publishing channels and a lock to serialize
	// access to the map.
	channels map[string]*channel
	lock     sync.RWMutex
}

// New creates a new RTMP server according to the given config
func New(config Config) (Server, error) {
	if len(config.App) == 0 {
		config.App = "/"
	}

	if config.Logger == nil {
		config.Logger = log.New("RTMP")
	}

	s := &server{
		app:       config.App,
		token:     config.Token,
		logger:    config.Logger,
		collector: config.Collector,
	}

	if s.collector == nil {
		s.collector = session.NewNullCollector()
	}

	s.server = &rtmp.Server{
		Addr:          config.Addr,
		TLSConfig:     config.TLSConfig.Clone(),
		HandlePlay:    s.handlePlay,
		HandlePublish: s.handlePublish,
	}

	s.channels = make(map[string]*channel)

	rtmp.Debug = false

	return s, nil
}

// ListenAndServe starts the RMTP server
func (s *server) ListenAndServe() error {
	return s.server.ListenAndServe()
}

func (s *server) ListenAndServeTLS(certFile, keyFile string) error {
	return s.server.ListenAndServeTLS(certFile, keyFile)
}

func (s *server) Close() {
	// Stop listening
	s.server.Close()

	s.lock.Lock()
	defer s.lock.Unlock()

	// Close all channels
	for _, ch := range s.channels {
		ch.Close()
	}
}

// Channels returns the list of streams that are
// publishing currently
func (s *server) Channels() []string {
	channels := []string{}

	s.lock.RLock()
	defer s.lock.RUnlock()

	for key := range s.channels {
		channels = append(channels, key)
	}

	return channels
}

func (s *server) log(who, action, path, message string, client net.Addr) {
	s.logger.Info().WithFields(log.Fields{
		"who":    who,
		"action": action,
		"path":   path,
		"client": client,
	}).Log(message)
}

// handlePlay is called when a RTMP client wants to play a stream
func (s *server) handlePlay(conn *rtmp.Conn) {
	client := conn.NetConn().RemoteAddr()

	// Check the token
	q := conn.URL.Query()
	token := q.Get("token")

	if len(s.token) != 0 && s.token != token {
		s.log("PLAY", "FORBIDDEN", conn.URL.Path, "invalid token ("+token+")", client)
		conn.Close()
		return
	}

	/*
		ip, _, _ := net.SplitHostPort(client.String())
		if s.collector.IsCollectableIP(ip) {
			maxBitrate := s.collector.MaxEgressBitrate()
			if maxBitrate > 0.0 {
				streamBitrate := s.collector.SessionTopIngressBitrate(conn.URL.Path) * 2.0
				currentBitrate := s.collector.CompanionEgressBitrate() * 1.15

				resultingBitrate := currentBitrate + streamBitrate

				if resultingBitrate >= maxBitrate {
					s.log("PLAY", "FORBIDDEN", conn.URL.Path, "bandwidth limit exceeded", client)
					conn.Close()
					return
				}
			}
		}
	*/

	// Look for the stream
	s.lock.RLock()
	ch := s.channels[conn.URL.Path]
	s.lock.RUnlock()

	if ch != nil {
		// Set the metadata for the client
		conn.SetMetaData(ch.metadata)

		s.log("PLAY", "START", conn.URL.Path, "", client)

		// Get a cursor and apply filters
		cursor := ch.queue.Oldest()

		filters := pktque.Filters{}

		if ch.hasVideo {
			// The first packet has to be a key frame
			filters = append(filters, &pktque.WaitKeyFrame{})
		}

		// Adjust the timestamp such that the stream starts from 0
		filters = append(filters, &pktque.FixTime{StartFromZero: true, MakeIncrement: true})

		demuxer := &pktque.FilterDemuxer{
			Filter:  filters,
			Demuxer: cursor,
		}

		id := ch.AddSubscriber(conn)

		// Transfer the data
		avutil.CopyFile(conn, demuxer)

		ch.RemoveSubscriber(id)

		s.log("PLAY", "STOP", conn.URL.Path, "", client)
	} else {
		s.log("PLAY", "NOTFOUND", conn.URL.Path, "", client)
	}

	conn.Close()
}

// handlePublish is called when a RTMP client wants to publish a stream
func (s *server) handlePublish(conn *rtmp.Conn) {
	client := conn.NetConn().RemoteAddr()

	// Check the token
	q := conn.URL.Query()
	token := q.Get("token")

	if len(s.token) != 0 && s.token != token {
		s.log("PUBLISH", "FORBIDDEN", conn.URL.Path, "invalid token ("+token+")", client)
		conn.Close()
		return
	}

	// Check the app patch
	if !strings.HasPrefix(conn.URL.Path, s.app) {
		s.log("PUBLISH", "FORBIDDEN", conn.URL.Path, "invalid app", client)
		conn.Close()
		return
	}

	// Check the stream if it contains any valid/known streams
	streams, _ := conn.Streams()

	if len(streams) == 0 {
		s.log("PUBLISH", "INVALID", conn.URL.Path, "no streams available", client)
		conn.Close()
		return
	}

	s.lock.Lock()

	ch := s.channels[conn.URL.Path]
	if ch == nil {
		// Create a new channel
		ch = newChannel(conn, s.collector)
		ch.metadata = conn.GetMetaData()
		ch.queue = pubsub.NewQueue()
		ch.queue.WriteHeader(streams)

		for _, stream := range streams {
			typ := stream.Type()

			switch {
			case typ.IsAudio():
				ch.hasAudio = true
			case typ.IsVideo():
				ch.hasVideo = true
			}
		}

		s.channels[conn.URL.Path] = ch
	} else {
		ch = nil
	}

	s.lock.Unlock()

	if ch == nil {
		s.log("PUBLISH", "CONFLICT", conn.URL.Path, "already publishing", client)
		conn.Close()
		return
	}

	s.log("PUBLISH", "START", conn.URL.Path, "", client)

	for _, stream := range streams {
		s.log("PUBLISH", "STREAM", conn.URL.Path, stream.Type().String(), client)
	}

	// Ingest the data
	avutil.CopyPackets(ch.queue, conn)

	s.lock.Lock()
	delete(s.channels, conn.URL.Path)
	s.lock.Unlock()

	ch.Close()

	s.log("PUBLISH", "STOP", conn.URL.Path, "", client)

	conn.Close()
}
