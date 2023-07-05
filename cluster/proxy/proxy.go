package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"sync"
	"time"

	"github.com/datarhei/core/v16/log"
	"github.com/datarhei/core/v16/restream/app"

	clientapi "github.com/datarhei/core-client-go/v16/api"
)

type Proxy interface {
	Start()
	Stop()

	AddNode(id string, node Node) (string, error)
	RemoveNode(id string) error

	ProxyReader
	Reader() ProxyReader

	AddProcess(nodeid string, config *app.Config, metadata map[string]interface{}) error
	DeleteProcess(nodeid string, id app.ProcessID) error
	UpdateProcess(nodeid string, id app.ProcessID, config *app.Config, metadata map[string]interface{}) error
	CommandProcess(nodeid string, id app.ProcessID, command string) error
}

type ProxyReader interface {
	ListNodes() []NodeReader
	GetNode(id string) (NodeReader, error)

	FindNodeFromProcess(id app.ProcessID) (string, error)

	Resources() map[string]NodeResources
	ListProcesses(ProcessListOptions) []clientapi.Process
	ListProxyProcesses() []Process
	ProbeProcess(nodeid string, id app.ProcessID) (clientapi.Probe, error)

	GetURL(prefix, path string) (*url.URL, error)
	GetFile(prefix, path string, offset int64) (io.ReadCloser, error)
	GetFileInfo(prefix, path string) (int64, time.Time, error)
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

func (p *proxyReader) FindNodeFromProcess(id app.ProcessID) (string, error) {
	if p.proxy == nil {
		return "", fmt.Errorf("no proxy provided")
	}

	return p.proxy.FindNodeFromProcess(id)
}

func (p *proxyReader) Resources() map[string]NodeResources {
	if p.proxy == nil {
		return nil
	}

	return p.proxy.Resources()
}

func (p *proxyReader) ListProcesses(options ProcessListOptions) []clientapi.Process {
	if p.proxy == nil {
		return nil
	}

	return p.proxy.ListProcesses(options)
}

func (p *proxyReader) ListProxyProcesses() []Process {
	if p.proxy == nil {
		return nil
	}

	return p.proxy.ListProxyProcesses()
}

func (p *proxyReader) ProbeProcess(nodeid string, id app.ProcessID) (clientapi.Probe, error) {
	if p.proxy == nil {
		return clientapi.Probe{
			Log: []string{fmt.Sprintf("no proxy for node %s provided", nodeid)},
		}, fmt.Errorf("no proxy provided")
	}

	return p.proxy.ProbeProcess(nodeid, id)
}

func (p *proxyReader) GetURL(prefix, path string) (*url.URL, error) {
	if p.proxy == nil {
		return nil, fmt.Errorf("no proxy provided")
	}

	return p.proxy.GetURL(prefix, path)
}

func (p *proxyReader) GetFile(prefix, path string, offset int64) (io.ReadCloser, error) {
	if p.proxy == nil {
		return nil, fmt.Errorf("no proxy provided")
	}

	return p.proxy.GetFile(prefix, path, offset)
}

func (p *proxyReader) GetFileInfo(prefix, path string) (int64, time.Time, error) {
	if p.proxy == nil {
		return 0, time.Time{}, fmt.Errorf("no proxy provided")
	}

	return p.proxy.GetFileInfo(prefix, path)
}

type ProxyConfig struct {
	ID string // ID of the node

	Logger log.Logger
}

type proxy struct {
	id string

	nodes    map[string]Node      // List of known nodes
	idfiles  map[string][]string  // Map from nodeid to list of files
	idupdate map[string]time.Time // Map from nodeid to time of last update
	fileid   map[string]string    // Map from file name to nodeid

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
		nodes:    map[string]Node{},
		idfiles:  map[string][]string{},
		idupdate: map[string]time.Time{},
		fileid:   map[string]string{},
		updates:  make(chan NodeFiles, 64),
		logger:   config.Logger,
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
				p.idfiles[update.ID] = update.Files
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

func (p *proxy) Resources() map[string]NodeResources {
	resources := map[string]NodeResources{}

	p.lock.RLock()
	defer p.lock.RUnlock()

	for id, node := range p.nodes {
		resources[id] = node.Resources()
	}

	return resources
}

func (p *proxy) AddNode(id string, node Node) (string, error) {
	about := node.About()

	//if id != about.ID {
	//	return "", fmt.Errorf("the provided (%s) and retrieved (%s) ID's don't match", id, about.ID)
	//}

	p.lock.Lock()
	defer p.lock.Unlock()

	if n, ok := p.nodes[id]; ok {
		n.StopFiles()

		delete(p.nodes, id)
	}

	p.nodes[id] = node

	node.StartFiles(p.updates)

	p.logger.Info().WithFields(log.Fields{
		"address": about.Address,
		"name":    about.Name,
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

func (p *proxy) GetURL(prefix, path string) (*url.URL, error) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	logger := p.logger.WithFields(log.Fields{
		"path":   path,
		"prefix": prefix,
	})

	id, ok := p.fileid[prefix+":"+path]
	if !ok {
		logger.Debug().Log("Not found")
		return nil, fmt.Errorf("file not found")
	}

	ts, ok := p.idupdate[id]
	if !ok {
		logger.Debug().Log("No age information found")
		return nil, fmt.Errorf("file not found")
	}

	if time.Since(ts) > 2*time.Second {
		logger.Debug().Log("File too old")
		return nil, fmt.Errorf("file not found")
	}

	node, ok := p.nodes[id]
	if !ok {
		logger.Debug().Log("Unknown node")
		return nil, fmt.Errorf("file not found")
	}

	url, err := node.GetURL(prefix, path)
	if err != nil {
		logger.Debug().Log("Invalid path")
		return nil, fmt.Errorf("file not found")
	}

	logger.Debug().WithField("url", url).Log("File cluster url")

	return url, nil
}

func (p *proxy) GetFile(prefix, path string, offset int64) (io.ReadCloser, error) {
	logger := p.logger.WithFields(log.Fields{
		"path":   path,
		"prefix": prefix,
	})

	node, err := p.getNodeForFile(prefix, path)
	if err != nil {
		logger.Debug().WithError(err).Log("File not available")
		return nil, fmt.Errorf("file not found")
	}

	data, err := node.GetFile(prefix, path, offset)
	if err != nil {
		logger.Debug().Log("Invalid path")
		return nil, fmt.Errorf("file not found")
	}

	logger.Debug().Log("File cluster path")

	return data, nil
}

func (p *proxy) GetFileInfo(prefix, path string) (int64, time.Time, error) {
	logger := p.logger.WithFields(log.Fields{
		"path":   path,
		"prefix": prefix,
	})

	node, err := p.getNodeForFile(prefix, path)
	if err != nil {
		logger.Debug().WithError(err).Log("File not available")
		return 0, time.Time{}, fmt.Errorf("file not found")
	}

	size, lastModified, err := node.GetFileInfo(prefix, path)
	if err != nil {
		logger.Debug().Log("Invalid path")
		return 0, time.Time{}, fmt.Errorf("file not found")
	}

	logger.Debug().Log("File cluster path")

	return size, lastModified, nil
}

func (p *proxy) getNodeForFile(prefix, path string) (Node, error) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	id, ok := p.fileid[prefix+":"+path]
	if !ok {
		return nil, fmt.Errorf("file not found")
	}

	ts, ok := p.idupdate[id]
	if !ok {
		return nil, fmt.Errorf("no age information found")
	}

	if time.Since(ts) > 2*time.Second {
		return nil, fmt.Errorf("file too old")
	}

	node, ok := p.nodes[id]
	if !ok {
		return nil, fmt.Errorf("unknown node")
	}

	return node, nil
}

type Process struct {
	NodeID    string
	Order     string
	State     string
	CPU       float64 // Current CPU load of this process, 0-100*ncpu
	Mem       uint64  // Currently consumed memory of this process in bytes
	Runtime   time.Duration
	UpdatedAt time.Time
	Config    *app.Config
	Metadata  map[string]interface{}
}

type ProcessListOptions struct {
	ID            []string
	Filter        []string
	Domain        string
	Reference     string
	IDPattern     string
	RefPattern    string
	OwnerPattern  string
	DomainPattern string
}

func (p *proxy) ListProxyProcesses() []Process {
	processChan := make(chan Process, 64)
	processList := []Process{}

	wgList := sync.WaitGroup{}
	wgList.Add(1)

	go func() {
		defer wgList.Done()

		for process := range processChan {
			processList = append(processList, process)
		}
	}()

	wg := sync.WaitGroup{}

	p.lock.RLock()
	for _, node := range p.nodes {
		wg.Add(1)

		go func(node Node, p chan<- Process) {
			defer wg.Done()

			processes, err := node.ProxyProcessList()
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

func (p *proxy) FindNodeFromProcess(id app.ProcessID) (string, error) {
	procs := p.ListProxyProcesses()
	nodeid := ""

	for _, p := range procs {
		if p.Config.ProcessID() != id {
			continue
		}

		nodeid = p.NodeID

		break
	}

	if len(nodeid) == 0 {
		return "", fmt.Errorf("the process '%s' is not registered with any node", id.String())
	}

	return nodeid, nil
}

func (p *proxy) ListProcesses(options ProcessListOptions) []clientapi.Process {
	processChan := make(chan clientapi.Process, 64)
	processList := []clientapi.Process{}

	wgList := sync.WaitGroup{}
	wgList.Add(1)

	go func() {
		defer wgList.Done()

		for process := range processChan {
			processList = append(processList, process)
		}
	}()

	wg := sync.WaitGroup{}

	p.lock.RLock()
	for _, node := range p.nodes {
		wg.Add(1)

		go func(node Node, p chan<- clientapi.Process) {
			defer wg.Done()

			processes, err := node.ProcessList(options)
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

func (p *proxy) AddProcess(nodeid string, config *app.Config, metadata map[string]interface{}) error {
	p.lock.RLock()
	defer p.lock.RUnlock()

	node, ok := p.nodes[nodeid]
	if !ok {
		return fmt.Errorf("node not found")
	}

	err := node.AddProcess(config, metadata)
	if err != nil {
		return err
	}

	return nil
}

func (p *proxy) DeleteProcess(nodeid string, id app.ProcessID) error {
	p.lock.RLock()
	defer p.lock.RUnlock()

	node, ok := p.nodes[nodeid]
	if !ok {
		return fmt.Errorf("node not found")
	}

	err := node.DeleteProcess(id)
	if err != nil {
		return err
	}

	return nil
}

func (p *proxy) UpdateProcess(nodeid string, id app.ProcessID, config *app.Config, metadata map[string]interface{}) error {
	p.lock.RLock()
	defer p.lock.RUnlock()

	node, ok := p.nodes[nodeid]
	if !ok {
		return fmt.Errorf("node not found")
	}

	return node.UpdateProcess(id, config, metadata)
}

func (p *proxy) CommandProcess(nodeid string, id app.ProcessID, command string) error {
	p.lock.RLock()
	defer p.lock.RUnlock()

	node, ok := p.nodes[nodeid]
	if !ok {
		return fmt.Errorf("node not found")
	}

	var err error = nil

	switch command {
	case "start":
		err = node.StartProcess(id)
	case "stop":
		err = node.StopProcess(id)
	case "restart":
		err = node.RestartProcess(id)
	case "reload":
		err = node.ReloadProcess(id)
	default:
		err = fmt.Errorf("unknown command: %s", command)
	}

	return err
}

func (p *proxy) ProbeProcess(nodeid string, id app.ProcessID) (clientapi.Probe, error) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	node, ok := p.nodes[nodeid]
	if !ok {
		probe := clientapi.Probe{
			Log: []string{fmt.Sprintf("the node %s where the process %s should reside on, doesn't exist", nodeid, id.String())},
		}
		return probe, fmt.Errorf("node not found")
	}

	return node.ProbeProcess(id)
}
