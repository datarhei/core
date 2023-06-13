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

	"github.com/datarhei/core/v16/restream/app"

	client "github.com/datarhei/core-client-go/v16"
	clientapi "github.com/datarhei/core-client-go/v16/api"
)

type Node interface {
	Connect() error
	Disconnect()

	Config() clientapi.ConfigV3

	StartFiles(updates chan<- NodeFiles) error
	StopFiles()

	GetURL(prefix, path string) (*url.URL, error)
	GetFile(prefix, path string) (io.ReadCloser, error)

	AddProcess(config *app.Config, metadata map[string]interface{}) error
	StartProcess(id app.ProcessID) error
	StopProcess(id app.ProcessID) error
	DeleteProcess(id app.ProcessID) error
	UpdateProcess(id app.ProcessID, config *app.Config, metadata map[string]interface{}) error

	NodeReader
}

type NodeReader interface {
	IPs() []string
	Ping() (time.Duration, error)
	About() NodeAbout
	Version() NodeVersion
	Resources() NodeResources

	Files() NodeFiles
	ProcessList() ([]Process, error)
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

type node struct {
	address string
	ips     []string

	peer       client.RestClient
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

	config clientapi.ConfigV3

	state       nodeState
	latency     float64 // Seconds
	stateLock   sync.RWMutex
	updates     chan<- NodeFiles
	filesList   []string
	lastUpdate  time.Time
	cancelFiles context.CancelFunc

	runningLock sync.Mutex
	running     bool

	secure      bool
	httpAddress *url.URL
	hasRTMP     bool
	rtmpAddress *url.URL
	hasSRT      bool
	srtAddress  *url.URL
}

func NewNode(address string) Node {
	n := &node{
		address: address,
		state:   stateDisconnected,
		secure:  strings.HasPrefix(address, "https://"),
	}

	return n
}

func (n *node) Connect() error {
	n.peerLock.Lock()
	defer n.peerLock.Unlock()

	if n.peer != nil {
		return nil
	}

	u, err := url.Parse(n.address)
	if err != nil {
		return fmt.Errorf("invalid address: %w", err)
	}

	host, _, err := net.SplitHostPort(u.Host)
	if err != nil {
		return fmt.Errorf("invalid address: %w", err)
	}

	addrs, err := net.LookupHost(host)
	if err != nil {
		return fmt.Errorf("lookup failed: %w", err)
	}

	peer, err := client.New(client.Config{
		Address: n.address,
		Client: &http.Client{
			Timeout: 5 * time.Second,
		},
	})
	if err != nil {
		return fmt.Errorf("creating client failed (%s): %w", n.address, err)
	}

	version, cfg, err := peer.Config()
	if err != nil {
		return err
	}

	if version != 3 {
		return fmt.Errorf("unsupported core config version: %d", version)
	}

	config, ok := cfg.Config.(clientapi.ConfigV3)
	if !ok {
		return fmt.Errorf("failed to convert config to expected version")
	}

	n.config = config

	n.httpAddress = u

	if config.RTMP.Enable {
		n.hasRTMP = true
		n.rtmpAddress = &url.URL{}
		n.rtmpAddress.Scheme = "rtmp:"

		isHostIP := net.ParseIP(host) != nil

		address := config.RTMP.Address
		if n.secure && config.RTMP.EnableTLS && !isHostIP {
			address = config.RTMP.AddressTLS
			n.rtmpAddress.Scheme = "rtmps:"
		}

		n.rtmpAddress.JoinPath(config.RTMP.App)

		n.rtmpAddress.Host = address
	}

	if config.SRT.Enable {
		n.hasSRT = true
		n.srtAddress = &url.URL{}
		n.srtAddress.Scheme = "srt:"
		n.srtAddress.Host = config.SRT.Address

		v := url.Values{}

		v.Set("mode", "caller")
		if len(config.SRT.Passphrase) != 0 {
			v.Set("passphrase", config.SRT.Passphrase)
		}

		n.srtAddress.RawQuery = v.Encode()
	}

	n.ips = addrs

	n.peer = peer

	ctx, cancel := context.WithCancel(context.Background())
	n.disconnect = cancel

	n.peerWg.Add(2)

	go func(ctx context.Context) {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		defer n.peerWg.Done()

		for {
			select {
			case <-ticker.C:
				// Ping
				latency, err := n.Ping()

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
	}(ctx)

	go func(ctx context.Context) {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		defer n.peerWg.Done()

		for {
			select {
			case <-ticker.C:
				// Metrics
				metrics, err := n.peer.Metrics(clientapi.MetricsQuery{
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
	}(ctx)

	return nil
}

func (n *node) Config() clientapi.ConfigV3 {
	return n.config
}

func (n *node) Ping() (time.Duration, error) {
	n.peerLock.RLock()
	defer n.peerLock.RUnlock()

	if n.peer == nil {
		return 0, fmt.Errorf("not connected")
	}

	ok, latency := n.peer.Ping()
	var err error = nil

	if !ok {
		err = fmt.Errorf("not connected")
	}

	return latency, err
}

func (n *node) Metrics(query clientapi.MetricsQuery) (clientapi.MetricsResponse, error) {
	n.peerLock.RLock()
	defer n.peerLock.RUnlock()

	if n.peer == nil {
		return clientapi.MetricsResponse{}, fmt.Errorf("not connected")
	}

	return n.peer.Metrics(query)
}

func (n *node) AboutPeer() (clientapi.About, error) {
	n.peerLock.RLock()
	defer n.peerLock.RUnlock()

	if n.peer == nil {
		return clientapi.About{}, fmt.Errorf("not connected")
	}

	return n.peer.About(), nil
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
	n.peerLock.Unlock()
}

func (n *node) StartFiles(updates chan<- NodeFiles) error {
	n.runningLock.Lock()
	defer n.runningLock.Unlock()

	if n.running {
		return nil
	}

	n.running = true
	n.updates = updates

	ctx, cancel := context.WithCancel(context.Background())
	n.cancelFiles = cancel

	go func(ctx context.Context) {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				n.files()

				select {
				case n.updates <- n.Files():
				default:
				}
			}
		}
	}(ctx)

	return nil
}

func (n *node) StopFiles() {
	n.runningLock.Lock()
	defer n.runningLock.Unlock()

	if !n.running {
		return
	}

	n.running = false

	n.cancelFiles()
}

func (n *node) About() NodeAbout {
	about, err := n.AboutPeer()
	if err != nil {
		return NodeAbout{
			State: stateDisconnected.String(),
		}
	}

	createdAt, err := time.Parse(time.RFC3339, about.CreatedAt)
	if err != nil {
		createdAt = time.Now()
	}

	n.stateLock.RLock()
	defer n.stateLock.RUnlock()

	state := n.state
	if time.Since(n.lastContact) > 3*time.Second {
		state = stateDisconnected
	}

	nodeAbout := NodeAbout{
		ID:          about.ID,
		Name:        about.Name,
		Address:     n.address,
		State:       state.String(),
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
	}

	return nodeAbout
}

func (n *node) Resources() NodeResources {
	n.stateLock.RLock()
	defer n.stateLock.RUnlock()

	r := NodeResources{
		IsThrottling: n.resources.throttling,
		NCPU:         n.resources.ncpu,
		CPU:          n.resources.cpu,
		CPULimit:     n.resources.cpuLimit,
		Mem:          n.resources.mem,
		MemLimit:     n.resources.memLimit,
	}

	return r
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

func (n *node) IPs() []string {
	return n.ips
}

func (n *node) Files() NodeFiles {
	id := n.About().ID

	n.stateLock.RLock()
	defer n.stateLock.RUnlock()

	state := NodeFiles{
		ID:         id,
		LastUpdate: n.lastUpdate,
	}

	if n.state != stateDisconnected && time.Since(n.lastUpdate) <= 2*time.Second {
		state.Files = make([]string, len(n.filesList))
		copy(state.Files, n.filesList)
	}

	return state
}

var errNoPeer = errors.New("no peer")

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
			e <- errNoPeer
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
			e <- errNoPeer
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
				e <- errNoPeer
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
				e <- errNoPeer
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
		u.JoinPath("memfs", resource)
	} else if prefix == "disk" {
		u = cloneURL(n.httpAddress)
		u.JoinPath(resource)
	} else if prefix == "rtmp" {
		u = cloneURL(n.rtmpAddress)
		u.JoinPath(resource)
	} else if prefix == "srt" {
		u = cloneURL(n.srtAddress)
	} else {
		return nil, fmt.Errorf("unknown prefix")
	}

	return u, nil
}

func (n *node) GetFile(prefix, path string) (io.ReadCloser, error) {
	n.peerLock.RLock()
	defer n.peerLock.RUnlock()

	if n.peer == nil {
		return nil, fmt.Errorf("not connected")
	}

	return n.peer.FilesystemGetFile(prefix, path)
}

func (n *node) ProcessList() ([]Process, error) {
	id := n.About().ID

	n.peerLock.RLock()
	defer n.peerLock.RUnlock()

	if n.peer == nil {
		return nil, fmt.Errorf("not connected")
	}

	list, err := n.peer.ProcessList(client.ProcessListOptions{
		Filter: []string{
			"state",
			"config",
			"metadata",
		},
	})
	if err != nil {
		return nil, err
	}

	processes := []Process{}

	for _, p := range list {
		process := Process{
			NodeID:    id,
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
		return fmt.Errorf("not connected")
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
		return fmt.Errorf("not connected")
	}

	return n.peer.ProcessCommand(client.NewProcessID(id.ID, id.Domain), "start")
}

func (n *node) StopProcess(id app.ProcessID) error {
	n.peerLock.RLock()
	defer n.peerLock.RUnlock()

	if n.peer == nil {
		return fmt.Errorf("not connected")
	}

	return n.peer.ProcessCommand(client.NewProcessID(id.ID, id.Domain), "stop")
}

func (n *node) DeleteProcess(id app.ProcessID) error {
	n.peerLock.RLock()
	defer n.peerLock.RUnlock()

	if n.peer == nil {
		return fmt.Errorf("not connected")
	}

	return n.peer.ProcessDelete(client.NewProcessID(id.ID, id.Domain))
}

func (n *node) UpdateProcess(id app.ProcessID, config *app.Config, metadata map[string]interface{}) error {
	n.peerLock.RLock()
	defer n.peerLock.RUnlock()

	if n.peer == nil {
		return fmt.Errorf("not connected")
	}

	cfg := convertConfig(config, metadata)

	return n.peer.ProcessUpdate(client.NewProcessID(id.ID, id.Domain), cfg)
}
