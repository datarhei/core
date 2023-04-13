package cluster

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	Stop()
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

	snapshotLogger := NewLogger(c.logger.WithComponent("raft"), hclog.Debug).Named("snapshot")
	snapShotStore, err := raft.NewFileSnapshotStoreWithLogger(filepath.Join(c.path, "snapshots"), 10, snapshotLogger)
	if err != nil {
		return nil, err
	}

	boltdb, err := raftboltdb.New(raftboltdb.Options{
		Path: filepath.Join(c.path, "store.db"),
		BoltOptions: &bbolt.Options{
			Timeout: 5 * time.Second,
		},
	})
	if err != nil {
		return nil, err
	}

	boltdb.Stats()

	raftConfig := raft.DefaultConfig()
	raftConfig.Logger = NewLogger(c.logger.WithComponent("raft"), hclog.Debug)

	raftTransport, err := raft.NewTCPTransportWithConfig("127.0.0.1:8090", nil, &raft.NetworkTransportConfig{
		ServerAddressProvider: nil,
		Logger:                NewLogger(c.logger.WithComponent("raft"), hclog.Debug).Named("transport"),
		Stream:                &raft.TCPStreamLayer{},
		MaxPool:               5,
		Timeout:               5 * time.Second,
	})
	if err != nil {
		boltdb.Close()
		return nil, err
	}

	node, err := raft.NewRaft(raftConfig, fsm, boltdb, boltdb, snapShotStore, raftTransport)
	if err != nil {
		boltdb.Close()
		return nil, err
	}

	node.BootstrapCluster(raft.Configuration{
		Servers: []raft.Server{
			{
				Suffrage: raft.Voter,
				ID:       raft.ServerID(config.Name),
				Address:  raftTransport.LocalAddr(),
			},
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel

	go func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
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
	}(ctx)

	return c, nil
}

func (c *cluster) Stop() {
	c.once.Do(func() {
		c.lock.Lock()
		defer c.lock.Unlock()

		for _, node := range c.nodes {
			node.stop()
		}

		c.nodes = map[string]*node{}

		c.cancel()
	})
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
