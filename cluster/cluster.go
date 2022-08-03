package cluster

import (
	"context"
	"fmt"
	"sync"

	"github.com/datarhei/core/v16/log"
)

type Cluster interface {
	AddNode(address, username, password string) (string, error)
	RemoveNode(id string) error
	ListNodes() []NodeReader
	GetNode(id string) (NodeReader, error)
	Stop()
}

type ClusterConfig struct {
	Logger log.Logger
}

type cluster struct {
	nodes   map[string]*node
	idfiles map[string][]string
	fileid  map[string]string

	updates chan NodeState

	lock   sync.RWMutex
	cancel context.CancelFunc
	once   sync.Once

	logger log.Logger
}

func New(config ClusterConfig) (Cluster, error) {
	c := &cluster{
		nodes:   map[string]*node{},
		idfiles: map[string][]string{},
		fileid:  map[string]string{},
		updates: make(chan NodeState, 64),
		logger:  config.Logger,
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
				c.logger.Info().WithField("node", state.ID).WithField("state", state.State).Log("got news from node")

				c.lock.Lock()

				// Cleanup
				files := c.idfiles[state.ID]
				for _, file := range files {
					delete(c.fileid, file)
				}
				delete(c.idfiles, state.ID)

				if state.State == "connected" {
					// Add files
					for _, file := range state.Files {
						c.fileid[file] = state.ID
					}
					c.idfiles[state.ID] = files
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
		return id, nil
	}

	c.nodes[id] = node

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
		return nil, fmt.Errorf("no such node")
	}

	return node, nil
}
