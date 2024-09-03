package session

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/datarhei/core/v16/encoding/json"
	"github.com/datarhei/core/v16/io/fs"
	"github.com/datarhei/core/v16/log"
	"github.com/lestrrat-go/strftime"
)

// Config is the configuration for creating a new registry
type Config struct {
	// PersistFS is a filesystem in whose root the session history will be persisted. If it is nil, the
	// history will not be persisted.
	PersistFS fs.Filesystem

	// PersistInterval is the duration between persisting the history. If 0 the history will
	// only be persisted at stopping the collector. If negative, the history will not be persisted.
	PersistInterval time.Duration

	// SessionLogPattern is a path inside the PersistFS where the individual sessions will
	// be logged. The path can contain strftime-placeholders in order to split the log files.
	// If this string is empty or PersistFS is nil, the sessions will not be logged.
	LogPattern string

	// SessionLogBufferDuration is the maximum duration session logs should be buffered before written
	// to the filesystem. If not provided, the default of 15 seconds will be used.
	LogBufferDuration time.Duration

	// Logger is an instance of a logger. If it is nil, no logs
	// will be written.
	Logger log.Logger
}

type RegistryReader interface {
	// Collectors returns an array of all registered IDs
	Collectors() []string

	// Collector returns the collector with the ID, or nil if the ID is not registered
	Collector(id string) Collector

	// Summary returns the summary from a collector with the ID, or an empty summary if the ID is not registered
	Summary(id string) Summary

	// Active returns the active sessions from a collector with the ID, or an empty list if the ID is not registered
	Active(id string) []Session
}

// The Registry interface
type Registry interface {
	// Register returns a new collector from conf and registers it under the id and error is nil. In case of error
	// the returned collector is nil and the error is not nil.
	Register(id string, conf CollectorConfig) (Collector, error)

	// Unregister unregisters the collector with the ID, returns error if the ID is not registered
	Unregister(id string) error

	// UnregisterAll unregisters al registered collectors
	UnregisterAll()

	RegistryReader

	Close() error
}

type registry struct {
	collector map[string]*collector

	persist struct {
		fs                fs.Filesystem
		interval          time.Duration
		cancel            context.CancelFunc
		sessionsCh        chan Session
		sessionsWg        sync.WaitGroup
		logPattern        *strftime.Strftime
		logBufferDuration time.Duration
		lock              sync.Mutex
	}

	logger log.Logger

	lock sync.Mutex
}

// New returns a new registry for collectors that implement the Registry interface. The error
// is non-nil if the registry can't be created.
func New(config Config) (Registry, error) {
	r := &registry{
		collector: make(map[string]*collector),
		logger:    config.Logger,
	}

	r.persist.fs = config.PersistFS
	r.persist.interval = config.PersistInterval

	if r.logger == nil {
		r.logger = log.New("Session")
	}

	pattern, err := strftime.New(config.LogPattern)
	if err != nil {
		return nil, err
	}

	r.persist.logPattern = pattern
	r.persist.logBufferDuration = config.LogBufferDuration
	if r.persist.logBufferDuration <= 0 {
		r.persist.logBufferDuration = 15 * time.Second
	}

	r.startPersister()

	return r, nil
}

func (r *registry) Close() error {
	r.UnregisterAll()
	r.stopPersister()

	return nil
}

func (r *registry) startPersister() {
	if r.persist.fs == nil {
		return
	}

	r.persist.lock.Lock()
	defer r.persist.lock.Unlock()

	if r.persist.interval > 0 {
		if r.persist.cancel == nil {
			ctx, cancel := context.WithCancel(context.Background())
			r.persist.cancel = cancel

			go r.historyPersister(ctx, r.persist.interval)
		}
	}

	if r.persist.logPattern != nil {
		r.persist.sessionsCh = make(chan Session, 128)
		r.persist.sessionsWg.Add(1)
	}

	go r.sessionPersister(r.persist.logPattern, r.persist.logBufferDuration, r.persist.sessionsCh)
}

func (r *registry) stopPersister() {
	if r.persist.fs == nil {
		return
	}

	r.persist.lock.Lock()
	defer r.persist.lock.Unlock()

	if r.persist.cancel != nil {
		r.persist.cancel()
		r.persist.cancel = nil
	}

	if r.persist.sessionsCh != nil {
		close(r.persist.sessionsCh)
		r.persist.sessionsWg.Wait()
		r.persist.sessionsCh = nil
	}
}

func (r *registry) historyPersister(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	r.logger.Debug().WithField("interval", interval).Log("History persister started")

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.persistAllCollectors()
		}
	}
}

func (r *registry) sessionPersister(pattern *strftime.Strftime, bufferDuration time.Duration, ch <-chan Session) {
	defer r.persist.sessionsWg.Done()

	r.logger.Debug().WithFields(log.Fields{
		"pattern": pattern.Pattern(),
		"buffer":  bufferDuration,
	}).Log("Session persister started")

	buffer := &bytes.Buffer{}
	path := pattern.FormatString(time.Now())

	enc := json.NewEncoder(buffer)

	ticker := time.NewTicker(bufferDuration)
	defer ticker.Stop()

	splitTime := time.Time{}

loop:
	for {
		select {
		case session, ok := <-ch:
			if !ok {
				break loop
			}
			currentPath := pattern.FormatString(session.ClosedAt)
			if currentPath != path && session.ClosedAt.After(splitTime) {
				if buffer.Len() > 0 {
					_, err := r.persist.fs.AppendFileReader(path, buffer, -1)
					if err != nil {
						r.logger.Error().WithError(err).WithField("path", path).Log("")
					}
				}
				buffer.Reset()
				r.logger.Info().WithFields(log.Fields{
					"previous": path,
					"current":  currentPath,
				}).Log("Creating new session log file")
				path = currentPath
				splitTime = session.ClosedAt
			}

			enc.Encode(&session)
		case t := <-ticker.C:
			if buffer.Len() > 0 {
				_, err := r.persist.fs.AppendFileReader(path, buffer, -1)
				if err != nil {
					r.logger.Error().WithError(err).WithField("path", path).Log("")
				} else {
					r.logger.Debug().WithField("path", path).Log("Persisted session log")
				}
			}
			currentPath := pattern.FormatString(t)
			if currentPath != path && t.After(splitTime) {
				buffer.Reset()
				r.logger.Info().WithFields(log.Fields{
					"previous": path,
					"current":  currentPath,
				}).Log("Creating new session log file")
				path = currentPath
				splitTime = t
			}
		}
	}

	if buffer.Len() > 0 {
		_, err := r.persist.fs.AppendFileReader(path, buffer, -1)
		if err != nil {
			r.logger.Error().WithError(err).WithField("path", path).Log("")
		} else {
			r.logger.Debug().WithField("path", path).Log("Persisted session log")
		}
		buffer.Reset()
	}

	buffer = nil
}

func (r *registry) persistAllCollectors() {
	wg := sync.WaitGroup{}

	r.lock.Lock()
	for id, m := range r.collector {
		wg.Add(1)
		go func(id string, m Collector) {
			defer wg.Done()

			s, err := m.Snapshot()
			if err != nil {
				return
			}

			sink, err := NewHistorySink(r.persist.fs, "/"+id+".json")
			if err != nil {
				return
			}

			if err := s.Persist(sink); err != nil {
				return
			}
		}(id, m)
	}
	r.lock.Unlock()

	wg.Wait()
}

func (r *registry) Register(id string, conf CollectorConfig) (Collector, error) {
	if len(id) == 0 {
		return nil, fmt.Errorf("invalid ID. empty IDs are not allowed")
	}

	re := regexp.MustCompile(`^[0-9A-Za-z]+$`)
	if !re.MatchString(id) {
		return nil, fmt.Errorf("invalid ID. only numbers and letters are allowed")
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	_, ok := r.collector[id]
	if ok {
		return nil, fmt.Errorf("a collector with the ID '%s' already exists", id)
	}

	collectHistory := r.persist.fs != nil && r.persist.interval >= 0

	m, err := newCollector(id, r.persist.sessionsCh, r.logger, collectHistory, conf)
	if err != nil {
		return nil, err
	}

	if r.persist.fs != nil {
		s, err := NewHistorySource(r.persist.fs, "/"+id+".json")
		if err == nil {
			err = m.Restore(s)
		}

		if err != nil {
			r.logger.Warn().WithError(err).WithField("file", "/"+id+".json").Log("Can't restore history")
			r.persist.fs.Rename("/"+id+".json", "/"+id+"."+strconv.FormatInt(time.Now().Unix(), 10)+".json")
		}
	}

	m.start()

	r.collector[id] = m

	return m, nil
}

func (r *registry) Unregister(id string) error {
	r.lock.Lock()
	defer r.lock.Unlock()

	return r.unregister(id)
}

func (r *registry) unregister(id string) error {
	m, ok := r.collector[id]
	if !ok {
		return fmt.Errorf("a collector with the ID '%s' doesn't exist", id)
	}

	m.stop()

	delete(r.collector, id)

	if r.persist.fs != nil && r.persist.interval >= 0 {
		s, err := m.Snapshot()
		if err != nil {
			return err
		}

		if s != nil {
			sink, err := NewHistorySink(r.persist.fs, "/"+id+".json")
			if err != nil {
				return err
			}

			if err := s.Persist(sink); err != nil {
				return err
			}
		}
	}

	return nil
}

func (r *registry) Collectors() []string {
	collectors := []string{}

	r.lock.Lock()
	defer r.lock.Unlock()

	for name := range r.collector {
		collectors = append(collectors, name)
	}

	return collectors
}

func (r *registry) Collector(id string) Collector {
	r.lock.Lock()
	defer r.lock.Unlock()

	m, ok := r.collector[id]
	if !ok {
		return nil
	}

	return m
}

func (r *registry) UnregisterAll() {
	r.lock.Lock()
	defer r.lock.Unlock()

	for id := range r.collector {
		r.unregister(id)
	}
}

func (r *registry) Summary(id string) Summary {
	r.lock.Lock()
	defer r.lock.Unlock()

	m, ok := r.collector[id]
	if !ok {
		return Summary{}
	}

	return m.Summary()
}

func (r *registry) Active(id string) []Session {
	r.lock.Lock()
	defer r.lock.Unlock()

	m, ok := r.collector[id]
	if !ok {
		return []Session{}
	}

	return m.Active()
}
