package cluster

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/datarhei/core/v16/log"
)

type Cluster interface {
	AddNode(address, username, password string) (string, error)
	RemoveNode(id string) error
	ListNodes() []NodeReader
	GetNode(id string) (NodeReader, error)
	Stop()
	GetFile(path string) (string, error)
}

type ClusterConfig struct {
	Logger log.Logger
}

type cluster struct {
	nodes    map[string]*node
	idfiles  map[string][]string
	idupdate map[string]time.Time
	fileid   map[string]string

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
		updates:  make(chan NodeState, 64),
		logger:   config.Logger,
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
				}).Log("got update")

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
		return nil, fmt.Errorf("node not found")
	}

	return node, nil
}

func (c *cluster) GetFile(path string) (string, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	c.logger.Debug().WithField("path", path).Log("opening")

	id, ok := c.fileid[path]
	if !ok {
		c.logger.Debug().WithField("path", path).Log("not found")
		return "", fmt.Errorf("file not found")
	}

	ts, ok := c.idupdate[id]
	if !ok {
		c.logger.Debug().WithField("path", path).Log("no age information found")
		return "", fmt.Errorf("file not found")
	}

	if time.Since(ts) > 2*time.Second {
		c.logger.Debug().WithField("path", path).Log("file too old")
		return "", fmt.Errorf("file not found")
	}

	node, ok := c.nodes[id]
	if !ok {
		c.logger.Debug().WithField("path", path).Log("unknown node")
		return "", fmt.Errorf("file not found")
	}

	url := node.Address() + "/" + filepath.Join("memfs", path)

	c.logger.Debug().WithField("url", url).Log("file cluster url")

	return url, nil
}
