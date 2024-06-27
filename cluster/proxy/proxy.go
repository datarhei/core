package proxy

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"sync"
	"time"

	"github.com/datarhei/core/v16/cluster/node"
	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/http/client"
	"github.com/datarhei/core/v16/log"
	"github.com/datarhei/core/v16/restream/app"
)

type ProcessListOptions = client.ProcessListOptions

type Proxy interface {
	AddNode(id string, node *node.Node) (string, error)
	RemoveNode(id string) error

	NodeList() []*node.Node
	NodeGet(id string) (*node.Node, error)

	ClusterProcessList() []node.Process

	ProcessList(options ProcessListOptions) []api.Process
	ProcessFindNodeID(id app.ProcessID) (string, error)
	ProcessAdd(nodeid string, config *app.Config, metadata map[string]interface{}) error
	ProcessDelete(nodeid string, id app.ProcessID) error
	ProcessUpdate(nodeid string, id app.ProcessID, config *app.Config, metadata map[string]interface{}) error
	ProcessCommand(nodeid string, id app.ProcessID, command string) error
	ProcessProbe(nodeid string, id app.ProcessID) (api.Probe, error)
	ProcessProbeConfig(nodeid string, config *app.Config) (api.Probe, error)

	FilesystemList(storage, pattern string) []api.FileInfo
	FilesystemGetFile(prefix, path string, offset int64) (io.ReadCloser, error)
	FilesystemGetFileInfo(prefix, path string) (int64, time.Time, error)

	ResourcesGetURL(prefix, path string) (*url.URL, error)
}

type ProxyConfig struct {
	ID string // ID of the node

	Logger log.Logger
}

type proxy struct {
	id string

	nodes map[string]*node.Node // List of known nodes
	lock  sync.RWMutex

	cache *Cache[string]

	logger log.Logger
}

var ErrNodeNotFound = errors.New("node not found")

func NewProxy(config ProxyConfig) (Proxy, error) {
	p := &proxy{
		id:     config.ID,
		nodes:  map[string]*node.Node{},
		cache:  NewCache[string](nil),
		logger: config.Logger,
	}

	if p.logger == nil {
		p.logger = log.New("")
	}

	return p, nil
}

func (p *proxy) AddNode(id string, node *node.Node) (string, error) {
	about := node.About()

	p.lock.Lock()
	defer p.lock.Unlock()

	if n, ok := p.nodes[id]; ok {
		n.Stop()
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
	p.lock.Lock()
	defer p.lock.Unlock()

	node, ok := p.nodes[id]
	if !ok {
		return ErrNodeNotFound
	}

	node.Stop()

	delete(p.nodes, id)

	p.logger.Info().WithFields(log.Fields{
		"id": id,
	}).Log("Removed node")

	return nil
}

func (p *proxy) NodeList() []*node.Node {
	list := []*node.Node{}

	p.lock.RLock()
	defer p.lock.RUnlock()

	for _, node := range p.nodes {
		list = append(list, node)
	}

	return list
}

func (p *proxy) NodeGet(id string) (*node.Node, error) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	node, ok := p.nodes[id]
	if !ok {
		return nil, fmt.Errorf("node not found")
	}

	return node, nil
}

func (p *proxy) ResourcesGetURL(prefix, path string) (*url.URL, error) {
	logger := p.logger.WithFields(log.Fields{
		"path":   path,
		"prefix": prefix,
	})

	node, err := p.getNodeForResource(prefix, path)
	if err != nil {
		logger.Debug().WithError(err).Log("Unknown node")
		return nil, fmt.Errorf("file not found: %w", err)
	}

	url, err := node.Core().ResourcesGetURL(prefix, path)
	if err != nil {
		logger.Debug().Log("Invalid path")
		return nil, fmt.Errorf("file not found")
	}

	logger.Debug().WithField("url", url).Log("File cluster url")

	return url, nil
}

func (p *proxy) FilesystemGetFile(prefix, path string, offset int64) (io.ReadCloser, error) {
	logger := p.logger.WithFields(log.Fields{
		"path":   path,
		"prefix": prefix,
	})

	node, err := p.getNodeForResource(prefix, path)
	if err != nil {
		logger.Debug().WithError(err).Log("File not available")
		return nil, fmt.Errorf("file not found")
	}

	data, err := node.Core().FilesystemGetFile(prefix, path, offset)
	if err != nil {
		logger.Debug().Log("Invalid path")
		return nil, fmt.Errorf("file not found")
	}

	logger.Debug().Log("File cluster path")

	return data, nil
}

func (p *proxy) FilesystemGetFileInfo(prefix, path string) (int64, time.Time, error) {
	logger := p.logger.WithFields(log.Fields{
		"path":   path,
		"prefix": prefix,
	})

	node, err := p.getNodeForResource(prefix, path)
	if err != nil {
		logger.Debug().WithError(err).Log("File not available")
		return 0, time.Time{}, fmt.Errorf("file not found")
	}

	size, lastModified, err := node.Core().FilesystemGetFileInfo(prefix, path)
	if err != nil {
		logger.Debug().Log("Invalid path")
		return 0, time.Time{}, fmt.Errorf("file not found")
	}

	logger.Debug().Log("File cluster path")

	return size, lastModified, nil
}

func (p *proxy) getNodeIDForResource(prefix, path string) (string, error) {
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

	p.lock.RLock()
	for id, n := range p.nodes {
		wg.Add(1)

		go func(nodeid string, node *node.Node, p chan<- string) {
			defer wg.Done()

			_, _, err := node.Core().ResourcesGetInfo(prefix, path)
			if err != nil {
				nodeid = ""
			}

			p <- nodeid
		}(id, n, nodesChan)
	}
	p.lock.RUnlock()

	wg.Wait()

	close(nodesChan)

	wgList.Wait()

	if len(nodeids) == 0 {
		return "", fmt.Errorf("file not found")
	}

	return nodeids[0], nil
}

func (p *proxy) getNodeForResource(prefix, path string) (*node.Node, error) {
	id, err := p.cache.Get(prefix + ":" + path)
	if err == nil {
		node, err := p.NodeGet(id)
		if err == nil {
			return node, nil
		}
	}

	id, err = p.getNodeIDForResource(prefix, path)
	if err != nil {
		return nil, err
	}

	p.cache.Put(prefix+":"+path, id, 5*time.Second)

	return p.NodeGet(id)
}

func (p *proxy) FilesystemList(storage, pattern string) []api.FileInfo {
	filesChan := make(chan []api.FileInfo, 64)
	filesList := []api.FileInfo{}

	wgList := sync.WaitGroup{}
	wgList.Add(1)

	go func() {
		defer wgList.Done()

		for list := range filesChan {
			filesList = append(filesList, list...)
		}
	}()

	wg := sync.WaitGroup{}

	p.lock.RLock()
	for _, n := range p.nodes {
		wg.Add(1)

		go func(node *node.Node, p chan<- []api.FileInfo) {
			defer wg.Done()

			files, err := node.Core().FilesystemList(storage, pattern)
			if err != nil {
				return
			}

			p <- files
		}(n, filesChan)
	}
	p.lock.RUnlock()

	wg.Wait()

	close(filesChan)

	wgList.Wait()

	return filesList
}

func (p *proxy) ClusterProcessList() []node.Process {
	processChan := make(chan []node.Process, 64)
	processList := []node.Process{}

	wgList := sync.WaitGroup{}
	wgList.Add(1)

	go func() {
		defer wgList.Done()

		for list := range processChan {
			processList = append(processList, list...)
		}
	}()

	wg := sync.WaitGroup{}

	p.lock.RLock()
	for _, n := range p.nodes {
		wg.Add(1)

		go func(node *node.Node, p chan<- []node.Process) {
			defer wg.Done()

			processes, err := node.Core().ClusterProcessList()
			if err != nil {
				return
			}

			p <- processes
		}(n, processChan)
	}
	p.lock.RUnlock()

	wg.Wait()

	close(processChan)

	wgList.Wait()

	return processList
}

func (p *proxy) ProcessFindNodeID(id app.ProcessID) (string, error) {
	procs := p.ClusterProcessList()
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

func (p *proxy) ProcessList(options client.ProcessListOptions) []api.Process {
	processChan := make(chan []api.Process, 64)
	processList := []api.Process{}

	wgList := sync.WaitGroup{}
	wgList.Add(1)

	go func() {
		defer wgList.Done()

		for list := range processChan {
			processList = append(processList, list...)
		}
	}()

	wg := sync.WaitGroup{}

	p.lock.RLock()
	for _, n := range p.nodes {
		wg.Add(1)

		go func(node *node.Node, p chan<- []api.Process) {
			defer wg.Done()

			processes, err := node.Core().ProcessList(options)
			if err != nil {
				return
			}

			p <- processes
		}(n, processChan)
	}
	p.lock.RUnlock()

	wg.Wait()

	close(processChan)

	wgList.Wait()

	return processList
}

func (p *proxy) ProcessAdd(nodeid string, config *app.Config, metadata map[string]interface{}) error {
	node, err := p.NodeGet(nodeid)
	if err != nil {
		return fmt.Errorf("node not found: %w", err)
	}

	return node.Core().ProcessAdd(config, metadata)
}

func (p *proxy) ProcessDelete(nodeid string, id app.ProcessID) error {
	node, err := p.NodeGet(nodeid)
	if err != nil {
		return fmt.Errorf("node not found: %w", err)
	}

	return node.Core().ProcessDelete(id)
}

func (p *proxy) ProcessUpdate(nodeid string, id app.ProcessID, config *app.Config, metadata map[string]interface{}) error {
	node, err := p.NodeGet(nodeid)
	if err != nil {
		return fmt.Errorf("node not found: %w", err)
	}

	return node.Core().ProcessUpdate(id, config, metadata)
}

func (p *proxy) ProcessCommand(nodeid string, id app.ProcessID, command string) error {
	node, err := p.NodeGet(nodeid)
	if err != nil {
		return fmt.Errorf("node not found: %w", err)
	}

	return node.Core().ProcessCommand(id, command)
}

func (p *proxy) ProcessProbe(nodeid string, id app.ProcessID) (api.Probe, error) {
	node, err := p.NodeGet(nodeid)
	if err != nil {
		probe := api.Probe{
			Log: []string{fmt.Sprintf("the node %s where the process %s should reside on, doesn't exist", nodeid, id.String())},
		}
		return probe, fmt.Errorf("node not found: %w", err)
	}

	return node.Core().ProcessProbe(id)
}

func (p *proxy) ProcessProbeConfig(nodeid string, config *app.Config) (api.Probe, error) {
	node, err := p.NodeGet(nodeid)
	if err != nil {
		probe := api.Probe{
			Log: []string{fmt.Sprintf("the node %s where the process config should be probed on, doesn't exist", nodeid)},
		}
		return probe, fmt.Errorf("node not found: %w", err)
	}

	return node.Core().ProcessProbeConfig(config)
}
