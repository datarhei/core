package cluster

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/datarhei/core/v16/cluster/client"
	"github.com/datarhei/core/v16/cluster/proxy"
)

type clusterNode struct {
	client client.APIClient

	version     string
	lastContact time.Time
	latency     time.Duration
	pingLock    sync.RWMutex

	runLock    sync.Mutex
	cancelPing context.CancelFunc

	proxyNode proxy.Node
}

func NewClusterNode(address string) *clusterNode {
	n := &clusterNode{
		version: "0.0.0",
		client: client.APIClient{
			Address: address,
			Client: &http.Client{
				Timeout: 5 * time.Second,
			},
		},
	}

	return n
}

func (n *clusterNode) Start() error {
	n.runLock.Lock()
	defer n.runLock.Unlock()

	if n.cancelPing != nil {
		return nil
	}

	version, err := n.client.Version()
	if err != nil {
		return err
	}

	n.version = version

	address, err := n.CoreAPIAddress()
	if err != nil {
		return err
	}

	n.proxyNode = proxy.NewNode(address)

	ctx, cancel := context.WithCancel(context.Background())
	n.cancelPing = cancel

	go n.ping(ctx)

	return nil
}

func (n *clusterNode) Stop() error {
	n.runLock.Lock()
	defer n.runLock.Unlock()

	if n.cancelPing == nil {
		return nil
	}

	n.cancelPing()
	n.cancelPing = nil

	return nil
}

func (n *clusterNode) Version() string {
	n.pingLock.RLock()
	defer n.pingLock.RUnlock()

	return n.version
}

func (n *clusterNode) CoreAPIAddress() (string, error) {
	return n.client.CoreAPIAddress()
}

func (n *clusterNode) ping(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			start := time.Now()
			version, err := n.client.Version()
			if err == nil {
				n.pingLock.Lock()
				n.version = version
				n.lastContact = time.Now()
				n.latency = time.Since(start)
				n.pingLock.Unlock()
			}
		case <-ctx.Done():
			return
		}
	}
}
