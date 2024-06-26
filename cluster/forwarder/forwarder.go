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
type Forwarder struct {
	ID     string
	Logger log.Logger

	lock   sync.RWMutex
	client apiclient.APIClient
}

type Config struct {
	ID     string
	Logger log.Logger
}

func New(config Config) (*Forwarder, error) {
	f := &Forwarder{
		ID:     config.ID,
		Logger: config.Logger,
	}

	if f.Logger == nil {
		f.Logger = log.New("")
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

func (f *Forwarder) SetLeader(address string) {
	f.lock.Lock()
	defer f.lock.Unlock()

	if f.client.Address == address {
		return
	}

	f.Logger.Debug().Log("Setting leader address to %s", address)

	f.client.Address = address
}

func (f *Forwarder) HasLeader() bool {
	return len(f.client.Address) != 0
}

func (f *Forwarder) Join(origin, id, raftAddress, peerAddress string) error {
	if origin == "" {
		origin = f.ID
	}

	r := apiclient.JoinRequest{
		ID:          id,
		RaftAddress: raftAddress,
	}

	f.Logger.Debug().WithField("request", r).Log("Forwarding to leader")

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

func (f *Forwarder) Leave(origin, id string) error {
	if origin == "" {
		origin = f.ID
	}

	f.Logger.Debug().WithField("id", id).Log("Forwarding to leader")

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.Leave(origin, id)
}

func (f *Forwarder) TransferLeadership(origin, id string) error {
	if origin == "" {
		origin = f.ID
	}

	f.Logger.Debug().WithField("id", id).Log("Transferring leadership")

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.TransferLeadership(origin, id)
}

func (f *Forwarder) Snapshot(origin string) (io.ReadCloser, error) {
	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.Snapshot(origin)
}

func (f *Forwarder) AddProcess(origin string, config *app.Config) error {
	if origin == "" {
		origin = f.ID
	}

	r := apiclient.AddProcessRequest{
		Config: *config,
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.AddProcess(origin, r)
}

func (f *Forwarder) UpdateProcess(origin string, id app.ProcessID, config *app.Config) error {
	if origin == "" {
		origin = f.ID
	}

	r := apiclient.UpdateProcessRequest{
		Config: *config,
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.UpdateProcess(origin, id, r)
}

func (f *Forwarder) SetProcessCommand(origin string, id app.ProcessID, command string) error {
	if origin == "" {
		origin = f.ID
	}

	r := apiclient.SetProcessCommandRequest{
		Command: command,
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.SetProcessCommand(origin, id, r)
}

func (f *Forwarder) SetProcessMetadata(origin string, id app.ProcessID, key string, data interface{}) error {
	if origin == "" {
		origin = f.ID
	}

	r := apiclient.SetProcessMetadataRequest{
		Metadata: data,
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.SetProcessMetadata(origin, id, key, r)
}

func (f *Forwarder) RemoveProcess(origin string, id app.ProcessID) error {
	if origin == "" {
		origin = f.ID
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.RemoveProcess(origin, id)
}

func (f *Forwarder) RelocateProcesses(origin string, relocations map[app.ProcessID]string) error {
	if origin == "" {
		origin = f.ID
	}

	r := apiclient.RelocateProcessesRequest{
		Map: relocations,
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.RelocateProcesses(origin, r)
}

func (f *Forwarder) AddIdentity(origin string, identity iamidentity.User) error {
	if origin == "" {
		origin = f.ID
	}

	r := apiclient.AddIdentityRequest{
		Identity: identity,
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.AddIdentity(origin, r)
}

func (f *Forwarder) UpdateIdentity(origin, name string, identity iamidentity.User) error {
	if origin == "" {
		origin = f.ID
	}

	r := apiclient.UpdateIdentityRequest{
		Identity: identity,
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.UpdateIdentity(origin, name, r)
}

func (f *Forwarder) SetPolicies(origin, name string, policies []iamaccess.Policy) error {
	if origin == "" {
		origin = f.ID
	}

	r := apiclient.SetPoliciesRequest{
		Policies: policies,
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.SetPolicies(origin, name, r)
}

func (f *Forwarder) RemoveIdentity(origin string, name string) error {
	if origin == "" {
		origin = f.ID
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.RemoveIdentity(origin, name)
}

func (f *Forwarder) CreateLock(origin string, name string, validUntil time.Time) error {
	if origin == "" {
		origin = f.ID
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

func (f *Forwarder) DeleteLock(origin string, name string) error {
	if origin == "" {
		origin = f.ID
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.Unlock(origin, name)
}

func (f *Forwarder) SetKV(origin, key, value string) error {
	if origin == "" {
		origin = f.ID
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

func (f *Forwarder) UnsetKV(origin, key string) error {
	if origin == "" {
		origin = f.ID
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.UnsetKV(origin, key)
}

func (f *Forwarder) GetKV(origin, key string) (string, time.Time, error) {
	if origin == "" {
		origin = f.ID
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.GetKV(origin, key)
}

func (f *Forwarder) SetNodeState(origin, nodeid, state string) error {
	if origin == "" {
		origin = f.ID
	}

	r := apiclient.SetNodeStateRequest{
		State: state,
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.SetNodeState(origin, nodeid, r)
}
