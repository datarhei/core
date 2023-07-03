package node

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/datarhei/core/v16/cluster/client"
	"github.com/datarhei/core/v16/cluster/proxy"
	"github.com/datarhei/core/v16/config"
)

type Node interface {
	Stop() error
	Version() string
	IPs() []string
	Status() (string, error)
	LastContact() time.Time
	Barrier(name string) (bool, error)

	CoreStatus() (string, error)
	CoreEssentials() (string, *config.Config, error)
	CoreConfig() (*config.Config, error)
	CoreAPIAddress() (string, error)

	Proxy() proxy.Node
}

type node struct {
	client client.APIClient

	id                 string
	address            string
	ips                []string
	version            string
	lastContact        time.Time
	lastContactErr     error
	lastCoreContact    time.Time
	lastCoreContactErr error
	latency            time.Duration
	pingLock           sync.RWMutex

	runLock    sync.Mutex
	cancelPing context.CancelFunc

	proxyNode proxy.Node
}

func New(id, address string) Node {
	n := &node{
		id:      id,
		address: address,
		version: "0.0.0",
		client: client.APIClient{
			Address: address,
			Client: &http.Client{
				Timeout: 5 * time.Second,
			},
		},
	}

	if host, _, err := net.SplitHostPort(address); err == nil {
		if addrs, err := net.LookupHost(host); err == nil {
			n.ips = addrs
		}
	}

	if version, err := n.client.Version(); err == nil {
		n.version = version
	}

	n.start(id)

	return n
}

func (n *node) start(id string) error {
	n.runLock.Lock()
	defer n.runLock.Unlock()

	if n.cancelPing != nil {
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	n.cancelPing = cancel

	n.lastCoreContactErr = fmt.Errorf("not started yet")
	n.lastContactErr = fmt.Errorf("not started yet")

	address, config, err := n.CoreEssentials()
	n.proxyNode = proxy.NewNode(id, address, config)

	if err != nil {
		n.lastCoreContactErr = err
		go func(ctx context.Context) {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					address, config, err := n.CoreEssentials()
					n.pingLock.Lock()
					if err == nil {
						n.proxyNode.SetEssentials(address, config)
						n.lastCoreContactErr = nil
					} else {
						n.lastCoreContactErr = err
					}
					n.pingLock.Unlock()

					if err == nil {
						return
					}
				case <-ctx.Done():
					return
				}
			}
		}(ctx)
	}

	go n.ping(ctx)
	go n.pingCore(ctx)

	return nil
}

func (n *node) Stop() error {
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

func (n *node) Version() string {
	n.pingLock.RLock()
	defer n.pingLock.RUnlock()

	return n.version
}

func (n *node) IPs() []string {
	return n.ips
}

func (n *node) Status() (string, error) {
	n.pingLock.RLock()
	defer n.pingLock.RUnlock()

	since := time.Since(n.lastContact)
	if since > 5*time.Second {
		return "offline", fmt.Errorf("the cluster API didn't respond for %s because: %w", since, n.lastContactErr)
	}

	return "online", nil
}

func (n *node) CoreStatus() (string, error) {
	n.pingLock.RLock()
	defer n.pingLock.RUnlock()

	since := time.Since(n.lastCoreContact)
	if since > 5*time.Second {
		return "offline", fmt.Errorf("the core API didn't respond for %s because: %w", since, n.lastCoreContactErr)
	}

	return "online", nil
}

func (n *node) LastContact() time.Time {
	n.pingLock.RLock()
	defer n.pingLock.RUnlock()

	return n.lastContact
}

func (n *node) CoreEssentials() (string, *config.Config, error) {
	address, err := n.CoreAPIAddress()
	if err != nil {
		return "", nil, err
	}

	config, err := n.CoreConfig()
	if err != nil {
		return "", nil, err
	}

	return address, config, nil
}

func (n *node) CoreConfig() (*config.Config, error) {
	return n.client.CoreConfig()
}

func (n *node) CoreAPIAddress() (string, error) {
	return n.client.CoreAPIAddress()
}

func (n *node) Barrier(name string) (bool, error) {
	return n.client.Barrier(name)
}

func (n *node) Proxy() proxy.Node {
	return n.proxyNode
}

func (n *node) ping(ctx context.Context) {
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

func (n *node) pingCore(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			_, err := n.proxyNode.IsConnected()

			if err == nil {
				n.pingLock.Lock()
				n.lastCoreContact = time.Now()
				n.lastCoreContactErr = nil
				n.pingLock.Unlock()
			} else {
				n.pingLock.Lock()
				n.lastCoreContactErr = fmt.Errorf("not connected to core api: %w", err)
				n.pingLock.Unlock()
			}
		case <-ctx.Done():
			return
		}
	}
}
