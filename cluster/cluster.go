package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	gonet "net"
	"net/url"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/datarhei/core/v16/autocert"
	clusterautocert "github.com/datarhei/core/v16/cluster/autocert"
	apiclient "github.com/datarhei/core/v16/cluster/client"
	"github.com/datarhei/core/v16/cluster/forwarder"
	"github.com/datarhei/core/v16/cluster/kvs"
	clusternode "github.com/datarhei/core/v16/cluster/node"
	"github.com/datarhei/core/v16/cluster/proxy"
	"github.com/datarhei/core/v16/cluster/raft"
	"github.com/datarhei/core/v16/cluster/store"
	"github.com/datarhei/core/v16/config"
	"github.com/datarhei/core/v16/ffmpeg/skills"
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
	CoreSkills() skills.Skills

	About() (ClusterAbout, error)
	IsClusterDegraded() (bool, error)
	IsDegraded() (bool, error)
	GetBarrier(name string) bool

	Join(origin, id, raftAddress, peerAddress string) error
	Leave(origin, id string) error // gracefully remove a node from the cluster
	Snapshot(origin string) (io.ReadCloser, error)

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

	CreateLock(origin string, name string, validUntil time.Time) (*kvs.Lock, error)
	DeleteLock(origin string, name string) error
	ListLocks() map[string]time.Time

	SetKV(origin, key, value string) error
	UnsetKV(origin, key string) error
	GetKV(origin, key string) (string, time.Time, error)
	ListKV(prefix string) map[string]store.Value

	ProxyReader() proxy.ProxyReader
	CertManager() autocert.Manager
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

	CoreConfig *config.Config
	CoreSkills skills.Skills

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
	skills      skills.Skills
	coreAddress string

	isDegraded        bool
	isDegradedErr     error
	isCoreDegraded    bool
	isCoreDegradedErr error
	stateLock         sync.Mutex

	isRaftLeader  bool
	hasRaftLeader bool
	isLeader      bool
	leaderLock    sync.Mutex

	isTLSRequired bool
	certManager   autocert.Manager

	nodes     map[string]clusternode.Node
	nodesLock sync.RWMutex

	barrier     map[string]bool
	barrierLock sync.RWMutex

	limiter net.IPLimiter
}

var ErrDegraded = errors.New("cluster is currently degraded")

func New(ctx context.Context, config Config) (Cluster, error) {
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

		isDegraded:    true,
		isDegradedErr: fmt.Errorf("cluster not yet startet"),

		isCoreDegraded:    true,
		isCoreDegradedErr: fmt.Errorf("cluster not yet started"),

		config: config.CoreConfig,
		skills: config.CoreSkills,
		nodes:  map[string]clusternode.Node{},

		barrier: map[string]bool{},

		limiter: config.IPLimiter,
	}

	if c.config == nil {
		return nil, fmt.Errorf("the core config must be provided")
	}

	if c.limiter == nil {
		c.limiter = net.NewNullIPLimiter()
	}

	c.isTLSRequired = c.config.TLS.Enable && c.config.TLS.Auto

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
		ID:     c.id,
		Logger: c.logger.WithField("logname", "proxy"),
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

	err = c.setup(ctx)
	if err != nil {
		c.Shutdown()
		return nil, fmt.Errorf("failed to setup cluster: %w", err)
	}

	return c, nil
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

	// Wait for all cluster nodes to leave degraded mode

	c.logger.Info().Log("Waiting for cluster to become operational ...")

	for {
		ok, err := c.IsClusterDegraded()
		if !ok {
			break
		}

		c.logger.Warn().WithError(err).Log("Cluster is in degraded state")

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

	if c.isTLSRequired {
		c.logger.Info().Log("Waiting for TLS certificates ...")

		// Create certificate manager
		hostnames, err := c.getClusterHostnames()
		if err != nil {
			return fmt.Errorf("tls: failed to assemble list of all configured hostnames: %w", err)
		}

		if len(hostnames) == 0 {
			return fmt.Errorf("no hostnames are configured")
		}

		kvs, err := NewClusterKVS(c, c.logger.WithComponent("KVS"))
		if err != nil {
			return fmt.Errorf("tls: cluster KVS: %w", err)
		}

		storage, err := clusterautocert.NewStorage(kvs, "core-cluster-certificates", c.logger.WithComponent("KVS"))
		if err != nil {
			return fmt.Errorf("tls: certificate store: %w", err)
		}

		if len(c.config.TLS.Secret) != 0 {
			storage = autocert.NewCryptoStorage(storage, autocert.NewCrypto(c.config.TLS.Secret))
		}

		manager, err := autocert.New(autocert.Config{
			Storage:         storage,
			DefaultHostname: hostnames[0],
			EmailAddress:    c.config.TLS.Email,
			IsProduction:    !c.config.TLS.Staging,
			Logger:          c.logger.WithComponent("Let's Encrypt"),
		})
		if err != nil {
			return fmt.Errorf("tls: certificate manager: %w", err)
		}

		c.certManager = manager

		resctx, rescancel := context.WithCancel(ctx)
		defer rescancel()

		err = manager.HTTPChallengeResolver(resctx, c.config.Address)
		if err != nil {
			return fmt.Errorf("tls: failed to start the HTTP challenge resolver: %w", err)
		}

		// We have to wait for all nodes to have the HTTP challenge resolver started
		err = c.Barrier(ctx, "acme")
		if err != nil {
			return fmt.Errorf("tls: failed on barrier: %w", err)
		}

		// Acquire certificates, all nodes can do this at the same time because everything
		// is synched via the storage.
		err = manager.AcquireCertificates(ctx, hostnames)
		if err != nil {
			return fmt.Errorf("tls: failed to acquire certificates: %w", err)
		}

		rescancel()

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

	if c.isDegraded {
		return c.isDegraded, c.isDegradedErr
	}

	return c.isCoreDegraded, c.isCoreDegradedErr
}

func (c *cluster) IsClusterDegraded() (bool, error) {
	c.stateLock.Lock()
	isDegraded, isDegradedErr := c.isDegraded, c.isDegradedErr
	c.stateLock.Unlock()

	if isDegraded {
		return isDegraded, isDegradedErr
	}

	servers, err := c.raft.Servers()
	if err != nil {
		return true, err
	}

	c.nodesLock.Lock()
	nodes := len(c.nodes)
	c.nodesLock.Unlock()

	if len(servers) != nodes {
		return true, fmt.Errorf("not all nodes are connected")
	}

	return false, nil
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

func (c *cluster) Snapshot(origin string) (io.ReadCloser, error) {
	if !c.IsRaftLeader() {
		c.logger.Debug().Log("Not leader, forwarding to leader")
		return c.forwarder.Snapshot(origin)
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

					node := clusternode.New(id, address)

					if err := verifyClusterVersion(node.Version()); err != nil {
						logger.Warn().Log("Version mismatch. Cluster will end up in degraded mode")
					}

					if _, err := c.proxy.AddNode(id, node.Proxy()); err != nil {
						logger.Warn().WithError(err).Log("Adding node")
						node.Stop()
						continue
					}

					c.nodes[id] = node

					ips := node.IPs()
					for _, ip := range ips {
						c.limiter.AddBlock(ip)
					}
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

				ips := node.IPs()
				for _, ip := range ips {
					c.limiter.RemoveBlock(ip)
				}

				delete(c.nodes, id)
			}

			c.nodesLock.Unlock()

			// Put the cluster in "degraded" mode in case there's a mismatch in expected values
			_, err = c.checkClusterNodes()

			c.stateLock.Lock()
			if err != nil {
				c.isDegraded = true
				c.isDegradedErr = err
			} else {
				c.isDegraded = false
				c.isDegradedErr = nil
			}
			c.stateLock.Unlock()

			// Put the cluster in "coreDegraded" mode in case there's a mismatch in expected values
			err = c.checkClusterCoreNodes()

			c.stateLock.Lock()
			if err != nil {
				c.isCoreDegraded = true
				c.isCoreDegradedErr = err
			} else {
				c.isCoreDegraded = false
				c.isCoreDegradedErr = nil
			}
			c.stateLock.Unlock()
		case <-c.shutdownCh:
			return
		}
	}
}

// checkClusterNodes returns a list of all hostnames configured on all nodes. The
// returned list will not contain any duplicates. An error is returned in case the
// node is not compatible.
func (c *cluster) checkClusterNodes() ([]string, error) {
	hostnames := map[string]struct{}{}

	c.nodesLock.RLock()
	defer c.nodesLock.RUnlock()

	for id, node := range c.nodes {
		if status, err := node.Status(); status == "offline" {
			return nil, fmt.Errorf("node %s is offline: %w", id, err)
		}

		version := node.Version()
		if err := verifyClusterVersion(version); err != nil {
			return nil, fmt.Errorf("node %s has a different version: %s: %w", id, version, err)
		}

		config, err := node.CoreConfig()
		if err != nil {
			return nil, fmt.Errorf("node %s has no configuration available: %w", id, err)
		}
		if err := verifyClusterConfig(c.config, config); err != nil {
			return nil, fmt.Errorf("node %s has a different configuration: %w", id, err)
		}

		skills, err := node.CoreSkills()
		if err != nil {
			return nil, fmt.Errorf("node %s has no FFmpeg skills available: %w", id, err)
		}
		if !c.skills.Equal(skills) {
			return nil, fmt.Errorf("node %s has mismatching FFmpeg skills", id)
		}

		for _, name := range config.Host.Name {
			hostnames[name] = struct{}{}
		}
	}

	names := []string{}

	for key := range hostnames {
		names = append(names, key)
	}

	sort.Strings(names)

	return names, nil
}

func (c *cluster) checkClusterCoreNodes() error {
	c.nodesLock.RLock()
	defer c.nodesLock.RUnlock()

	for id, node := range c.nodes {
		if status, err := node.CoreStatus(); status == "offline" {
			return fmt.Errorf("node %s core is offline: %w", id, err)
		}
	}

	return nil
}

// getClusterHostnames return a list of all hostnames configured on all nodes. The
// returned list will not contain any duplicates.
func (c *cluster) getClusterHostnames() ([]string, error) {
	hostnames := map[string]struct{}{}

	c.nodesLock.RLock()
	defer c.nodesLock.RUnlock()

	for id, node := range c.nodes {
		config, err := node.CoreConfig()
		if err != nil {
			return nil, fmt.Errorf("node %s has no configuration available: %w", id, err)
		}

		for _, name := range config.Host.Name {
			hostnames[name] = struct{}{}
		}
	}

	names := []string{}

	for key := range hostnames {
		names = append(names, key)
	}

	sort.Strings(names)

	return names, nil
}

// getClusterBarrier returns whether all nodes are currently on the same barrier.
func (c *cluster) getClusterBarrier(name string) (bool, error) {
	c.nodesLock.RLock()
	defer c.nodesLock.RUnlock()

	for _, node := range c.nodes {
		ok, err := node.Barrier(name)
		if !ok {
			return false, err
		}
	}

	return true, nil
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
	if local == nil || remote == nil {
		return fmt.Errorf("config is not available")
	}

	if local.Cluster.Enable != remote.Cluster.Enable {
		return fmt.Errorf("cluster.enable is different")
	}

	if local.Cluster.SyncInterval != remote.Cluster.SyncInterval {
		return fmt.Errorf("cluster.sync_interval_sec is different")
	}

	if local.Cluster.NodeRecoverTimeout != remote.Cluster.NodeRecoverTimeout {
		return fmt.Errorf("cluster.node_recover_timeout_sec is different")
	}

	if local.Cluster.EmergencyLeaderTimeout != remote.Cluster.EmergencyLeaderTimeout {
		return fmt.Errorf("cluster.emergency_leader_timeout_sec is different")
	}

	if !local.API.Auth.Enable {
		return fmt.Errorf("api.auth.enable must be true")
	}

	if local.API.Auth.Enable != remote.API.Auth.Enable {
		return fmt.Errorf("api.auth.enable is different")
	}

	if local.API.Auth.Username != remote.API.Auth.Username {
		return fmt.Errorf("api.auth.username is different")
	}

	if local.API.Auth.Password != remote.API.Auth.Password {
		return fmt.Errorf("api.auth.password is different")
	}

	if local.API.Auth.JWT.Secret != remote.API.Auth.JWT.Secret {
		return fmt.Errorf("api.auth.jwt.secret is different")
	}

	if local.RTMP.Enable != remote.RTMP.Enable {
		return fmt.Errorf("rtmp.enable is different")
	}

	if local.RTMP.Enable {
		if local.RTMP.App != remote.RTMP.App {
			return fmt.Errorf("rtmp.app is different")
		}
	}

	if local.SRT.Enable != remote.SRT.Enable {
		return fmt.Errorf("srt.enable is different")
	}

	if local.SRT.Enable {
		if local.SRT.Passphrase != remote.SRT.Passphrase {
			return fmt.Errorf("srt.passphrase is different")
		}
	}

	if local.Resources.MaxCPUUsage == 0 || remote.Resources.MaxCPUUsage == 0 {
		return fmt.Errorf("resources.max_cpu_usage must be defined")
	}

	if local.Resources.MaxMemoryUsage == 0 || remote.Resources.MaxMemoryUsage == 0 {
		return fmt.Errorf("resources.max_memory_usage must be defined")
	}

	if local.TLS.Enable != remote.TLS.Enable {
		return fmt.Errorf("tls.enable is different")
	}

	if local.TLS.Enable {
		if local.TLS.Auto != remote.TLS.Auto {
			return fmt.Errorf("tls.auto is different")
		}

		if len(local.Host.Name) == 0 || len(remote.Host.Name) == 0 {
			return fmt.Errorf("host.name must be set")
		}

		if local.TLS.Auto {
			if local.TLS.Email != remote.TLS.Email {
				return fmt.Errorf("tls.email is different")
			}

			if local.TLS.Staging != remote.TLS.Staging {
				return fmt.Errorf("tls.staging is different")
			}

			if local.TLS.Secret != remote.TLS.Secret {
				return fmt.Errorf("tls.secret is different")
			}
		}
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

func (c *cluster) ProxyReader() proxy.ProxyReader {
	return c.proxy.Reader()
}
