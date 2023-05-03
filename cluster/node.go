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

type NodeReader interface {
	Address() string
	IPs() []string
	State() NodeState
}

type NodeState struct {
	ID         string
	State      string
	Files      []string
	LastUpdate time.Time
}

type NodeSpecs struct {
	ID      string
	Address string
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
	address    string
	ips        []string
	state      nodeState
	username   string
	password   string
	updates    chan<- NodeState
	peer       client.RestClient
	filesList  []string
	lastUpdate time.Time
	lock       sync.RWMutex
	cancel     context.CancelFunc
	once       sync.Once

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

func newNode(address string, updates chan<- NodeState) (*node, error) {
	u, err := url.Parse(address)
	if err != nil {
		return nil, fmt.Errorf("invalid address: %w", err)
	}

	host, _, err := net.SplitHostPort(u.Host)
	if err != nil {
		return nil, fmt.Errorf("invalid address: %w", err)
	}

	addrs, err := net.LookupHost(host)
	if err != nil {
		return nil, fmt.Errorf("lookup failed: %w", err)
	}

	username := u.User.Username()
	password := ""
	if pw, ok := u.User.Password(); ok {
		password = pw
	}

	n := &node{
		address:  address,
		ips:      addrs,
		username: username,
		password: password,
		state:    stateDisconnected,
		updates:  updates,
		prefix:   regexp.MustCompile(`^[a-z]+:`),
		host:     host,
		secure:   strings.HasPrefix(address, "https://"),
	}

	peer, err := client.New(client.Config{
		Address:    address,
		Username:   username,
		Password:   password,
		Auth0Token: "",
		Client: &http.Client{
			Timeout: 5 * time.Second,
		},
	})
	if err != nil {
		return nil, err
	}

	config, err := peer.Config()
	if err != nil {
		return nil, err
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

	n.peer = peer

	ctx, cancel := context.WithCancel(context.Background())
	n.cancel = cancel

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

	return n, nil
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
	n.lock.RLock()
	defer n.lock.RUnlock()

	state := NodeState{
		ID:         n.peer.ID(),
		LastUpdate: n.lastUpdate,
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

func (n *node) stop() {
	n.once.Do(func() { n.cancel() })
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

	n.lock.Lock()

	n.filesList = make([]string, len(filesList))
	copy(n.filesList, filesList)
	n.lastUpdate = time.Now()
	n.state = stateConnected

	n.lock.Unlock()
}

func (n *node) getURL(path string) (string, error) {
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

func (n *node) getFile(path string) (io.ReadCloser, error) {
	// Remove prefix from path
	prefix := n.prefix.FindString(path)
	path = n.prefix.ReplaceAllString(path, "")

	if prefix == "mem:" {
		return n.peer.MemFSGetFile(path)
	} else if prefix == "disk:" {
		return n.peer.DiskFSGetFile(path)
	}

	return nil, fmt.Errorf("unknown prefix")
}
