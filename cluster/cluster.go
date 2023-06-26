package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	gonet "net"
	"net/url"
	"strconv"
	"sync"
	"time"

	apiclient "github.com/datarhei/core/v16/cluster/client"
	"github.com/datarhei/core/v16/cluster/forwarder"
	clusteriam "github.com/datarhei/core/v16/cluster/iam"
	clusteriamadapter "github.com/datarhei/core/v16/cluster/iam/adapter"
	"github.com/datarhei/core/v16/cluster/proxy"
	"github.com/datarhei/core/v16/cluster/raft"
	"github.com/datarhei/core/v16/cluster/store"
	"github.com/datarhei/core/v16/config"
	"github.com/datarhei/core/v16/iam"
	iamaccess "github.com/datarhei/core/v16/iam/access"
	iamidentity "github.com/datarhei/core/v16/iam/identity"
	"github.com/datarhei/core/v16/log"
	"github.com/datarhei/core/v16/net"
	"github.com/datarhei/core/v16/restream/app"
)

type Cluster interface {
	// Address returns the raft address of this node
	Address() string

	// ClusterAPIAddress returns the address of the cluster API of a node
	// with the given raft address.
	ClusterAPIAddress(raftAddress string) (string, error)

	// CoreAPIAddress returns the address of the core API of a node with
	// the given raft address.
	CoreAPIAddress(raftAddress string) (string, error)
	CoreConfig() *config.Config

	About() (ClusterAbout, error)

	Join(origin, id, raftAddress, peerAddress string) error
	Leave(origin, id string) error // gracefully remove a node from the cluster
	Snapshot() (io.ReadCloser, error)

	Shutdown() error

	ListProcesses() []store.Process
	GetProcess(id app.ProcessID) (store.Process, error)
	AddProcess(origin string, config *app.Config) error
	RemoveProcess(origin string, id app.ProcessID) error
	UpdateProcess(origin string, id app.ProcessID, config *app.Config) error
	SetProcessCommand(origin string, id app.ProcessID, order string) error
	SetProcessMetadata(origin string, id app.ProcessID, key string, data interface{}) error

	IAM(superuser iamidentity.User, jwtRealm, jwtSecret string) (iam.IAM, error)
	ListIdentities() (time.Time, []iamidentity.User)
	ListIdentity(name string) (time.Time, iamidentity.User, error)
	ListPolicies() (time.Time, []iamaccess.Policy)
	ListUserPolicies(name string) (time.Time, []iamaccess.Policy)
	AddIdentity(origin string, identity iamidentity.User) error
	UpdateIdentity(origin, name string, identity iamidentity.User) error
	SetPolicies(origin, name string, policies []iamaccess.Policy) error
	RemoveIdentity(origin string, name string) error

	CreateLock(origin string, name string, validUntil time.Time) (*Lock, error)
	DeleteLock(origin string, name string) error
	ListLocks() map[string]time.Time

	SetKV(origin, key, value string) error
	UnsetKV(origin, key string) error
	GetKV(key string) (string, time.Time, error)
	ListKV(prefix string) map[string]store.Value

	ProxyReader() proxy.ProxyReader
}

type Peer struct {
	ID      string
	Address string
}

type Config struct {
	ID      string // ID of the node
	Name    string // Name of the node
	Path    string // Path where to store all cluster data
	Address string // Listen address for the raft protocol
	Peers   []Peer // Address of a member of a cluster to join

	SyncInterval           time.Duration // Interval between aligning the process in the cluster DB with the processes on the nodes
	NodeRecoverTimeout     time.Duration // Timeout for a node to recover before rebalancing the processes
	EmergencyLeaderTimeout time.Duration // Timeout for establishing the emergency leadership after lost contact to raft leader

	CoreAPIAddress  string // Address of the core API
	CoreAPIUsername string // Username for the core API
	CoreAPIPassword string // Password for the core API

	Config *config.Config

	IPLimiter net.IPLimiter
	Logger    log.Logger
}

type cluster struct {
	id   string
	name string
	path string

	logger log.Logger

	raft                    raft.Raft
	raftRemoveGracePeriod   time.Duration
	raftAddress             string
	raftNotifyCh            chan bool
	raftEmergencyNotifyCh   chan bool
	raftLeaderObservationCh chan string

	store store.Store

	cancelLeaderShip context.CancelFunc

	shutdown     bool
	shutdownCh   chan struct{}
	shutdownLock sync.Mutex

	syncInterval           time.Duration
	nodeRecoverTimeout     time.Duration
	emergencyLeaderTimeout time.Duration

	forwarder forwarder.Forwarder
	api       API
	proxy     proxy.Proxy

	config      *config.Config
	coreAddress string

	isDegraded    bool
	isDegradedErr error
	stateLock     sync.Mutex

	isRaftLeader  bool
	hasRaftLeader bool
	isLeader      bool
	leaderLock    sync.Mutex

	nodes     map[string]*clusterNode
	nodesLock sync.RWMutex
}

var ErrDegraded = errors.New("cluster is currently degraded")

func New(config Config) (Cluster, error) {
	c := &cluster{
		id:     config.ID,
		name:   config.Name,
		path:   config.Path,
		logger: config.Logger,

		raftAddress:           config.Address,
		raftRemoveGracePeriod: 5 * time.Second,

		shutdownCh: make(chan struct{}),

		syncInterval:           config.SyncInterval,
		nodeRecoverTimeout:     config.NodeRecoverTimeout,
		emergencyLeaderTimeout: config.EmergencyLeaderTimeout,

		config: config.Config,
		nodes:  map[string]*clusterNode{},
	}

	if c.config == nil {
		return nil, fmt.Errorf("the core config must be provided")
	}

	u, err := url.Parse(config.CoreAPIAddress)
	if err != nil {
		return nil, fmt.Errorf("invalid core API address: %w", err)
	}

	if len(config.CoreAPIPassword) == 0 {
		u.User = url.User(config.CoreAPIUsername)
	} else {
		u.User = url.UserPassword(config.CoreAPIUsername, config.CoreAPIPassword)
	}

	c.coreAddress = u.String()

	if c.logger == nil {
		c.logger = log.New("")
	}

	store, err := store.NewStore(store.Config{
		Logger: c.logger.WithField("logname", "fsm"),
	})
	if err != nil {
		return nil, err
	}

	c.store = store

	api, err := NewAPI(APIConfig{
		ID:      c.id,
		Cluster: c,
		Logger:  c.logger.WithField("logname", "api"),
	})
	if err != nil {
		return nil, err
	}

	go func(api API) {
		api.Start()
	}(api)

	c.api = api

	nodeproxy, err := proxy.NewProxy(proxy.ProxyConfig{
		ID:        c.id,
		Name:      c.name,
		IPLimiter: config.IPLimiter,
		Logger:    c.logger.WithField("logname", "proxy"),
	})
	if err != nil {
		c.Shutdown()
		return nil, err
	}

	go func(nodeproxy proxy.Proxy) {
		nodeproxy.Start()
	}(nodeproxy)

	c.proxy = nodeproxy

	if forwarder, err := forwarder.New(forwarder.ForwarderConfig{
		ID:     c.id,
		Logger: c.logger.WithField("logname", "forwarder"),
	}); err != nil {
		c.Shutdown()
		return nil, err
	} else {
		c.forwarder = forwarder
	}

	c.logger.Debug().Log("Starting raft")

	peers := []raft.Peer{}

	for _, p := range config.Peers {
		peers = append(peers, raft.Peer{
			ID:      p.ID,
			Address: p.Address,
		})
	}

	c.raftNotifyCh = make(chan bool, 16)
	c.raftLeaderObservationCh = make(chan string, 16)
	c.raftEmergencyNotifyCh = make(chan bool, 16)

	raft, err := raft.New(raft.Config{
		ID:                  config.ID,
		Path:                config.Path,
		Address:             config.Address,
		Peers:               peers,
		Store:               store,
		LeadershipNotifyCh:  c.raftNotifyCh,
		LeaderObservationCh: c.raftLeaderObservationCh,
		Logger:              c.logger.WithComponent("Raft"),
	})
	if err != nil {
		c.Shutdown()
		return nil, err
	}

	c.raft = raft

	if len(config.Peers) != 0 {
		for i := 0; i < len(config.Peers); i++ {
			peerAddress, err := c.ClusterAPIAddress(config.Peers[i].Address)
			if err != nil {
				c.Shutdown()
				return nil, err
			}

			go func(peerAddress string) {
				ticker := time.NewTicker(time.Second)
				defer ticker.Stop()

				for {
					select {
					case <-c.shutdownCh:
						return
					case <-ticker.C:
						err := c.Join("", c.id, c.raftAddress, peerAddress)
						if err != nil {
							c.logger.Warn().WithError(err).Log("Join cluster")
							continue
						}

						return
					}
				}
			}(peerAddress)
		}
	}

	go c.trackNodeChanges()
	go c.trackLeaderChanges()
	go c.monitorLeadership()
	go c.sentinel()

	return c, nil
}

func (c *cluster) Address() string {
	return c.raftAddress
}

func (c *cluster) ClusterAPIAddress(raftAddress string) (string, error) {
	if len(raftAddress) == 0 {
		raftAddress = c.Address()
	}

	return clusterAPIAddress(raftAddress)
}

func clusterAPIAddress(raftAddress string) (string, error) {
	host, port, err := gonet.SplitHostPort(raftAddress)
	if err != nil {
		return "", err
	}

	p, err := strconv.Atoi(port)
	if err != nil {
		return "", err
	}

	return gonet.JoinHostPort(host, strconv.Itoa(p+1)), nil
}

func (c *cluster) CoreAPIAddress(raftAddress string) (string, error) {
	if len(raftAddress) == 0 {
		raftAddress = c.Address()
	}

	if raftAddress == c.Address() {
		return c.coreAddress, nil
	}

	addr, err := c.ClusterAPIAddress(raftAddress)
	if err != nil {
		return "", err
	}

	client := apiclient.APIClient{
		Address: addr,
	}

	coreAddress, err := client.CoreAPIAddress()

	return coreAddress, err
}

func (c *cluster) CoreConfig() *config.Config {
	return c.config.Clone()
}

func (c *cluster) Shutdown() error {
	c.logger.Info().Log("Shutting down cluster")
	c.shutdownLock.Lock()
	defer c.shutdownLock.Unlock()

	if c.shutdown {
		return nil
	}

	c.shutdown = true
	close(c.shutdownCh)

	for id, node := range c.nodes {
		node.Stop()
		if c.proxy != nil {
			c.proxy.RemoveNode(id)
		}
	}

	if c.proxy != nil {
		c.proxy.Stop()
	}

	if c.api != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		c.api.Shutdown(ctx)
	}

	if c.raft != nil {
		c.raft.Shutdown()
		c.raft = nil
	}

	return nil
}

func (c *cluster) IsRaftLeader() bool {
	c.leaderLock.Lock()
	defer c.leaderLock.Unlock()

	return c.isRaftLeader
}

func (c *cluster) IsDegraded() (bool, error) {
	c.stateLock.Lock()
	defer c.stateLock.Unlock()

	return c.isDegraded, c.isDegradedErr
}

func (c *cluster) Leave(origin, id string) error {
	if len(id) == 0 {
		id = c.id
	}

	c.logger.Debug().WithFields(log.Fields{
		"nodeid": id,
	}).Log("Received leave request for server")

	if !c.IsRaftLeader() {
		// Tell the leader to remove us
		err := c.forwarder.Leave(origin, id)
		if err != nil {
			return err
		}

		// Wait for us being removed from the configuration
		left := false
		limit := time.Now().Add(c.raftRemoveGracePeriod)
		for !left && time.Now().Before(limit) {
			c.logger.Debug().Log("Waiting for getting removed from the configuration")
			// Sleep a while before we check.
			time.Sleep(50 * time.Millisecond)

			// Get the latest configuration.
			servers, err := c.raft.Servers()
			if err != nil {
				c.logger.Error().WithError(err).Log("Raft configuration")
				break
			}

			// See if we are no longer included.
			left = true
			for _, server := range servers {
				if server.Address == c.raftAddress {
					left = false
					break
				}
			}
		}

		if !left {
			c.logger.Warn().Log("Failed to leave raft configuration gracefully, timeout")
		}

		return nil
	}

	// Count the number of servers in the cluster
	servers, err := c.raft.Servers()
	if err != nil {
		c.logger.Error().WithError(err).Log("Raft configuration")
		return err
	}

	numPeers := len(servers)

	if id == c.id {
		// We're going to remove ourselves
		if numPeers <= 1 {
			// Don't do so if we're the only server in the cluster
			c.logger.Debug().Log("We're the leader without any peers, not doing anything")
			return nil
		}

		// Transfer the leadership to another server
		err := c.leadershipTransfer()
		if err != nil {
			c.logger.Warn().WithError(err).Log("Transfer leadership")
			return err
		}

		// Wait for new leader election
		for {
			c.logger.Debug().Log("Waiting for new leader election")

			time.Sleep(50 * time.Millisecond)

			c.leaderLock.Lock()
			hasLeader := c.hasRaftLeader
			c.leaderLock.Unlock()

			if hasLeader {
				break
			}
		}

		// Tell the new leader to remove us
		err = c.forwarder.Leave("", id)
		if err != nil {
			return err
		}

		// Wait for us being removed from the configuration
		left := false
		limit := time.Now().Add(c.raftRemoveGracePeriod)
		for !left && time.Now().Before(limit) {
			c.logger.Debug().Log("Waiting for getting removed from the configuration")
			// Sleep a while before we check.
			time.Sleep(50 * time.Millisecond)

			// Get the latest configuration.
			servers, err := c.raft.Servers()
			if err != nil {
				c.logger.Error().WithError(err).Log("Raft configuration")
				break
			}

			// See if we are no longer included.
			left = true
			for _, server := range servers {
				if server.Address == c.raftAddress {
					left = false
					break
				}
			}
		}

		return nil
	}

	// Remove another sever from the cluster
	err = c.raft.RemoveServer(id)
	if err != nil {
		c.logger.Error().WithError(err).WithFields(log.Fields{
			"nodeid": id,
		}).Log("Remove server")

		return err
	}

	return nil
}

func (c *cluster) Join(origin, id, raftAddress, peerAddress string) error {
	if !c.IsRaftLeader() {
		c.logger.Debug().Log("Not leader, forwarding to leader")
		return c.forwarder.Join(origin, id, raftAddress, peerAddress)
	}

	c.logger.Debug().WithFields(log.Fields{
		"nodeid":  id,
		"address": raftAddress,
	}).Log("Received join request for remote server")

	servers, err := c.raft.Servers()
	if err != nil {
		c.logger.Error().WithError(err).Log("Raft configuration")
		return err
	}

	nodeExists := false

	for _, srv := range servers {
		// If a node already exists with either the joining node's ID or address,
		// that node may need to be removed from the config first.
		if srv.ID == id || srv.Address == raftAddress {
			// However if *both* the ID and the address are the same, then nothing -- not even
			// a join operation -- is needed.
			if srv.ID == id && srv.Address == raftAddress {
				nodeExists = true
				c.logger.Debug().WithFields(log.Fields{
					"nodeid":  id,
					"address": raftAddress,
				}).Log("Server is already member of cluster, ignoring join request")
			} else {
				err := c.raft.RemoveServer(srv.ID)
				if err != nil {
					c.logger.Error().WithError(err).WithFields(log.Fields{
						"nodeid":  id,
						"address": raftAddress,
					}).Log("Removing existing node")
					return fmt.Errorf("error removing existing node %s at %s: %w", id, raftAddress, err)
				}
			}
		}
	}

	if !nodeExists {
		err := c.raft.AddServer(id, raftAddress)
		if err != nil {
			return err
		}
	}

	c.logger.Info().WithFields(log.Fields{
		"nodeid":  id,
		"address": raftAddress,
	}).Log("Joined successfully")

	return nil
}

func (c *cluster) Snapshot() (io.ReadCloser, error) {
	if !c.IsRaftLeader() {
		c.logger.Debug().Log("Not leader, forwarding to leader")
		return c.forwarder.Snapshot()
	}

	return c.raft.Snapshot()
}

func (c *cluster) trackNodeChanges() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Get the latest configuration.
			servers, err := c.raft.Servers()
			if err != nil {
				c.logger.Error().WithError(err).Log("Raft configuration")
				continue
			}

			c.nodesLock.Lock()

			removeNodes := map[string]struct{}{}
			for id := range c.nodes {
				removeNodes[id] = struct{}{}
			}

			for _, server := range servers {
				id := server.ID

				_, ok := c.nodes[id]
				if !ok {
					logger := c.logger.WithFields(log.Fields{
						"id":      server.ID,
						"address": server.Address,
					})

					address, err := clusterAPIAddress(server.Address)
					if err != nil {
						logger.Warn().WithError(err).Log("Discovering cluster API address")
					}

					cnode := NewClusterNode(id, address)

					if err := verifyClusterVersion(cnode.Version()); err != nil {
						logger.Warn().Log("Version mismatch. Cluster will end up in degraded mode")
					}

					if _, err := c.proxy.AddNode(id, cnode.Proxy()); err != nil {
						logger.Warn().WithError(err).Log("Adding node")
						cnode.Stop()
						continue
					}

					c.nodes[id] = cnode
				} else {
					delete(removeNodes, id)
				}
			}

			for id := range removeNodes {
				node, ok := c.nodes[id]
				if !ok {
					continue
				}

				c.proxy.RemoveNode(id)
				node.Stop()
				delete(c.nodes, id)
			}

			c.nodesLock.Unlock()

			// Put the cluster in "degraded" mode in case there's a mismatch in expected values
			err = c.checkClusterNodes()

			c.stateLock.Lock()
			if err != nil {
				c.isDegraded = true
				c.isDegradedErr = err
			} else {
				c.isDegraded = false
				c.isDegradedErr = nil
			}
			c.stateLock.Unlock()
		case <-c.shutdownCh:
			return
		}
	}
}

func (c *cluster) checkClusterNodes() error {
	c.nodesLock.RLock()
	defer c.nodesLock.RUnlock()

	for id, node := range c.nodes {
		if status, err := node.Status(); status == "offline" {
			return fmt.Errorf("node %s is offline: %w", id, err)
		}

		version := node.Version()
		if err := verifyClusterVersion(version); err != nil {
			return fmt.Errorf("node %s has a different version: %s: %w", id, version, err)
		}

		config, err := node.CoreConfig()
		if err != nil {
			return fmt.Errorf("node %s has no configuration available: %w", id, err)
		}
		if err := verifyClusterConfig(c.config, config); err != nil {
			return fmt.Errorf("node %s has a different configuration: %w", id, err)
		}
	}

	return nil
}

func verifyClusterVersion(v string) error {
	version, err := ParseClusterVersion(v)
	if err != nil {
		return fmt.Errorf("parsing version %s: %w", v, err)
	}

	if !Version.Equal(version) {
		return fmt.Errorf("version %s not equal to expected version %s", version.String(), Version.String())
	}

	return nil
}

func verifyClusterConfig(local, remote *config.Config) error {
	if remote == nil {
		return fmt.Errorf("config is not available")
	}

	if local.Cluster.Enable != remote.Cluster.Enable {
		return fmt.Errorf("cluster.enable is different")
	}

	if local.Cluster.SyncInterval != remote.Cluster.SyncInterval {
		return fmt.Errorf("cluster.sync_interval_sec is different, local: %ds vs. remote: %ds", local.Cluster.SyncInterval, remote.Cluster.SyncInterval)
	}

	if local.Cluster.NodeRecoverTimeout != remote.Cluster.NodeRecoverTimeout {
		return fmt.Errorf("cluster.node_recover_timeout_sec is different, local: %ds vs. remote: %ds", local.Cluster.NodeRecoverTimeout, remote.Cluster.NodeRecoverTimeout)
	}

	if local.Cluster.EmergencyLeaderTimeout != remote.Cluster.EmergencyLeaderTimeout {
		return fmt.Errorf("cluster.emergency_leader_timeout_sec is different, local: %ds vs. remote: %ds", local.Cluster.EmergencyLeaderTimeout, remote.Cluster.EmergencyLeaderTimeout)
	}

	if local.RTMP.Enable != remote.RTMP.Enable {
		return fmt.Errorf("rtmp.enable is different")
	}

	if local.RTMP.App != remote.RTMP.App {
		return fmt.Errorf("rtmp.app is different")
	}

	if local.SRT.Enable != remote.SRT.Enable {
		return fmt.Errorf("srt.enable is different")
	}

	if local.SRT.Passphrase != remote.SRT.Passphrase {
		return fmt.Errorf("srt.passphrase is different")
	}

	if local.Resources.MaxCPUUsage == 0 || remote.Resources.MaxCPUUsage == 0 {
		return fmt.Errorf("resources.max_cpu_usage must be defined")
	}

	if local.Resources.MaxMemoryUsage == 0 || remote.Resources.MaxMemoryUsage == 0 {
		return fmt.Errorf("resources.max_memory_usage must be defined")
	}

	return nil
}

// trackLeaderChanges registers an Observer with raft in order to receive updates
// about leader changes, in order to keep the forwarder up to date.
func (c *cluster) trackLeaderChanges() {
	for {
		select {
		case leaderAddress := <-c.raftLeaderObservationCh:
			c.logger.Debug().WithFields(log.Fields{
				"address": leaderAddress,
			}).Log("Leader observation")
			if len(leaderAddress) != 0 {
				leaderAddress, _ = c.ClusterAPIAddress(leaderAddress)
			}
			c.forwarder.SetLeader(leaderAddress)
			c.leaderLock.Lock()
			if len(leaderAddress) == 0 {
				c.hasRaftLeader = false
			} else {
				c.hasRaftLeader = true
			}
			c.leaderLock.Unlock()
		case <-c.shutdownCh:
			return
		}
	}
}

func (c *cluster) ListProcesses() []store.Process {
	return c.store.ListProcesses()
}

func (c *cluster) GetProcess(id app.ProcessID) (store.Process, error) {
	return c.store.GetProcess(id)
}

func (c *cluster) AddProcess(origin string, config *app.Config) error {
	if ok, _ := c.IsDegraded(); ok {
		return ErrDegraded
	}

	if !c.IsRaftLeader() {
		return c.forwarder.AddProcess(origin, config)
	}

	cmd := &store.Command{
		Operation: store.OpAddProcess,
		Data: &store.CommandAddProcess{
			Config: config,
		},
	}

	return c.applyCommand(cmd)
}

func (c *cluster) RemoveProcess(origin string, id app.ProcessID) error {
	if ok, _ := c.IsDegraded(); ok {
		return ErrDegraded
	}

	if !c.IsRaftLeader() {
		return c.forwarder.RemoveProcess(origin, id)
	}

	cmd := &store.Command{
		Operation: store.OpRemoveProcess,
		Data: &store.CommandRemoveProcess{
			ID: id,
		},
	}

	return c.applyCommand(cmd)
}

func (c *cluster) UpdateProcess(origin string, id app.ProcessID, config *app.Config) error {
	if ok, _ := c.IsDegraded(); ok {
		return ErrDegraded
	}

	if !c.IsRaftLeader() {
		return c.forwarder.UpdateProcess(origin, id, config)
	}

	cmd := &store.Command{
		Operation: store.OpUpdateProcess,
		Data: &store.CommandUpdateProcess{
			ID:     id,
			Config: config,
		},
	}

	return c.applyCommand(cmd)
}

func (c *cluster) SetProcessCommand(origin string, id app.ProcessID, command string) error {
	if ok, _ := c.IsDegraded(); ok {
		return ErrDegraded
	}

	if !c.IsRaftLeader() {
		return c.forwarder.SetProcessCommand(origin, id, command)
	}

	if command == "start" || command == "stop" {
		cmd := &store.Command{
			Operation: store.OpSetProcessOrder,
			Data: &store.CommandSetProcessOrder{
				ID:    id,
				Order: command,
			},
		}

		return c.applyCommand(cmd)
	}

	procs := c.proxy.ListProxyProcesses()
	nodeid := ""

	for _, p := range procs {
		if p.Config.ProcessID() != id {
			continue
		}

		nodeid = p.NodeID

		break
	}

	if len(nodeid) == 0 {
		return fmt.Errorf("the process '%s' is not registered with any node", id.String())
	}

	return c.proxy.CommandProcess(nodeid, id, command)
}

func (c *cluster) SetProcessMetadata(origin string, id app.ProcessID, key string, data interface{}) error {
	if ok, _ := c.IsDegraded(); ok {
		return ErrDegraded
	}

	if !c.IsRaftLeader() {
		return c.forwarder.SetProcessMetadata(origin, id, key, data)
	}

	cmd := &store.Command{
		Operation: store.OpSetProcessMetadata,
		Data: &store.CommandSetProcessMetadata{
			ID:   id,
			Key:  key,
			Data: data,
		},
	}

	return c.applyCommand(cmd)
}

func (c *cluster) IAM(superuser iamidentity.User, jwtRealm, jwtSecret string) (iam.IAM, error) {
	policyAdapter, err := clusteriamadapter.NewPolicyAdapter(c.store)
	if err != nil {
		return nil, fmt.Errorf("cluster policy adapter: %w", err)
	}

	identityAdapter, err := clusteriamadapter.NewIdentityAdapter(c.store)
	if err != nil {
		return nil, fmt.Errorf("cluster identitry adapter: %w", err)
	}

	iam, err := clusteriam.New(iam.Config{
		PolicyAdapter:   policyAdapter,
		IdentityAdapter: identityAdapter,
		Superuser:       superuser,
		JWTRealm:        jwtRealm,
		JWTSecret:       jwtSecret,
		Logger:          c.logger.WithComponent("IAM"),
	}, c.store)
	if err != nil {
		return nil, fmt.Errorf("cluster iam: %w", err)
	}

	return iam, nil
}

func (c *cluster) ListIdentities() (time.Time, []iamidentity.User) {
	users := c.store.ListUsers()

	return users.UpdatedAt, users.Users
}

func (c *cluster) ListIdentity(name string) (time.Time, iamidentity.User, error) {
	user := c.store.GetUser(name)

	if len(user.Users) == 0 {
		return time.Time{}, iamidentity.User{}, fmt.Errorf("not found")
	}

	return user.UpdatedAt, user.Users[0], nil
}

func (c *cluster) ListPolicies() (time.Time, []iamaccess.Policy) {
	policies := c.store.ListPolicies()

	return policies.UpdatedAt, policies.Policies
}

func (c *cluster) ListUserPolicies(name string) (time.Time, []iamaccess.Policy) {
	policies := c.store.ListUserPolicies(name)

	return policies.UpdatedAt, policies.Policies
}

func (c *cluster) AddIdentity(origin string, identity iamidentity.User) error {
	if ok, _ := c.IsDegraded(); ok {
		return ErrDegraded
	}

	if !c.IsRaftLeader() {
		return c.forwarder.AddIdentity(origin, identity)
	}

	cmd := &store.Command{
		Operation: store.OpAddIdentity,
		Data: &store.CommandAddIdentity{
			Identity: identity,
		},
	}

	return c.applyCommand(cmd)
}

func (c *cluster) UpdateIdentity(origin, name string, identity iamidentity.User) error {
	if ok, _ := c.IsDegraded(); ok {
		return ErrDegraded
	}

	if !c.IsRaftLeader() {
		return c.forwarder.UpdateIdentity(origin, name, identity)
	}

	cmd := &store.Command{
		Operation: store.OpUpdateIdentity,
		Data: &store.CommandUpdateIdentity{
			Name:     name,
			Identity: identity,
		},
	}

	return c.applyCommand(cmd)
}

func (c *cluster) SetPolicies(origin, name string, policies []iamaccess.Policy) error {
	if ok, _ := c.IsDegraded(); ok {
		return ErrDegraded
	}

	if !c.IsRaftLeader() {
		return c.forwarder.SetPolicies(origin, name, policies)
	}

	cmd := &store.Command{
		Operation: store.OpSetPolicies,
		Data: &store.CommandSetPolicies{
			Name:     name,
			Policies: policies,
		},
	}

	return c.applyCommand(cmd)
}

func (c *cluster) RemoveIdentity(origin string, name string) error {
	if ok, _ := c.IsDegraded(); ok {
		return ErrDegraded
	}

	if !c.IsRaftLeader() {
		return c.forwarder.RemoveIdentity(origin, name)
	}

	cmd := &store.Command{
		Operation: store.OpRemoveIdentity,
		Data: &store.CommandRemoveIdentity{
			Name: name,
		},
	}

	return c.applyCommand(cmd)
}

func (c *cluster) CreateLock(origin string, name string, validUntil time.Time) (*Lock, error) {
	if ok, _ := c.IsDegraded(); ok {
		return nil, ErrDegraded
	}

	if !c.IsRaftLeader() {
		err := c.forwarder.CreateLock(origin, name, validUntil)
		if err != nil {
			return nil, err
		}

		l := &Lock{
			ValidUntil: validUntil,
		}

		return l, nil
	}

	cmd := &store.Command{
		Operation: store.OpCreateLock,
		Data: &store.CommandCreateLock{
			Name:       name,
			ValidUntil: validUntil,
		},
	}

	err := c.applyCommand(cmd)
	if err != nil {
		return nil, err
	}

	l := &Lock{
		ValidUntil: validUntil,
	}

	return l, nil
}

func (c *cluster) DeleteLock(origin string, name string) error {
	if ok, _ := c.IsDegraded(); ok {
		return ErrDegraded
	}

	if !c.IsRaftLeader() {
		return c.forwarder.DeleteLock(origin, name)
	}

	cmd := &store.Command{
		Operation: store.OpDeleteLock,
		Data: &store.CommandDeleteLock{
			Name: name,
		},
	}

	return c.applyCommand(cmd)
}

func (c *cluster) ListLocks() map[string]time.Time {
	return c.store.ListLocks()
}

func (c *cluster) SetKV(origin, key, value string) error {
	if ok, _ := c.IsDegraded(); ok {
		return ErrDegraded
	}

	if !c.IsRaftLeader() {
		return c.forwarder.SetKV(origin, key, value)
	}

	cmd := &store.Command{
		Operation: store.OpSetKV,
		Data: &store.CommandSetKV{
			Key:   key,
			Value: value,
		},
	}

	return c.applyCommand(cmd)
}

func (c *cluster) UnsetKV(origin, key string) error {
	if ok, _ := c.IsDegraded(); ok {
		return ErrDegraded
	}

	if !c.IsRaftLeader() {
		return c.forwarder.UnsetKV(origin, key)
	}

	cmd := &store.Command{
		Operation: store.OpUnsetKV,
		Data: &store.CommandUnsetKV{
			Key: key,
		},
	}

	return c.applyCommand(cmd)
}

func (c *cluster) GetKV(key string) (string, time.Time, error) {
	value, err := c.store.GetFromKVS(key)
	if err != nil {
		return "", time.Time{}, err
	}

	return value.Value, value.UpdatedAt, nil
}

func (c *cluster) ListKV(prefix string) map[string]store.Value {
	storeValues := c.store.ListKVS(prefix)

	return storeValues
}

func (c *cluster) applyCommand(cmd *store.Command) error {
	b, err := json.Marshal(cmd)
	if err != nil {
		return err
	}

	err = c.raft.Apply(b)
	if err != nil {
		return fmt.Errorf("apply command: %w", err)
	}

	return nil
}

type ClusterRaftServer struct {
	ID      string
	Address string
	Voter   bool
	Leader  bool
}

type ClusterRaftStats struct {
	State       string
	LastContact time.Duration
	NumPeers    uint64
}

type ClusterRaft struct {
	Server []ClusterRaftServer
	Stats  ClusterRaftStats
}

type ClusterAbout struct {
	ID                string
	Address           string
	ClusterAPIAddress string
	CoreAPIAddress    string
	Raft              ClusterRaft
	Nodes             []proxy.NodeAbout
	Version           ClusterVersion
	Degraded          bool
	DegradedErr       error
}

func (c *cluster) About() (ClusterAbout, error) {
	degraded, degradedErr := c.IsDegraded()

	about := ClusterAbout{
		ID:          c.id,
		Address:     c.Address(),
		Degraded:    degraded,
		DegradedErr: degradedErr,
	}

	if address, err := c.ClusterAPIAddress(""); err == nil {
		about.ClusterAPIAddress = address
	}

	if address, err := c.CoreAPIAddress(""); err == nil {
		about.CoreAPIAddress = address
	}

	stats := c.raft.Stats()

	about.Raft.Stats.State = stats.State
	about.Raft.Stats.LastContact = stats.LastContact
	about.Raft.Stats.NumPeers = stats.NumPeers

	servers, err := c.raft.Servers()
	if err != nil {
		c.logger.Error().WithError(err).Log("Raft configuration")
		return ClusterAbout{}, err
	}

	for _, server := range servers {
		node := ClusterRaftServer{
			ID:      server.ID,
			Address: server.Address,
			Voter:   server.Voter,
			Leader:  server.Leader,
		}

		about.Raft.Server = append(about.Raft.Server, node)
	}

	about.Version = Version

	nodes := c.ProxyReader().ListNodes()
	for _, node := range nodes {
		about.Nodes = append(about.Nodes, node.About())
	}

	return about, nil
}

func (c *cluster) sentinel() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	isEmergencyLeader := false

	for {
		select {
		case <-c.shutdownCh:
			return
		case <-ticker.C:
			stats := c.raft.Stats()

			c.logger.Debug().WithFields(log.Fields{
				"state":        stats.State,
				"last_contact": stats.LastContact,
				"num_peers":    stats.NumPeers,
			}).Log("Stats")

			if stats.LastContact > c.emergencyLeaderTimeout && !isEmergencyLeader {
				c.logger.Warn().Log("Force leadership due to lost contact to leader")
				c.raftEmergencyNotifyCh <- true
				isEmergencyLeader = true
			} else if stats.LastContact <= c.emergencyLeaderTimeout && isEmergencyLeader {
				c.logger.Warn().Log("Stop forced leadership due to contact to leader")
				c.raftEmergencyNotifyCh <- false
				isEmergencyLeader = false
			}
		}
	}
}

func (c *cluster) ProxyReader() proxy.ProxyReader {
	return c.proxy.Reader()
}
