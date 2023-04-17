package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	gonet "net"
	"path/filepath"
	"reflect"
	"sync"
	"time"

	"github.com/datarhei/core/v16/log"
	"github.com/datarhei/core/v16/net"
	hclog "github.com/hashicorp/go-hclog"
	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb/v2"
	"go.etcd.io/bbolt"
)

/*
	/api/v3:
		GET /cluster/db/node - list all nodes that are stored in the FSM - Cluster.Store.ListNodes()
		POST /cluster/db/node - add a node to the FSM - Cluster.Store.AddNode()
		DELETE /cluster/db/node/:id - remove a node from the FSM - Cluster.Store.RemoveNode()

		GET /cluster/db/process - list all process configs that are stored in the FSM - Cluster.Store.ListProcesses()
		POST /cluster/db/process - add a process config to the FSM - Cluster.Store.AddProcess()
		PUT /cluster/db/process/:id - update a process config in the FSM - Cluster.Store.UpdateProcess()
		DELETE /cluster/db/process/:id - remove a process config from the FSM - Cluster.Store.RemoveProcess()

		** for the processes, the leader will decide where to actually run them. the process configs will
		also be added to the regular process DB of each core.

		POST /cluster/join - join the cluster - Cluster.Join()
		DELETE /cluster/:id - leave the cluster - Cluster.Leave()

		** all these endpoints will forward the request to the leader.
*/

var ErrNodeNotFound = errors.New("node not found")

type ClusterReader interface {
	GetURL(path string) (string, error)
	GetFile(path string) (io.ReadCloser, error)
}

type dummyClusterReader struct{}

func NewDummyClusterReader() ClusterReader {
	return &dummyClusterReader{}
}

func (r *dummyClusterReader) GetURL(path string) (string, error) {
	return "", fmt.Errorf("not implemented in dummy cluster")
}

func (r *dummyClusterReader) GetFile(path string) (io.ReadCloser, error) {
	return nil, fmt.Errorf("not implemented in dummy cluster")
}

type Cluster interface {
	AddNode(address, username, password string) (string, error)
	RemoveNode(id string) error
	ListNodes() []NodeReader
	GetNode(id string) (NodeReader, error)
	Leave() error // gracefully leave the cluster
	Shutdown() error
	ClusterReader
}

type ClusterConfig struct {
	ID        string
	Name      string
	Path      string
	IPLimiter net.IPLimiter
	Logger    log.Logger
	Bootstrap bool
	Address   string
}

type cluster struct {
	id   string
	name string
	path string

	nodes    map[string]*node     // List of known nodes
	idfiles  map[string][]string  // Map from nodeid to list of files
	idupdate map[string]time.Time // Map from nodeid to time of last update
	fileid   map[string]string    // Map from file name to nodeid

	limiter net.IPLimiter

	updates chan NodeState

	lock   sync.RWMutex
	cancel context.CancelFunc
	once   sync.Once

	logger log.Logger

	raft                  *raft.Raft
	raftTransport         *raft.NetworkTransport
	raftAddress           string
	raftNotifyCh          <-chan bool
	raftStore             *raftboltdb.BoltStore
	raftRemoveGracePeriod time.Duration

	store Store

	reassertLeaderCh chan chan error

	leaveCh chan struct{}

	shutdown     bool
	shutdownCh   chan struct{}
	shutdownLock sync.Mutex
}

func New(config ClusterConfig) (Cluster, error) {
	c := &cluster{
		id:       config.ID,
		name:     config.Name,
		path:     config.Path,
		nodes:    map[string]*node{},
		idfiles:  map[string][]string{},
		idupdate: map[string]time.Time{},
		fileid:   map[string]string{},
		limiter:  config.IPLimiter,
		updates:  make(chan NodeState, 64),
		logger:   config.Logger,

		raftAddress: config.Address,

		reassertLeaderCh: make(chan chan error),
		leaveCh:          make(chan struct{}),
		shutdownCh:       make(chan struct{}),
	}

	if c.limiter == nil {
		c.limiter = net.NewNullIPLimiter()
	}

	if c.logger == nil {
		c.logger = log.New("")
	}

	store, err := NewStore()
	if err != nil {
		return nil, err
	}

	c.store = store

	c.logger.Debug().Log("starting raft")

	err = c.startRaft(store, config.Bootstrap, false)
	if err != nil {
		return nil, err
	}

	go func() {
		for {
			select {
			case <-c.shutdownCh:
				return
			case state := <-c.updates:
				c.logger.Debug().WithFields(log.Fields{
					"node":  state.ID,
					"state": state.State,
					"files": len(state.Files),
				}).Log("Got update")

				c.lock.Lock()

				// Cleanup
				files := c.idfiles[state.ID]
				for _, file := range files {
					delete(c.fileid, file)
				}
				delete(c.idfiles, state.ID)
				delete(c.idupdate, state.ID)

				if state.State == "connected" {
					// Add files
					for _, file := range state.Files {
						c.fileid[file] = state.ID
					}
					c.idfiles[state.ID] = files
					c.idupdate[state.ID] = state.LastUpdate
				}

				c.lock.Unlock()
			}
		}
	}()

	return c, nil
}

func (c *cluster) Shutdown() error {
	c.logger.Info().Log("shutting down cluster")
	c.shutdownLock.Lock()
	defer c.shutdownLock.Unlock()

	if c.shutdown {
		return nil
	}

	c.shutdown = true
	close(c.shutdownCh)

	c.lock.Lock()
	defer c.lock.Unlock()

	for _, node := range c.nodes {
		node.stop()
	}

	c.nodes = map[string]*node{}

	c.shutdownRaft()

	return nil
}

// https://github.com/hashicorp/consul/blob/44b39240a86bc94ddc67bc105286ab450bd869a9/agent/consul/server.go#L1369
func (c *cluster) Leave() error {
	addr := c.raftTransport.LocalAddr()

	// Get the latest configuration.
	future := c.raft.GetConfiguration()
	if err := future.Error(); err != nil {
		c.logger.Error().WithError(err).Log("failed to get raft configuration")
		return err
	}

	numPeers := len(future.Configuration().Servers)

	// If we are the current leader, and we have any other peers (cluster has multiple
	// servers), we should do a RemoveServer/RemovePeer to safely reduce the quorum size.
	// If we are not the leader, then we should issue our leave intention and wait to be
	// removed for some reasonable period of time.
	isLeader := c.IsLeader()
	if isLeader && numPeers > 1 {
		if err := c.leadershipTransfer(); err == nil {
			isLeader = false
		} else {
			c.StoreRemoveNode(c.id)
			future := c.raft.RemoveServer(raft.ServerID(c.id), 0, 0)
			if err := future.Error(); err != nil {
				c.logger.Error().WithError(err).Log("failed to remove ourself as raft peer")
			}
		}
	}

	// If we were not leader, wait to be safely removed from the cluster. We
	// must wait to allow the raft replication to take place, otherwise an
	// immediate shutdown could cause a loss of quorum.
	if !isLeader {
		// Send leave-request to leader
		// DELETE leader//api/v3/cluster/node/:id

		left := false
		limit := time.Now().Add(c.raftRemoveGracePeriod)
		for !left && time.Now().Before(limit) {
			// Sleep a while before we check.
			time.Sleep(50 * time.Millisecond)

			// Get the latest configuration.
			future := c.raft.GetConfiguration()
			if err := future.Error(); err != nil {
				c.logger.Error().WithError(err).Log("failed to get raft configuration")
				break
			}

			// See if we are no longer included.
			left = true
			for _, server := range future.Configuration().Servers {
				if server.Address == addr {
					left = false
					break
				}
			}
		}

		if !left {
			c.logger.Warn().Log("failed to leave raft configuration gracefully, timeout")
		}
	}

	return nil
}

func (c *cluster) IsLeader() bool {
	return c.raft.State() == raft.Leader
}

func (c *cluster) leave(id string) error {
	if !c.IsLeader() {
		return fmt.Errorf("not leader")
	}

	c.logger.Debug().WithFields(log.Fields{
		"nodeid": id,
	}).Log("received leave request for remote node")

	if id == c.id {
		err := c.leadershipTransfer()
		if err != nil {
			c.logger.Warn().WithError(err).Log("failed to transfer leadership")
		}
	}

	future := c.raft.RemoveServer(raft.ServerID(id), 0, 0)
	if err := future.Error(); err != nil {
		c.logger.Error().WithError(err).WithFields(log.Fields{
			"nodeid": id,
		}).Log("failed to remove node")
	}

	return nil
}

func (c *cluster) Join(id, raftAddress, apiAddress, username, password string) error {
	if !c.IsLeader() {
		return fmt.Errorf("not leader")
	}

	c.logger.Debug().WithFields(log.Fields{
		"nodeid":  id,
		"address": raftAddress,
	}).Log("received join request for remote node")

	configFuture := c.raft.GetConfiguration()
	if err := configFuture.Error(); err != nil {
		c.logger.Error().WithError(err).Log("failed to get raft configuration")
		return err
	}

	for _, srv := range configFuture.Configuration().Servers {
		// If a node already exists with either the joining node's ID or address,
		// that node may need to be removed from the config first.
		if srv.ID == raft.ServerID(id) || srv.Address == raft.ServerAddress(raftAddress) {
			// However if *both* the ID and the address are the same, then nothing -- not even
			// a join operation -- is needed.
			if srv.ID == raft.ServerID(id) && srv.Address == raft.ServerAddress(raftAddress) {
				c.logger.Debug().WithFields(log.Fields{
					"nodeid":  id,
					"address": raftAddress,
				}).Log("node is already member of cluster, ignoring join request")
				return nil
			}

			future := c.raft.RemoveServer(srv.ID, 0, 0)
			if err := future.Error(); err != nil {
				c.logger.Error().WithError(err).WithFields(log.Fields{
					"nodeid":  id,
					"address": raftAddress,
				}).Log("error removing existing node")
				return fmt.Errorf("error removing existing node %s at %s: %w", id, raftAddress, err)
			}
		}
	}

	f := c.raft.AddVoter(raft.ServerID(id), raft.ServerAddress(raftAddress), 0, 5*time.Second)
	if err := f.Error(); err != nil {
		return err
	}

	if err := c.StoreAddNode(id, apiAddress, username, password); err != nil {
		future := c.raft.RemoveServer(raft.ServerID(id), 0, 0)
		if err := future.Error(); err != nil {
			c.logger.Error().WithError(err).WithFields(log.Fields{
				"nodeid":  id,
				"address": raftAddress,
			}).Log("error removing existing node")
			return err
		}
		return err
	}

	c.logger.Info().WithFields(log.Fields{
		"nodeid":  id,
		"address": raftAddress,
	}).Log("node joined successfully")

	return nil
}

type command struct {
	Operation string
	Data      interface{}
}

type addNodeCommand struct {
	ID       string
	Address  string
	Username string
	Password string
}

func (c *cluster) StoreListNodes() []addNodeCommand {
	c.store.ListNodes()

	return nil
}

func (c *cluster) StoreAddNode(id, address, username, password string) error {
	if !c.IsLeader() {
		return fmt.Errorf("not leader")
	}

	com := &command{
		Operation: "addNode",
		Data: &addNodeCommand{
			ID:       id,
			Address:  address,
			Username: username,
			Password: password,
		},
	}

	b, err := json.Marshal(com)
	if err != nil {
		return err
	}

	future := c.raft.Apply(b, 5*time.Second)
	if err := future.Error(); err != nil {
		return fmt.Errorf("applying command failed: %w", err)
	}

	return nil
}

type removeNodeCommand struct {
	ID string
}

func (c *cluster) StoreRemoveNode(id string) error {
	if !c.IsLeader() {
		return fmt.Errorf("not leader")
	}

	com := &command{
		Operation: "removeNode",
		Data: &removeNodeCommand{
			ID: id,
		},
	}

	b, err := json.Marshal(com)
	if err != nil {
		return err
	}

	future := c.raft.Apply(b, 5*time.Second)
	if err := future.Error(); err != nil {
		return fmt.Errorf("applying command failed: %w", err)
	}

	return nil
}

// trackLeaderChanges registers an Observer with raft in order to receive updates
// about leader changes, in order to keep the forwarder up to date.
func (c *cluster) trackLeaderChanges() {
	obsCh := make(chan raft.Observation, 16)
	observer := raft.NewObserver(obsCh, false, func(o *raft.Observation) bool {
		_, leaderOK := o.Data.(raft.LeaderObservation)
		_, peerOK := o.Data.(raft.PeerObservation)

		return leaderOK || peerOK
	})
	c.raft.RegisterObserver(observer)

	for {
		select {
		case obs := <-obsCh:
			if leaderObs, ok := obs.Data.(raft.LeaderObservation); ok {
				// TODO: update the forwarder
				c.logger.Debug().WithFields(log.Fields{
					"id":      leaderObs.LeaderID,
					"address": leaderObs.LeaderAddr,
				}).Log("new leader observation")
			} else if peerObs, ok := obs.Data.(raft.PeerObservation); ok {
				c.logger.Debug().WithFields(log.Fields{
					"removed": peerObs.Removed,
					"address": peerObs.Peer.Address,
				}).Log("new peer observation")
			} else {
				c.logger.Debug().WithField("type", reflect.TypeOf(obs.Data)).Log("got unknown observation type from raft")
				continue
			}
		case <-c.shutdownCh:
			c.raft.DeregisterObserver(observer)
			return
		}
	}
}

func (c *cluster) AddNode(address, username, password string) (string, error) {
	node, err := newNode(address, username, password, c.updates)
	if err != nil {
		return "", err
	}

	id := node.ID()

	if id == c.id {
		return "", fmt.Errorf("can't add myself as node or a node with the same ID")
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	if _, ok := c.nodes[id]; ok {
		node.stop()
		return id, nil
	}

	ips := node.IPs()
	for _, ip := range ips {
		c.limiter.AddBlock(ip)
	}

	c.nodes[id] = node

	c.logger.Info().WithFields(log.Fields{
		"address": address,
		"id":      id,
	}).Log("Added node")

	return id, nil
}

func (c *cluster) RemoveNode(id string) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	node, ok := c.nodes[id]
	if !ok {
		return ErrNodeNotFound
	}

	node.stop()

	delete(c.nodes, id)

	ips := node.IPs()

	for _, ip := range ips {
		c.limiter.RemoveBlock(ip)
	}

	c.leave(id)

	c.logger.Info().WithFields(log.Fields{
		"id": id,
	}).Log("Removed node")

	return nil
}

func (c *cluster) ListNodes() []NodeReader {
	list := []NodeReader{}

	c.lock.RLock()
	defer c.lock.RUnlock()

	for _, node := range c.nodes {
		list = append(list, node)
	}

	return list
}

func (c *cluster) GetNode(id string) (NodeReader, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	node, ok := c.nodes[id]
	if !ok {
		return nil, fmt.Errorf("node not found")
	}

	return node, nil
}

func (c *cluster) GetURL(path string) (string, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	id, ok := c.fileid[path]
	if !ok {
		c.logger.Debug().WithField("path", path).Log("Not found")
		return "", fmt.Errorf("file not found")
	}

	ts, ok := c.idupdate[id]
	if !ok {
		c.logger.Debug().WithField("path", path).Log("No age information found")
		return "", fmt.Errorf("file not found")
	}

	if time.Since(ts) > 2*time.Second {
		c.logger.Debug().WithField("path", path).Log("File too old")
		return "", fmt.Errorf("file not found")
	}

	node, ok := c.nodes[id]
	if !ok {
		c.logger.Debug().WithField("path", path).Log("Unknown node")
		return "", fmt.Errorf("file not found")
	}

	url, err := node.getURL(path)
	if err != nil {
		c.logger.Debug().WithField("path", path).Log("Invalid path")
		return "", fmt.Errorf("file not found")
	}

	c.logger.Debug().WithField("url", url).Log("File cluster url")

	return url, nil
}

func (c *cluster) GetFile(path string) (io.ReadCloser, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	id, ok := c.fileid[path]
	if !ok {
		c.logger.Debug().WithField("path", path).Log("Not found")
		return nil, fmt.Errorf("file not found")
	}

	ts, ok := c.idupdate[id]
	if !ok {
		c.logger.Debug().WithField("path", path).Log("No age information found")
		return nil, fmt.Errorf("file not found")
	}

	if time.Since(ts) > 2*time.Second {
		c.logger.Debug().WithField("path", path).Log("File too old")
		return nil, fmt.Errorf("file not found")
	}

	node, ok := c.nodes[id]
	if !ok {
		c.logger.Debug().WithField("path", path).Log("Unknown node")
		return nil, fmt.Errorf("file not found")
	}

	data, err := node.getFile(path)
	if err != nil {
		c.logger.Debug().WithField("path", path).Log("Invalid path")
		return nil, fmt.Errorf("file not found")
	}

	c.logger.Debug().WithField("path", path).Log("File cluster path")

	return data, nil
}

func (c *cluster) startRaft(fsm raft.FSM, bootstrap, inmem bool) error {
	defer func() {
		if c.raft == nil && c.raftStore != nil {
			c.raftStore.Close()
		}
	}()

	c.raftRemoveGracePeriod = 5 * time.Second

	addr, err := gonet.ResolveTCPAddr("tcp", c.raftAddress)
	if err != nil {
		return err
	}

	c.logger.Debug().Log("address: %s", addr)

	transport, err := raft.NewTCPTransportWithLogger(c.raftAddress, addr, 3, 10*time.Second, NewLogger(c.logger, hclog.Debug).Named("raft-transport"))
	if err != nil {
		return err
	}

	c.raftTransport = transport

	snapshotLogger := NewLogger(c.logger, hclog.Debug).Named("raft-snapshot")
	snapshots, err := raft.NewFileSnapshotStoreWithLogger(filepath.Join(c.path, "snapshots"), 10, snapshotLogger)
	if err != nil {
		return err
	}

	var logStore raft.LogStore
	var stableStore raft.StableStore
	if inmem {
		logStore = raft.NewInmemStore()
		stableStore = raft.NewInmemStore()
	} else {
		bolt, err := raftboltdb.New(raftboltdb.Options{
			Path: filepath.Join(c.path, "raftlog.db"),
			BoltOptions: &bbolt.Options{
				Timeout: 5 * time.Second,
			},
		})
		if err != nil {
			return fmt.Errorf("bolt: %w", err)
		}
		logStore = bolt
		stableStore = bolt

		cacheStore, err := raft.NewLogCache(512, logStore)
		if err != nil {
			return err
		}
		logStore = cacheStore

		c.raftStore = bolt
	}

	cfg := raft.DefaultConfig()
	cfg.LocalID = raft.ServerID(c.id)
	cfg.Logger = NewLogger(c.logger, hclog.Debug).Named("raft")

	if bootstrap {
		hasState, err := raft.HasExistingState(logStore, stableStore, snapshots)
		if err != nil {
			return err
		}
		if !hasState {
			configuration := raft.Configuration{
				Servers: []raft.Server{
					{
						Suffrage: raft.Voter,
						ID:       raft.ServerID(c.id),
						Address:  transport.LocalAddr(),
					},
				},
			}

			if err := raft.BootstrapCluster(cfg,
				logStore, stableStore, snapshots, transport, configuration); err != nil {
				return err
			}
		}
	}

	// Set up a channel for reliable leader notifications.
	raftNotifyCh := make(chan bool, 10)
	cfg.NotifyCh = raftNotifyCh
	c.raftNotifyCh = raftNotifyCh

	node, err := raft.NewRaft(cfg, fsm, logStore, stableStore, snapshots, transport)
	if err != nil {
		return err
	}

	c.raft = node

	go c.trackLeaderChanges()
	go c.monitorLeadership()

	c.logger.Debug().Log("raft started")

	return nil
}

func (c *cluster) shutdownRaft() {
	if c.raft != nil {
		c.raftTransport.Close()
		future := c.raft.Shutdown()
		if err := future.Error(); err != nil {
			c.logger.Warn().WithError(err).Log("error shutting down raft")
		}
		if c.raftStore != nil {
			c.raftStore.Close()
		}
	}
}

// nodeLoop is run by every node in the cluster. This is mainly to check the list
// of nodes from the FSM, in order to connect to them and to fetch their file lists.
func (c *cluster) nodeLoop() {}
