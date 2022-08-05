package cluster

import (
	"context"
	"fmt"
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
	State() NodeState
}

type NodeState struct {
	ID         string
	State      string
	Files      []string
	LastUpdate time.Time
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
	state      nodeState
	username   string
	password   string
	updates    chan<- NodeState
	peer       client.RestClient
	fileList   []string
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
	srtPort       string
	srtPassphrase string
	srtToken      string

	prefix *regexp.Regexp
}

func newNode(address, username, password string, updates chan<- NodeState) (*node, error) {
	u, err := url.Parse(address)
	if err != nil {
		return nil, fmt.Errorf("invalid address: %w", err)
	}

	host, _, err := net.SplitHostPort(u.Host)
	if err != nil {
		return nil, fmt.Errorf("invalid address: %w", err)
	}

	n := &node{
		address:  address,
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

		_, port, err := net.SplitHostPort(config.Config.SRT.Address)
		if err != nil {
			n.hasSRT = false
		} else {
			n.srtPort = port
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
				n.lock.Lock()
				n.files()
				n.lock.Unlock()

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
		state.Files = make([]string, len(n.fileList))
		copy(state.Files, n.fileList)
	}

	return state
}

func (n *node) stop() {
	n.once.Do(func() { n.cancel() })
}

func (n *node) files() {
	memfsfiles, errMemfs := n.peer.MemFSList("name", "asc")
	diskfsfiles, errDiskfs := n.peer.DiskFSList("name", "asc")
	rtmpfiles, errRTMP := n.peer.RTMPChannels()

	n.lastUpdate = time.Now()

	if errMemfs != nil || errDiskfs != nil || errRTMP != nil {
		n.fileList = nil
		n.state = stateDisconnected
		return
	}

	n.state = stateConnected

	n.fileList = make([]string, len(memfsfiles)+len(diskfsfiles)+len(rtmpfiles))

	nfiles := 0

	for _, file := range memfsfiles {
		n.fileList[nfiles] = "memfs:" + file.Name
		nfiles++
	}

	for _, file := range diskfsfiles {
		n.fileList[nfiles] = "diskfs:" + file.Name
		nfiles++
	}

	for _, file := range rtmpfiles {
		n.fileList[nfiles] = "rtmp:" + file.Name
		nfiles++
	}

	return
}

func (n *node) GetURL(path string) (string, error) {
	// Remove prefix from path
	prefix := n.prefix.FindString(path)
	path = n.prefix.ReplaceAllString(path, "")

	u := ""

	if prefix == "memfs:" {
		u = n.address + "/" + filepath.Join("memfs", path)
	} else if prefix == "diskfs:" {
		u = n.address + path
	} else if prefix == "rtmp:" {
		u = n.rtmpAddress + path
		if len(n.rtmpToken) != 0 {
			u += "?token=" + url.QueryEscape(n.rtmpToken)
		}
	} else {
		return "", fmt.Errorf("unknown prefix")
	}

	return u, nil
}
