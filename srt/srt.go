package srt

import (
	"container/ring"
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/datarhei/core/v16/cluster/proxy"
	"github.com/datarhei/core/v16/iam"
	iamidentity "github.com/datarhei/core/v16/iam/identity"
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

	Proxy proxy.ProxyReader

	IAM iam.IAM
}

// Server represents a SRT server
type Server interface {
	// ListenAndServe starts the SRT server
	ListenAndServe() error

	// Close stops the RTMP server and closes all connections
	Close()

	// Channels return a list of currently publishing streams
	Channels() []Channel
}

// server implements the Server interface
type server struct {
	addr       string
	token      string
	passphrase string

	collector session.Collector

	server srt.Server

	// Map of publishing channels and a lock to serialize access to the map. The map
	// index is the name of the resource.
	channels map[string]*channel
	lock     sync.RWMutex

	logger log.Logger

	srtlogger       srt.Logger
	srtloggerCancel context.CancelFunc
	srtlog          map[string]*ring.Ring // Per logtopic a dedicated ring buffer
	srtlogLock      sync.RWMutex

	proxy proxy.ProxyReader

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
		proxy:      config.Proxy,
	}

	if s.collector == nil {
		s.collector = session.NewNullCollector()
	}

	if s.proxy == nil {
		s.proxy = proxy.NewNullProxyReader()
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

type Channel struct {
	Name        string                // Resource
	SocketId    uint32                // Socketid
	Subscriber  []uint32              // List of subscribed sockedids
	Connections map[uint32]Connection // Map from socketid to connection
	Log         map[string][]Log      // Map of topic to log entries
}

func (s *server) Channels() []Channel {
	channels := []Channel{}

	socket2channel := map[uint32]int{}

	s.lock.RLock()
	for id, ch := range s.channels {
		socketId := ch.publisher.conn.SocketId()
		channel := Channel{
			Name:        id,
			SocketId:    socketId,
			Subscriber:  []uint32{},
			Connections: map[uint32]Connection{},
			Log:         map[string][]Log{},
		}

		socket2channel[socketId] = len(channels)

		stats := &srt.Statistics{}
		ch.publisher.conn.Stats(stats)

		channel.Connections[socketId] = Connection{
			Stats: *stats,
			Log:   map[string][]Log{},
		}

		for _, c := range ch.subscriber {
			socketId := c.conn.SocketId()
			channel.Subscriber = append(channel.Subscriber, socketId)

			stats := &srt.Statistics{}
			c.conn.Stats(stats)

			channel.Connections[socketId] = Connection{
				Stats: *stats,
				Log:   map[string][]Log{},
			}

			socket2channel[socketId] = len(channels)
		}

		channels = append(channels, channel)
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

			if ll.SocketId == 0 {
				return
			}

			ch, ok := socket2channel[ll.SocketId]
			if !ok {
				return
			}

			channel := channels[ch]

			conn, ok := channel.Connections[ll.SocketId]
			if !ok {
				return
			}

			conn.Log[topic] = append(conn.Log[topic], log)

			channel.Connections[ll.SocketId] = conn

			channels[ch] = channel
		})
	}
	s.srtlogLock.RUnlock()

	return channels
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

func (s *server) log(who, handler, action, resource, message string, client net.Addr) {
	s.logger.Info().WithFields(log.Fields{
		"who":      who,
		"handler":  handler,
		"action":   action,
		"resource": resource,
		"client":   client.String(),
	}).Log(message)
}

func (s *server) handleConnect(req srt.ConnRequest) srt.ConnType {
	mode := srt.REJECT
	client := req.RemoteAddr()
	streamId := req.StreamId()

	if req.Version() != 5 {
		s.log("", "CONNECT", "INVALID", streamId, "unsupported version", client)
		return srt.REJECT
	}

	si, err := url.ParseStreamId(streamId)
	if err != nil {
		s.log("", "CONNECT", "INVALID", streamId, err.Error(), client)
		return srt.REJECT
	}

	if len(si.Resource) == 0 {
		s.log("", "CONNECT", "INVALID", streamId, "stream resource not provided", client)
		return srt.REJECT
	}

	if si.Mode == "publish" {
		mode = srt.PUBLISH
	} else if si.Mode == "request" {
		mode = srt.SUBSCRIBE
	} else {
		s.log("", "CONNECT", "INVALID", si.Resource, "invalid connection mode", client)
		return srt.REJECT
	}

	if len(s.passphrase) != 0 {
		if !req.IsEncrypted() {
			s.log("", "CONNECT", "FORBIDDEN", si.Resource, "connection has to be encrypted", client)
			return srt.REJECT
		}

		if err := req.SetPassphrase(s.passphrase); err != nil {
			s.log("", "CONNECT", "FORBIDDEN", si.Resource, err.Error(), client)
			return srt.REJECT
		}
	} else {
		if req.IsEncrypted() {
			s.log("", "CONNECT", "INVALID", si.Resource, "connection must not be encrypted", client)
			return srt.REJECT
		}
	}

	identity, err := s.findIdentityFromToken(si.Token)
	if err != nil {
		s.logger.Debug().WithError(err).Log("invalid token")
		s.log(identity, "CONNECT", "FORBIDDEN", si.Resource, "invalid token", client)
		return srt.REJECT
	}

	domain := s.findDomainFromPlaypath(si.Resource)
	resource := "srt:" + si.Resource
	action := "PLAY"
	if mode == srt.PUBLISH {
		action = "PUBLISH"
	}

	if !s.iam.Enforce(identity, domain, resource, action) {
		s.log(identity, "CONNECT", "FORBIDDEN", si.Resource, "access denied", client)
		return srt.REJECT
	}

	return mode
}

func (s *server) handlePublish(conn srt.Conn) {
	s.publish(conn, false)
}

func (s *server) publish(conn srt.Conn, isProxy bool) error {
	streamId := conn.StreamId()
	client := conn.RemoteAddr()

	si, _ := url.ParseStreamId(streamId)
	identity, _ := s.findIdentityFromToken(si.Token)

	// Look for the stream
	s.lock.Lock()
	ch := s.channels[si.Resource]
	if ch == nil {
		ch = newChannel(conn, si.Resource, isProxy, s.collector)
		s.channels[si.Resource] = ch
	} else {
		ch = nil
	}
	s.lock.Unlock()

	if ch == nil {
		s.log(identity, "PUBLISH", "CONFLICT", si.Resource, "already publishing", client)
		conn.Close()
		return fmt.Errorf("already publishing this resource")
	}

	s.log(identity, "PUBLISH", "START", si.Resource, "", client)

	// Blocks until connection closes
	err := ch.pubsub.Publish(conn)

	s.lock.Lock()
	delete(s.channels, si.Resource)
	s.lock.Unlock()

	ch.Close()

	s.log(identity, "PUBLISH", "STOP", si.Resource, err.Error(), client)

	conn.Close()

	return nil
}

func (s *server) handleSubscribe(conn srt.Conn) {
	defer conn.Close()

	streamId := conn.StreamId()
	client := conn.RemoteAddr()

	si, _ := url.ParseStreamId(streamId)
	identity, _ := s.findIdentityFromToken(si.Token)

	// Look for the stream locally
	s.lock.RLock()
	ch := s.channels[si.Resource]
	s.lock.RUnlock()

	if ch == nil {
		// Check in the cluster for the stream and proxy it
		srturl, err := s.proxy.GetURL("srt", si.Resource)
		if err != nil {
			s.log(identity, "PLAY", "NOTFOUND", si.Resource, "no publisher for this resource found", client)
			return
		}

		peerurl := srturl.String()

		config := srt.DefaultConfig()
		config.StreamId = streamId
		config.Latency = 200 * time.Millisecond // This might be a value obtained from the cluster
		address, err := config.UnmarshalURL(peerurl)
		peerurl = config.MarshalURL(address)
		if err != nil {
			s.logger.Error().WithField("address", peerurl).WithError(err).Log("Parsing proxy address failed")
			s.log(identity, "PLAY", "NOTFOUND", si.Resource, "no publisher for this resource found", client)
			return
		}
		src, err := srt.Dial("srt", address, config)
		if err != nil {
			s.logger.Error().WithField("address", peerurl).WithError(err).Log("Proxying address failed")
			s.log(identity, "PLAY", "NOTFOUND", si.Resource, "no publisher for this resource found", client)
			return
		}

		wg := sync.WaitGroup{}
		wg.Add(1)

		go func() {
			s.log(identity, "PLAY", "PROXYSTART", peerurl, "", client)
			wg.Done()
			err := s.publish(src, true)
			if err != nil {
				s.logger.Error().WithField("address", srturl).WithError(err).Log("Proxying address failed")
			}
			s.log(identity, "PLAY", "PROXYSTOP", peerurl, "", client)
		}()

		// Wait for the goroutine to start
		wg.Wait()

		// Wait for channel to become available
		ticker := time.NewTicker(200 * time.Millisecond)
		tickerStart := time.Now()

		for range ticker.C {
			s.lock.RLock()
			ch = s.channels[si.Resource]
			s.lock.RUnlock()

			if ch != nil {
				break
			}

			if time.Since(tickerStart) > 2*time.Second {
				break
			}
		}

		ticker.Stop()
	}

	if ch != nil {
		s.log(identity, "PLAY", "START", si.Resource, "", client)

		id := ch.AddSubscriber(conn, si.Resource)

		// Blocks until connection closes
		err := ch.pubsub.Subscribe(conn)

		s.log(identity, "PLAY", "STOP", si.Resource, err.Error(), client)

		ch.RemoveSubscriber(id)
	}
}

func (s *server) findIdentityFromToken(key string) (string, error) {
	if len(key) == 0 {
		return "$anon", nil
	}

	var identity iamidentity.Verifier
	var err error

	var token string

	before, after, found := strings.Cut(key, ":")
	if !found {
		identity = s.iam.GetDefaultVerifier()
		token = before
	} else {
		identity, err = s.iam.GetVerifier(before)
		token = after
	}

	if err != nil {
		return "$anon", nil
	}

	if ok, err := identity.VerifyServiceToken(token); !ok {
		if err != nil {
			err = fmt.Errorf("invalid token: %w", err)
		} else {
			err = fmt.Errorf("invalid token")
		}

		return "$anon", err
	}

	return identity.Name(), nil
}

func (s *server) findDomainFromPlaypath(path string) string {
	elements := strings.Split(path, "/")
	if len(elements) == 1 {
		return "$none"
	}

	domain := elements[0]

	if s.iam.HasDomain(domain) {
		return domain
	}

	return "$none"
}
