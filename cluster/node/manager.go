package node

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"sort"
	"sync"
	"time"

	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/http/client"
	"github.com/datarhei/core/v16/log"
	"github.com/datarhei/core/v16/restream/app"
)

type ProcessListOptions = client.ProcessListOptions

type ManagerConfig struct {
	ID string // ID of the node

	Logger log.Logger
}

type Manager struct {
	id string

	nodes map[string]*Node // List of known nodes
	lock  sync.RWMutex

	cache *Cache[string]

	logger log.Logger
}

var ErrNodeNotFound = errors.New("node not found")

func NewManager(config ManagerConfig) (*Manager, error) {
	p := &Manager{
		id:     config.ID,
		nodes:  map[string]*Node{},
		cache:  NewCache[string](nil),
		logger: config.Logger,
	}

	if p.logger == nil {
		p.logger = log.New("")
	}

	return p, nil
}

func (p *Manager) NodeAdd(id string, node *Node) (string, error) {
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

func (p *Manager) NodeRemove(id string) (*Node, error) {
	p.lock.Lock()
	defer p.lock.Unlock()

	node, ok := p.nodes[id]
	if !ok {
		return nil, ErrNodeNotFound
	}

	node.Stop()

	delete(p.nodes, id)

	p.logger.Info().WithFields(log.Fields{
		"id": id,
	}).Log("Removed node")

	return node, nil
}

func (p *Manager) NodesClear() {
	p.lock.Lock()
	defer p.lock.Unlock()

	for _, node := range p.nodes {
		node.Stop()
	}

	p.nodes = map[string]*Node{}

	p.logger.Info().Log("Removed all nodes")
}

func (p *Manager) NodeHasNode(id string) bool {
	p.lock.RLock()
	defer p.lock.RUnlock()

	_, hasNode := p.nodes[id]

	return hasNode
}

func (p *Manager) NodeIDs() []string {
	list := []string{}

	p.lock.RLock()
	defer p.lock.RUnlock()

	for id := range p.nodes {
		list = append(list, id)
	}

	return list
}

func (p *Manager) NodeCount() int {
	p.lock.RLock()
	defer p.lock.RUnlock()

	return len(p.nodes)
}

func (p *Manager) NodeList() []*Node {
	list := []*Node{}

	p.lock.RLock()
	defer p.lock.RUnlock()

	for _, node := range p.nodes {
		list = append(list, node)
	}

	return list
}

func (p *Manager) NodeGet(id string) (*Node, error) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	node, ok := p.nodes[id]
	if !ok {
		return nil, fmt.Errorf("node not found")
	}

	return node, nil
}

func (p *Manager) NodeCheckCompatibility(skipSkillsCheck bool) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	local, hasLocal := p.nodes[p.id]
	if !hasLocal {
		local = nil
	}

	for id, node := range p.nodes {
		if id == p.id {
			continue
		}

		node.CheckCompatibility(local, skipSkillsCheck)
	}
}

func (p *Manager) Barrier(name string) (bool, error) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	for _, node := range p.nodes {
		ok, err := node.Barrier(name)
		if !ok {
			return false, err
		}
	}

	return true, nil
}

// getClusterHostnames return a list of all hostnames configured on all nodes. The
// returned list will not contain any duplicates.
func (p *Manager) GetHostnames(common bool) ([]string, error) {
	hostnames := map[string]int{}

	p.lock.RLock()
	defer p.lock.RUnlock()

	for id, node := range p.nodes {
		config, err := node.CoreConfig(true)
		if err != nil {
			return nil, fmt.Errorf("node %s has no configuration available: %w", id, err)
		}

		for _, name := range config.Host.Name {
			hostnames[name]++
		}
	}

	names := []string{}

	for key, value := range hostnames {
		if common && value != len(p.nodes) {
			continue
		}

		names = append(names, key)
	}

	sort.Strings(names)

	return names, nil
}

func (p *Manager) MediaGetURL(prefix, path string) (*url.URL, error) {
	logger := p.logger.WithFields(log.Fields{
		"path":   path,
		"prefix": prefix,
	})

	node, err := p.getNodeForMedia(prefix, path)
	if err != nil {
		logger.Debug().WithError(err).Log("Unknown node")
		return nil, fmt.Errorf("file not found: %w", err)
	}

	url, err := node.Core().MediaGetURL(prefix, path)
	if err != nil {
		logger.Debug().Log("Invalid path")
		return nil, fmt.Errorf("file not found")
	}

	logger.Debug().WithField("url", url).Log("File cluster url")

	return url, nil
}

func (p *Manager) FilesystemGetFile(prefix, path string, offset int64) (io.ReadCloser, error) {
	logger := p.logger.WithFields(log.Fields{
		"path":   path,
		"prefix": prefix,
	})

	node, err := p.getNodeForMedia(prefix, path)
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

func (p *Manager) FilesystemGetFileInfo(prefix, path string) (int64, time.Time, error) {
	logger := p.logger.WithFields(log.Fields{
		"path":   path,
		"prefix": prefix,
	})

	node, err := p.getNodeForMedia(prefix, path)
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

func (p *Manager) getNodeIDForMedia(prefix, path string) (string, error) {
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

		go func(nodeid string, node *Node, p chan<- string) {
			defer wg.Done()

			_, _, err := node.Core().MediaGetInfo(prefix, path)
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

func (p *Manager) getNodeForMedia(prefix, path string) (*Node, error) {
	id, err := p.cache.Get(prefix + ":" + path)
	if err == nil {
		node, err := p.NodeGet(id)
		if err == nil {
			return node, nil
		}
	}

	id, err = p.getNodeIDForMedia(prefix, path)
	if err != nil {
		return nil, err
	}

	p.cache.Put(prefix+":"+path, id, 5*time.Second)

	return p.NodeGet(id)
}

func (p *Manager) FilesystemList(storage, pattern string) []api.FileInfo {
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

		go func(node *Node, p chan<- []api.FileInfo) {
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

func (p *Manager) ClusterProcessList() []Process {
	processChan := make(chan []Process, 64)
	processList := []Process{}

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

		go func(node *Node, p chan<- []Process) {
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

func (p *Manager) ProcessFindNodeID(id app.ProcessID) (string, error) {
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

func (p *Manager) FindNodeForResources(nodeid string, cpu float64, memory uint64) string {
	p.lock.RLock()
	defer p.lock.RUnlock()

	if len(nodeid) != 0 {
		node, ok := p.nodes[nodeid]
		if ok {
			r := node.About().Resources
			if r.Error == nil && r.CPU+cpu <= r.CPULimit && r.Mem+memory <= r.MemLimit && !r.IsThrottling {
				return nodeid
			}
		}
	}

	for nodeid, node := range p.nodes {
		r := node.About().Resources
		if r.Error == nil && r.CPU+cpu <= r.CPULimit && r.Mem+memory <= r.MemLimit && !r.IsThrottling {
			return nodeid
		}
	}

	return ""
}

func (p *Manager) ProcessList(options client.ProcessListOptions) []api.Process {
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

		go func(node *Node, p chan<- []api.Process) {
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

func (p *Manager) ProcessGet(nodeid string, id app.ProcessID, filter []string) (api.Process, error) {
	node, err := p.NodeGet(nodeid)
	if err != nil {
		return api.Process{}, fmt.Errorf("node not found: %w", err)
	}

	list, err := node.Core().ProcessList(client.ProcessListOptions{
		ID:     []string{id.ID},
		Filter: filter,
		Domain: id.Domain,
	})
	if err != nil {
		return api.Process{}, err
	}

	return list[0], nil
}

func (p *Manager) ProcessAdd(nodeid string, config *app.Config, metadata map[string]interface{}) error {
	node, err := p.NodeGet(nodeid)
	if err != nil {
		return fmt.Errorf("node not found: %w", err)
	}

	return node.Core().ProcessAdd(config, metadata)
}

func (p *Manager) ProcessDelete(nodeid string, id app.ProcessID) error {
	node, err := p.NodeGet(nodeid)
	if err != nil {
		return fmt.Errorf("node not found: %w", err)
	}

	return node.Core().ProcessDelete(id)
}

func (p *Manager) ProcessUpdate(nodeid string, id app.ProcessID, config *app.Config, metadata map[string]interface{}) error {
	node, err := p.NodeGet(nodeid)
	if err != nil {
		return fmt.Errorf("node not found: %w", err)
	}

	return node.Core().ProcessUpdate(id, config, metadata)
}

func (p *Manager) ProcessReportSet(nodeid string, id app.ProcessID, report *app.Report) error {
	node, err := p.NodeGet(nodeid)
	if err != nil {
		return fmt.Errorf("node not found: %w", err)
	}

	return node.Core().ProcessReportSet(id, report)
}

func (p *Manager) ProcessCommand(nodeid string, id app.ProcessID, command string) error {
	node, err := p.NodeGet(nodeid)
	if err != nil {
		return fmt.Errorf("node not found: %w", err)
	}

	return node.Core().ProcessCommand(id, command)
}

func (p *Manager) ProcessProbe(nodeid string, id app.ProcessID) (api.Probe, error) {
	node, err := p.NodeGet(nodeid)
	if err != nil {
		probe := api.Probe{
			Log: []string{fmt.Sprintf("the node %s where the process %s should reside on, doesn't exist", nodeid, id.String())},
		}
		return probe, fmt.Errorf("node not found: %w", err)
	}

	return node.Core().ProcessProbe(id)
}

func (p *Manager) ProcessProbeConfig(nodeid string, config *app.Config) (api.Probe, error) {
	node, err := p.NodeGet(nodeid)
	if err != nil {
		probe := api.Probe{
			Log: []string{fmt.Sprintf("the node %s where the process config should be probed on, doesn't exist", nodeid)},
		}
		return probe, fmt.Errorf("node not found: %w", err)
	}

	return node.Core().ProcessProbeConfig(config)
}
