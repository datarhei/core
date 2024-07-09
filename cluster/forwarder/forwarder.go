package forwarder

import (
	"io"
	"net/http"
	"sync"
	"time"

	apiclient "github.com/datarhei/core/v16/cluster/client"
	"github.com/datarhei/core/v16/log"
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
