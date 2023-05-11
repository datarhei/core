package store

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/datarhei/core/v16/log"
	"github.com/datarhei/core/v16/restream/app"

	"github.com/hashicorp/raft"
)

type Store interface {
	raft.FSM

	ProcessList() []app.Config
	GetProcess(id string) (app.Config, error)
}

type operation string

const (
	OpAddProcess    operation = "addProcess"
	OpRemoveProcess operation = "removeProcess"
)

type Command struct {
	Operation operation
	Data      interface{}
}

type CommandAddProcess struct {
	app.Config
}

type CommandRemoveProcess struct {
	ID string
}

// Implement a FSM
type store struct {
	lock    sync.RWMutex
	Process map[string]app.Config

	logger log.Logger
}

type Config struct {
	Logger log.Logger
}

func NewStore(config Config) (Store, error) {
	s := &store{
		Process: map[string]app.Config{},
		logger:  config.Logger,
	}

	if s.logger == nil {
		s.logger = log.New("")
	}

	return s, nil
}

func (s *store) Apply(entry *raft.Log) interface{} {
	logger := s.logger.WithFields(log.Fields{
		"index": entry.Index,
		"term":  entry.Term,
	})

	logger.Debug().WithField("data", string(entry.Data)).Log("New entry")

	c := Command{}

	err := json.Unmarshal(entry.Data, &c)
	if err != nil {
		logger.Error().WithError(err).Log("Invalid entry")
		return fmt.Errorf("invalid log entry")
	}

	logger.Debug().WithField("operation", c.Operation).Log("")

	switch c.Operation {
	case OpAddProcess:
		b, _ := json.Marshal(c.Data)
		cmd := CommandAddProcess{}
		json.Unmarshal(b, &cmd)

		s.lock.Lock()
		s.Process[cmd.ID] = cmd.Config
		s.lock.Unlock()
	case OpRemoveProcess:
		b, _ := json.Marshal(c.Data)
		cmd := CommandRemoveProcess{}
		json.Unmarshal(b, &cmd)

		s.lock.Lock()
		delete(s.Process, cmd.ID)
		s.lock.Unlock()
	}

	s.lock.RLock()
	s.logger.Debug().WithField("processes", s.Process).Log("")
	s.lock.RUnlock()
	return nil
}

func (s *store) Snapshot() (raft.FSMSnapshot, error) {
	s.logger.Debug().Log("Snapshot request")

	s.lock.Lock()
	defer s.lock.Unlock()

	data, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}

	return &fsmSnapshot{
		data: data,
	}, nil
}

func (s *store) Restore(snapshot io.ReadCloser) error {
	s.logger.Debug().Log("Snapshot restore")

	defer snapshot.Close()

	s.lock.Lock()
	defer s.lock.Unlock()

	dec := json.NewDecoder(snapshot)
	if err := dec.Decode(s); err != nil {
		return err
	}

	return nil
}

func (s *store) ProcessList() []app.Config {
	s.lock.RLock()
	defer s.lock.RUnlock()

	processes := []app.Config{}

	for _, cfg := range s.Process {
		processes = append(processes, *cfg.Clone())
	}

	return processes
}

func (s *store) GetProcess(id string) (app.Config, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	cfg, ok := s.Process[id]
	if !ok {
		return app.Config{}, fmt.Errorf("not found")
	}

	return *cfg.Clone(), nil
}

type fsmSnapshot struct {
	data []byte
}

func (s *fsmSnapshot) Persist(sink raft.SnapshotSink) error {
	if _, err := sink.Write(s.data); err != nil {
		sink.Cancel()
		return err
	}

	sink.Close()
	return nil
}

func (s *fsmSnapshot) Release() {
	s.data = nil
}
