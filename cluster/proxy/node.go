package proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/datarhei/core/v16/client"
	httpapi "github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/restream/app"
)

type Node interface {
	Connect() error
	Disconnect()

	StartFiles(updates chan<- NodeFiles) error
	StopFiles()

	GetURL(path string) (string, error)
	GetFile(path string) (io.ReadCloser, error)

	ProcessList() ([]ProcessConfig, error)
	ProcessAdd(*app.Config) error
	ProcessStart(id string) error
	ProcessStop(id string) error
	ProcessDelete(id string) error

	NodeReader
}

type NodeReader interface {
	ID() string
	Address() string
	IPs() []string
	Files() NodeFiles
	State() NodeState
}

type NodeFiles struct {
	ID         string
	Files      []string
	LastUpdate time.Time
}

type NodeResources struct {
	NCPU     float64
	CPU      float64
	CPULimit float64
	Mem      float64
	MemTotal float64
	MemLimit float64
}

type NodeState struct {
	ID          string
	State       string
	LastContact time.Time
	Latency     time.Duration
	Resources   NodeResources
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
	cancelPing context.CancelFunc

	lastContact time.Time

	resources struct {
		ncpu     float64
		cpu      float64
		mem      float64
		memTotal float64
	}

	state       nodeState
	latency     float64 // Seconds
	stateLock   sync.RWMutex
	updates     chan<- NodeFiles
	filesList   []string
	lastUpdate  time.Time
	cancelFiles context.CancelFunc

	runningLock sync.Mutex
	running     bool

	host          string
	secure        bool
	hasRTMP       bool
	rtmpAddress   string
	rtmpToken     string
	hasSRT        bool
	srtAddress    string
	srtPassphrase string
	srtToken      string
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
		Address:    n.address,
		Auth0Token: "",
		Client: &http.Client{
			Timeout: 5 * time.Second,
		},
	})
	if err != nil {
		return fmt.Errorf("creating client failed (%s): %w", n.address, err)
	}

	config, err := peer.Config()
	if err != nil {
		return err
	}

	if config.Config.RTMP.Enable {
		n.hasRTMP = true
		n.rtmpAddress = "rtmp://"

		isHostIP := net.ParseIP(host) != nil

		address := config.Config.RTMP.Address
		if n.secure && config.Config.RTMP.EnableTLS && !isHostIP {
			address = config.Config.RTMP.AddressTLS
			n.rtmpAddress = "rtmps://"
		}

		_, port, err := net.SplitHostPort(address)
		if err != nil {
			n.hasRTMP = false
		} else {
			n.rtmpAddress += host + ":" + port
			n.rtmpToken = config.Config.RTMP.Token
		}
	}

	if config.Config.SRT.Enable {
		n.hasSRT = true
		n.srtAddress = "srt://"

		_, port, err := net.SplitHostPort(config.Config.SRT.Address)
		if err != nil {
			n.hasSRT = false
		} else {
			n.srtAddress += host + ":" + port
			n.srtPassphrase = config.Config.SRT.Passphrase
			n.srtToken = config.Config.SRT.Token
		}
	}

	n.ips = addrs
	n.host = host

	n.peer = peer

	ctx, cancel := context.WithCancel(context.Background())
	n.cancelPing = cancel

	go func(ctx context.Context) {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// Ping
				ok, latency := n.peer.Ping()

				n.stateLock.Lock()
				if !ok {
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

		for {
			select {
			case <-ticker.C:
				// Metrics
				metrics, err := n.peer.Metrics(httpapi.MetricsQuery{
					Metrics: []httpapi.MetricsQueryMetric{
						{
							Name: "cpu_ncpu",
						},
						{
							Name: "cpu_idle",
						},
						{
							Name: "mem_total",
						},
						{
							Name: "mem_free",
						},
					},
				})
				if err != nil {
					n.stateLock.Lock()
					n.resources.cpu = 100
					n.resources.ncpu = 1
					n.resources.mem = 100
					n.resources.memTotal = 1
					n.stateLock.Unlock()
				}

				cpu_ncpu := .0
				cpu_idle := .0
				mem_total := .0
				mem_free := .0

				for _, x := range metrics.Metrics {
					if x.Name == "cpu_idle" {
						cpu_idle = x.Values[0].Value
					} else if x.Name == "cpu_ncpu" {
						cpu_ncpu = x.Values[0].Value
					} else if x.Name == "mem_total" {
						mem_total = x.Values[0].Value
					} else if x.Name == "mem_free" {
						mem_free = x.Values[0].Value
					}
				}

				n.stateLock.Lock()
				n.resources.ncpu = cpu_ncpu
				n.resources.cpu = 100 - cpu_idle
				if mem_total != 0 {
					n.resources.mem = (mem_total - mem_free) / mem_total * 100
					n.resources.memTotal = mem_total
				} else {
					n.resources.mem = 100
					n.resources.memTotal = 1
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

func (n *node) Disconnect() {
	n.peerLock.Lock()
	defer n.peerLock.Unlock()

	if n.cancelPing != nil {
		n.cancelPing()
		n.cancelPing = nil
	}

	n.peer = nil
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

func (n *node) Address() string {
	return n.address
}

func (n *node) IPs() []string {
	return n.ips
}

func (n *node) ID() string {
	n.peerLock.RLock()
	defer n.peerLock.RUnlock()

	if n.peer == nil {
		return ""
	}

	return n.peer.ID()
}

func (n *node) Files() NodeFiles {
	n.stateLock.RLock()
	defer n.stateLock.RUnlock()

	state := NodeFiles{
		ID:         n.ID(),
		LastUpdate: n.lastUpdate,
	}

	if n.state != stateDisconnected && time.Since(n.lastUpdate) <= 2*time.Second {
		state.Files = make([]string, len(n.filesList))
		copy(state.Files, n.filesList)
	}

	return state
}

func (n *node) State() NodeState {
	n.stateLock.RLock()
	defer n.stateLock.RUnlock()

	state := NodeState{
		ID:          n.ID(),
		LastContact: n.lastContact,
		State:       n.state.String(),
		Latency:     time.Duration(n.latency * float64(time.Second)),
		Resources: NodeResources{
			NCPU:     n.resources.ncpu,
			CPU:      n.resources.cpu,
			CPULimit: 90,
			Mem:      n.resources.mem,
			MemTotal: n.resources.memTotal,
			MemLimit: 90,
		},
	}

	return state
}

func (n *node) files() {
	filesChan := make(chan string, 1024)
	filesList := []string{}

	wgList := sync.WaitGroup{}
	wgList.Add(1)

	go func() {
		defer wgList.Done()

		for file := range filesChan {
			if len(file) == 0 {
				return
			}

			filesList = append(filesList, file)
		}
	}()

	wg := sync.WaitGroup{}
	wg.Add(2)

	go func(f chan<- string) {
		defer wg.Done()

		n.peerLock.RLock()
		defer n.peerLock.RUnlock()

		if n.peer == nil {
			return
		}

		files, err := n.peer.MemFSList("name", "asc")
		if err != nil {
			return
		}

		for _, file := range files {
			f <- "mem:" + file.Name
		}
	}(filesChan)

	go func(f chan<- string) {
		defer wg.Done()

		n.peerLock.RLock()
		defer n.peerLock.RUnlock()

		if n.peer == nil {
			return
		}

		files, err := n.peer.DiskFSList("name", "asc")
		if err != nil {
			return
		}

		for _, file := range files {
			f <- "disk:" + file.Name
		}
	}(filesChan)

	if n.hasRTMP {
		wg.Add(1)

		go func(f chan<- string) {
			defer wg.Done()

			n.peerLock.RLock()
			defer n.peerLock.RUnlock()

			if n.peer == nil {
				return
			}

			files, err := n.peer.RTMPChannels()
			if err != nil {
				return
			}

			for _, file := range files {
				f <- "rtmp:" + file.Name
			}
		}(filesChan)
	}

	if n.hasSRT {
		wg.Add(1)

		go func(f chan<- string) {
			defer wg.Done()

			n.peerLock.RLock()
			defer n.peerLock.RUnlock()

			if n.peer == nil {
				return
			}

			files, err := n.peer.SRTChannels()
			if err != nil {
				return
			}

			for _, file := range files {
				f <- "srt:" + file.Name
			}
		}(filesChan)
	}

	wg.Wait()

	filesChan <- ""

	wgList.Wait()

	n.stateLock.Lock()

	n.filesList = make([]string, len(filesList))
	copy(n.filesList, filesList)
	n.lastUpdate = time.Now()
	n.lastContact = time.Now()

	n.stateLock.Unlock()
}

func (n *node) GetURL(path string) (string, error) {
	prefix, path, found := strings.Cut(path, ":")
	if !found {
		return "", fmt.Errorf("no prefix provided")
	}

	u := ""

	if prefix == "mem" {
		u = n.address + "/" + filepath.Join("memfs", path)
	} else if prefix == "disk" {
		u = n.address + path
	} else if prefix == "rtmp" {
		u = n.rtmpAddress + path
		if len(n.rtmpToken) != 0 {
			u += "?token=" + url.QueryEscape(n.rtmpToken)
		}
	} else if prefix == "srt" {
		u = n.srtAddress + "?mode=caller"
		if len(n.srtPassphrase) != 0 {
			u += "&passphrase=" + url.QueryEscape(n.srtPassphrase)
		}
		streamid := "#!:m=request,r=" + path
		if len(n.srtToken) != 0 {
			streamid += ",token=" + n.srtToken
		}
		u += "&streamid=" + url.QueryEscape(streamid)
	} else {
		return "", fmt.Errorf("unknown prefix")
	}

	return u, nil
}

func (n *node) GetFile(path string) (io.ReadCloser, error) {
	prefix, path, found := strings.Cut(path, ":")
	if !found {
		return nil, fmt.Errorf("no prefix provided")
	}

	n.peerLock.RLock()
	defer n.peerLock.RUnlock()

	if n.peer == nil {
		return nil, fmt.Errorf("not connected")
	}

	if prefix == "mem" {
		return n.peer.MemFSGetFile(path)
	} else if prefix == "disk" {
		return n.peer.DiskFSGetFile(path)
	}

	return nil, fmt.Errorf("unknown prefix")
}

func (n *node) ProcessList() ([]ProcessConfig, error) {
	n.peerLock.RLock()
	defer n.peerLock.RUnlock()

	if n.peer == nil {
		return nil, fmt.Errorf("not connected")
	}

	list, err := n.peer.ProcessList(nil, []string{
		"state",
		"config",
	})
	if err != nil {
		return nil, err
	}

	processes := []ProcessConfig{}

	for _, p := range list {
		process := ProcessConfig{
			NodeID:  n.ID(),
			Order:   p.State.Order,
			State:   p.State.State,
			Mem:     float64(p.State.Memory) / float64(n.resources.memTotal),
			Runtime: time.Duration(p.State.Runtime) * time.Second,
			Config:  p.Config.Marshal(),
		}

		if x, err := p.State.CPU.Float64(); err == nil {
			process.CPU = x / n.resources.ncpu
		} else {
			process.CPU = 100
		}

		processes = append(processes, process)
	}

	return processes, nil
}

func (n *node) ProcessAdd(config *app.Config) error {
	n.peerLock.RLock()
	defer n.peerLock.RUnlock()

	if n.peer == nil {
		return fmt.Errorf("not connected")
	}

	cfg := httpapi.ProcessConfig{}
	cfg.Unmarshal(config)

	return n.peer.ProcessAdd(cfg)
}

func (n *node) ProcessStart(id string) error {
	n.peerLock.RLock()
	defer n.peerLock.RUnlock()

	if n.peer == nil {
		return fmt.Errorf("not connected")
	}

	return n.peer.ProcessCommand(id, "start")
}

func (n *node) ProcessStop(id string) error {
	n.peerLock.RLock()
	defer n.peerLock.RUnlock()

	if n.peer == nil {
		return fmt.Errorf("not connected")
	}

	return n.peer.ProcessCommand(id, "stop")
}

func (n *node) ProcessDelete(id string) error {
	n.peerLock.RLock()
	defer n.peerLock.RUnlock()

	if n.peer == nil {
		return fmt.Errorf("not connected")
	}

	return n.peer.ProcessDelete(id)
}
