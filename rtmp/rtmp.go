// Package rtmp provides an RTMP server
package rtmp

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/datarhei/core/v16/cluster/node"
	enctoken "github.com/datarhei/core/v16/encoding/token"
	"github.com/datarhei/core/v16/event"
	"github.com/datarhei/core/v16/iam"
	iamidentity "github.com/datarhei/core/v16/iam/identity"
	"github.com/datarhei/core/v16/log"
	rtmpurl "github.com/datarhei/core/v16/rtmp/url"
	"github.com/datarhei/core/v16/session"

	"github.com/datarhei/joy4/av/avutil"
	"github.com/datarhei/joy4/av/pktque"
	"github.com/datarhei/joy4/format"
	"github.com/datarhei/joy4/format/flv"
	"github.com/datarhei/joy4/format/rtmp"
)

// ErrServerClosed is returned by ListenAndServe (or ListenAndServeTLS) if the server
// has been closed regularly with the Close() function.
var ErrServerClosed = rtmp.ErrServerClosed

func init() {
	format.RegisterAll()
}

// Config for a new RTMP server
type Config struct {
	// Logger. Optional.
	Logger log.Logger

	Collector session.Collector

	// The address the RTMP server should listen on, e.g. ":1935"
	Addr string

	// The address the RTMPS server should listen on, e.g. ":1936"
	TLSAddr string

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

	Proxy *node.Manager

	IAM iam.IAM
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

	PlayFLV(remote net.Addr, u *url.URL) (io.ReadCloser, error)

	event.MediaSource
}

// server is an implementation of the Server interface
type server struct {
	// Configuration parameter taken from the Config
	app       string
	token     string
	logger    log.Logger
	collector session.Collector

	// A joy4 RTMP server instance
	server    *rtmp.Server
	tlsServer *rtmp.Server

	// Map of publishing channels and a lock to serialize
	// access to the map.
	channels map[string]*channel
	lock     sync.RWMutex

	proxy *node.Manager

	iam iam.IAM

	events *event.PubSub
}

// New creates a new RTMP server according to the given config
func New(config Config) (Server, error) {
	if len(config.App) == 0 {
		config.App = "/"
	}

	if config.Logger == nil {
		config.Logger = log.New("")
	}

	s := &server{
		app:       filepath.Join("/", config.App),
		token:     config.Token,
		logger:    config.Logger,
		collector: config.Collector,
		proxy:     config.Proxy,
		iam:       config.IAM,
		events:    event.NewPubSub(),
	}

	if s.collector == nil {
		s.collector = session.NewNullCollector()
	}

	s.server = &rtmp.Server{
		Addr:          config.Addr,
		HandlePlay:    s.handlePlay,
		HandlePublish: s.handlePublish,
	}

	if len(config.TLSAddr) != 0 {
		s.tlsServer = &rtmp.Server{
			Addr:          config.TLSAddr,
			TLSConfig:     config.TLSConfig.Clone(),
			HandlePlay:    s.handlePlay,
			HandlePublish: s.handlePublish,
		}
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
	if s.tlsServer == nil {
		return fmt.Errorf("RTMPS server is not configured")
	}

	return s.tlsServer.ListenAndServeTLS(certFile, keyFile)
}

func (s *server) Close() {
	// Stop listening
	s.server.Close()

	if s.tlsServer != nil {
		s.tlsServer.Close()
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	// Close all channels
	for _, ch := range s.channels {
		ch.Close()
	}
}

// Channels returns the list of streams that are publishing currently, excluding proxied channels
func (s *server) Channels() []string {
	channels := []string{}

	s.lock.RLock()
	defer s.lock.RUnlock()

	for key, channel := range s.channels {
		if channel.isProxy {
			continue
		}

		channels = append(channels, key)
	}

	return channels
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

type PlayError struct {
	Message  string
	Identity string
	Playpath string
	Details  string
	Err      error
}

func (e PlayError) Error() string {
	return fmt.Sprintf("%s %s %s %s", e.Identity, e.Message, e.Playpath, e.Details)
}

func (s *server) play(remote net.Addr, u *url.URL) (*channel, string, string, error) {
	playpath, token, isStreamkey := rtmpurl.GetToken(u)

	playpath, _ = rtmpurl.RemovePathPrefix(playpath, s.app)

	identity, err := s.findIdentityFromStreamKey(token)
	if err != nil {
		s.logger.Debug().WithError(err).Log("invalid streamkey")
		return nil, "", playpath, &PlayError{"FORBIDDEN", "", playpath, "invalid streamkey (" + token + ")", err}
	}

	if identity == "$anon" && isStreamkey {
		// If the token was part of the path, we add it back
		playpath = filepath.Join(playpath, token)
	}

	domain := s.findDomainFromPlaypath(playpath)
	resource := playpath

	if !s.iam.Enforce(identity, domain, "rtmp", resource, "PLAY") {
		return nil, identity, playpath, &PlayError{"FORBIDDEN", "", playpath, "access denied", nil}
	}

	// Look for the stream
	s.lock.RLock()
	ch := s.channels[playpath]
	s.lock.RUnlock()

	if ch == nil && s.proxy != nil {
		// Check in the cluster for that stream
		url, err := s.proxy.MediaGetURL("rtmp", playpath)
		if err != nil {
			return nil, identity, playpath, &PlayError{"NOTFOUND", identity, playpath, "", err}
		}

		url = url.JoinPath(token)
		peerurl := url.String()

		src, err := avutil.Open(peerurl)
		if err != nil {
			s.logger.Error().WithField("address", url).WithError(err).Log("Proxying address failed")
			return nil, identity, playpath, &PlayError{"NOTFOUND", identity, playpath, "", err}
		}

		c := newConnectionFromDemuxCloser(src)

		wg := sync.WaitGroup{}
		wg.Add(1)

		go func() {
			s.log(identity, "PLAY", "PROXYSTART", peerurl, "", remote)
			wg.Done()
			err := s.publish(c, playpath, remote, identity, true)
			if err != nil {
				s.logger.Error().WithField("address", url).WithError(err).Log("Proxying address failed")
			}
			s.log(identity, "PLAY", "PROXYSTOP", peerurl, "", remote)
		}()

		// Wait for the goroutine to start
		wg.Wait()

		// Wait for channel to become available
		ticker := time.NewTicker(200 * time.Millisecond)
		tickerStart := time.Now()

		for range ticker.C {
			s.lock.RLock()
			ch = s.channels[playpath]
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

	if ch == nil {
		return nil, identity, playpath, &PlayError{"NOTFOUND", identity, playpath, "", err}
	}

	return ch, identity, playpath, nil
}

// handlePlay is called when a RTMP client wants to play a stream
func (s *server) handlePlay(conn *rtmp.Conn) {
	defer conn.Close()

	remote := conn.NetConn().RemoteAddr()

	ch, identity, playpath, err := s.play(remote, conn.URL)
	if err != nil {
		var rtmperr PlayError
		if errors.As(err, &rtmperr) {
			s.log(rtmperr.Identity, "PLAY", rtmperr.Message, rtmperr.Playpath, rtmperr.Details, remote)
		}
		return
	}

	s.log(identity, "PLAY", "START", playpath, "", remote)

	// Get a cursor and apply filters
	cursor := ch.queue.Oldest()

	filters := pktque.Filters{}

	if ch.hasVideo {
		// The first packet has to be a key frame
		filters = append(filters, &pktque.WaitKeyFrame{})
	}

	// Adjust the timestamp such that the stream starts from 0
	filters = append(filters, &pktque.FixTime{StartFromZero: true, MakeIncrement: false})

	demuxer := &pktque.FilterDemuxer{
		Filter:  filters,
		Demuxer: cursor,
	}

	id := ch.AddSubscriber(conn, remote.String(), playpath, identity)

	// Transfer the data, blocks until done
	avutil.CopyFile(conn, demuxer)

	ch.RemoveSubscriber(id)

	s.log(identity, "PLAY", "STOP", playpath, "", remote)
}

// handlePublish is called when a RTMP client wants to publish a stream
func (s *server) handlePublish(conn *rtmp.Conn) {
	defer conn.Close()

	remote := conn.NetConn().RemoteAddr()
	playpath, token, isStreamkey := rtmpurl.GetToken(conn.URL)

	playpath, app := rtmpurl.RemovePathPrefix(playpath, s.app)

	identity, err := s.findIdentityFromStreamKey(token)
	if err != nil {
		s.logger.Debug().WithError(err).Log("invalid streamkey")
		s.log(identity, "PUBLISH", "FORBIDDEN", playpath, "invalid streamkey ("+token+")", remote)
		return
	}

	if identity == "$anon" && isStreamkey {
		// If the token was part of the path, we add it back
		playpath = filepath.Join(playpath, token)
	}

	// Check the app patch
	if app != s.app {
		s.log(identity, "PUBLISH", "FORBIDDEN", playpath, "invalid app", remote)
		return
	}

	domain := s.findDomainFromPlaypath(playpath)
	resource := playpath

	if !s.iam.Enforce(identity, domain, "rtmp", resource, "PUBLISH") {
		s.log(identity, "PUBLISH", "FORBIDDEN", playpath, "access denied", remote)
		return
	}

	// Check if this stream is already published on the cluster
	if s.proxy != nil {
		_, err = s.proxy.MediaGetURL("rtmp", playpath)
		if err == nil {
			s.log(identity, "PUBLISH", "CONFLICT", playpath, "already publishing", remote)
			return
		}
	}

	err = s.publish(conn, playpath, remote, identity, false)
	if err != nil {
		s.logger.WithField("path", conn.URL.Path).WithError(err).Log("")
	}
}

func (s *server) publish(src connection, playpath string, remote net.Addr, identity string, isProxy bool) error {
	// Check the streams if it contains any valid/known streams
	streams, _ := src.Streams()

	if len(streams) == 0 {
		s.log(identity, "PUBLISH", "INVALID", playpath, "no streams available", remote)
		return fmt.Errorf("no streams are available")
	}

	s.lock.Lock()

	ch := s.channels[playpath]
	if ch == nil {
		reference := strings.TrimPrefix(strings.TrimSuffix(playpath, filepath.Ext(playpath)), s.app+"/")

		// Create a new channel
		ch = newChannel(src, playpath, reference, remote, streams, isProxy, identity, s.collector)

		for _, stream := range streams {
			typ := stream.Type()

			switch {
			case typ.IsAudio():
				ch.hasAudio = true
			case typ.IsVideo():
				ch.hasVideo = true
			}
		}

		s.channels[playpath] = ch
	} else {
		ch = nil
	}

	s.lock.Unlock()

	if ch == nil {
		s.log(identity, "PUBLISH", "CONFLICT", playpath, "already publishing", remote)
		return fmt.Errorf("already publishing")
	}

	s.log(identity, "PUBLISH", "START", playpath, "", remote)

	for _, stream := range streams {
		s.log(identity, "PUBLISH", "STREAM", playpath, stream.Type().String(), remote)
	}

	if !isProxy {
		s.events.Publish(event.NewMediaEvent("create", playpath))
	}

	// Ingest the data, blocks until done
	avutil.CopyPackets(ch.queue, src)

	s.lock.Lock()
	delete(s.channels, playpath)
	s.lock.Unlock()

	ch.Close()

	if !isProxy {
		s.events.Publish(event.NewMediaEvent("remove", playpath))
	}

	s.log(identity, "PUBLISH", "STOP", playpath, "", remote)

	return nil
}

func (s *server) findIdentityFromStreamKey(key string) (string, error) {
	if len(key) == 0 {
		return "$anon", nil
	}

	var identity iamidentity.Verifier = nil
	var err error = nil
	var isDefaultIdentity bool = false

	var token string

	username, token := enctoken.Unmarshal(key)
	if len(username) == 0 {
		// Legacy compatibility. If the key is only the token, then it
		// is assumed that it belongs to the superuser.
		identity = s.iam.GetDefaultVerifier()
		isDefaultIdentity = true
	} else {
		identity, err = s.iam.GetVerifier(username)
	}

	if err != nil {
		return "$anon", nil
	}

	if ok, err := identity.VerifyServiceToken(token); !ok {
		if err != nil {
			err = fmt.Errorf("invalid token: %w", err)
		} else {
			err = fmt.Errorf("invlid token")
		}

		if isDefaultIdentity {
			// If verifying the token fails and if this is the superuser,
			// then assume anonymous access
			return "$anon", nil
		}

		return identity.Name(), err
	}

	return identity.Name(), nil
}

// findDomainFromPlaypath finds the domain in the path. The domain is
// the first path element. If there's only one path element, it is not
// considered the domain. It is assumed that the app is not part of
// the provided path.
func (s *server) findDomainFromPlaypath(path string) string {
	elements := rtmpurl.SplitPath(path)
	if len(elements) == 1 {
		return "$none"
	}

	domain := elements[0]

	if s.iam.HasDomain(domain) {
		return domain
	}

	return "$none"
}

func (s *server) Events() (<-chan event.Event, event.CancelFunc, error) {
	ch, cancel := s.events.Subscribe()

	return ch, cancel, nil
}

func (s *server) MediaList() []string {
	return s.Channels()
}

func (s *server) PlayFLV(remote net.Addr, u *url.URL) (io.ReadCloser, error) {
	ch, identity, playpath, err := s.play(remote, u)
	if err != nil {
		return nil, err
	}

	s.log(identity, "FLVPLAY", "START", playpath, "", remote)

	// Get a cursor and apply filters
	cursor := ch.queue.Oldest()

	filters := pktque.Filters{}

	if ch.hasVideo {
		// The first packet has to be a key frame
		filters = append(filters, &pktque.WaitKeyFrame{})
	}

	// Adjust the timestamp such that the stream starts from 0
	filters = append(filters, &pktque.FixTime{StartFromZero: true, MakeIncrement: false})

	demuxer := &pktque.FilterDemuxer{
		Filter:  filters,
		Demuxer: cursor,
	}

	r, w := io.Pipe()

	muxer := flv.NewMuxer(w)

	conn := newConnectionFromMuxer(muxer)

	id := ch.AddSubscriber(conn, remote.String(), playpath, identity)

	go func() {
		defer w.Close()

		// Transfer the data, blocks until done
		avutil.CopyFile(muxer, demuxer)

		ch.RemoveSubscriber(id)

		s.log(identity, "FLVPLAY", "STOP", playpath, "", remote)
	}()

	return r, err
}
