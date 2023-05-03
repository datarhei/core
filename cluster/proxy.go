package cluster

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/datarhei/core/v16/log"
	"github.com/datarhei/core/v16/net"
)

type Proxy interface {
	Start()
	Stop()

	AddNode(address string) (string, error)
	RemoveNode(id string) error
	ListNodes() []NodeReader
	GetNode(id string) (NodeReader, error)

	GetURL(path string) (string, error)
	GetFile(path string) (io.ReadCloser, error)
}

type ProxyConfig struct {
	ID   string // ID of the node
	Name string // Name of the node

	IPLimiter net.IPLimiter
	Logger    log.Logger
}

type proxy struct {
	id   string
	name string

	nodes    map[string]*node     // List of known nodes
	idfiles  map[string][]string  // Map from nodeid to list of files
	idupdate map[string]time.Time // Map from nodeid to time of last update
	fileid   map[string]string    // Map from file name to nodeid

	limiter net.IPLimiter

	updates chan NodeState

	lock   sync.RWMutex
	cancel context.CancelFunc

	running bool

	logger log.Logger
}

func NewProxy(config ProxyConfig) (Proxy, error) {
	p := &proxy{
		id:       config.ID,
		name:     config.Name,
		nodes:    map[string]*node{},
		idfiles:  map[string][]string{},
		idupdate: map[string]time.Time{},
		fileid:   map[string]string{},
		limiter:  config.IPLimiter,
		updates:  make(chan NodeState, 64),
		logger:   config.Logger,
	}

	if p.limiter == nil {
		p.limiter = net.NewNullIPLimiter()
	}

	if p.logger == nil {
		p.logger = log.New("")
	}

	return p, nil
}

func (p *proxy) Start() {
	p.lock.Lock()
	defer p.lock.Unlock()

	if p.running {
		return
	}

	p.running = true

	p.logger.Debug().Log("starting proxy")

	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel

	go func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				return
			case state := <-p.updates:
				p.logger.Debug().WithFields(log.Fields{
					"node":  state.ID,
					"state": state.State,
					"files": len(state.Files),
				}).Log("Got update")

				p.lock.Lock()

				// Cleanup
				files := p.idfiles[state.ID]
				for _, file := range files {
					delete(p.fileid, file)
				}
				delete(p.idfiles, state.ID)
				delete(p.idupdate, state.ID)

				if state.State == "connected" {
					// Add files
					for _, file := range state.Files {
						p.fileid[file] = state.ID
					}
					p.idfiles[state.ID] = files
					p.idupdate[state.ID] = state.LastUpdate
				}

				p.lock.Unlock()
			}
		}
	}(ctx)
}

func (p *proxy) Stop() {
	p.lock.Lock()
	defer p.lock.Unlock()

	if !p.running {
		return
	}

	p.running = false

	p.logger.Debug().Log("stopping proxy")

	for _, node := range p.nodes {
		node.stop()
	}

	p.nodes = map[string]*node{}
}

func (p *proxy) AddNode(address string) (string, error) {
	node, err := newNode(address, p.updates)
	if err != nil {
		return "", err
	}

	id := node.ID()

	if id == p.id {
		return "", fmt.Errorf("can't add myself as node or a node with the same ID")
	}

	p.lock.Lock()
	defer p.lock.Unlock()

	if _, ok := p.nodes[id]; ok {
		node.stop()
		return id, nil
	}

	ips := node.IPs()
	for _, ip := range ips {
		p.limiter.AddBlock(ip)
	}

	p.nodes[id] = node

	p.logger.Info().WithFields(log.Fields{
		"address": address,
		"id":      id,
	}).Log("Added node")

	return id, nil
}

func (p *proxy) RemoveNode(id string) error {
	p.lock.Lock()
	defer p.lock.Unlock()

	node, ok := p.nodes[id]
	if !ok {
		return ErrNodeNotFound
	}

	node.stop()

	delete(p.nodes, id)

	ips := node.IPs()

	for _, ip := range ips {
		p.limiter.RemoveBlock(ip)
	}

	p.logger.Info().WithFields(log.Fields{
		"id": id,
	}).Log("Removed node")

	return nil
}

func (p *proxy) ListNodes() []NodeReader {
	list := []NodeReader{}

	p.lock.RLock()
	defer p.lock.RUnlock()

	for _, node := range p.nodes {
		list = append(list, node)
	}

	return list
}

func (p *proxy) GetNode(id string) (NodeReader, error) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	node, ok := p.nodes[id]
	if !ok {
		return nil, fmt.Errorf("node not found")
	}

	return node, nil
}

func (c *proxy) GetURL(path string) (string, error) {
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

func (p *proxy) GetFile(path string) (io.ReadCloser, error) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	id, ok := p.fileid[path]
	if !ok {
		p.logger.Debug().WithField("path", path).Log("Not found")
		return nil, fmt.Errorf("file not found")
	}

	ts, ok := p.idupdate[id]
	if !ok {
		p.logger.Debug().WithField("path", path).Log("No age information found")
		return nil, fmt.Errorf("file not found")
	}

	if time.Since(ts) > 2*time.Second {
		p.logger.Debug().WithField("path", path).Log("File too old")
		return nil, fmt.Errorf("file not found")
	}

	node, ok := p.nodes[id]
	if !ok {
		p.logger.Debug().WithField("path", path).Log("Unknown node")
		return nil, fmt.Errorf("file not found")
	}

	data, err := node.getFile(path)
	if err != nil {
		p.logger.Debug().WithField("path", path).Log("Invalid path")
		return nil, fmt.Errorf("file not found")
	}

	p.logger.Debug().WithField("path", path).Log("File cluster path")

	return data, nil
}
