package cluster

import (
	"context"
	"errors"
	"fmt"
	"io"
	gonet "net"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/datarhei/core/v16/autocert"
	clusterautocert "github.com/datarhei/core/v16/cluster/autocert"
	apiclient "github.com/datarhei/core/v16/cluster/client"
	"github.com/datarhei/core/v16/cluster/forwarder"
	"github.com/datarhei/core/v16/cluster/kvs"
	clusternode "github.com/datarhei/core/v16/cluster/node"
	"github.com/datarhei/core/v16/cluster/raft"
	"github.com/datarhei/core/v16/cluster/store"
	"github.com/datarhei/core/v16/config"
	"github.com/datarhei/core/v16/encoding/json"
	"github.com/datarhei/core/v16/ffmpeg/skills"
	"github.com/datarhei/core/v16/iam"
	iamidentity "github.com/datarhei/core/v16/iam/identity"
	iampolicy "github.com/datarhei/core/v16/iam/policy"
	"github.com/datarhei/core/v16/log"
	"github.com/datarhei/core/v16/net"
	"github.com/datarhei/core/v16/resources"
	"github.com/datarhei/core/v16/restream/app"
)

type Cluster interface {
	Start(ctx context.Context) error
	Shutdown() error

	// Address returns the raft address of this node
	Address() string

	// ClusterAPIAddress returns the address of the cluster API of a node
	// with the given raft address.
	ClusterAPIAddress(raftAddress string) (string, error)

	// CoreAPIAddress returns the address of the core API of a node with
	// the given raft address.
	CoreAPIAddress(raftAddress string) (string, error)
	CoreConfig() *config.Config
	CoreSkills() skills.Skills

	About() (ClusterAbout, error)
	GetBarrier(name string) bool

	Join(origin, id, raftAddress, peerAddress string) error
	Leave(origin, id string) error              // gracefully remove a node from the cluster
	TransferLeadership(origin, id string) error // transfer leadership to another node
	Snapshot(origin string) (io.ReadCloser, error)
	HasRaftLeader() bool

	ProcessAdd(origin string, config *app.Config) error
	ProcessGet(origin string, id app.ProcessID, stale bool) (store.Process, string, error)
	ProcessRemove(origin string, id app.ProcessID) error
	ProcessUpdate(origin string, id app.ProcessID, config *app.Config) error
	ProcessSetCommand(origin string, id app.ProcessID, order string) error
	ProcessSetMetadata(origin string, id app.ProcessID, key string, data interface{}) error
	ProcessGetMetadata(origin string, id app.ProcessID, key string) (interface{}, error)
	ProcessesRelocate(origin string, relocations map[app.ProcessID]string) error

	IAM(superuser iamidentity.User, jwtRealm, jwtSecret string) (iam.IAM, error)
	IAMIdentityAdd(origin string, identity iamidentity.User) error
	IAMIdentityUpdate(origin, name string, identity iamidentity.User) error
	IAMIdentityRemove(origin string, name string) error
	IAMPoliciesSet(origin, name string, policies []iampolicy.Policy) error

	LockCreate(origin string, name string, validUntil time.Time) (*kvs.Lock, error)
	LockDelete(origin string, name string) error

	KVSet(origin, key, value string) error
	KVUnset(origin, key string) error
	KVGet(origin, key string, stale bool) (string, time.Time, error)

	NodeSetState(origin, id, state string) error

	Manager() *clusternode.Manager
	CertManager() autocert.Manager
	Store() store.Store

	Resources() (resources.Info, error)
}

type Peer struct {
	ID      string
	Address string
}

type DebugConfig struct {
	DisableFFmpegCheck bool
}

type Config struct {
	ID      string // ID of the cluster
	NodeID  string // ID of the node
	Name    string // Name of the node
	Path    string // Path where to store all cluster data
	Address string // Listen address for the raft protocol
	Peers   []Peer // Address of a member of a cluster to join

	SyncInterval           time.Duration // Interval between aligning the process in the cluster DB with the processes on the nodes
	NodeRecoverTimeout     time.Duration // Timeout for a node to recover before rebalancing the processes
	EmergencyLeaderTimeout time.Duration // Timeout for establishing the emergency leadership after lost contact to raft leader

	CoreConfig *config.Config
	CoreSkills skills.Skills

	IPLimiter net.IPLimiter
	Resources resources.Resources
	Logger    log.Logger

	Debug DebugConfig
}

type cluster struct {
	id     string
	nodeID string
	name   string
	path   string

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
	shutdownWg   sync.WaitGroup

	syncInterval           time.Duration
	nodeRecoverTimeout     time.Duration
	emergencyLeaderTimeout time.Duration

	forwarder *forwarder.Forwarder
	api       API
	manager   *clusternode.Manager

	config      *config.Config
	skills      skills.Skills
	coreAddress string

	hostnames []string
	stateLock sync.RWMutex

	isRaftLeader  bool
	hasRaftLeader bool
	isLeader      bool
	leaderLock    sync.Mutex

	isTLSRequired bool
	clusterKVS    ClusterKVS
	certManager   autocert.Manager

	barrier     map[string]bool
	barrierLock sync.RWMutex

	limiter net.IPLimiter

	resources resources.Resources

	debugDisableFFmpegCheck bool
}

var ErrDegraded = errors.New("cluster is currently degraded")
var ErrUnknownNode = errors.New("unknown node id")

func New(config Config) (Cluster, error) {
	c := &cluster{
		id:     config.ID,
		nodeID: config.NodeID,
		name:   config.Name,
		path:   config.Path,
		logger: config.Logger,

		raftAddress:           config.Address,
		raftRemoveGracePeriod: 5 * time.Second,

		shutdownCh: make(chan struct{}),

		syncInterval:           config.SyncInterval,
		nodeRecoverTimeout:     config.NodeRecoverTimeout,
		emergencyLeaderTimeout: config.EmergencyLeaderTimeout,

		config: config.CoreConfig,
		skills: config.CoreSkills,

		barrier: map[string]bool{},

		limiter: config.IPLimiter,

		resources: config.Resources,

		debugDisableFFmpegCheck: config.Debug.DisableFFmpegCheck,
	}

	if c.config == nil {
		return nil, fmt.Errorf("the core config must be provided")
	}

	if c.limiter == nil {
		c.limiter = net.NewNullIPLimiter()
	}

	c.isTLSRequired = c.config.TLS.Enable && c.config.TLS.Auto
	if c.isTLSRequired {
		if len(c.config.Host.Name) == 0 {
			return nil, fmt.Errorf("tls: at least one hostname must be configured")
		}
	}

	host, port, err := gonet.SplitHostPort(c.config.Address)
	if err != nil {
		return nil, fmt.Errorf("invalid core address: %s: %w", c.config.Address, err)
	}

	chost, _, err := gonet.SplitHostPort(c.config.Cluster.Address)
	if err != nil {
		return nil, fmt.Errorf("invalid cluster address: %s: %w", c.config.Cluster.Address, err)
	}

	if len(chost) == 0 {
		return nil, fmt.Errorf("invalid cluster address: %s: %w", c.config.Cluster.Address, err)
	}

	if len(host) == 0 {
		host = chost
	}

	u := &url.URL{
		Scheme: "http",
		Host:   gonet.JoinHostPort(host, port),
		Path:   "/",
	}

	if len(c.config.API.Auth.Password) == 0 {
		u.User = url.User(c.config.API.Auth.Username)
	} else {
		u.User = url.UserPassword(c.config.API.Auth.Username, c.config.API.Auth.Password)
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
		ID:      c.nodeID,
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

	nodemanager, err := clusternode.NewManager(clusternode.ManagerConfig{
		ID:     c.nodeID,
		Logger: c.logger.WithField("logname", "proxy"),
	})
	if err != nil {
		c.Shutdown()
		return nil, err
	}

	c.manager = nodemanager

	if forwarder, err := forwarder.New(forwarder.Config{
		ID:     c.nodeID,
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
		if p.ID == config.NodeID && p.Address == config.Address {
			continue
		}

		peers = append(peers, raft.Peer{
			ID:      p.ID,
			Address: p.Address,
		})
	}

	c.raftNotifyCh = make(chan bool, 16)
	c.raftLeaderObservationCh = make(chan string, 16)
	c.raftEmergencyNotifyCh = make(chan bool, 16)

	raft, err := raft.New(raft.Config{
		ID:                  config.NodeID,
		Path:                config.Path,
		Address:             config.Address,
		Peers:               peers,
		Store:               store,
		LeadershipNotifyCh:  c.raftNotifyCh,
		LeaderObservationCh: c.raftLeaderObservationCh,
		Logger:              c.logger.WithComponent("Raft").WithField("address", config.Address),
	})
	if err != nil {
		c.Shutdown()
		return nil, err
	}

	c.raft = raft

	if len(peers) != 0 {
		for _, p := range peers {
			peerAddress, err := c.ClusterAPIAddress(p.Address)
			if err != nil {
				c.Shutdown()
				return nil, err
			}

			c.shutdownWg.Add(1)

			go func(peerAddress string) {
				defer c.shutdownWg.Done()

				ticker := time.NewTicker(time.Second)
				defer ticker.Stop()

				for {
					select {
					case <-c.shutdownCh:
						return
					case <-ticker.C:
						err := c.Join("", c.nodeID, c.raftAddress, peerAddress)
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

	c.shutdownWg.Add(4)

	go c.trackNodeChanges()
	go c.trackLeaderChanges()
	go c.monitorLeadership()
	go c.sentinel()

	if c.isTLSRequired {
		kvs, err := NewClusterKVS(c, c.logger.WithComponent("KVS"))
		if err != nil {
			return nil, fmt.Errorf("tls: cluster KVS: %w", err)
		}

		storage, err := clusterautocert.NewStorage(kvs, "core-cluster-certificates", c.logger.WithComponent("KVS"))
		if err != nil {
			return nil, fmt.Errorf("tls: certificate store: %w", err)
		}

		if len(c.config.TLS.Secret) != 0 {
			storage = autocert.NewCryptoStorage(storage, autocert.NewCrypto(c.config.TLS.Secret))
		}

		manager, err := autocert.New(autocert.Config{
			Storage:         storage,
			DefaultHostname: c.config.Host.Name[0],
			EmailAddress:    c.config.TLS.Email,
			IsProduction:    !c.config.TLS.Staging,
			Logger:          c.logger.WithComponent("Let's Encrypt"),
		})
		if err != nil {
			return nil, fmt.Errorf("tls: certificate manager: %w", err)
		}

		c.clusterKVS = kvs
		c.certManager = manager
	}

	return c, nil
}

func (c *cluster) Start(ctx context.Context) error {
	err := c.setup(ctx)
	if err != nil {
		c.Shutdown()
		return fmt.Errorf("failed to setup cluster: %w", err)
	}

	<-c.shutdownCh

	c.shutdownWg.Wait()

	return nil
}

func (c *cluster) setup(ctx context.Context) error {
	// Wait for a leader to be selected
	c.logger.Info().Log("Waiting for a leader to be elected ...")

	for {
		_, leader := c.raft.Leader()
		if len(leader) != 0 {
			break
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("starting cluster has been aborted: %w", ctx.Err())
		default:
		}

		time.Sleep(500 * time.Millisecond)
	}

	c.logger.Info().Log("Leader has been elected")

	if c.certManager != nil {
		// Load certificates into cache, in case we already have them in the KV store. It
		// allows the API to serve requests. This requires a raft leader.
		c.clusterKVS.AllowStaleKeys(true)
		c.certManager.CacheManagedCertificate(context.Background(), c.config.Host.Name)
		c.clusterKVS.AllowStaleKeys(false)
	}

	// Wait for all cluster nodes to leave degraded mode
	c.logger.Info().Log("Waiting for cluster to become operational ...")

	for {
		ok, err := c.isClusterOperational()
		if !ok {
			break
		}

		c.logger.Warn().WithError(err).Log("Waiting for all nodes to be registered")

		select {
		case <-ctx.Done():
			return fmt.Errorf("starting cluster has been aborted: %w: %s", ctx.Err(), err.Error())
		default:
		}

		time.Sleep(time.Second)
	}

	err := c.Barrier(ctx, "operational")
	if err != nil {
		return fmt.Errorf("failed on barrier: %w", err)
	}

	c.logger.Info().Log("Cluster is operational")

	if c.certManager != nil {
		c.logger.Info().Log("Waiting for TLS certificates ...")

		// Create certificate manager
		hostnames, err := c.getClusterHostnames()
		if err != nil {
			return fmt.Errorf("tls: failed to assemble list of all configured hostnames: %w", err)
		}

		if len(hostnames) == 0 {
			return fmt.Errorf("no hostnames are configured")
		}

		// We have to wait for all nodes to have the HTTP challenge resolver started
		err = c.Barrier(ctx, "acme")
		if err != nil {
			return fmt.Errorf("tls: failed on barrier: %w", err)
		}

		// Acquire certificates, all nodes can do this at the same time because everything
		// is synched via the storage.
		err = c.certManager.AcquireCertificates(ctx, hostnames)
		if err != nil {
			return fmt.Errorf("tls: failed to acquire certificates: %w", err)
		}

		c.logger.Info().Log("TLS certificates acquired")
	}

	c.logger.Info().Log("Waiting for cluster to become ready ...")

	err = c.Barrier(ctx, "ready")
	if err != nil {
		return fmt.Errorf("failed on barrier: %w", err)
	}

	c.logger.Info().Log("Cluster is ready")

	return nil
}

func (c *cluster) GetBarrier(name string) bool {
	c.barrierLock.RLock()
	defer c.barrierLock.RUnlock()

	return c.barrier[name]
}

func (c *cluster) Barrier(ctx context.Context, name string) error {
	c.barrierLock.Lock()
	c.barrier[name] = true
	c.barrierLock.Unlock()

	for {
		ok, err := c.getClusterBarrier(name)
		if ok {
			break
		}

		c.logger.Warn().WithField("name", name).WithError(err).Log("Waiting for barrier")

		select {
		case <-ctx.Done():
			return fmt.Errorf("barrier %s: starting cluster has been aborted: %w: %s", name, ctx.Err(), err.Error())
		default:
		}

		time.Sleep(time.Second)
	}

	return nil
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

func (c *cluster) CoreSkills() skills.Skills {
	return c.skills
}

func (c *cluster) CertManager() autocert.Manager {
	return c.certManager
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

	c.shutdownWg.Wait()

	if c.manager != nil {
		c.manager.NodesClear()
	}

	if c.api != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		c.api.Shutdown(ctx)
	}

	if c.raft != nil {
		c.raft.Shutdown()
	}

	return nil
}

func (c *cluster) IsRaftLeader() bool {
	c.leaderLock.Lock()
	defer c.leaderLock.Unlock()

	return c.isRaftLeader
}

func (c *cluster) HasRaftLeader() bool {
	c.leaderLock.Lock()
	defer c.leaderLock.Unlock()

	return c.hasRaftLeader
}

func (c *cluster) isClusterOperational() (bool, error) {
	servers, err := c.raft.Servers()
	if err != nil {
		return true, err
	}

	serverCount := len(servers)
	nodeCount := c.manager.NodeCount()

	if serverCount != nodeCount {
		return true, fmt.Errorf("%d of %d nodes registered", nodeCount, serverCount)
	}

	return false, nil
}

func (c *cluster) Leave(origin, id string) error {
	if len(id) == 0 {
		id = c.nodeID
	}

	if !c.manager.NodeHasNode(id) {
		return ErrUnknownNode
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

	if id == c.nodeID {
		// We're going to remove ourselves
		if numPeers <= 1 {
			// Don't do so if we're the only server in the cluster
			c.logger.Debug().Log("We're the leader without any peers, not doing anything")
			return nil
		}

		// Transfer the leadership to another server
		err := c.leadershipTransfer("")
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

func (c *cluster) TransferLeadership(origin, id string) error {
	if !c.IsRaftLeader() {
		c.logger.Debug().Log("Not leader, forwarding to leader")
		return c.forwarder.TransferLeadership(origin, id)
	}

	return c.leadershipTransfer(id)
}

func (c *cluster) Snapshot(origin string) (io.ReadCloser, error) {
	if !c.IsRaftLeader() {
		c.logger.Debug().Log("Not leader, forwarding to leader")
		return c.forwarder.Snapshot(origin)
	}

	return c.raft.Snapshot()
}

func (c *cluster) trackNodeChanges() {
	defer c.shutdownWg.Done()

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

			removeNodes := map[string]struct{}{}
			for _, id := range c.manager.NodeIDs() {
				removeNodes[id] = struct{}{}
			}

			for _, server := range servers {
				id := server.ID

				if !c.manager.NodeHasNode(id) {
					logger := c.logger.WithFields(log.Fields{
						"id":      server.ID,
						"address": server.Address,
					})

					address, err := clusterAPIAddress(server.Address)
					if err != nil {
						logger.Warn().WithError(err).Log("Discovering cluster API address")
					}

					node := clusternode.New(clusternode.Config{
						ID:      id,
						Address: address,
						Logger: c.logger.WithComponent("ClusterNode").WithFields(log.Fields{
							"id":      id,
							"address": address,
						}),
					})

					if _, err := c.manager.NodeAdd(id, node); err != nil {
						logger.Warn().WithError(err).Log("Adding node")
						node.Stop()
						continue
					}

					ips := node.IPs()
					for _, ip := range ips {
						c.limiter.AddBlock(ip)
					}
				} else {
					delete(removeNodes, id)
				}
			}

			for id := range removeNodes {
				if node, err := c.manager.NodeRemove(id); err != nil {
					ips := node.IPs()
					for _, ip := range ips {
						c.limiter.RemoveBlock(ip)
					}
				}
			}

			// Put the cluster in "degraded" mode in case there's a mismatch in expected values
			c.manager.NodeCheckCompatibility(c.debugDisableFFmpegCheck)

			hostnames, _ := c.manager.GetHostnames(true)

			c.stateLock.Lock()
			c.hostnames = hostnames
			c.stateLock.Unlock()
		case <-c.shutdownCh:
			return
		}
	}
}

// getClusterHostnames return a list of all hostnames configured on all nodes. The
// returned list will not contain any duplicates.
func (c *cluster) getClusterHostnames() ([]string, error) {
	return c.manager.GetHostnames(false)
}

// getClusterBarrier returns whether all nodes are currently on the same barrier.
func (c *cluster) getClusterBarrier(name string) (bool, error) {
	return c.manager.Barrier(name)
}

// trackLeaderChanges registers an Observer with raft in order to receive updates
// about leader changes, in order to keep the forwarder up to date.
func (c *cluster) trackLeaderChanges() {
	defer c.shutdownWg.Done()

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

			servers, err := c.raft.Servers()
			if err != nil {
				c.logger.Error().WithError(err).Log("Raft configuration")
				break
			}

			isNodeInCluster := false
			for _, server := range servers {
				if c.nodeID == server.ID {
					isNodeInCluster = true
					break
				}
			}

			if !isNodeInCluster {
				// We're not anymore part of the cluster, shutdown
				c.logger.Warn().WithField("id", c.nodeID).Log("This node left the cluster. Shutting down.")
				c.Shutdown()
			}

		case <-c.shutdownCh:
			return
		}
	}
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

func (c *cluster) sentinel() {
	defer c.shutdownWg.Done()

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

			if stats.NumPeers > 1 {
				// Enable emergency leadership only in a configuration with two nodes.
				break
			}

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

func (c *cluster) Resources() (resources.Info, error) {
	if c.resources == nil {
		return resources.Info{}, fmt.Errorf("resource information is not available")
	}

	return c.resources.Info(), nil
}

func (c *cluster) Manager() *clusternode.Manager {
	if c.manager == nil {
		return nil
	}

	return c.manager
}

func (c *cluster) Store() store.Store {
	if c.store == nil {
		return nil
	}

	return c.store
}
