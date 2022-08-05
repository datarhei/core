package cluster

import (
	"context"
	"net/http"
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
}

func newNode(address, username, password string, updates chan<- NodeState) (*node, error) {
	n := &node{
		address:  address,
		username: username,
		password: password,
		state:    stateDisconnected,
		updates:  updates,
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

	n.lastUpdate = time.Now()

	if errMemfs != nil || errDiskfs != nil {
		n.fileList = nil
		n.state = stateDisconnected
		return
	}

	n.state = stateConnected

	n.fileList = make([]string, len(memfsfiles)+len(diskfsfiles))

	nfiles := 0

	for _, file := range memfsfiles {
		n.fileList[nfiles] = "memfs:" + file.Name
		nfiles++
	}

	for _, file := range diskfsfiles {
		n.fileList[nfiles] = "diskfs:" + file.Name
		nfiles++
	}

	return
}
