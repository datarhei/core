package store

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/datarhei/core/v16/encoding/json"
	"github.com/datarhei/core/v16/iam/access"
	"github.com/datarhei/core/v16/iam/identity"
	"github.com/datarhei/core/v16/log"
	"github.com/datarhei/core/v16/restream/app"

	"github.com/hashicorp/raft"
)

type Store interface {
	raft.FSM

	OnApply(func(op Operation))

	ListProcesses() []Process
	GetProcess(id app.ProcessID) (Process, error)
	GetProcessNodeMap() map[string]string
	GetProcessRelocateMap() map[string]string

	ListUsers() Users
	GetUser(name string) Users
	ListPolicies() Policies
	ListUserPolicies(name string) Policies

	HasLock(name string) bool
	ListLocks() map[string]time.Time

	ListKVS(prefix string) map[string]Value
	GetFromKVS(key string) (Value, error)

	ListNodes() map[string]Node
}

type Process struct {
	CreatedAt time.Time
	UpdatedAt time.Time
	Config    *app.Config
	Order     string
	Metadata  map[string]interface{}
	Error     string
}

type Users struct {
	UpdatedAt time.Time
	Users     []identity.User
}

type Policies struct {
	UpdatedAt time.Time
	Policies  []access.Policy
}

type Value struct {
	Value     string
	UpdatedAt time.Time
}

type Node struct {
	State     string
	UpdatedAt time.Time
}

type Operation string

const (
	OpAddProcess           Operation = "addProcess"
	OpRemoveProcess        Operation = "removeProcess"
	OpUpdateProcess        Operation = "updateProcess"
	OpSetRelocateProcess   Operation = "setRelocateProcess"
	OpUnsetRelocateProcess Operation = "unsetRelocateProcess"
	OpSetProcessOrder      Operation = "setProcessOrder"
	OpSetProcessMetadata   Operation = "setProcessMetadata"
	OpSetProcessError      Operation = "setProcessError"
	OpAddIdentity          Operation = "addIdentity"
	OpUpdateIdentity       Operation = "updateIdentity"
	OpRemoveIdentity       Operation = "removeIdentity"
	OpSetPolicies          Operation = "setPolicies"
	OpSetProcessNodeMap    Operation = "setProcessNodeMap"
	OpCreateLock           Operation = "createLock"
	OpDeleteLock           Operation = "deleteLock"
	OpClearLocks           Operation = "clearLocks"
	OpSetKV                Operation = "setKV"
	OpUnsetKV              Operation = "unsetKV"
	OpSetNodeState         Operation = "setNodeState"
)

type Command struct {
	Operation Operation
	Data      interface{}
}

type CommandAddProcess struct {
	Config *app.Config
}

type CommandRemoveProcess struct {
	ID app.ProcessID
}

type CommandUpdateProcess struct {
	ID     app.ProcessID
	Config *app.Config
}

type CommandSetRelocateProcess struct {
	Map map[app.ProcessID]string
}

type CommandUnsetRelocateProcess struct {
	ID []app.ProcessID
}

type CommandSetProcessOrder struct {
	ID    app.ProcessID
	Order string
}

type CommandSetProcessMetadata struct {
	ID   app.ProcessID
	Key  string
	Data interface{}
}

type CommandSetProcessError struct {
	ID    app.ProcessID
	Error string
}

type CommandSetProcessNodeMap struct {
	Map map[string]string
}

type CommandAddIdentity struct {
	Identity identity.User
}

type CommandUpdateIdentity struct {
	Name     string
	Identity identity.User
}

type CommandRemoveIdentity struct {
	Name string
}

type CommandSetPolicies struct {
	Name     string
	Policies []access.Policy
}

type CommandCreateLock struct {
	Name       string
	ValidUntil time.Time
}

type CommandDeleteLock struct {
	Name string
}

type CommandClearLocks struct{}

type CommandSetKV struct {
	Key   string
	Value string
}

type CommandUnsetKV struct {
	Key string
}

type CommandSetNodeState struct {
	NodeID string
	State  string
}

type storeData struct {
	Version            uint64
	Process            map[string]Process // processid -> process
	ProcessNodeMap     map[string]string  // processid -> nodeid
	ProcessRelocateMap map[string]string  // processid -> nodeid

	Users struct {
		UpdatedAt time.Time
		Users     map[string]identity.User
		userlist  identity.UserList
	}

	Policies struct {
		UpdatedAt time.Time
		Policies  map[string][]access.Policy
	}

	Locks map[string]time.Time

	KVS map[string]Value

	Nodes map[string]Node
}

func (s *storeData) init() {
	now := time.Now()

	s.Version = 1
	s.Process = map[string]Process{}
	s.ProcessNodeMap = map[string]string{}
	s.ProcessRelocateMap = map[string]string{}
	s.Users.UpdatedAt = now
	s.Users.Users = map[string]identity.User{}
	s.Users.userlist = identity.NewUserList()
	s.Policies.UpdatedAt = now
	s.Policies.Policies = map[string][]access.Policy{}
	s.Locks = map[string]time.Time{}
	s.KVS = map[string]Value{}
	s.Nodes = map[string]Node{}
}

// store implements a raft.FSM
type store struct {
	lock     sync.RWMutex
	callback func(op Operation)

	logger log.Logger

	data storeData
}

type Config struct {
	Logger log.Logger
}

func NewStore(config Config) (Store, error) {
	s := &store{
		logger: config.Logger,
	}

	if s.logger == nil {
		s.logger = log.New("")
	}

	s.data.init()

	return s, nil
}

func (s *store) Apply(entry *raft.Log) interface{} {
	if entry.Type != raft.LogCommand {
		return nil
	}

	logger := s.logger.WithFields(log.Fields{
		"index": entry.Index,
		"term":  entry.Term,
	})

	logger.Debug().WithField("data", string(entry.Data)).Log("New entry")

	c := Command{}

	err := json.Unmarshal(entry.Data, &c)
	if err != nil {
		logger.Error().WithError(err).Log("Invalid entry")
		return fmt.Errorf("invalid log entry, index: %d, term: %d", entry.Index, entry.Term)
	}

	logger.Debug().WithField("operation", c.Operation).Log("")

	err = s.applyCommand(c)
	if err != nil {
		logger.Debug().WithError(err).WithField("operation", c.Operation).Log("")
		return err
	}

	s.lock.RLock()
	if s.callback != nil {
		s.callback(c.Operation)
	}
	s.lock.RUnlock()

	return nil
}

func decodeCommand[T any](cmd T, data any) error {
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}

	err = json.Unmarshal(b, cmd)

	return err
}

func (s *store) applyCommand(c Command) error {
	var err error = nil

	switch c.Operation {
	case OpAddProcess:
		cmd := CommandAddProcess{}
		err = decodeCommand(&cmd, c.Data)
		if err != nil {
			break
		}

		err = s.addProcess(cmd)
	case OpRemoveProcess:
		cmd := CommandRemoveProcess{}
		err = decodeCommand(&cmd, c.Data)
		if err != nil {
			break
		}

		err = s.removeProcess(cmd)
	case OpUpdateProcess:
		cmd := CommandUpdateProcess{}
		err = decodeCommand(&cmd, c.Data)
		if err != nil {
			break
		}

		err = s.updateProcess(cmd)
	case OpSetRelocateProcess:
		cmd := CommandSetRelocateProcess{}
		err = decodeCommand(&cmd, c.Data)
		if err != nil {
			break
		}

		err = s.setRelocateProcess(cmd)
	case OpUnsetRelocateProcess:
		cmd := CommandUnsetRelocateProcess{}
		err = decodeCommand(&cmd, c.Data)
		if err != nil {
			break
		}

		err = s.unsetRelocateProcess(cmd)
	case OpSetProcessOrder:
		cmd := CommandSetProcessOrder{}
		err = decodeCommand(&cmd, c.Data)
		if err != nil {
			break
		}

		err = s.setProcessOrder(cmd)
	case OpSetProcessMetadata:
		cmd := CommandSetProcessMetadata{}
		err = decodeCommand(&cmd, c.Data)
		if err != nil {
			break
		}

		err = s.setProcessMetadata(cmd)
	case OpSetProcessError:
		cmd := CommandSetProcessError{}
		err = decodeCommand(&cmd, c.Data)
		if err != nil {
			break
		}

		err = s.setProcessError(cmd)
	case OpAddIdentity:
		cmd := CommandAddIdentity{}
		err = decodeCommand(&cmd, c.Data)
		if err != nil {
			break
		}

		err = s.addIdentity(cmd)
	case OpUpdateIdentity:
		cmd := CommandUpdateIdentity{}
		err = decodeCommand(&cmd, c.Data)
		if err != nil {
			break
		}

		err = s.updateIdentity(cmd)
	case OpRemoveIdentity:
		cmd := CommandRemoveIdentity{}
		err = decodeCommand(&cmd, c.Data)
		if err != nil {
			break
		}

		err = s.removeIdentity(cmd)
	case OpSetPolicies:
		cmd := CommandSetPolicies{}
		err = decodeCommand(&cmd, c.Data)
		if err != nil {
			break
		}

		err = s.setPolicies(cmd)
	case OpSetProcessNodeMap:
		cmd := CommandSetProcessNodeMap{}
		err = decodeCommand(&cmd, c.Data)
		if err != nil {
			break
		}

		err = s.setProcessNodeMap(cmd)
	case OpCreateLock:
		cmd := CommandCreateLock{}
		err = decodeCommand(&cmd, c.Data)
		if err != nil {
			break
		}

		err = s.createLock(cmd)
	case OpDeleteLock:
		cmd := CommandDeleteLock{}
		err = decodeCommand(&cmd, c.Data)
		if err != nil {
			break
		}

		err = s.deleteLock(cmd)
	case OpClearLocks:
		cmd := CommandClearLocks{}
		err = decodeCommand(&cmd, c.Data)
		if err != nil {
			break
		}

		err = s.clearLocks(cmd)
	case OpSetKV:
		cmd := CommandSetKV{}
		err = decodeCommand(&cmd, c.Data)
		if err != nil {
			break
		}

		err = s.setKV(cmd)
	case OpUnsetKV:
		cmd := CommandUnsetKV{}
		err = decodeCommand(&cmd, c.Data)
		if err != nil {
			break
		}

		err = s.unsetKV(cmd)
	case OpSetNodeState:
		cmd := CommandSetNodeState{}
		err = decodeCommand(&cmd, c.Data)
		if err != nil {
			break
		}

		err = s.setNodeState(cmd)
	default:
		s.logger.Warn().WithField("operation", c.Operation).Log("Unknown operation")
		err = fmt.Errorf("unknown operation: %s", c.Operation)
	}

	return err
}

func (s *store) OnApply(fn func(op Operation)) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.callback = fn
}

func (s *store) Snapshot() (raft.FSMSnapshot, error) {
	s.logger.Debug().Log("Snapshot request")

	s.lock.RLock()
	defer s.lock.RUnlock()

	data, err := json.Marshal(&s.data)
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

	data := storeData{}
	data.init()

	dec := json.NewDecoder(snapshot)
	if err := dec.Decode(&data); err != nil {
		return err
	}

	if data.ProcessNodeMap == nil {
		data.ProcessNodeMap = map[string]string{}
	}

	if data.ProcessRelocateMap == nil {
		data.ProcessRelocateMap = map[string]string{}
	}

	for id, p := range data.Process {
		if p.Metadata != nil {
			continue
		}

		p.Metadata = map[string]interface{}{}
		data.Process[id] = p
	}

	now := time.Now()

	for name, u := range data.Users.Users {
		data.Users.userlist.Add(u)

		if u.CreatedAt.IsZero() {
			u.CreatedAt = now
		}

		if u.UpdatedAt.IsZero() {
			u.UpdatedAt = now
		}

		data.Users.Users[name] = u
	}

	for name, policies := range data.Policies.Policies {
		for i, p := range policies {
			policies[i] = s.updatePolicy(p)
		}

		data.Policies.Policies[name] = policies
	}

	if data.Version == 0 {
		data.Version = 1
	}

	s.data = data

	return nil
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
