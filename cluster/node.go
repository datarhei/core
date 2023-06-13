package cluster

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/datarhei/core/v16/cluster/client"
	"github.com/datarhei/core/v16/cluster/proxy"
	"github.com/datarhei/core/v16/config"
)

type clusterNode struct {
	client client.APIClient

	version            string
	lastContact        time.Time
	lastContactErr     error
	lastCoreContact    time.Time
	lastCoreContactErr error
	latency            time.Duration
	pingLock           sync.RWMutex

	runLock    sync.Mutex
	cancelPing context.CancelFunc

	proxyLock    sync.Mutex
	proxyNode    proxy.Node
	proxyNodeErr error
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

	version, err := n.client.Version()
	if err != nil {
		version = "0.0.0"
	}

	n.version = version

	p := proxy.NewNode(address)
	err = p.Connect()
	if err == nil {
		n.proxyNode = p
	} else {
		n.proxyNodeErr = err
	}

	return n
}

func (n *clusterNode) Start() error {
	n.runLock.Lock()
	defer n.runLock.Unlock()

	if n.cancelPing != nil {
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	n.cancelPing = cancel

	go n.ping(ctx)
	go n.pingCore(ctx)

	address, _ := n.CoreAPIAddress()

	p := proxy.NewNode(address)
	err := p.Connect()
	if err != nil {
		n.proxyNodeErr = err

		go func(ctx context.Context, address string) {
			ticker := time.NewTicker(3 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					p := proxy.NewNode(address)
					err := p.Connect()
					if err == nil {
						n.proxyNode = p
						return
					}
				case <-ctx.Done():
					return
				}
			}
		}(ctx, address)
	} else {
		n.proxyNode = p
	}

	return nil
}

func (n *clusterNode) Stop() error {
	n.runLock.Lock()
	defer n.runLock.Unlock()

	if n.cancelPing == nil {
		return nil
	}

	n.proxyNode.Disconnect()

	n.cancelPing()
	n.cancelPing = nil

	return nil
}

func (n *clusterNode) Version() string {
	n.pingLock.RLock()
	defer n.pingLock.RUnlock()

	return n.version
}

func (n *clusterNode) Status() (string, error) {
	n.pingLock.RLock()
	defer n.pingLock.RUnlock()

	since := time.Since(n.lastContact)
	if since > 5*time.Second {
		return "offline", fmt.Errorf("the cluster API didn't respond for %s because: %w", since, n.lastContactErr)
	}

	since = time.Since(n.lastCoreContact)
	if since > 5*time.Second {
		return "offline", fmt.Errorf("the core API didn't respond for %s because: %w", since, n.lastCoreContactErr)
	}

	return "online", nil
}

func (n *clusterNode) LastContact() time.Time {
	n.pingLock.RLock()
	defer n.pingLock.RUnlock()

	return n.lastContact
}

func (n *clusterNode) Config() *config.Config {
	return nil
}

func (n *clusterNode) CoreAPIAddress() (string, error) {
	return n.client.CoreAPIAddress()
}

func (n *clusterNode) Proxy() proxy.Node {
	return n.proxyNode
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
			} else {
				n.pingLock.Lock()
				n.lastContactErr = err
				n.pingLock.Unlock()
			}
		case <-ctx.Done():
			return
		}
	}
}

func (n *clusterNode) pingCore(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			var err error
			n.proxyLock.Lock()
			if n.proxyNode == nil {
				err = fmt.Errorf("can't connect to core api: %w", n.proxyNodeErr)
			} else {
				_, err = n.proxyNode.Ping()
			}
			n.proxyLock.Unlock()

			if err == nil {
				n.pingLock.Lock()
				n.lastCoreContact = time.Now()
				n.pingLock.Unlock()
			} else {
				n.pingLock.Lock()
				n.lastCoreContactErr = err
				n.pingLock.Unlock()
			}
		case <-ctx.Done():
			return
		}
	}
}
