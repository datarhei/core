package store

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/datarhei/core/v16/iam/access"
	"github.com/datarhei/core/v16/iam/identity"
	"github.com/datarhei/core/v16/log"
	"github.com/datarhei/core/v16/restream/app"

	"github.com/hashicorp/raft"
)

type Store interface {
	raft.FSM

	ProcessList() []Process
	GetProcess(id string) (Process, error)
}

type Process struct {
	CreatedAt time.Time
	UpdatedAt time.Time
	Config    *app.Config
}

type Users struct {
	UpdatedAt time.Time
	Users     []identity.User
}

type Policies struct {
	UpdatedAt time.Time
	Policies  []access.Policy
}

type operation string

const (
	OpAddProcess    operation = "addProcess"
	OpRemoveProcess operation = "removeProcess"
	OpUpdateProcess operation = "updateProcess"
	OpAddIdentity   operation = "addIdentity"
)

type Command struct {
	Operation operation
	Data      interface{}
}

type CommandAddProcess struct {
	app.Config
}

type CommandUpdateProcess struct {
	ID     string
	Config app.Config
}

type CommandRemoveProcess struct {
	ID string
}

type CommandAddIdentity struct {
	Identity any
}

// Implement a FSM
type store struct {
	lock    sync.RWMutex
	Process map[string]Process

	logger log.Logger
}

type Config struct {
	Logger log.Logger
}

func NewStore(config Config) (Store, error) {
	s := &store{
		Process: map[string]Process{},
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
		_, ok := s.Process[cmd.ID]
		if !ok {
			now := time.Now()
			s.Process[cmd.ID] = Process{
				CreatedAt: now,
				UpdatedAt: now,
				Config:    &cmd.Config,
			}
		}
		s.lock.Unlock()
	case OpRemoveProcess:
		b, _ := json.Marshal(c.Data)
		cmd := CommandRemoveProcess{}
		json.Unmarshal(b, &cmd)

		s.lock.Lock()
		delete(s.Process, cmd.ID)
		s.lock.Unlock()
	case OpUpdateProcess:
		b, _ := json.Marshal(c.Data)
		cmd := CommandUpdateProcess{}
		json.Unmarshal(b, &cmd)

		s.lock.Lock()
		_, ok := s.Process[cmd.ID]
		if ok {
			if cmd.ID == cmd.Config.ID {
				s.Process[cmd.ID] = Process{
					UpdatedAt: time.Now(),
					Config:    &cmd.Config,
				}
			} else {
				_, ok := s.Process[cmd.Config.ID]
				if !ok {
					delete(s.Process, cmd.ID)
					s.Process[cmd.Config.ID] = Process{
						UpdatedAt: time.Now(),
						Config:    &cmd.Config,
					}
				} else {
					return fmt.Errorf("the process with the ID %s already exists", cmd.Config.ID)
				}
			}
		}
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

func (s *store) ProcessList() []Process {
	s.lock.RLock()
	defer s.lock.RUnlock()

	processes := []Process{}

	for _, p := range s.Process {
		processes = append(processes, Process{
			UpdatedAt: p.UpdatedAt,
			Config:    p.Config.Clone(),
		})
	}

	return processes
}

func (s *store) GetProcess(id string) (Process, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	process, ok := s.Process[id]
	if !ok {
		return Process{}, fmt.Errorf("not found")
	}

	return Process{
		UpdatedAt: process.UpdatedAt,
		Config:    process.Config.Clone(),
	}, nil
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
