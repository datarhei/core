package forwarder

import (
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	apiclient "github.com/datarhei/core/v16/cluster/client"
	"github.com/datarhei/core/v16/log"
	"github.com/datarhei/core/v16/restream/app"
)

// Forwarder forwards any HTTP request from a follower to the leader
type Forwarder interface {
	SetLeader(address string)
	HasLeader() bool
	Join(origin, id, raftAddress, peerAddress string) error
	Leave(origin, id string) error
	Snapshot() (io.ReadCloser, error)
	AddProcess(origin string, config *app.Config) error
	UpdateProcess() error
	RemoveProcess(origin, id string) error
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

	tr := &http.Transport{
		MaxIdleConns:    10,
		IdleConnTimeout: 30 * time.Second,
	}

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

	f.logger.Debug().Log("setting leader address to %s", address)

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
		Origin:      origin,
		ID:          id,
		RaftAddress: raftAddress,
	}

	f.logger.Debug().WithField("request", r).Log("forwarding to leader")

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	if len(peerAddress) != 0 {
		client = apiclient.APIClient{
			Address: peerAddress,
			Client:  f.client.Client,
		}
	}

	return client.Join(r)
}

func (f *forwarder) Leave(origin, id string) error {
	if origin == "" {
		origin = f.id
	}

	r := apiclient.LeaveRequest{
		Origin: origin,
		ID:     id,
	}

	f.logger.Debug().WithField("request", r).Log("forwarding to leader")

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.Leave(r)
}

func (f *forwarder) Snapshot() (io.ReadCloser, error) {
	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.Snapshot()
}

func (f *forwarder) AddProcess(origin string, config *app.Config) error {
	if origin == "" {
		origin = f.id
	}

	r := apiclient.AddProcessRequest{
		Origin: origin,
		Config: *config,
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.AddProcess(r)
}

func (f *forwarder) UpdateProcess() error {
	return fmt.Errorf("not implemented")
}

func (f *forwarder) RemoveProcess(origin, id string) error {
	if origin == "" {
		origin = f.id
	}

	r := apiclient.RemoveProcessRequest{
		Origin: origin,
		ID:     id,
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.RemoveProcess(r)
}
