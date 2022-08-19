// Package rtmp provides an RTMP server
package rtmp

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/datarhei/core/v16/cluster"
	"github.com/datarhei/core/v16/log"
	"github.com/datarhei/core/v16/session"

	"github.com/datarhei/joy4/av/avutil"
	"github.com/datarhei/joy4/av/pktque"
	"github.com/datarhei/joy4/format"
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

	Cluster cluster.ClusterReader
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
	server    *rtmp.Server
	tlsServer *rtmp.Server

	// Map of publishing channels and a lock to serialize
	// access to the map.
	channels map[string]*channel
	lock     sync.RWMutex

	cluster cluster.ClusterReader
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
		app:       config.App,
		token:     config.Token,
		logger:    config.Logger,
		collector: config.Collector,
		cluster:   config.Cluster,
	}

	if s.collector == nil {
		s.collector = session.NewNullCollector()
	}

	if s.cluster == nil {
		s.cluster = cluster.NewDummyClusterReader()
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
		"client": client.String(),
	}).Log(message)
}

// handlePlay is called when a RTMP client wants to play a stream
func (s *server) handlePlay(conn *rtmp.Conn) {
	defer conn.Close()

	remote := conn.NetConn().RemoteAddr()

	// Check the token
	q := conn.URL.Query()
	token := q.Get("token")

	if len(s.token) != 0 && s.token != token {
		s.log("PLAY", "FORBIDDEN", conn.URL.Path, "invalid token ("+token+")", remote)
		return
	}

	// Look for the stream
	s.lock.RLock()
	ch := s.channels[conn.URL.Path]
	s.lock.RUnlock()

	if ch == nil {
		// Check in the cluster for that stream
		url, err := s.cluster.GetURL("rtmp:" + conn.URL.Path)
		if err != nil {
			s.log("PLAY", "NOTFOUND", conn.URL.Path, "", remote)
			return
		}

		src, err := avutil.Open(url)
		if err != nil {
			s.logger.Error().WithField("address", url).WithError(err).Log("Proxying address failed")
			s.log("PLAY", "NOTFOUND", conn.URL.Path, "", remote)
			return
		}

		c := newConnectionFromDemuxer(src)

		wg := sync.WaitGroup{}
		wg.Add(1)

		go func() {
			s.log("PLAY", "PROXYSTART", url, "", remote)
			wg.Done()
			err := s.publish(c, conn.URL, remote, true)
			if err != nil {
				s.logger.Error().WithField("address", url).WithError(err).Log("Proxying address failed")
			}
			s.log("PLAY", "PROXYSTOP", url, "", remote)
		}()

		// Wait for the goroutine to start
		wg.Wait()

		// Wait for channel to become available
		ticker := time.NewTicker(200 * time.Millisecond)
		tickerStart := time.Now()

		for range ticker.C {
			s.lock.RLock()
			ch = s.channels[conn.URL.Path]
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
		// Send the metadata to the client
		conn.WriteHeader(ch.streams)

		s.log("PLAY", "START", conn.URL.Path, "", remote)

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

		// Transfer the data, blocks until done
		avutil.CopyFile(conn, demuxer)

		ch.RemoveSubscriber(id)

		s.log("PLAY", "STOP", conn.URL.Path, "", remote)
	} else {
		s.log("PLAY", "NOTFOUND", conn.URL.Path, "", remote)
	}
}

// handlePublish is called when a RTMP client wants to publish a stream
func (s *server) handlePublish(conn *rtmp.Conn) {
	defer conn.Close()

	remote := conn.NetConn().RemoteAddr()

	if len(s.token) != 0 {
		// Check the token
		token := conn.URL.Query().Get("token")

		if s.token != token {
			s.log("PUBLISH", "FORBIDDEN", conn.URL.Path, "invalid token ("+token+")", remote)
			return
		}
	}

	// Check the app patch
	if !strings.HasPrefix(conn.URL.Path, s.app) {
		s.log("PUBLISH", "FORBIDDEN", conn.URL.Path, "invalid app", remote)
		return
	}

	err := s.publish(conn, conn.URL, remote, false)
	if err != nil {
		s.logger.WithField("path", conn.URL.Path).WithError(err).Log("")
	}
}

func (s *server) publish(src connection, u *url.URL, remote net.Addr, isProxy bool) error {
	// Check the streams if it contains any valid/known streams
	streams, _ := src.Streams()

	if len(streams) == 0 {
		s.log("PUBLISH", "INVALID", u.Path, "no streams available", remote)
		return fmt.Errorf("no streams are available")
	}

	s.lock.Lock()

	ch := s.channels[u.Path]
	if ch == nil {
		reference := strings.TrimPrefix(strings.TrimSuffix(u.Path, filepath.Ext(u.Path)), s.app+"/")

		// Create a new channel
		ch = newChannel(src, u, reference, remote, streams, isProxy, s.collector)

		for _, stream := range streams {
			typ := stream.Type()

			switch {
			case typ.IsAudio():
				ch.hasAudio = true
			case typ.IsVideo():
				ch.hasVideo = true
			}
		}

		s.channels[u.Path] = ch
	} else {
		ch = nil
	}

	s.lock.Unlock()

	if ch == nil {
		s.log("PUBLISH", "CONFLICT", u.Path, "already publishing", remote)
		return fmt.Errorf("already publishing")
	}

	s.log("PUBLISH", "START", u.Path, "", remote)

	for _, stream := range streams {
		s.log("PUBLISH", "STREAM", u.Path, stream.Type().String(), remote)
	}

	// Ingest the data, blocks until done
	avutil.CopyPackets(ch.queue, src)

	s.lock.Lock()
	delete(s.channels, u.Path)
	s.lock.Unlock()

	ch.Close()

	s.log("PUBLISH", "STOP", u.Path, "", remote)

	return nil
}
