package node

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/datarhei/core/v16/cluster/client"
	"github.com/datarhei/core/v16/config"
	"github.com/datarhei/core/v16/ffmpeg/skills"
	"github.com/datarhei/core/v16/log"
)

type Node struct {
	id      string
	address string
	ips     []string
	version string

	node            client.APIClient
	nodeAbout       About
	nodeLastContact time.Time
	nodeLastErr     error
	nodeLatency     float64

	core            *Core
	coreAbout       CoreAbout
	coreLastContact time.Time
	coreLastErr     error
	coreLatency     float64

	lock    sync.RWMutex
	runLock sync.Mutex
	cancel  context.CancelFunc

	logger log.Logger
}

type Config struct {
	ID      string
	Address string

	Logger log.Logger
}

func New(config Config) *Node {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.MaxIdleConns = 10
	tr.IdleConnTimeout = 30 * time.Second

	n := &Node{
		id:      config.ID,
		address: config.Address,
		version: "0.0.0",
		node: client.APIClient{
			Address: config.Address,
			Client: &http.Client{
				Transport: tr,
				Timeout:   5 * time.Second,
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

	if version, err := n.node.Version(); err == nil {
		n.version = version
	}

	ctx, cancel := context.WithCancel(context.Background())
	n.cancel = cancel

	n.nodeLastErr = fmt.Errorf("not started yet")
	n.coreLastErr = fmt.Errorf("not started yet")

	address, coreConfig, err := n.CoreEssentials()
	n.core = NewCore(n.id, n.logger.WithComponent("ClusterProxyNode").WithField("address", address))
	n.core.SetEssentials(address, coreConfig)

	n.coreLastErr = err

	go n.updateCore(ctx)
	go n.ping(ctx)
	go n.pingCore(ctx)

	return n
}

func (n *Node) Stop() error {
	n.runLock.Lock()
	defer n.runLock.Unlock()

	if n.cancel == nil {
		return nil
	}

	n.core.Stop()

	n.cancel()
	n.cancel = nil

	return nil
}

var maxLastContact time.Duration = 5 * time.Second

type About struct {
	ID          string
	Name        string
	Version     string
	Address     string
	State       string
	LastContact time.Time
	Latency     time.Duration
	Error       error
	Core        CoreAbout
	Resources   Resources
}

type Resources struct {
	IsThrottling bool    // Whether this core is currently throttling
	NCPU         float64 // Number of CPU on this node
	CPU          float64 // Current CPU load, 0-100*ncpu
	CPULimit     float64 // Defined CPU load limit, 0-100*ncpu
	Mem          uint64  // Currently used memory in bytes
	MemLimit     uint64  // Defined memory limit in bytes
	Error        error   // Last error
}

func (n *Node) About() About {
	n.lock.RLock()
	defer n.lock.RUnlock()

	a := About{
		ID:      n.id,
		Version: n.Version(),
		Address: n.address,
	}

	a.Name = n.coreAbout.Name
	a.Error = n.nodeLastErr
	a.LastContact = n.nodeLastContact
	if time.Since(a.LastContact) > maxLastContact || n.nodeLastErr != nil {
		a.State = "offline"
	} else {
		a.State = "online"
	}
	a.Latency = time.Duration(n.nodeLatency * float64(time.Second))
	a.Resources = n.nodeAbout.Resources

	a.Core = n.coreAbout
	a.Core.Error = n.coreLastErr
	a.Core.LastContact = n.coreLastContact
	if time.Since(a.Core.LastContact) > maxLastContact || n.coreLastErr != nil {
		a.Core.Status = "offline"
	} else {
		a.Core.Status = "online"
	}
	a.Core.Latency = time.Duration(n.coreLatency * float64(time.Second))

	return a
}

func (n *Node) Version() string {
	n.lock.RLock()
	defer n.lock.RUnlock()

	return n.version
}

func (n *Node) IPs() []string {
	return n.ips
}

func (n *Node) Status() (string, error) {
	n.lock.RLock()
	defer n.lock.RUnlock()

	since := time.Since(n.nodeLastContact)
	if since > maxLastContact {
		return "offline", fmt.Errorf("the cluster API didn't respond for %s because: %w", since, n.nodeLastErr)
	}

	return "online", nil
}

func (n *Node) CoreStatus() (string, error) {
	n.lock.RLock()
	defer n.lock.RUnlock()

	since := time.Since(n.coreLastContact)
	if since > maxLastContact {
		return "offline", fmt.Errorf("the core API didn't respond for %s because: %w", since, n.coreLastErr)
	}

	return "online", nil
}

func (n *Node) LastContact() time.Time {
	n.lock.RLock()
	defer n.lock.RUnlock()

	return n.nodeLastContact
}

func (n *Node) CoreEssentials() (string, *config.Config, error) {
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

func (n *Node) CoreConfig() (*config.Config, error) {
	return n.node.CoreConfig()
}

func (n *Node) CoreSkills() (skills.Skills, error) {
	return n.node.CoreSkills()
}

func (n *Node) CoreAPIAddress() (string, error) {
	return n.node.CoreAPIAddress()
}

func (n *Node) Barrier(name string) (bool, error) {
	return n.node.Barrier(name)
}

func (n *Node) CoreAbout() CoreAbout {
	return n.About().Core
}

func (n *Node) Core() *Core {
	return n.core
}

func (n *Node) ping(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			start := time.Now()
			about, err := n.node.About()
			n.lock.Lock()
			if err == nil {
				n.version = about.Version
				n.nodeAbout = About{
					ID:      about.ID,
					Version: about.Version,
					Address: about.Address,
					Error:   err,
					Resources: Resources{
						IsThrottling: about.Resources.IsThrottling,
						NCPU:         about.Resources.NCPU,
						CPU:          about.Resources.CPU,
						CPULimit:     about.Resources.CPULimit,
						Mem:          about.Resources.Mem,
						MemLimit:     about.Resources.MemLimit,
						Error:        about.Resources.Error,
					},
				}
				n.nodeLastContact = time.Now()
				n.nodeLastErr = nil
				n.nodeLatency = n.nodeLatency*0.2 + time.Since(start).Seconds()*0.8
			} else {
				n.nodeLastErr = err

				n.logger.Warn().WithError(err).Log("Failed to ping cluster API")
			}
			n.lock.Unlock()
		case <-ctx.Done():
			return
		}
	}
}

func (n *Node) updateCore(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			address, config, err := n.CoreEssentials()
			n.lock.Lock()
			if err == nil {
				n.core.SetEssentials(address, config)
				n.coreLastErr = nil
			} else {
				n.coreLastErr = err
				n.logger.Error().WithError(err).Log("Failed to retrieve core essentials")
			}
			n.lock.Unlock()
		case <-ctx.Done():
			return
		}
	}
}

func (n *Node) pingCore(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			start := time.Now()
			about, err := n.core.About()

			n.lock.Lock()
			if err == nil {
				n.coreLastContact = time.Now()
				n.coreLastErr = nil
				n.coreAbout = about
				n.coreLatency = n.coreLatency*0.2 + time.Since(start).Seconds()*0.8
			} else {
				n.coreLastErr = fmt.Errorf("not connected to core api: %w", err)
				n.logger.Warn().WithError(err).Log("not connected to core API")
			}
			n.lock.Unlock()
		case <-ctx.Done():
			return
		}
	}
}
