package forwarder

import (
	"io"
	"net/http"
	"sync"
	"time"

	apiclient "github.com/datarhei/core/v16/cluster/client"
	iamaccess "github.com/datarhei/core/v16/iam/access"
	iamidentity "github.com/datarhei/core/v16/iam/identity"
	"github.com/datarhei/core/v16/log"
	"github.com/datarhei/core/v16/restream/app"
)

// Forwarder forwards any HTTP request from a follower to the leader
type Forwarder interface {
	SetLeader(address string)
	HasLeader() bool

	Join(origin, id, raftAddress, peerAddress string) error
	Leave(origin, id string) error
	TransferLeadership(origin, id string) error
	Snapshot(origin string) (io.ReadCloser, error)

	AddProcess(origin string, config *app.Config) error
	UpdateProcess(origin string, id app.ProcessID, config *app.Config) error
	RemoveProcess(origin string, id app.ProcessID) error
	SetProcessCommand(origin string, id app.ProcessID, command string) error
	SetProcessMetadata(origin string, id app.ProcessID, key string, data interface{}) error
	RelocateProcesses(origin string, relocations map[app.ProcessID]string) error

	AddIdentity(origin string, identity iamidentity.User) error
	UpdateIdentity(origin, name string, identity iamidentity.User) error
	SetPolicies(origin, name string, policies []iamaccess.Policy) error
	RemoveIdentity(origin string, name string) error

	CreateLock(origin string, name string, validUntil time.Time) error
	DeleteLock(origin string, name string) error

	SetKV(origin, key, value string) error
	UnsetKV(origin, key string) error
	GetKV(origin, key string) (string, time.Time, error)

	SetNodeState(origin, nodeid, state string) error
}

type forwarder struct {
	id   string
	lock sync.RWMutex

	client apiclient.APIClient

	logger log.Logger
}

type ForwarderConfig struct {
	ID     string
	Logger log.Logger
}

func New(config ForwarderConfig) (Forwarder, error) {
	f := &forwarder{
		id:     config.ID,
		logger: config.Logger,
	}

	if f.logger == nil {
		f.logger = log.New("")
	}

	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.MaxIdleConns = 10
	tr.IdleConnTimeout = 30 * time.Second

	client := &http.Client{
		Transport: tr,
		Timeout:   5 * time.Second,
	}

	f.client = apiclient.APIClient{
		Client: client,
	}

	return f, nil
}

func (f *forwarder) SetLeader(address string) {
	f.lock.Lock()
	defer f.lock.Unlock()

	if f.client.Address == address {
		return
	}

	f.logger.Debug().Log("Setting leader address to %s", address)

	f.client.Address = address
}

func (f *forwarder) HasLeader() bool {
	return len(f.client.Address) != 0
}

func (f *forwarder) Join(origin, id, raftAddress, peerAddress string) error {
	if origin == "" {
		origin = f.id
	}

	r := apiclient.JoinRequest{
		ID:          id,
		RaftAddress: raftAddress,
	}

	f.logger.Debug().WithField("request", r).Log("Forwarding to leader")

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	if len(peerAddress) != 0 {
		client = apiclient.APIClient{
			Address: peerAddress,
			Client:  f.client.Client,
		}
	}

	return client.Join(origin, r)
}

func (f *forwarder) Leave(origin, id string) error {
	if origin == "" {
		origin = f.id
	}

	f.logger.Debug().WithField("id", id).Log("Forwarding to leader")

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.Leave(origin, id)
}

func (f *forwarder) TransferLeadership(origin, id string) error {
	if origin == "" {
		origin = f.id
	}

	f.logger.Debug().WithField("id", id).Log("Transferring leadership")

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.TransferLeadership(origin, id)
}

func (f *forwarder) Snapshot(origin string) (io.ReadCloser, error) {
	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.Snapshot(origin)
}

func (f *forwarder) AddProcess(origin string, config *app.Config) error {
	if origin == "" {
		origin = f.id
	}

	r := apiclient.AddProcessRequest{
		Config: *config,
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.AddProcess(origin, r)
}

func (f *forwarder) UpdateProcess(origin string, id app.ProcessID, config *app.Config) error {
	if origin == "" {
		origin = f.id
	}

	r := apiclient.UpdateProcessRequest{
		Config: *config,
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.UpdateProcess(origin, id, r)
}

func (f *forwarder) SetProcessCommand(origin string, id app.ProcessID, command string) error {
	if origin == "" {
		origin = f.id
	}

	r := apiclient.SetProcessCommandRequest{
		Command: command,
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.SetProcessCommand(origin, id, r)
}

func (f *forwarder) SetProcessMetadata(origin string, id app.ProcessID, key string, data interface{}) error {
	if origin == "" {
		origin = f.id
	}

	r := apiclient.SetProcessMetadataRequest{
		Metadata: data,
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.SetProcessMetadata(origin, id, key, r)
}

func (f *forwarder) RemoveProcess(origin string, id app.ProcessID) error {
	if origin == "" {
		origin = f.id
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.RemoveProcess(origin, id)
}

func (f *forwarder) RelocateProcesses(origin string, relocations map[app.ProcessID]string) error {
	if origin == "" {
		origin = f.id
	}

	r := apiclient.RelocateProcessesRequest{
		Map: relocations,
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.RelocateProcesses(origin, r)
}

func (f *forwarder) AddIdentity(origin string, identity iamidentity.User) error {
	if origin == "" {
		origin = f.id
	}

	r := apiclient.AddIdentityRequest{
		Identity: identity,
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.AddIdentity(origin, r)
}

func (f *forwarder) UpdateIdentity(origin, name string, identity iamidentity.User) error {
	if origin == "" {
		origin = f.id
	}

	r := apiclient.UpdateIdentityRequest{
		Identity: identity,
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.UpdateIdentity(origin, name, r)
}

func (f *forwarder) SetPolicies(origin, name string, policies []iamaccess.Policy) error {
	if origin == "" {
		origin = f.id
	}

	r := apiclient.SetPoliciesRequest{
		Policies: policies,
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.SetPolicies(origin, name, r)
}

func (f *forwarder) RemoveIdentity(origin string, name string) error {
	if origin == "" {
		origin = f.id
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.RemoveIdentity(origin, name)
}

func (f *forwarder) CreateLock(origin string, name string, validUntil time.Time) error {
	if origin == "" {
		origin = f.id
	}

	r := apiclient.LockRequest{
		Name:       name,
		ValidUntil: validUntil,
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.Lock(origin, r)
}

func (f *forwarder) DeleteLock(origin string, name string) error {
	if origin == "" {
		origin = f.id
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.Unlock(origin, name)
}

func (f *forwarder) SetKV(origin, key, value string) error {
	if origin == "" {
		origin = f.id
	}

	r := apiclient.SetKVRequest{
		Key:   key,
		Value: value,
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.SetKV(origin, r)
}

func (f *forwarder) UnsetKV(origin, key string) error {
	if origin == "" {
		origin = f.id
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.UnsetKV(origin, key)
}

func (f *forwarder) GetKV(origin, key string) (string, time.Time, error) {
	if origin == "" {
		origin = f.id
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.GetKV(origin, key)
}

func (f *forwarder) SetNodeState(origin, nodeid, state string) error {
	if origin == "" {
		origin = f.id
	}

	r := apiclient.SetNodeStateRequest{
		State: state,
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.SetNodeState(origin, nodeid, r)
}
