package store

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"strings"
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

	OnApply(func(op Operation))

	ListProcesses() []Process
	GetProcess(id app.ProcessID) (Process, error)
	GetProcessNodeMap() map[string]string

	ListUsers() Users
	GetUser(name string) Users
	ListPolicies() Policies
	ListUserPolicies(name string) Policies

	HasLock(name string) bool
	ListLocks() map[string]time.Time

	ListKVS(prefix string) map[string]Value
	GetFromKVS(key string) (Value, error)
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

type Operation string

const (
	OpAddProcess         Operation = "addProcess"
	OpRemoveProcess      Operation = "removeProcess"
	OpUpdateProcess      Operation = "updateProcess"
	OpSetProcessOrder    Operation = "setProcessOrder"
	OpSetProcessMetadata Operation = "setProcessMetadata"
	OpSetProcessError    Operation = "setProcessError"
	OpAddIdentity        Operation = "addIdentity"
	OpUpdateIdentity     Operation = "updateIdentity"
	OpRemoveIdentity     Operation = "removeIdentity"
	OpSetPolicies        Operation = "setPolicies"
	OpSetProcessNodeMap  Operation = "setProcessNodeMap"
	OpCreateLock         Operation = "createLock"
	OpDeleteLock         Operation = "deleteLock"
	OpClearLocks         Operation = "clearLocks"
	OpSetKV              Operation = "setKV"
	OpUnsetKV            Operation = "unsetKV"
)

type Command struct {
	Operation Operation
	Data      interface{}
}

type CommandAddProcess struct {
	Config *app.Config
}

type CommandUpdateProcess struct {
	ID     app.ProcessID
	Config *app.Config
}

type CommandRemoveProcess struct {
	ID app.ProcessID
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

type CommandSetProcessNodeMap struct {
	Map map[string]string
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

type storeData struct {
	Version        uint64
	Process        map[string]Process
	ProcessNodeMap map[string]string

	Users struct {
		UpdatedAt time.Time
		Users     map[string]identity.User
	}

	Policies struct {
		UpdatedAt time.Time
		Policies  map[string][]access.Policy
	}

	Locks map[string]time.Time

	KVS map[string]Value
}

func (s *storeData) init() {
	now := time.Now()

	s.Version = 1
	s.Process = map[string]Process{}
	s.ProcessNodeMap = map[string]string{}
	s.Users.UpdatedAt = now
	s.Users.Users = map[string]identity.User{}
	s.Policies.UpdatedAt = now
	s.Policies.Policies = map[string][]access.Policy{}
	s.Locks = map[string]time.Time{}
	s.KVS = map[string]Value{}
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
	default:
		s.logger.Warn().WithField("operation", c.Operation).Log("Unknown operation")
		err = fmt.Errorf("unknown operation: %s", c.Operation)
	}

	return err
}

func (s *store) addProcess(cmd CommandAddProcess) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	id := cmd.Config.ProcessID().String()

	if cmd.Config.LimitCPU <= 0 || cmd.Config.LimitMemory <= 0 {
		return fmt.Errorf("the process with the ID '%s' must have limits defined", id)
	}

	_, ok := s.data.Process[id]
	if ok {
		return fmt.Errorf("the process with the ID '%s' already exists", id)
	}

	order := "stop"
	if cmd.Config.Autostart {
		order = "start"
		cmd.Config.Autostart = false
	}

	now := time.Now()
	s.data.Process[id] = Process{
		CreatedAt: now,
		UpdatedAt: now,
		Config:    cmd.Config,
		Order:     order,
		Metadata:  map[string]interface{}{},
	}

	return nil
}

func (s *store) removeProcess(cmd CommandRemoveProcess) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	id := cmd.ID.String()

	_, ok := s.data.Process[id]
	if !ok {
		return fmt.Errorf("the process with the ID '%s' doesn't exist", id)
	}

	delete(s.data.Process, id)

	return nil
}

func (s *store) updateProcess(cmd CommandUpdateProcess) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	srcid := cmd.ID.String()
	dstid := cmd.Config.ProcessID().String()

	if cmd.Config.LimitCPU <= 0 || cmd.Config.LimitMemory <= 0 {
		return fmt.Errorf("the process with the ID '%s' must have limits defined", dstid)
	}

	p, ok := s.data.Process[srcid]
	if !ok {
		return fmt.Errorf("the process with the ID '%s' doesn't exists", srcid)
	}

	if p.Config.Equal(cmd.Config) {
		return nil
	}

	if srcid == dstid {
		p.UpdatedAt = time.Now()
		p.Config = cmd.Config

		s.data.Process[srcid] = p

		return nil
	}

	_, ok = s.data.Process[dstid]
	if ok {
		return fmt.Errorf("the process with the ID '%s' already exists", dstid)
	}

	now := time.Now()

	p.CreatedAt = now
	p.UpdatedAt = now
	p.Config = cmd.Config

	delete(s.data.Process, srcid)
	s.data.Process[dstid] = p

	return nil
}

func (s *store) setProcessOrder(cmd CommandSetProcessOrder) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	id := cmd.ID.String()

	p, ok := s.data.Process[id]
	if !ok {
		return fmt.Errorf("the process with the ID '%s' doesn't exists", cmd.ID)
	}

	p.Order = cmd.Order
	p.UpdatedAt = time.Now()

	s.data.Process[id] = p

	return nil
}

func (s *store) setProcessMetadata(cmd CommandSetProcessMetadata) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	id := cmd.ID.String()

	p, ok := s.data.Process[id]
	if !ok {
		return fmt.Errorf("the process with the ID '%s' doesn't exists", cmd.ID)
	}

	if p.Metadata == nil {
		p.Metadata = map[string]interface{}{}
	}

	if cmd.Data == nil {
		delete(p.Metadata, cmd.Key)
	} else {
		p.Metadata[cmd.Key] = cmd.Data
	}
	p.UpdatedAt = time.Now()

	s.data.Process[id] = p

	return nil
}

func (s *store) setProcessError(cmd CommandSetProcessError) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	id := cmd.ID.String()

	p, ok := s.data.Process[id]
	if !ok {
		return fmt.Errorf("the process with the ID '%s' doesn't exists", cmd.ID)
	}

	p.Error = cmd.Error

	s.data.Process[id] = p

	return nil
}

func (s *store) addIdentity(cmd CommandAddIdentity) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if cmd.Identity.Name == "$anon" {
		return fmt.Errorf("the identity with the name '%s' can't be created", cmd.Identity.Name)
	}

	_, ok := s.data.Users.Users[cmd.Identity.Name]
	if ok {
		return fmt.Errorf("the identity with the name '%s' already exists", cmd.Identity.Name)
	}

	s.data.Users.UpdatedAt = time.Now()
	s.data.Users.Users[cmd.Identity.Name] = cmd.Identity

	return nil
}

func (s *store) updateIdentity(cmd CommandUpdateIdentity) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if cmd.Name == "$anon" {
		return fmt.Errorf("the identity with the name '%s' can't be updated", cmd.Name)
	}

	_, ok := s.data.Users.Users[cmd.Name]
	if !ok {
		return fmt.Errorf("the identity with the name '%s' doesn't exist", cmd.Name)
	}

	if cmd.Name == cmd.Identity.Name {
		s.data.Users.UpdatedAt = time.Now()
		s.data.Users.Users[cmd.Identity.Name] = cmd.Identity

		return nil
	}

	_, ok = s.data.Users.Users[cmd.Identity.Name]
	if ok {
		return fmt.Errorf("the identity with the name '%s' already exists", cmd.Identity.Name)
	}

	now := time.Now()

	s.data.Users.UpdatedAt = now
	s.data.Users.Users[cmd.Identity.Name] = cmd.Identity
	s.data.Policies.UpdatedAt = now
	s.data.Policies.Policies[cmd.Identity.Name] = s.data.Policies.Policies[cmd.Name]

	delete(s.data.Users.Users, cmd.Name)
	delete(s.data.Policies.Policies, cmd.Name)

	return nil
}

func (s *store) removeIdentity(cmd CommandRemoveIdentity) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	delete(s.data.Users.Users, cmd.Name)
	s.data.Users.UpdatedAt = time.Now()
	delete(s.data.Policies.Policies, cmd.Name)
	s.data.Policies.UpdatedAt = time.Now()

	return nil
}

func (s *store) setPolicies(cmd CommandSetPolicies) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if cmd.Name != "$anon" {
		if _, ok := s.data.Users.Users[cmd.Name]; !ok {
			return fmt.Errorf("the identity with the name '%s' doesn't exist", cmd.Name)
		}
	}

	for i, p := range cmd.Policies {
		if len(p.Domain) != 0 {
			continue
		}

		p.Domain = "$none"
		cmd.Policies[i] = p
	}

	delete(s.data.Policies.Policies, cmd.Name)
	s.data.Policies.Policies[cmd.Name] = cmd.Policies
	s.data.Policies.UpdatedAt = time.Now()

	return nil
}

func (s *store) setProcessNodeMap(cmd CommandSetProcessNodeMap) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.data.ProcessNodeMap = cmd.Map

	return nil
}

func (s *store) createLock(cmd CommandCreateLock) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	validUntil, ok := s.data.Locks[cmd.Name]

	if ok {
		if time.Now().Before(validUntil) {
			return fmt.Errorf("the lock with the ID '%s' already exists", cmd.Name)
		}
	}

	s.data.Locks[cmd.Name] = cmd.ValidUntil

	return nil
}

func (s *store) deleteLock(cmd CommandDeleteLock) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if _, ok := s.data.Locks[cmd.Name]; !ok {
		return nil
	}

	delete(s.data.Locks, cmd.Name)

	return nil
}

func (s *store) clearLocks(cmd CommandClearLocks) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	for name, validUntil := range s.data.Locks {
		if time.Now().Before(validUntil) {
			// Lock is still valid
			continue
		}

		delete(s.data.Locks, name)
	}

	return nil
}

func (s *store) setKV(cmd CommandSetKV) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	value := s.data.KVS[cmd.Key]

	value.Value = cmd.Value
	value.UpdatedAt = time.Now()

	s.data.KVS[cmd.Key] = value

	return nil
}

func (s *store) unsetKV(cmd CommandUnsetKV) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if _, ok := s.data.KVS[cmd.Key]; !ok {
		return fs.ErrNotExist
	}

	delete(s.data.KVS, cmd.Key)

	return nil
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

	for id, p := range data.Process {
		if p.Metadata != nil {
			continue
		}

		p.Metadata = map[string]interface{}{}
		data.Process[id] = p
	}

	if data.Version == 0 {
		data.Version = 1
	}

	s.data = data

	return nil
}

func (s *store) ListProcesses() []Process {
	s.lock.RLock()
	defer s.lock.RUnlock()

	processes := []Process{}

	for _, p := range s.data.Process {
		processes = append(processes, Process{
			CreatedAt: p.CreatedAt,
			UpdatedAt: p.UpdatedAt,
			Config:    p.Config.Clone(),
			Order:     p.Order,
			Metadata:  p.Metadata,
			Error:     p.Error,
		})
	}

	return processes
}

func (s *store) GetProcess(id app.ProcessID) (Process, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	process, ok := s.data.Process[id.String()]
	if !ok {
		return Process{}, fmt.Errorf("not found")
	}

	return Process{
		CreatedAt: process.CreatedAt,
		UpdatedAt: process.UpdatedAt,
		Config:    process.Config.Clone(),
		Order:     process.Order,
		Metadata:  process.Metadata,
		Error:     process.Error,
	}, nil
}

func (s *store) ListUsers() Users {
	s.lock.RLock()
	defer s.lock.RUnlock()

	u := Users{
		UpdatedAt: s.data.Users.UpdatedAt,
	}

	for _, user := range s.data.Users.Users {
		u.Users = append(u.Users, user)
	}

	return u
}

func (s *store) GetUser(name string) Users {
	s.lock.RLock()
	defer s.lock.RUnlock()

	u := Users{
		UpdatedAt: s.data.Users.UpdatedAt,
	}

	if user, ok := s.data.Users.Users[name]; ok {
		u.Users = append(u.Users, user)
	}

	return u
}

func (s *store) ListPolicies() Policies {
	s.lock.RLock()
	defer s.lock.RUnlock()

	p := Policies{
		UpdatedAt: s.data.Policies.UpdatedAt,
	}

	for _, policies := range s.data.Policies.Policies {
		p.Policies = append(p.Policies, policies...)
	}

	return p
}

func (s *store) ListUserPolicies(name string) Policies {
	s.lock.RLock()
	defer s.lock.RUnlock()

	p := Policies{
		UpdatedAt: s.data.Policies.UpdatedAt,
	}

	p.Policies = append(p.Policies, s.data.Policies.Policies[name]...)

	return p
}

func (s *store) GetProcessNodeMap() map[string]string {
	s.lock.RLock()
	defer s.lock.RUnlock()

	m := map[string]string{}

	for key, value := range s.data.ProcessNodeMap {
		m[key] = value
	}

	return m
}

func (s *store) HasLock(name string) bool {
	s.lock.RLock()
	defer s.lock.RUnlock()

	_, ok := s.data.Locks[name]

	return ok
}

func (s *store) ListLocks() map[string]time.Time {
	s.lock.RLock()
	defer s.lock.RUnlock()

	m := map[string]time.Time{}

	for key, value := range s.data.Locks {
		m[key] = value
	}

	return m
}

func (s *store) ListKVS(prefix string) map[string]Value {
	s.lock.RLock()
	defer s.lock.RUnlock()

	m := map[string]Value{}

	for key, value := range s.data.KVS {
		if !strings.HasPrefix(key, prefix) {
			continue
		}

		m[key] = value
	}

	return m
}

func (s *store) GetFromKVS(key string) (Value, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	value, ok := s.data.KVS[key]
	if !ok {
		return Value{}, fs.ErrNotExist
	}

	return value, nil
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
