package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/datarhei/core/v16/config"
	"github.com/datarhei/core/v16/restream/app"

	client "github.com/datarhei/core-client-go/v16"
	clientapi "github.com/datarhei/core-client-go/v16/api"
)

type Node interface {
	SetEssentials(address string, config *config.Config)
	IsConnected() (bool, error)
	Disconnect()

	AddProcess(config *app.Config, metadata map[string]interface{}) error
	StartProcess(id app.ProcessID) error
	StopProcess(id app.ProcessID) error
	RestartProcess(id app.ProcessID) error
	ReloadProcess(id app.ProcessID) error
	DeleteProcess(id app.ProcessID) error
	UpdateProcess(id app.ProcessID, config *app.Config, metadata map[string]interface{}) error
	ProbeProcess(id app.ProcessID) (clientapi.Probe, error)

	NodeReader
}

type NodeReader interface {
	Ping() (time.Duration, error)
	About() NodeAbout
	Version() NodeVersion
	Resources() NodeResources

	Files() NodeFiles

	GetURL(prefix, path string) (*url.URL, error)
	GetFile(prefix, path string, offset int64) (io.ReadCloser, error)
	GetFileInfo(prefix, path string) (int64, time.Time, error)

	ProcessList(ProcessListOptions) ([]clientapi.Process, error)
	ProxyProcessList() ([]Process, error)
}

type NodeFiles struct {
	ID         string
	Files      []string
	LastUpdate time.Time
}

type NodeResources struct {
	IsThrottling bool    // Whether this core is currently throttling
	NCPU         float64 // Number of CPU on this node
	CPU          float64 // Current CPU load, 0-100*ncpu
	CPULimit     float64 // Defined CPU load limit, 0-100*ncpu
	Mem          uint64  // Currently used memory in bytes
	MemLimit     uint64  // Defined memory limit in bytes
}

type NodeAbout struct {
	ID          string
	Name        string
	Address     string
	State       string
	Error       error
	CreatedAt   time.Time
	Uptime      time.Duration
	LastContact time.Time
	Latency     time.Duration
	Resources   NodeResources
}

type NodeVersion struct {
	Number   string
	Commit   string
	Branch   string
	Build    time.Time
	Arch     string
	Compiler string
}

type nodeState string

func (n nodeState) String() string {
	return string(n)
}

const (
	stateDisconnected nodeState = "disconnected"
	stateConnected    nodeState = "connected"
)

var ErrNoPeer = errors.New("not connected to the core API: client not available")

type node struct {
	id      string
	address string

	peer       client.RestClient
	peerErr    error
	peerLock   sync.RWMutex
	peerWg     sync.WaitGroup
	disconnect context.CancelFunc

	lastContact time.Time

	resources struct {
		throttling bool
		ncpu       float64
		cpu        float64
		cpuLimit   float64
		mem        uint64
		memLimit   uint64
	}

	config *config.Config

	state      nodeState
	latency    float64 // Seconds
	stateLock  sync.RWMutex
	filesList  []string
	lastUpdate time.Time

	secure      bool
	httpAddress *url.URL
	hasRTMP     bool
	rtmpAddress *url.URL
	hasSRT      bool
	srtAddress  *url.URL
}

func NewNode(id, address string, config *config.Config) Node {
	n := &node{
		id:      id,
		address: address,
		config:  config,
		state:   stateDisconnected,
		secure:  strings.HasPrefix(address, "https://"),
	}

	n.resources.throttling = true
	n.resources.cpu = 100
	n.resources.ncpu = 1
	n.resources.cpuLimit = 100
	n.resources.mem = 0
	n.resources.memLimit = 0

	ctx, cancel := context.WithCancel(context.Background())
	n.disconnect = cancel

	err := n.connect()
	if err != nil {
		n.peerErr = err
	}

	n.peerWg.Add(4)

	go func(ctx context.Context) {
		// This tries to reconnect to the core API. If everything's
		// fine, this is a no-op.
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		defer n.peerWg.Done()

		for {
			select {
			case <-ticker.C:
				err := n.connect()

				n.peerLock.Lock()
				n.peerErr = err
				n.peerLock.Unlock()
			case <-ctx.Done():
				return
			}
		}
	}(ctx)

	go n.pingPeer(ctx, &n.peerWg)
	go n.updateResources(ctx, &n.peerWg)
	go n.updateFiles(ctx, &n.peerWg)

	return n
}

func (n *node) SetEssentials(address string, config *config.Config) {
	n.peerLock.Lock()
	defer n.peerLock.Unlock()

	if address != n.address {
		n.address = address
		n.peer = nil // force reconnet
	}

	if n.config == nil && config != nil {
		n.config = config
		n.peer = nil // force reconnect
	}

	if n.config.UpdatedAt != config.UpdatedAt {
		n.config = config
		n.peer = nil // force reconnect
	}
}

func (n *node) connect() error {
	n.peerLock.Lock()
	defer n.peerLock.Unlock()

	if n.peer != nil && n.state != stateDisconnected {
		return nil
	}

	if len(n.address) == 0 {
		return fmt.Errorf("no address provided")
	}

	if n.config == nil {
		return fmt.Errorf("config not available")
	}

	u, err := url.Parse(n.address)
	if err != nil {
		return fmt.Errorf("invalid address (%s): %w", n.address, err)
	}

	nodehost, _, err := net.SplitHostPort(u.Host)
	if err != nil {
		return fmt.Errorf("invalid address (%s): %w", u.Host, err)
	}

	peer, err := client.New(client.Config{
		Address: u.String(),
		Client: &http.Client{
			Timeout: 5 * time.Second,
		},
	})
	if err != nil {
		return fmt.Errorf("creating client failed (%s): %w", n.address, err)
	}

	n.httpAddress = u

	if n.config.RTMP.Enable {
		n.hasRTMP = true
		n.rtmpAddress = &url.URL{}
		n.rtmpAddress.Scheme = "rtmp"

		isHostIP := net.ParseIP(nodehost) != nil

		address := n.config.RTMP.Address
		if n.secure && n.config.RTMP.EnableTLS && !isHostIP {
			address = n.config.RTMP.AddressTLS
			n.rtmpAddress.Scheme = "rtmps"
		}

		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return fmt.Errorf("invalid rtmp address '%s': %w", address, err)
		}

		if len(host) == 0 {
			n.rtmpAddress.Host = net.JoinHostPort(nodehost, port)
		} else {
			n.rtmpAddress.Host = net.JoinHostPort(host, port)
		}

		n.rtmpAddress = n.rtmpAddress.JoinPath(n.config.RTMP.App)
	}

	if n.config.SRT.Enable {
		n.hasSRT = true
		n.srtAddress = &url.URL{}
		n.srtAddress.Scheme = "srt"

		host, port, err := net.SplitHostPort(n.config.SRT.Address)
		if err != nil {
			return fmt.Errorf("invalid srt address '%s': %w", n.config.SRT.Address, err)
		}

		if len(host) == 0 {
			n.srtAddress.Host = net.JoinHostPort(nodehost, port)
		} else {
			n.srtAddress.Host = net.JoinHostPort(host, port)
		}

		v := url.Values{}

		v.Set("mode", "caller")
		if len(n.config.SRT.Passphrase) != 0 {
			v.Set("passphrase", n.config.SRT.Passphrase)
		}

		n.srtAddress.RawQuery = v.Encode()
	}

	n.peer = peer

	return nil
}

func (n *node) IsConnected() (bool, error) {
	n.peerLock.RLock()
	defer n.peerLock.RUnlock()

	if n.peer == nil {
		return false, ErrNoPeer
	}

	if n.peerErr != nil {
		return false, n.peerErr
	}

	return true, nil
}

func (n *node) Disconnect() {
	n.peerLock.Lock()
	if n.disconnect != nil {
		n.disconnect()
		n.disconnect = nil
	}
	n.peerLock.Unlock()

	n.peerWg.Wait()

	n.peerLock.Lock()
	n.peer = nil
	n.peerErr = fmt.Errorf("disconnected")
	n.peerLock.Unlock()
}

func (n *node) pingPeer(ctx context.Context, wg *sync.WaitGroup) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	defer wg.Done()

	for {
		select {
		case <-ticker.C:
			// Ping
			latency, err := n.Ping()

			n.peerLock.Lock()
			n.peerErr = err
			n.peerLock.Unlock()

			n.stateLock.Lock()
			if err != nil {
				n.state = stateDisconnected
			} else {
				n.lastContact = time.Now()
				n.state = stateConnected
			}
			n.latency = n.latency*0.2 + latency.Seconds()*0.8
			n.stateLock.Unlock()
		case <-ctx.Done():
			return
		}
	}
}

func (n *node) updateResources(ctx context.Context, wg *sync.WaitGroup) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	defer wg.Done()

	for {
		select {
		case <-ticker.C:
			// Metrics
			metrics, err := n.Metrics(clientapi.MetricsQuery{
				Metrics: []clientapi.MetricsQueryMetric{
					{Name: "cpu_ncpu"},
					{Name: "cpu_idle"},
					{Name: "cpu_limit"},
					{Name: "cpu_throttling"},
					{Name: "mem_total"},
					{Name: "mem_free"},
					{Name: "mem_limit"},
					{Name: "mem_throttling"},
				},
			})

			n.peerLock.Lock()
			n.peerErr = err
			n.peerLock.Unlock()

			if err != nil {
				n.stateLock.Lock()
				n.resources.throttling = true
				n.resources.cpu = 100
				n.resources.ncpu = 1
				n.resources.cpuLimit = 100
				n.resources.mem = 0
				n.resources.memLimit = 0
				n.state = stateDisconnected
				n.stateLock.Unlock()

				continue
			}

			cpu_ncpu := .0
			cpu_idle := .0
			cpu_limit := .0
			mem_total := uint64(0)
			mem_free := uint64(0)
			mem_limit := uint64(0)
			throttling := .0

			for _, x := range metrics.Metrics {
				if x.Name == "cpu_idle" {
					cpu_idle = x.Values[0].Value
				} else if x.Name == "cpu_ncpu" {
					cpu_ncpu = x.Values[0].Value
				} else if x.Name == "cpu_limit" {
					cpu_limit = x.Values[0].Value
				} else if x.Name == "cpu_throttling" {
					throttling += x.Values[0].Value
				} else if x.Name == "mem_total" {
					mem_total = uint64(x.Values[0].Value)
				} else if x.Name == "mem_free" {
					mem_free = uint64(x.Values[0].Value)
				} else if x.Name == "mem_limit" {
					mem_limit = uint64(x.Values[0].Value)
				} else if x.Name == "mem_throttling" {
					throttling += x.Values[0].Value
				}
			}

			n.stateLock.Lock()
			if throttling > 0 {
				n.resources.throttling = true
			} else {
				n.resources.throttling = false
			}
			n.resources.ncpu = cpu_ncpu
			n.resources.cpu = (100 - cpu_idle) * cpu_ncpu
			n.resources.cpuLimit = cpu_limit * cpu_ncpu
			if mem_total != 0 {
				n.resources.mem = mem_total - mem_free
				n.resources.memLimit = mem_limit
			} else {
				n.resources.mem = 0
				n.resources.memLimit = 0
			}
			n.lastContact = time.Now()
			n.stateLock.Unlock()
		case <-ctx.Done():
			return
		}
	}
}

func (n *node) updateFiles(ctx context.Context, wg *sync.WaitGroup) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	defer wg.Done()

	for {
		select {
		case <-ticker.C:
			n.files()
		case <-ctx.Done():
			return
		}
	}
}

func (n *node) Ping() (time.Duration, error) {
	n.peerLock.RLock()
	defer n.peerLock.RUnlock()

	if n.peer == nil {
		return 0, ErrNoPeer
	}

	latency, err := n.peer.Ping()

	return latency, err
}

func (n *node) Metrics(query clientapi.MetricsQuery) (clientapi.MetricsResponse, error) {
	n.peerLock.RLock()
	defer n.peerLock.RUnlock()

	if n.peer == nil {
		return clientapi.MetricsResponse{}, ErrNoPeer
	}

	return n.peer.Metrics(query)
}

func (n *node) AboutPeer() (clientapi.About, error) {
	n.peerLock.RLock()
	defer n.peerLock.RUnlock()

	if n.peer == nil {
		return clientapi.About{}, ErrNoPeer
	}

	return n.peer.About(false)
}

func (n *node) About() NodeAbout {
	about, err := n.AboutPeer()

	n.stateLock.RLock()
	defer n.stateLock.RUnlock()

	if err != nil {
		return NodeAbout{
			ID:          n.id,
			Address:     n.address,
			State:       stateDisconnected.String(),
			Error:       err,
			LastContact: n.lastContact,
			Resources: NodeResources{
				IsThrottling: true,
				NCPU:         1,
			},
		}
	}

	createdAt, err := time.Parse(time.RFC3339, about.CreatedAt)
	if err != nil {
		createdAt = time.Now()
	}

	state := n.state
	if time.Since(n.lastContact) > 3*time.Second {
		state = stateDisconnected
	}

	nodeAbout := NodeAbout{
		ID:          n.id,
		Name:        about.Name,
		Address:     n.address,
		State:       state.String(),
		Error:       n.peerErr,
		CreatedAt:   createdAt,
		Uptime:      time.Since(createdAt),
		LastContact: n.lastContact,
		Latency:     time.Duration(n.latency * float64(time.Second)),
		Resources: NodeResources{
			IsThrottling: n.resources.throttling,
			NCPU:         n.resources.ncpu,
			CPU:          n.resources.cpu,
			CPULimit:     n.resources.cpuLimit,
			Mem:          n.resources.mem,
			MemLimit:     n.resources.memLimit,
		},
	}

	if state == stateDisconnected {
		nodeAbout.Uptime = 0
		nodeAbout.Latency = 0
		nodeAbout.Resources.IsThrottling = true
		nodeAbout.Resources.NCPU = 1
	}

	return nodeAbout
}

func (n *node) Resources() NodeResources {
	about := n.About()

	return about.Resources
}

func (n *node) Version() NodeVersion {
	about, err := n.AboutPeer()
	if err != nil {
		return NodeVersion{}
	}

	build, err := time.Parse(time.RFC3339, about.Version.Build)
	if err != nil {
		build = time.Time{}
	}

	version := NodeVersion{
		Number:   about.Version.Number,
		Commit:   about.Version.Commit,
		Branch:   about.Version.Branch,
		Build:    build,
		Arch:     about.Version.Arch,
		Compiler: about.Version.Compiler,
	}

	return version
}

func (n *node) Files() NodeFiles {
	id := n.About().ID

	n.stateLock.RLock()
	defer n.stateLock.RUnlock()

	files := NodeFiles{
		ID:         id,
		LastUpdate: n.lastUpdate,
	}

	if n.state != stateDisconnected && time.Since(n.lastUpdate) <= 2*time.Second {
		files.Files = make([]string, len(n.filesList))
		copy(files.Files, n.filesList)
	}

	return files
}

func (n *node) files() {
	errorsChan := make(chan error, 8)
	filesChan := make(chan string, 1024)
	filesList := []string{}
	errorList := []error{}

	wgList := sync.WaitGroup{}
	wgList.Add(1)

	go func() {
		defer wgList.Done()

		for file := range filesChan {
			filesList = append(filesList, file)
		}

		for err := range errorsChan {
			errorList = append(errorList, err)
		}
	}()

	wg := sync.WaitGroup{}
	wg.Add(2)

	go func(f chan<- string, e chan<- error) {
		defer wg.Done()

		n.peerLock.RLock()
		defer n.peerLock.RUnlock()

		if n.peer == nil {
			e <- ErrNoPeer
			return
		}

		files, err := n.peer.MemFSList("name", "asc")
		if err != nil {
			e <- err
			return
		}

		for _, file := range files {
			f <- "mem:" + file.Name
		}
	}(filesChan, errorsChan)

	go func(f chan<- string, e chan<- error) {
		defer wg.Done()

		n.peerLock.RLock()
		defer n.peerLock.RUnlock()

		if n.peer == nil {
			e <- ErrNoPeer
			return
		}

		files, err := n.peer.DiskFSList("name", "asc")
		if err != nil {
			e <- err
			return
		}

		for _, file := range files {
			f <- "disk:" + file.Name
		}
	}(filesChan, errorsChan)

	if n.hasRTMP {
		wg.Add(1)

		go func(f chan<- string, e chan<- error) {
			defer wg.Done()

			n.peerLock.RLock()
			defer n.peerLock.RUnlock()

			if n.peer == nil {
				e <- ErrNoPeer
				return
			}

			files, err := n.peer.RTMPChannels()
			if err != nil {
				e <- err
				return
			}

			for _, file := range files {
				f <- "rtmp:" + file.Name
			}
		}(filesChan, errorsChan)
	}

	if n.hasSRT {
		wg.Add(1)

		go func(f chan<- string, e chan<- error) {
			defer wg.Done()

			n.peerLock.RLock()
			defer n.peerLock.RUnlock()

			if n.peer == nil {
				e <- ErrNoPeer
				return
			}

			files, err := n.peer.SRTChannels()
			if err != nil {
				e <- err
				return
			}

			for _, file := range files {
				f <- "srt:" + file.Name
			}
		}(filesChan, errorsChan)
	}

	wg.Wait()

	close(filesChan)
	close(errorsChan)

	wgList.Wait()

	n.stateLock.Lock()

	if len(errorList) == 0 {
		n.filesList = make([]string, len(filesList))
		copy(n.filesList, filesList)
		n.lastUpdate = time.Now()
		n.lastContact = n.lastUpdate
	}

	n.stateLock.Unlock()
}

func cloneURL(src *url.URL) *url.URL {
	dst := &url.URL{
		Scheme:      src.Scheme,
		Opaque:      src.Opaque,
		User:        nil,
		Host:        src.Host,
		Path:        src.Path,
		RawPath:     src.RawPath,
		OmitHost:    src.OmitHost,
		ForceQuery:  src.ForceQuery,
		RawQuery:    src.RawQuery,
		Fragment:    src.Fragment,
		RawFragment: src.RawFragment,
	}

	if src.User != nil {
		username := src.User.Username()
		password, ok := src.User.Password()

		if ok {
			dst.User = url.UserPassword(username, password)
		} else {
			dst.User = url.User(username)
		}
	}

	return dst
}

func (n *node) GetURL(prefix, resource string) (*url.URL, error) {
	var u *url.URL

	if prefix == "mem" {
		u = cloneURL(n.httpAddress)
		u = u.JoinPath("memfs", resource)
	} else if prefix == "disk" {
		u = cloneURL(n.httpAddress)
		u = u.JoinPath(resource)
	} else if prefix == "rtmp" {
		u = cloneURL(n.rtmpAddress)
		u = u.JoinPath(resource)
	} else if prefix == "srt" {
		u = cloneURL(n.srtAddress)
	} else {
		return nil, fmt.Errorf("unknown prefix")
	}

	return u, nil
}

func (n *node) GetFile(prefix, path string, offset int64) (io.ReadCloser, error) {
	n.peerLock.RLock()
	defer n.peerLock.RUnlock()

	if n.peer == nil {
		return nil, ErrNoPeer
	}

	return n.peer.FilesystemGetFileOffset(prefix, path, offset)
}

func (n *node) GetFileInfo(prefix, path string) (int64, time.Time, error) {
	n.peerLock.RLock()
	defer n.peerLock.RUnlock()

	if n.peer == nil {
		return 0, time.Time{}, ErrNoPeer
	}

	info, err := n.peer.FilesystemList(prefix, path, "", "")
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("file not found: %w", err)
	}

	if len(info) != 1 {
		return 0, time.Time{}, fmt.Errorf("ambigous result")
	}

	return info[0].Size, time.Unix(info[0].LastMod, 0), nil
}

func (n *node) ProcessList(options ProcessListOptions) ([]clientapi.Process, error) {
	n.peerLock.RLock()
	defer n.peerLock.RUnlock()

	if n.peer == nil {
		return nil, ErrNoPeer
	}

	return n.peer.ProcessList(client.ProcessListOptions{
		ID:            options.ID,
		Filter:        options.Filter,
		Domain:        options.Domain,
		Reference:     options.Reference,
		IDPattern:     options.IDPattern,
		RefPattern:    options.RefPattern,
		OwnerPattern:  options.OwnerPattern,
		DomainPattern: options.DomainPattern,
	})
}

func (n *node) ProxyProcessList() ([]Process, error) {
	list, err := n.ProcessList(ProcessListOptions{
		Filter: []string{"config", "state", "metadata"},
	})
	if err != nil {
		return nil, err
	}

	nodeid := n.About().ID

	processes := []Process{}

	for _, p := range list {
		process := Process{
			NodeID:    nodeid,
			Order:     p.State.Order,
			State:     p.State.State,
			Mem:       p.State.Memory,
			CPU:       p.State.CPU * n.resources.ncpu,
			Runtime:   time.Duration(p.State.Runtime) * time.Second,
			UpdatedAt: time.Unix(p.UpdatedAt, 0),
			Metadata:  p.Metadata,
		}

		cfg := &app.Config{
			ID:             p.Config.ID,
			Owner:          p.Config.Owner,
			Domain:         p.Config.Domain,
			Reference:      p.Config.Reference,
			Input:          []app.ConfigIO{},
			Output:         []app.ConfigIO{},
			Options:        p.Config.Options,
			Reconnect:      p.Config.Reconnect,
			ReconnectDelay: p.Config.ReconnectDelay,
			Autostart:      p.Config.Autostart,
			StaleTimeout:   p.Config.StaleTimeout,
			Timeout:        p.Config.Timeout,
			Scheduler:      p.Config.Scheduler,
			LogPatterns:    p.Config.LogPatterns,
			LimitCPU:       p.Config.Limits.CPU,
			LimitMemory:    p.Config.Limits.Memory * 1024 * 1024,
			LimitWaitFor:   p.Config.Limits.WaitFor,
		}

		for _, d := range p.Config.Input {
			cfg.Input = append(cfg.Input, app.ConfigIO{
				ID:      d.ID,
				Address: d.Address,
				Options: d.Options,
			})
		}

		for _, d := range p.Config.Output {
			output := app.ConfigIO{
				ID:      d.ID,
				Address: d.Address,
				Options: d.Options,
				Cleanup: []app.ConfigIOCleanup{},
			}

			for _, c := range d.Cleanup {
				output.Cleanup = append(output.Cleanup, app.ConfigIOCleanup{
					Pattern:       c.Pattern,
					MaxFiles:      c.MaxFiles,
					MaxFileAge:    c.MaxFileAge,
					PurgeOnDelete: c.PurgeOnDelete,
				})
			}

			cfg.Output = append(cfg.Output, output)
		}

		process.Config = cfg

		processes = append(processes, process)
	}

	return processes, nil
}

func (n *node) AddProcess(config *app.Config, metadata map[string]interface{}) error {
	n.peerLock.RLock()
	defer n.peerLock.RUnlock()

	if n.peer == nil {
		return ErrNoPeer
	}

	cfg := convertConfig(config, metadata)

	return n.peer.ProcessAdd(cfg)
}

func convertConfig(config *app.Config, metadata map[string]interface{}) clientapi.ProcessConfig {
	cfg := clientapi.ProcessConfig{
		ID:             config.ID,
		Owner:          config.Owner,
		Domain:         config.Domain,
		Type:           "ffmpeg",
		Reference:      config.Reference,
		Input:          []clientapi.ProcessConfigIO{},
		Output:         []clientapi.ProcessConfigIO{},
		Options:        config.Options,
		Reconnect:      config.Reconnect,
		ReconnectDelay: config.ReconnectDelay,
		Autostart:      config.Autostart,
		StaleTimeout:   config.StaleTimeout,
		Timeout:        config.Timeout,
		Scheduler:      config.Scheduler,
		LogPatterns:    config.LogPatterns,
		Limits: clientapi.ProcessConfigLimits{
			CPU:     config.LimitCPU,
			Memory:  config.LimitMemory / 1024 / 1024,
			WaitFor: config.LimitWaitFor,
		},
		Metadata: metadata,
	}

	for _, d := range config.Input {
		cfg.Input = append(cfg.Input, clientapi.ProcessConfigIO{
			ID:      d.ID,
			Address: d.Address,
			Options: d.Options,
		})
	}

	for _, d := range config.Output {
		output := clientapi.ProcessConfigIO{
			ID:      d.ID,
			Address: d.Address,
			Options: d.Options,
			Cleanup: []clientapi.ProcessConfigIOCleanup{},
		}

		for _, c := range d.Cleanup {
			output.Cleanup = append(output.Cleanup, clientapi.ProcessConfigIOCleanup{
				Pattern:       c.Pattern,
				MaxFiles:      c.MaxFiles,
				MaxFileAge:    c.MaxFileAge,
				PurgeOnDelete: c.PurgeOnDelete,
			})
		}

		cfg.Output = append(cfg.Output, output)
	}

	return cfg
}

func (n *node) StartProcess(id app.ProcessID) error {
	n.peerLock.RLock()
	defer n.peerLock.RUnlock()

	if n.peer == nil {
		return ErrNoPeer
	}

	return n.peer.ProcessCommand(client.NewProcessID(id.ID, id.Domain), "start")
}

func (n *node) StopProcess(id app.ProcessID) error {
	n.peerLock.RLock()
	defer n.peerLock.RUnlock()

	if n.peer == nil {
		return ErrNoPeer
	}

	return n.peer.ProcessCommand(client.NewProcessID(id.ID, id.Domain), "stop")
}

func (n *node) RestartProcess(id app.ProcessID) error {
	n.peerLock.RLock()
	defer n.peerLock.RUnlock()

	if n.peer == nil {
		return ErrNoPeer
	}

	return n.peer.ProcessCommand(client.NewProcessID(id.ID, id.Domain), "restart")
}

func (n *node) ReloadProcess(id app.ProcessID) error {
	n.peerLock.RLock()
	defer n.peerLock.RUnlock()

	if n.peer == nil {
		return ErrNoPeer
	}

	return n.peer.ProcessCommand(client.NewProcessID(id.ID, id.Domain), "reload")
}

func (n *node) DeleteProcess(id app.ProcessID) error {
	n.peerLock.RLock()
	defer n.peerLock.RUnlock()

	if n.peer == nil {
		return ErrNoPeer
	}

	return n.peer.ProcessDelete(client.NewProcessID(id.ID, id.Domain))
}

func (n *node) UpdateProcess(id app.ProcessID, config *app.Config, metadata map[string]interface{}) error {
	n.peerLock.RLock()
	defer n.peerLock.RUnlock()

	if n.peer == nil {
		return ErrNoPeer
	}

	cfg := convertConfig(config, metadata)

	return n.peer.ProcessUpdate(client.NewProcessID(id.ID, id.Domain), cfg)
}

func (n *node) ProbeProcess(id app.ProcessID) (clientapi.Probe, error) {
	n.peerLock.RLock()
	defer n.peerLock.RUnlock()

	if n.peer == nil {
		probe := clientapi.Probe{
			Log: []string{fmt.Sprintf("the node %s where the process %s resides, is not connected", n.id, id.String())},
		}
		return probe, ErrNoPeer
	}

	probe, err := n.peer.ProcessProbe(client.NewProcessID(id.ID, id.Domain))

	probe.Log = append([]string{fmt.Sprintf("probed on node: %s", n.id)}, probe.Log...)

	return probe, err
}
