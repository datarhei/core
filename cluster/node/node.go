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
	"github.com/datarhei/core/v16/ffmpeg/skills"
	"github.com/datarhei/core/v16/log"
)

type Node interface {
	Stop() error
	About() About
	Version() string
	IPs() []string
	Status() (string, error)
	LastContact() time.Time
	Barrier(name string) (bool, error)

	CoreStatus() (string, error)
	CoreEssentials() (string, *config.Config, error)
	CoreConfig() (*config.Config, error)
	CoreSkills() (skills.Skills, error)
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
	latency            float64
	pingLock           sync.RWMutex

	runLock    sync.Mutex
	cancelPing context.CancelFunc

	proxyNode proxy.Node

	logger log.Logger
}

type Config struct {
	ID      string
	Address string

	Logger log.Logger
}

func New(config Config) Node {
	n := &node{
		id:      config.ID,
		address: config.Address,
		version: "0.0.0",
		client: client.APIClient{
			Address: config.Address,
			Client: &http.Client{
				Timeout: 5 * time.Second,
			},
		},
		logger: config.Logger,
	}

	if n.logger == nil {
		n.logger = log.New("")
	}

	if host, _, err := net.SplitHostPort(n.address); err == nil {
		if addrs, err := net.LookupHost(host); err == nil {
			n.ips = addrs
		}
	}

	if version, err := n.client.Version(); err == nil {
		n.version = version
	}

	n.start(n.id)

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
	n.proxyNode = proxy.NewNode(proxy.NodeConfig{
		ID:      id,
		Address: address,
		Config:  config,
		Logger:  n.logger.WithComponent("ClusterProxyNode").WithField("address", address),
	})

	n.lastCoreContactErr = err

	if err != nil {
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
						n.logger.Error().WithError(err).Log("Failed to retrieve core essentials")
					}
					n.pingLock.Unlock()
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

var maxLastContact time.Duration = 5 * time.Second

type AboutCore struct {
	Address     string
	State       string
	StateError  error
	Status      string
	Error       error
	CreatedAt   time.Time
	Uptime      time.Duration
	LastContact time.Duration
	Latency     time.Duration
}

type About struct {
	ID          string
	Name        string
	Version     string
	Address     string
	Status      string
	LastContact time.Duration
	Latency     time.Duration
	Error       error
	Core        AboutCore
	Resources   proxy.NodeResources
}

func (n *node) About() About {
	a := About{
		ID:      n.id,
		Version: n.Version(),
		Address: n.address,
	}

	n.pingLock.RLock()
	a.LastContact = time.Since(n.lastContact)
	if a.LastContact > maxLastContact {
		a.Status = "offline"
	} else {
		a.Status = "online"
	}
	a.Latency = time.Duration(n.latency * float64(time.Second))
	a.Error = n.lastContactErr

	coreError := n.lastCoreContactErr
	n.pingLock.RUnlock()

	about := n.CoreAbout()

	a.Name = about.Name
	a.Core.Address = about.Address
	a.Core.State = about.State
	a.Core.StateError = about.Error
	a.Core.CreatedAt = about.CreatedAt
	a.Core.Uptime = about.Uptime
	a.Core.LastContact = time.Since(about.LastContact)
	if a.Core.LastContact > maxLastContact {
		a.Core.Status = "offline"
	} else {
		a.Core.Status = "online"
	}
	a.Core.Error = coreError
	a.Core.Latency = about.Latency
	a.Resources = about.Resources

	return a
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
	if since > maxLastContact {
		return "offline", fmt.Errorf("the cluster API didn't respond for %s because: %w", since, n.lastContactErr)
	}

	return "online", nil
}

func (n *node) CoreStatus() (string, error) {
	n.pingLock.RLock()
	defer n.pingLock.RUnlock()

	since := time.Since(n.lastCoreContact)
	if since > maxLastContact {
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

func (n *node) CoreSkills() (skills.Skills, error) {
	return n.client.CoreSkills()
}

func (n *node) CoreAPIAddress() (string, error) {
	return n.client.CoreAPIAddress()
}

func (n *node) CoreAbout() proxy.NodeAbout {
	return n.proxyNode.About()
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
				n.lastContactErr = nil
				n.latency = n.latency*0.2 + time.Since(start).Seconds()*0.8
				n.pingLock.Unlock()
			} else {
				n.pingLock.Lock()
				n.lastContactErr = err
				n.pingLock.Unlock()

				n.logger.Warn().WithError(err).Log("Failed to ping cluster API")
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

				n.logger.Warn().WithError(err).Log("not connected to core API")
			}
		case <-ctx.Done():
			return
		}
	}
}
