package cluster

import (
	"context"
	"encoding/json"
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
	"github.com/datarhei/core/v16/cluster/proxy"
	"github.com/datarhei/core/v16/cluster/raft"
	"github.com/datarhei/core/v16/cluster/store"
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

	ProxyReader() proxy.ProxyReader
}

type Peer struct {
	ID      string
	Address string
}

type ClusterConfig struct {
	ID        string // ID of the node
	Name      string // Name of the node
	Path      string // Path where to store all cluster data
	Bootstrap bool   // Whether to bootstrap a cluster
	Recover   bool   // Whether to recover this node
	Address   string // Listen address for the raft protocol
	Peers     []Peer // Address of a member of a cluster to join

	CoreAPIAddress  string // Address of the core API
	CoreAPIUsername string // Username for the core API
	CoreAPIPassword string // Password for the core API

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

	forwarder forwarder.Forwarder
	api       API
	proxy     proxy.Proxy

	coreAddress string

	isRaftLeader  bool
	hasRaftLeader bool
	isLeader      bool
	leaderLock    sync.Mutex

	nodes     map[string]proxy.Node
	nodesLock sync.RWMutex
}

func New(config ClusterConfig) (Cluster, error) {
	c := &cluster{
		id:     config.ID,
		name:   config.Name,
		path:   config.Path,
		logger: config.Logger,

		raftAddress:           config.Address,
		raftRemoveGracePeriod: 5 * time.Second,

		shutdownCh: make(chan struct{}),

		nodes: map[string]proxy.Node{},
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
		Bootstrap:           config.Bootstrap,
		Recover:             config.Recover,
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

	host, port, _ := gonet.SplitHostPort(raftAddress)

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
		node.Disconnect()
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

	// connect to the peer's API in order to find out if our version is compatible
	address, err := c.CoreAPIAddress(raftAddress)
	if err != nil {
		return fmt.Errorf("peer API doesn't respond: %w", err)
	}

	node := proxy.NewNode(address)
	err = node.Connect()
	if err != nil {
		return fmt.Errorf("couldn't connect to peer: %w", err)
	}
	defer node.Disconnect()

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
	ticker := time.NewTicker(time.Second)
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
					address, err := c.CoreAPIAddress(server.Address)
					if err != nil {
						c.logger.Warn().WithError(err).WithFields(log.Fields{
							"id":      id,
							"address": server.Address,
						}).Log("Discovering core API address")
						continue
					}

					node := proxy.NewNode(address)
					err = node.Connect()
					if err != nil {
						c.logger.Warn().WithError(err).WithFields(log.Fields{
							"id":      id,
							"address": server.Address,
						}).Log("Connecting to core API")
						continue
					}

					if _, err := c.proxy.AddNode(id, node); err != nil {
						c.logger.Warn().WithError(err).WithFields(log.Fields{
							"id":      id,
							"address": address,
						}).Log("Adding node")
					}

					c.nodes[id] = node
				} else {
					delete(removeNodes, id)
				}
			}

			for id := range removeNodes {
				node, ok := c.nodes[id]
				if !ok {
					continue
				}

				node.Disconnect()
				c.proxy.RemoveNode(id)
				delete(c.nodes, id)
			}

			c.nodesLock.Unlock()
		case <-c.shutdownCh:
			return
		}
	}
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
	return c.store.ProcessList()
}

func (c *cluster) GetProcess(id app.ProcessID) (store.Process, error) {
	return c.store.GetProcess(id)
}

func (c *cluster) AddProcess(origin string, config *app.Config) error {
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

func (c *cluster) SetProcessMetadata(origin string, id app.ProcessID, key string, data interface{}) error {
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
	policyAdapter, err := clusteriam.NewPolicyAdapter(c.store)
	if err != nil {
		return nil, fmt.Errorf("cluster policy adapter: %w", err)
	}

	identityAdapter, err := clusteriam.NewIdentityAdapter(c.store)
	if err != nil {
		return nil, fmt.Errorf("cluster identitry adapter: %w", err)
	}

	iam, err := clusteriam.New(iam.Config{
		PolicyAdapter:   policyAdapter,
		IdentityAdapter: identityAdapter,
		Superuser:       superuser,
		JWTRealm:        jwtRealm,
		JWTSecret:       jwtSecret,
		Logger:          c.logger.WithField("logname", "iam"),
	}, c.store)
	if err != nil {
		return nil, fmt.Errorf("cluster iam: %w", err)
	}

	return iam, nil
}

func (c *cluster) ListIdentities() (time.Time, []iamidentity.User) {
	users := c.store.UserList()

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
	policies := c.store.PolicyList()

	return policies.UpdatedAt, policies.Policies
}

func (c *cluster) ListUserPolicies(name string) (time.Time, []iamaccess.Policy) {
	policies := c.store.PolicyUserList(name)

	return policies.UpdatedAt, policies.Policies
}

func (c *cluster) AddIdentity(origin string, identity iamidentity.User) error {
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

type ClusterServer struct {
	ID      string
	Address string
	Voter   bool
	Leader  bool
}

type ClusterStats struct {
	State       string
	LastContact time.Duration
	NumPeers    uint64
}

type ClusterAbout struct {
	ID                string
	Address           string
	ClusterAPIAddress string
	CoreAPIAddress    string
	Nodes             []ClusterServer
	Stats             ClusterStats
}

func (c *cluster) About() (ClusterAbout, error) {
	about := ClusterAbout{
		ID:      c.id,
		Address: c.Address(),
	}

	if address, err := c.ClusterAPIAddress(""); err == nil {
		about.ClusterAPIAddress = address
	}

	if address, err := c.CoreAPIAddress(""); err == nil {
		about.CoreAPIAddress = address
	}

	stats := c.raft.Stats()

	about.Stats.State = stats.State
	about.Stats.LastContact = stats.LastContact
	about.Stats.NumPeers = stats.NumPeers

	servers, err := c.raft.Servers()
	if err != nil {
		c.logger.Error().WithError(err).Log("Raft configuration")
		return ClusterAbout{}, err
	}

	for _, server := range servers {
		node := ClusterServer{
			ID:      server.ID,
			Address: server.Address,
			Voter:   server.Voter,
			Leader:  server.Leader,
		}

		about.Nodes = append(about.Nodes, node)
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
				"last_contact": stats.LastContact.String(),
				"num_peers":    stats.NumPeers,
			}).Log("Stats")

			if stats.LastContact > 10*time.Second && !isEmergencyLeader {
				c.logger.Warn().Log("Force leadership due to lost contact to leader")
				c.raftEmergencyNotifyCh <- true
				isEmergencyLeader = true
			} else if stats.LastContact <= 10*time.Second && isEmergencyLeader {
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
