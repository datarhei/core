package proxy

import (
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
	GetNodeReader(id string) (NodeReader, error)

	FindNodeFromProcess(id app.ProcessID) (string, error)

	Resources() map[string]NodeResources
	ListProcesses(ProcessListOptions) map[string][]clientapi.Process
	ListProxyProcesses() []Process
	ProbeProcess(nodeid string, id app.ProcessID) (clientapi.Probe, error)

	GetURL(prefix, path string) (*url.URL, error)
	GetFile(prefix, path string, offset int64) (io.ReadCloser, error)
	GetFileInfo(prefix, path string) (int64, time.Time, error)
}

type ProxyConfig struct {
	ID string // ID of the node

	Logger log.Logger
}

type proxy struct {
	id string

	nodes     map[string]Node // List of known nodes
	nodesLock sync.RWMutex

	lock    sync.RWMutex
	running bool

	cache *Cache[string]

	logger log.Logger
}

var ErrNodeNotFound = errors.New("node not found")

func NewProxy(config ProxyConfig) (Proxy, error) {
	p := &proxy{
		id:     config.ID,
		nodes:  map[string]Node{},
		cache:  NewCache[string](nil),
		logger: config.Logger,
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
}

func (p *proxy) Stop() {
	p.lock.Lock()
	defer p.lock.Unlock()

	if !p.running {
		return
	}

	p.running = false

	p.logger.Debug().Log("Stopping proxy")

	p.nodes = map[string]Node{}
}

func (p *proxy) Reader() ProxyReader {
	return p
}

func (p *proxy) Resources() map[string]NodeResources {
	resources := map[string]NodeResources{}

	p.nodesLock.RLock()
	defer p.nodesLock.RUnlock()

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

	p.nodesLock.Lock()
	defer p.nodesLock.Unlock()

	if n, ok := p.nodes[id]; ok {
		n.Disconnect()
		delete(p.nodes, id)
	}

	p.nodes[id] = node

	p.logger.Info().WithFields(log.Fields{
		"address": about.Address,
		"name":    about.Name,
		"id":      id,
	}).Log("Added node")

	return id, nil
}

func (p *proxy) RemoveNode(id string) error {
	p.nodesLock.Lock()
	defer p.nodesLock.Unlock()

	node, ok := p.nodes[id]
	if !ok {
		return ErrNodeNotFound
	}

	node.Disconnect()

	delete(p.nodes, id)

	p.logger.Info().WithFields(log.Fields{
		"id": id,
	}).Log("Removed node")

	return nil
}

func (p *proxy) ListNodes() []NodeReader {
	list := []NodeReader{}

	p.nodesLock.RLock()
	defer p.nodesLock.RUnlock()

	for _, node := range p.nodes {
		list = append(list, node)
	}

	return list
}

func (p *proxy) GetNode(id string) (Node, error) {
	p.nodesLock.RLock()
	defer p.nodesLock.RUnlock()

	node, ok := p.nodes[id]
	if !ok {
		return nil, fmt.Errorf("node not found")
	}

	return node, nil
}

func (p *proxy) GetNodeReader(id string) (NodeReader, error) {
	return p.GetNode(id)
}

func (p *proxy) GetURL(prefix, path string) (*url.URL, error) {
	logger := p.logger.WithFields(log.Fields{
		"path":   path,
		"prefix": prefix,
	})

	node, err := p.getNodeForFile(prefix, path)
	if err != nil {
		logger.Debug().WithError(err).Log("Unknown node")
		return nil, fmt.Errorf("file not found: %w", err)
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

func (p *proxy) getNodeIDForFile(prefix, path string) (string, error) {
	// this is only for mem and disk prefixes
	nodesChan := make(chan string, 16)
	nodeids := []string{}

	wgList := sync.WaitGroup{}
	wgList.Add(1)

	go func() {
		defer wgList.Done()

		for nodeid := range nodesChan {
			if len(nodeid) == 0 {
				continue
			}

			nodeids = append(nodeids, nodeid)
		}
	}()

	wg := sync.WaitGroup{}

	p.nodesLock.RLock()
	for id, node := range p.nodes {
		wg.Add(1)

		go func(nodeid string, node Node, p chan<- string) {
			defer wg.Done()

			_, _, err := node.GetResourceInfo(prefix, path)
			if err != nil {
				nodeid = ""
			}

			p <- nodeid
		}(id, node, nodesChan)
	}
	p.nodesLock.RUnlock()

	wg.Wait()

	close(nodesChan)

	wgList.Wait()

	if len(nodeids) == 0 {
		return "", fmt.Errorf("file not found")
	}

	return nodeids[0], nil
}

func (p *proxy) getNodeForFile(prefix, path string) (Node, error) {
	id, err := p.cache.Get(prefix + ":" + path)
	if err == nil {
		node, err := p.GetNode(id)
		if err == nil {
			return node, nil
		}
	}

	id, err = p.getNodeIDForFile(prefix, path)
	if err != nil {
		return nil, err
	}

	p.cache.Put(prefix+":"+path, id, 5*time.Second)

	return p.GetNode(id)
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

	p.nodesLock.RLock()
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
	p.nodesLock.RUnlock()

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

type processList struct {
	nodeid    string
	processes []clientapi.Process
}

func (p *proxy) ListProcesses(options ProcessListOptions) map[string][]clientapi.Process {
	processChan := make(chan processList, 64)
	processMap := map[string][]clientapi.Process{}

	wgList := sync.WaitGroup{}
	wgList.Add(1)

	go func() {
		defer wgList.Done()

		for list := range processChan {
			processMap[list.nodeid] = list.processes
		}
	}()

	wg := sync.WaitGroup{}

	p.nodesLock.RLock()
	for _, node := range p.nodes {
		wg.Add(1)

		go func(node Node, p chan<- processList) {
			defer wg.Done()

			processes, err := node.ProcessList(options)
			if err != nil {
				return
			}

			p <- processList{
				nodeid:    node.About().ID,
				processes: processes,
			}

		}(node, processChan)
	}
	p.nodesLock.RUnlock()

	wg.Wait()

	close(processChan)

	wgList.Wait()

	return processMap
}

func (p *proxy) AddProcess(nodeid string, config *app.Config, metadata map[string]interface{}) error {
	node, err := p.GetNode(nodeid)
	if err != nil {
		return fmt.Errorf("node not found: %w", err)
	}

	return node.AddProcess(config, metadata)
}

func (p *proxy) DeleteProcess(nodeid string, id app.ProcessID) error {
	node, err := p.GetNode(nodeid)
	if err != nil {
		return fmt.Errorf("node not found: %w", err)
	}

	return node.DeleteProcess(id)
}

func (p *proxy) UpdateProcess(nodeid string, id app.ProcessID, config *app.Config, metadata map[string]interface{}) error {
	node, err := p.GetNode(nodeid)
	if err != nil {
		return fmt.Errorf("node not found: %w", err)
	}

	return node.UpdateProcess(id, config, metadata)
}

func (p *proxy) CommandProcess(nodeid string, id app.ProcessID, command string) error {
	node, err := p.GetNode(nodeid)
	if err != nil {
		return fmt.Errorf("node not found: %w", err)
	}

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
	node, err := p.GetNode(nodeid)
	if err != nil {
		probe := clientapi.Probe{
			Log: []string{fmt.Sprintf("the node %s where the process %s should reside on, doesn't exist", nodeid, id.String())},
		}
		return probe, fmt.Errorf("node not found: %w", err)
	}

	return node.ProbeProcess(id)
}
