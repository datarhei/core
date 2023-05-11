package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/datarhei/core/v16/log"
	"github.com/datarhei/core/v16/net"
	"github.com/datarhei/core/v16/restream/app"
)

type Proxy interface {
	Start()
	Stop()

	AddNode(id string, node Node) (string, error)
	RemoveNode(id string) error

	ProxyReader
	Reader() ProxyReader

	ProxyProcessor
	Processor() ProxyProcessor
}

type ProxyProcessor interface {
	Resources() map[string]NodeResources

	ProcessList() []ProcessConfig
	ProcessAdd(nodeid string, config *app.Config) error
	ProcessDelete(nodeid string, id string) error
	ProcessStart(nodeid string, id string) error
}

type proxyProcessor struct {
	proxy *proxy
}

func (p *proxyProcessor) Resources() map[string]NodeResources {
	if p.proxy == nil {
		return nil
	}

	return p.proxy.Resources()
}

func (p *proxyProcessor) ProcessList() []ProcessConfig {
	if p.proxy == nil {
		return nil
	}

	return p.proxy.ProcessList()
}

func (p *proxyProcessor) ProcessAdd(nodeid string, config *app.Config) error {
	if p.proxy == nil {
		return fmt.Errorf("no proxy provided")
	}

	return p.proxy.ProcessAdd(nodeid, config)
}

func (p *proxyProcessor) ProcessDelete(nodeid string, id string) error {
	if p.proxy == nil {
		return fmt.Errorf("no proxy provided")
	}

	return p.proxy.ProcessDelete(nodeid, id)
}

func (p *proxyProcessor) ProcessStart(nodeid string, id string) error {
	if p.proxy == nil {
		return fmt.Errorf("no proxy provided")
	}

	return p.proxy.ProcessStart(nodeid, id)
}

type ProxyReader interface {
	ListNodes() []NodeReader
	GetNode(id string) (NodeReader, error)

	GetURL(path string) (string, error)
	GetFile(path string) (io.ReadCloser, error)
}

func NewNullProxyReader() ProxyReader {
	return &proxyReader{}
}

type proxyReader struct {
	proxy *proxy
}

func (p *proxyReader) ListNodes() []NodeReader {
	if p.proxy == nil {
		return nil
	}

	return p.proxy.ListNodes()
}

func (p *proxyReader) GetNode(id string) (NodeReader, error) {
	if p.proxy == nil {
		return nil, fmt.Errorf("no proxy provided")
	}

	return p.proxy.GetNode(id)
}

func (p *proxyReader) GetURL(path string) (string, error) {
	if p.proxy == nil {
		return "", fmt.Errorf("no proxy provided")
	}

	return p.proxy.GetURL(path)
}

func (p *proxyReader) GetFile(path string) (io.ReadCloser, error) {
	if p.proxy == nil {
		return nil, fmt.Errorf("no proxy provided")
	}

	return p.proxy.GetFile(path)
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

	nodes    map[string]Node      // List of known nodes
	idfiles  map[string][]string  // Map from nodeid to list of files
	idupdate map[string]time.Time // Map from nodeid to time of last update
	fileid   map[string]string    // Map from file name to nodeid

	limiter net.IPLimiter

	updates chan NodeFiles

	lock   sync.RWMutex
	cancel context.CancelFunc

	running bool

	logger log.Logger
}

var ErrNodeNotFound = errors.New("node not found")

func NewProxy(config ProxyConfig) (Proxy, error) {
	p := &proxy{
		id:       config.ID,
		name:     config.Name,
		nodes:    map[string]Node{},
		idfiles:  map[string][]string{},
		idupdate: map[string]time.Time{},
		fileid:   map[string]string{},
		limiter:  config.IPLimiter,
		updates:  make(chan NodeFiles, 64),
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

	p.logger.Debug().Log("Starting proxy")

	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel

	go func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				return
			case update := <-p.updates:
				p.logger.Debug().WithFields(log.Fields{
					"node":  update.ID,
					"files": len(update.Files),
				}).Log("Got update")

				if p.id == update.ID {
					continue
				}

				p.lock.Lock()

				// Cleanup
				files := p.idfiles[update.ID]
				for _, file := range files {
					delete(p.fileid, file)
				}
				delete(p.idfiles, update.ID)
				delete(p.idupdate, update.ID)

				// Add files
				for _, file := range update.Files {
					p.fileid[file] = update.ID
				}
				p.idfiles[update.ID] = files
				p.idupdate[update.ID] = update.LastUpdate

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

	p.logger.Debug().Log("Stopping proxy")

	p.cancel()
	p.cancel = nil

	for _, node := range p.nodes {
		node.StopFiles()
	}

	p.nodes = map[string]Node{}
}

func (p *proxy) Reader() ProxyReader {
	return &proxyReader{
		proxy: p,
	}
}

func (p *proxy) Processor() ProxyProcessor {
	return &proxyProcessor{
		proxy: p,
	}
}

func (p *proxy) Resources() map[string]NodeResources {
	resources := map[string]NodeResources{}

	p.lock.RLock()
	defer p.lock.RUnlock()

	for _, node := range p.nodes {
		resources[node.ID()] = node.State().Resources
	}

	return resources
}

func (p *proxy) AddNode(id string, node Node) (string, error) {
	if id != node.ID() {
		return "", fmt.Errorf("the provided (%s) and retrieved (%s) ID's don't match", id, node.ID())
	}

	p.lock.Lock()
	defer p.lock.Unlock()

	if n, ok := p.nodes[id]; ok {
		n.StopFiles()

		delete(p.nodes, id)

		ips := node.IPs()

		for _, ip := range ips {
			p.limiter.RemoveBlock(ip)
		}

		return id, nil
	}

	ips := node.IPs()
	for _, ip := range ips {
		p.limiter.AddBlock(ip)
	}

	p.nodes[id] = node

	node.StartFiles(p.updates)

	p.logger.Info().WithFields(log.Fields{
		"address": node.Address(),
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

	node.StopFiles()

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

	url, err := node.GetURL(path)
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

	data, err := node.GetFile(path)
	if err != nil {
		p.logger.Debug().WithField("path", path).Log("Invalid path")
		return nil, fmt.Errorf("file not found")
	}

	p.logger.Debug().WithField("path", path).Log("File cluster path")

	return data, nil
}

type ProcessConfig struct {
	NodeID  string
	Order   string
	State   string
	CPU     float64 // Current CPU load of this process, 0-100*ncpu
	Mem     uint64  // Currently consumed memory of this process in bytes
	Runtime time.Duration
	Config  *app.Config
}

func (p *proxy) ProcessList() []ProcessConfig {
	processChan := make(chan ProcessConfig, 64)
	processList := []ProcessConfig{}

	wgList := sync.WaitGroup{}
	wgList.Add(1)

	go func() {
		defer wgList.Done()

		for file := range processChan {
			processList = append(processList, file)
		}
	}()

	wg := sync.WaitGroup{}

	p.lock.RLock()
	for _, node := range p.nodes {
		wg.Add(1)

		go func(node Node, p chan<- ProcessConfig) {
			defer wg.Done()

			processes, err := node.ProcessList()
			if err != nil {
				return
			}

			for _, process := range processes {
				p <- process
			}
		}(node, processChan)
	}
	p.lock.RUnlock()

	wg.Wait()

	close(processChan)

	wgList.Wait()

	return processList
}

func (p *proxy) ProcessAdd(nodeid string, config *app.Config) error {
	p.lock.RLock()
	defer p.lock.RUnlock()

	node, ok := p.nodes[nodeid]
	if !ok {
		return fmt.Errorf("node not found")
	}

	err := node.ProcessAdd(config)
	if err != nil {
		return err
	}

	err = node.ProcessStart(config.ID)
	if err != nil {
		return err
	}

	return nil
}

func (p *proxy) ProcessDelete(nodeid string, id string) error {
	p.lock.RLock()
	defer p.lock.RUnlock()

	node, ok := p.nodes[nodeid]
	if !ok {
		return fmt.Errorf("node not found")
	}

	err := node.ProcessDelete(id)
	if err != nil {
		return err
	}

	return nil
}

func (p *proxy) ProcessStart(nodeid string, id string) error {
	p.lock.RLock()
	defer p.lock.RUnlock()

	node, ok := p.nodes[nodeid]
	if !ok {
		return fmt.Errorf("node not found")
	}

	err := node.ProcessStart(id)
	if err != nil {
		return err
	}

	return nil
}
