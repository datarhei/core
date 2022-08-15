package cluster

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/datarhei/core/v16/log"
	"github.com/datarhei/core/v16/net"
)

type ClusterReader interface {
	GetURL(path string) (string, error)
}

type dummyClusterReader struct{}

func NewDummyClusterReader() ClusterReader {
	return &dummyClusterReader{}
}

func (r *dummyClusterReader) GetURL(path string) (string, error) {
	return "", fmt.Errorf("not implemented in dummy cluster")
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
	IPLimiter net.IPLimiter
	Logger    log.Logger
}

type cluster struct {
	nodes    map[string]*node
	idfiles  map[string][]string
	idupdate map[string]time.Time
	fileid   map[string]string

	limiter net.IPLimiter

	updates chan NodeState

	lock   sync.RWMutex
	cancel context.CancelFunc
	once   sync.Once

	logger log.Logger
}

func New(config ClusterConfig) (Cluster, error) {
	c := &cluster{
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
		return nil
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

	url, err := node.GetURL(path)
	if err != nil {
		c.logger.Debug().WithField("path", path).Log("Invalid path")
		return "", fmt.Errorf("file not found")
	}

	c.logger.Debug().WithField("url", url).Log("File cluster url")

	return url, nil
}
