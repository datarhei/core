package cluster

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/datarhei/core/v16/client"
)

type Node interface {
	Connect() error
	Disconnect()

	Start(updates chan<- NodeState) error
	Stop()

	GetURL(path string) (string, error)
	GetFile(path string) (io.ReadCloser, error)

	NodeReader
}

type NodeReader interface {
	ID() string
	Address() string
	IPs() []string
	State() NodeState
}

type NodeState struct {
	ID         string
	State      string
	Files      []string
	LastPing   time.Time
	LastUpdate time.Time
	Latency    time.Duration
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
	lastPing   time.Time
	cancelPing context.CancelFunc

	state       nodeState
	latency     float64 // Seconds
	stateLock   sync.RWMutex
	updates     chan<- NodeState
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

	prefix *regexp.Regexp
}

func NewNode(address string) Node {
	n := &node{
		address: address,
		state:   stateDisconnected,
		prefix:  regexp.MustCompile(`^[a-z]+:`),
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
				// ping
				ok, latency := n.peer.Ping()

				n.stateLock.Lock()
				if !ok {
					n.state = stateDisconnected
				} else {
					n.lastPing = time.Now()
					n.state = stateConnected
				}
				n.latency = n.latency*0.2 + latency.Seconds()*0.8
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

func (n *node) Start(updates chan<- NodeState) error {
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
				case n.updates <- n.State():
				default:
				}
			}
		}
	}(ctx)

	return nil
}

func (n *node) Stop() {
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
	return n.peer.ID()
}

func (n *node) State() NodeState {
	n.stateLock.RLock()
	defer n.stateLock.RUnlock()

	state := NodeState{
		ID:         n.peer.ID(),
		LastPing:   n.lastPing,
		LastUpdate: n.lastUpdate,
		Latency:    time.Duration(n.latency * float64(time.Second)),
	}

	if n.state == stateDisconnected || time.Since(n.lastUpdate) > 2*time.Second {
		state.State = stateDisconnected.String()
	} else {
		state.State = n.state.String()
		state.Files = make([]string, len(n.filesList))
		copy(state.Files, n.filesList)
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

	n.stateLock.Unlock()
}

func (n *node) GetURL(path string) (string, error) {
	// Remove prefix from path
	prefix := n.prefix.FindString(path)
	path = n.prefix.ReplaceAllString(path, "")

	u := ""

	if prefix == "mem:" {
		u = n.address + "/" + filepath.Join("memfs", path)
	} else if prefix == "disk:" {
		u = n.address + path
	} else if prefix == "rtmp:" {
		u = n.rtmpAddress + path
		if len(n.rtmpToken) != 0 {
			u += "?token=" + url.QueryEscape(n.rtmpToken)
		}
	} else if prefix == "srt:" {
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
	// Remove prefix from path
	prefix := n.prefix.FindString(path)
	path = n.prefix.ReplaceAllString(path, "")

	n.peerLock.RLock()
	defer n.peerLock.RUnlock()

	if prefix == "mem:" {
		return n.peer.MemFSGetFile(path)
	} else if prefix == "disk:" {
		return n.peer.DiskFSGetFile(path)
	}

	return nil, fmt.Errorf("unknown prefix")
}
