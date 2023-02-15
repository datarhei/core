package srt

import (
	"container/ring"
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/datarhei/core/v16/iam"
	"github.com/datarhei/core/v16/log"
	"github.com/datarhei/core/v16/session"
	"github.com/datarhei/core/v16/srt/url"
	srt "github.com/datarhei/gosrt"
)

// ErrServerClosed is returned by ListenAndServe if the server
// has been closed regularly with the Close() function.
var ErrServerClosed = srt.ErrServerClosed

// Config for a new SRT server
type Config struct {
	// The address the SRT server should listen on, e.g. ":1935"
	Addr string

	// A token that needs to be added to the URL as query string
	// in order push or pull a stream. The key for the query
	// parameter is "token". Optional. By default no token is
	// required.
	Token string

	Passphrase string

	// Logger. Optional.
	Logger log.Logger

	Collector session.Collector

	SRTLogTopics []string

	IAM iam.IAM
}

// Server represents a SRT server
type Server interface {
	// ListenAndServe starts the SRT server
	ListenAndServe() error

	// Close stops the RTMP server and closes all connections
	Close()

	// Channels return a list of currently publishing streams
	Channels() Channels
}

// server implements the Server interface
type server struct {
	addr       string
	token      string
	passphrase string

	collector session.Collector

	server srt.Server

	// Map of publishing channels and a lock to serialize
	// access to the map.
	channels map[string]*channel
	lock     sync.RWMutex

	logger log.Logger

	srtlogger       srt.Logger
	srtloggerCancel context.CancelFunc
	srtlog          map[string]*ring.Ring
	srtlogLock      sync.RWMutex

	iam iam.IAM
}

func New(config Config) (Server, error) {
	s := &server{
		addr:       config.Addr,
		token:      config.Token,
		passphrase: config.Passphrase,
		collector:  config.Collector,
		iam:        config.IAM,
		logger:     config.Logger,
	}

	if s.collector == nil {
		s.collector = session.NewNullCollector()
	}

	if s.logger == nil {
		s.logger = log.New("")
	}

	s.srtlogger = srt.NewLogger(config.SRTLogTopics)

	s.srtlogLock.Lock()
	s.srtlog = make(map[string]*ring.Ring)
	s.srtlogLock.Unlock()

	s.channels = make(map[string]*channel)

	srtconfig := srt.DefaultConfig()

	srtconfig.Passphrase = config.Passphrase
	srtconfig.Logger = s.srtlogger

	s.server.Addr = config.Addr
	s.server.Config = &srtconfig
	s.server.HandleConnect = s.handleConnect
	s.server.HandlePublish = s.handlePublish
	s.server.HandleSubscribe = s.handleSubscribe

	return s, nil
}

func (s *server) ListenAndServe() error {
	var ctx context.Context
	ctx, s.srtloggerCancel = context.WithCancel(context.Background())

	go s.srtlogListener(ctx)

	return s.server.ListenAndServe()
}

func (s *server) Close() {
	s.server.Shutdown()

	s.srtloggerCancel()
}

type Log struct {
	Timestamp time.Time
	Message   []string
}

type Connection struct {
	Log   map[string][]Log
	Stats srt.Statistics
}

type Channels struct {
	Publisher   map[string]uint32
	Subscriber  map[string][]uint32
	Connections map[uint32]Connection
	Log         map[string][]Log
}

func (s *server) Channels() Channels {
	st := Channels{
		Publisher:   map[string]uint32{},
		Subscriber:  map[string][]uint32{},
		Connections: map[uint32]Connection{},
		Log:         map[string][]Log{},
	}

	s.lock.RLock()
	for id, ch := range s.channels {
		socketId := ch.publisher.conn.SocketId()
		st.Publisher[id] = socketId

		stats := &srt.Statistics{}
		ch.publisher.conn.Stats(stats)

		st.Connections[socketId] = Connection{
			Stats: *stats,
			Log:   map[string][]Log{},
		}

		for _, c := range ch.subscriber {
			socketId := c.conn.SocketId()
			st.Subscriber[id] = append(st.Subscriber[id], socketId)

			stats := &srt.Statistics{}
			c.conn.Stats(stats)

			st.Connections[socketId] = Connection{
				Stats: *stats,
				Log:   map[string][]Log{},
			}
		}
	}
	s.lock.RUnlock()

	s.srtlogLock.RLock()
	for topic, buf := range s.srtlog {

		buf.Do(func(l interface{}) {
			if l == nil {
				return
			}

			ll := l.(srt.Log)

			log := Log{
				Timestamp: ll.Time,
				Message:   strings.Split(ll.Message, "\n"),
			}

			if ll.SocketId != 0 {
				if _, ok := st.Connections[ll.SocketId]; ok {
					st.Connections[ll.SocketId].Log[topic] = append(st.Connections[ll.SocketId].Log[topic], log)
				}
			} else {
				st.Log[topic] = append(st.Log[topic], log)
			}
		})
	}
	s.srtlogLock.RUnlock()

	return st
}

func (s *server) srtlogListener(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case l := <-s.srtlogger.Listen():
			s.srtlogLock.Lock()
			if buf := s.srtlog[l.Topic]; buf == nil {
				s.srtlog[l.Topic] = ring.New(100)
			}
			s.srtlog[l.Topic].Value = l
			s.srtlog[l.Topic] = s.srtlog[l.Topic].Next()
			s.srtlogLock.Unlock()
		}
	}
}

func (s *server) log(handler, action, resource, message string, client net.Addr) {
	s.logger.Info().WithFields(log.Fields{
		"handler":  handler,
		"status":   action,
		"resource": resource,
		"client":   client.String(),
	}).Log(message)
}

func (s *server) handleConnect(req srt.ConnRequest) srt.ConnType {
	mode := srt.REJECT
	client := req.RemoteAddr()
	streamId := req.StreamId()

	si, err := url.ParseStreamId(streamId)
	if err != nil {
		s.log("CONNECT", "INVALID", "", err.Error(), client)
		return srt.REJECT
	}

	if len(si.Resource) == 0 {
		s.log("CONNECT", "INVALID", "", "stream resource not provided", client)
		return srt.REJECT
	}

	if si.Mode == "publish" {
		mode = srt.PUBLISH
	} else if si.Mode == "request" {
		mode = srt.SUBSCRIBE
	} else {
		s.log("CONNECT", "INVALID", si.Resource, "invalid connection mode", client)
		return srt.REJECT
	}

	if len(s.passphrase) != 0 {
		if !req.IsEncrypted() {
			s.log("CONNECT", "FORBIDDEN", si.Resource, "connection has to be encrypted", client)
			return srt.REJECT
		}

		if err := req.SetPassphrase(s.passphrase); err != nil {
			s.log("CONNECT", "FORBIDDEN", si.Resource, err.Error(), client)
			return srt.REJECT
		}
	} else {
		if req.IsEncrypted() {
			s.log("CONNECT", "INVALID", si.Resource, "connection must not be encrypted", client)
			return srt.REJECT
		}
	}

	identity, err := s.findIdentityFromToken(si.Token)
	if err != nil {
		s.logger.Debug().WithError(err).Log("invalid token")
		s.log("PUBLISH", "FORBIDDEN", si.Resource, "invalid token", client)
		return srt.REJECT
	}

	domain := s.findDomainFromPlaypath(si.Resource)
	resource := "srt:" + si.Resource
	action := "PLAY"
	if mode == srt.PUBLISH {
		action = "PUBLISH"
	}

	if !s.iam.Enforce(identity, domain, resource, action) {
		s.log("PUBLISH", "FORBIDDEN", si.Resource, "access denied", client)
		return srt.REJECT
	}

	return mode
}

func (s *server) handlePublish(conn srt.Conn) {
	streamId := conn.StreamId()
	client := conn.RemoteAddr()

	si, _ := url.ParseStreamId(streamId)

	// Look for the stream
	s.lock.Lock()
	ch := s.channels[si.Resource]
	if ch == nil {
		ch = newChannel(conn, si.Resource, false, s.collector)
		s.channels[si.Resource] = ch
	} else {
		ch = nil
	}
	s.lock.Unlock()

	if ch == nil {
		s.log("PUBLISH", "CONFLICT", si.Resource, "already publishing", client)
		conn.Close()
		return
	}

	s.log("PUBLISH", "START", si.Resource, "", client)

	ch.pubsub.Publish(conn)

	s.lock.Lock()
	delete(s.channels, si.Resource)
	s.lock.Unlock()

	ch.Close()

	s.log("PUBLISH", "STOP", si.Resource, "", client)

	conn.Close()
}

func (s *server) handleSubscribe(conn srt.Conn) {
	streamId := conn.StreamId()
	client := conn.RemoteAddr()

	si, _ := url.ParseStreamId(streamId)

	// Look for the stream
	s.lock.RLock()
	ch := s.channels[si.Resource]
	s.lock.RUnlock()

	if ch == nil {
		s.log("SUBSCRIBE", "NOTFOUND", si.Resource, "no publisher for this resource found", client)
		conn.Close()
		return
	}

	s.log("SUBSCRIBE", "START", si.Resource, "", client)

	id := ch.AddSubscriber(conn, si.Resource)

	ch.pubsub.Subscribe(conn)

	s.log("SUBSCRIBE", "STOP", si.Resource, "", client)

	ch.RemoveSubscriber(id)

	conn.Close()
}

func (s *server) findIdentityFromToken(key string) (string, error) {
	if len(key) == 0 {
		return "$anon", nil
	}

	var identity iam.IdentityVerifier
	var err error

	var token string

	elements := strings.Split(key, ":")
	if len(elements) == 1 {
		identity, err = s.iam.GetDefaultIdentity()
		token = elements[0]
	} else {
		identity, err = s.iam.GetIdentity(elements[0])
		token = elements[1]
	}

	if err != nil {
		return "$anon", nil
	}

	if ok, err := identity.VerifyServiceToken(token); !ok {
		return "$anon", fmt.Errorf("invalid token: %w", err)
	}

	return identity.Name(), nil
}

func (s *server) findDomainFromPlaypath(path string) string {
	elements := strings.Split(path, "/")
	if len(elements) == 1 {
		return "$none"
	}

	domain := elements[0]

	if s.iam.IsDomain(domain) {
		return domain
	}

	return "$none"
}
