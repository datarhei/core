package cluster

import (
	"context"
	"errors"
	"fmt"
	"io"
	gonet "net"
	"path/filepath"
	"sync"
	"time"

	"github.com/datarhei/core/v16/log"
	"github.com/datarhei/core/v16/net"
	hclog "github.com/hashicorp/go-hclog"
	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb/v2"
	"go.etcd.io/bbolt"
)

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

	fsm, err := NewFSM()
	if err != nil {
		return nil, err
	}

	c.startRaft(fsm, true, false)

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

	transport, err := raft.NewTCPTransportWithLogger(c.raftAddress, addr, 3, 10*time.Second, NewLogger(c.logger.WithComponent("raft"), hclog.Debug).Named("transport"))
	if err != nil {
		return err
	}

	snapshotLogger := NewLogger(c.logger.WithComponent("raft"), hclog.Debug).Named("snapshot")
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
	cfg.Logger = NewLogger(c.logger.WithComponent("raft"), hclog.Debug)

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

	go c.monitorLeadership()

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
