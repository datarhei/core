package node

import (
	"context"
	"errors"
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

	compatibilityErr error

	config *config.Config
	skills *skills.Skills

	lock   sync.RWMutex
	cancel context.CancelFunc

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

	address, coreConfig, coreSkills, err := n.CoreEssentials()
	n.config = coreConfig
	n.skills = coreSkills

	n.core = NewCore(n.id, n.logger.WithComponent("ClusterCore").WithField("address", address))
	n.core.SetEssentials(address, coreConfig)

	n.coreLastErr = err

	go n.updateCore(ctx, 5*time.Second)
	go n.ping(ctx, time.Second)
	go n.pingCore(ctx, time.Second)

	return n
}

func (n *Node) Stop() error {
	n.lock.Lock()
	defer n.lock.Unlock()

	if n.cancel == nil {
		return nil
	}

	n.cancel()
	n.cancel = nil

	n.core.Stop()

	return nil
}

var maxLastContact time.Duration = 5 * time.Second

type About struct {
	ID          string
	Name        string
	Version     string
	Address     string
	State       string
	Uptime      time.Duration
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
		Version: n.version,
		Address: n.address,
	}

	a.Name = n.coreAbout.Name
	a.Error = n.nodeLastErr
	a.LastContact = n.nodeLastContact
	if time.Since(a.LastContact) > maxLastContact {
		a.State = "offline"
	} else if n.nodeLastErr != nil {
		a.State = "degraded"
	} else if n.compatibilityErr != nil {
		a.State = "degraded"
		a.Error = n.compatibilityErr
	} else {
		a.State = "online"
	}
	a.Latency = time.Duration(n.nodeLatency * float64(time.Second))

	a.Resources = n.nodeAbout.Resources
	if a.Resources.Error != nil {
		a.Resources.CPU = a.Resources.CPULimit
		a.Resources.Mem = a.Resources.MemLimit
		a.Resources.IsThrottling = true
	}

	a.Core = n.coreAbout
	a.Core.Error = n.coreLastErr
	a.Core.LastContact = n.coreLastContact
	a.Core.Latency = time.Duration(n.coreLatency * float64(time.Second))

	if a.State == "online" {
		if a.Resources.Error != nil {
			a.State = "degraded"
			a.Error = a.Resources.Error
		}
	}

	if a.State == "online" {
		if time.Since(a.Core.LastContact) > maxLastContact {
			a.Core.State = "offline"
		} else if n.coreLastErr != nil {
			a.Core.State = "degraded"
			a.Error = n.coreLastErr
		} else {
			a.Core.State = "online"
		}

		a.State = a.Core.State
	}

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

func (n *Node) CoreEssentials() (string, *config.Config, *skills.Skills, error) {
	address, err := n.CoreAPIAddress()
	if err != nil {
		return "", nil, nil, err
	}

	config, err := n.CoreConfig(false)
	if err != nil {
		return "", nil, nil, err
	}

	skills, err := n.CoreSkills(false)
	if err != nil {
		return "", nil, nil, err
	}

	return address, config, skills, nil
}

func (n *Node) CoreConfig(cached bool) (*config.Config, error) {
	if cached {
		n.lock.RLock()
		config := n.config
		n.lock.RUnlock()

		if config != nil {
			return config, nil
		}
	}

	return n.node.CoreConfig()
}

func (n *Node) CoreSkills(cached bool) (*skills.Skills, error) {
	if cached {
		n.lock.RLock()
		skills := n.skills
		n.lock.RUnlock()

		if skills != nil {
			return skills, nil
		}
	}

	skills, err := n.node.CoreSkills()

	return &skills, err
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

func (n *Node) CheckCompatibility(other *Node, skipSkillsCheck bool) {
	err := n.checkCompatibility(other, skipSkillsCheck)

	n.lock.Lock()
	n.compatibilityErr = err
	n.lock.Unlock()
}

func (n *Node) checkCompatibility(other *Node, skipSkillsCheck bool) error {
	if other == nil {
		return fmt.Errorf("no other node available to compare to")
	}

	n.lock.RLock()
	version := n.version
	config := n.config
	skills := n.skills
	n.lock.RUnlock()

	otherVersion := other.Version()
	otherConfig, _ := other.CoreConfig(true)
	otherSkills, _ := other.CoreSkills(true)

	err := verifyVersion(version, otherVersion)
	if err != nil {
		return fmt.Errorf("version: %w", err)
	}

	err = verifyConfig(config, otherConfig)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	if !skipSkillsCheck {
		err := verifySkills(skills, otherSkills)
		if err != nil {
			return fmt.Errorf("skills: %w", err)
		}
	}

	return nil
}

func verifyVersion(local, other string) error {
	if local != other {
		return fmt.Errorf("actual: %s, expected %s", local, other)
	}

	return nil
}

func verifyConfig(local, other *config.Config) error {
	if local == nil || other == nil {
		return fmt.Errorf("config is not available")
	}

	if local.Cluster.Enable != other.Cluster.Enable {
		return fmt.Errorf("cluster.enable: actual: %v, expected: %v", local.Cluster.Enable, other.Cluster.Enable)
	}

	if local.Cluster.ID != other.Cluster.ID {
		return fmt.Errorf("cluster.id: actual: %v, expected: %v", local.Cluster.ID, other.Cluster.ID)
	}

	if local.Cluster.SyncInterval != other.Cluster.SyncInterval {
		return fmt.Errorf("cluster.sync_interval_sec: actual: %v, expected: %v", local.Cluster.SyncInterval, other.Cluster.SyncInterval)
	}

	if local.Cluster.NodeRecoverTimeout != other.Cluster.NodeRecoverTimeout {
		return fmt.Errorf("cluster.node_recover_timeout_sec: actual: %v, expected: %v", local.Cluster.NodeRecoverTimeout, other.Cluster.NodeRecoverTimeout)
	}

	if local.Cluster.EmergencyLeaderTimeout != other.Cluster.EmergencyLeaderTimeout {
		return fmt.Errorf("cluster.emergency_leader_timeout_sec: actual: %v, expected: %v", local.Cluster.EmergencyLeaderTimeout, other.Cluster.EmergencyLeaderTimeout)
	}

	if local.Cluster.Debug.DisableFFmpegCheck != other.Cluster.Debug.DisableFFmpegCheck {
		return fmt.Errorf("cluster.debug.disable_ffmpeg_check: actual: %v, expected: %v", local.Cluster.Debug.DisableFFmpegCheck, other.Cluster.Debug.DisableFFmpegCheck)
	}

	if !local.API.Auth.Enable {
		return fmt.Errorf("api.auth.enable must be enabled")
	}

	if local.API.Auth.Enable != other.API.Auth.Enable {
		return fmt.Errorf("api.auth.enable: actual: %v, expected: %v", local.API.Auth.Enable, other.API.Auth.Enable)
	}

	if local.API.Auth.Username != other.API.Auth.Username {
		return fmt.Errorf("api.auth.username: actual: %v, expected: %v", local.API.Auth.Username, other.API.Auth.Username)
	}

	if local.API.Auth.Password != other.API.Auth.Password {
		return fmt.Errorf("api.auth.password: actual: %v, expected: %v", local.API.Auth.Password, other.API.Auth.Password)
	}

	if local.API.Auth.JWT.Secret != other.API.Auth.JWT.Secret {
		return fmt.Errorf("api.auth.jwt.secret: actual: %v, expected: %v", local.API.Auth.JWT.Secret, other.API.Auth.JWT.Secret)
	}

	if local.RTMP.Enable != other.RTMP.Enable {
		return fmt.Errorf("rtmp.enable: actual: %v, expected: %v", local.RTMP.Enable, other.RTMP.Enable)
	}

	if local.RTMP.Enable {
		if local.RTMP.App != other.RTMP.App {
			return fmt.Errorf("rtmp.app: actual: %v, expected: %v", local.RTMP.App, other.RTMP.App)
		}
	}

	if local.SRT.Enable != other.SRT.Enable {
		return fmt.Errorf("srt.enable: actual: %v, expected: %v", local.SRT.Enable, other.SRT.Enable)
	}

	if local.SRT.Enable {
		if local.SRT.Passphrase != other.SRT.Passphrase {
			return fmt.Errorf("srt.passphrase: actual: %v, expected: %v", local.SRT.Passphrase, other.SRT.Passphrase)
		}
	}

	if local.Resources.MaxCPUUsage == 0 || other.Resources.MaxCPUUsage == 0 {
		return fmt.Errorf("resources.max_cpu_usage")
	}

	if local.Resources.MaxMemoryUsage == 0 || other.Resources.MaxMemoryUsage == 0 {
		return fmt.Errorf("resources.max_memory_usage")
	}

	if local.TLS.Enable != other.TLS.Enable {
		return fmt.Errorf("tls.enable: actual: %v, expected: %v", local.TLS.Enable, other.TLS.Enable)
	}

	if local.TLS.Enable {
		if local.TLS.Auto != other.TLS.Auto {
			return fmt.Errorf("tls.auto: actual: %v, expected: %v", local.TLS.Auto, other.TLS.Auto)
		}

		if len(local.Host.Name) == 0 || len(other.Host.Name) == 0 {
			return fmt.Errorf("host.name must be set")
		}

		if local.TLS.Auto {
			if local.TLS.Email != other.TLS.Email {
				return fmt.Errorf("tls.email: actual: %v, expected: %v", local.TLS.Email, other.TLS.Email)
			}

			if local.TLS.Staging != other.TLS.Staging {
				return fmt.Errorf("tls.staging: actual: %v, expected: %v", local.TLS.Staging, other.TLS.Staging)
			}

			if local.TLS.Secret != other.TLS.Secret {
				return fmt.Errorf("tls.secret: actual: %v, expected: %v", local.TLS.Secret, other.TLS.Secret)
			}
		}
	}

	return nil
}

func verifySkills(local, other *skills.Skills) error {
	if local == nil || other == nil {
		return fmt.Errorf("skills are not available")
	}

	if err := local.Equal(*other); err != nil {
		return err
	}

	return nil
}

func (n *Node) ping(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
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
					Uptime:  time.Since(about.StartedAt),
					Error:   err,
					Resources: Resources{
						IsThrottling: about.Resources.IsThrottling,
						NCPU:         about.Resources.NCPU,
						CPU:          about.Resources.CPU,
						CPULimit:     about.Resources.CPULimit,
						Mem:          about.Resources.Mem,
						MemLimit:     about.Resources.MemLimit,
						Error:        nil,
					},
				}
				if len(about.Resources.Error) != 0 {
					n.nodeAbout.Resources.Error = errors.New(about.Resources.Error)
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

func (n *Node) updateCore(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			address, config, skills, err := n.CoreEssentials()

			n.lock.Lock()
			if err == nil {
				n.config = config
				n.skills = skills
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

func (n *Node) pingCore(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
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
			}
			n.lock.Unlock()
		case <-ctx.Done():
			return
		}
	}
}
