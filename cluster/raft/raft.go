package raft

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	gonet "net"
	"path/filepath"
	"reflect"
	"strconv"
	"sync"
	"time"

	raftlogger "github.com/datarhei/core/v16/cluster/logger"
	"github.com/datarhei/core/v16/cluster/store"
	"github.com/datarhei/core/v16/log"
	"go.etcd.io/bbolt"

	"github.com/hashicorp/go-hclog"
	hcraft "github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb/v2"
)

type Raft interface {
	Shutdown()

	// IsLeader returns whether this node is the leader.
	IsLeader() bool

	// Leader returns the address and ID of the current leader.
	Leader() (string, string)

	// Servers returns the list of servers in the cluster.
	Servers() ([]Server, error)
	Stats() Stats
	Apply([]byte) error

	Barrier(time.Duration) error

	AddServer(id, address string) error
	RemoveServer(id string) error
	LeadershipTransfer(id string) error

	Snapshot() (io.ReadCloser, error)
}

type raft struct {
	id   string
	path string

	raft          *hcraft.Raft
	raftTransport *hcraft.NetworkTransport
	raftAddress   string
	raftNotifyCh  chan bool
	raftStore     *raftboltdb.BoltStore
	raftStart     time.Time

	shutdown     bool
	shutdownCh   chan struct{}
	shutdownLock sync.Mutex

	leadershipNotifyCh  chan bool
	leaderObservationCh chan string

	isLeader   bool
	leaderLock sync.Mutex

	logger log.Logger
}

type Peer struct {
	ID      string
	Address string
}

type Server struct {
	ID      string
	Address string
	Voter   bool
	Leader  bool
}

type Stats struct {
	State       string
	LastContact time.Duration
	NumPeers    uint64
}

type Config struct {
	ID      string // ID of the node
	Path    string // Path where to store all cluster data
	Address string // Listen address for the raft protocol
	Peers   []Peer // Address of a member of a cluster to join

	Store hcraft.FSM

	// A channel where to push "true" if this server is the leader
	// and "false" if this server is not the leader.
	LeadershipNotifyCh chan bool

	// A channel where to push leader observations. An observation is
	// the address of the leader or an empty string if there's currently
	// no leader.
	LeaderObservationCh chan string

	Logger log.Logger
}

func New(config Config) (Raft, error) {
	r := &raft{
		id:                  config.ID,
		path:                config.Path,
		raftAddress:         config.Address,
		leadershipNotifyCh:  config.LeadershipNotifyCh,
		leaderObservationCh: config.LeaderObservationCh,
		shutdownCh:          make(chan struct{}),
		logger:              config.Logger,
	}

	if r.logger == nil {
		r.logger = log.New("")
	}

	err := r.start(config.Store, config.Peers, false)
	if err != nil {
		return nil, fmt.Errorf("failed to start raft: %w", err)
	}

	r.raftStart = time.Now()

	return r, nil
}

func (r *raft) Shutdown() {
	r.shutdownLock.Lock()
	defer r.shutdownLock.Unlock()

	if r.shutdown {
		return
	}

	r.shutdown = true
	close(r.shutdownCh)

	if r.raft != nil {
		r.raftTransport.Close()
		future := r.raft.Shutdown()
		if err := future.Error(); err != nil {
			r.logger.Warn().WithError(err).Log("Shutting down raft")
		}
		if r.raftStore != nil {
			r.raftStore.Close()
		}
	}
}

func (r *raft) IsLeader() bool {
	r.leaderLock.Lock()
	defer r.leaderLock.Unlock()

	return r.isLeader
}

func (r *raft) Leader() (string, string) {
	leaderAddress, leaderID := r.raft.LeaderWithID()

	return string(leaderAddress), string(leaderID)
}

func (r *raft) Servers() ([]Server, error) {
	future := r.raft.GetConfiguration()
	if err := future.Error(); err != nil {
		return nil, fmt.Errorf("failed to get raft configuration: %w", err)
	}

	_, leaderID := r.Leader()

	servers := []Server{}

	for _, server := range future.Configuration().Servers {
		node := Server{
			ID:      string(server.ID),
			Address: string(server.Address),
			Voter:   server.Suffrage == hcraft.Voter,
			Leader:  string(server.ID) == leaderID,
		}

		servers = append(servers, node)
	}

	return servers, nil
}

func (r *raft) Stats() Stats {
	stats := Stats{}

	s := r.raft.Stats()

	stats.State = s["state"]

	var lastContactSince time.Duration

	lastContact := s["last_contact"]
	if lastContact == "never" {
		lastContactSince = time.Since(r.raftStart)
	} else {
		if d, err := time.ParseDuration(lastContact); err == nil {
			lastContactSince = d
		} else {
			lastContactSince = time.Since(r.raftStart)
		}
	}

	stats.LastContact = lastContactSince

	if x, err := strconv.ParseUint(s["num_peers"], 10, 64); err == nil {
		stats.NumPeers = x
	}

	return stats
}

func (r *raft) Apply(data []byte) error {
	future := r.raft.Apply(data, 5*time.Second)
	if err := future.Error(); err != nil {
		return fmt.Errorf("applying command failed: %w", err)
	}

	res := future.Response()
	if res != nil {
		if err, ok := res.(error); ok {
			return err
		}
	}

	return nil
}

func (r *raft) Barrier(timeout time.Duration) error {
	future := r.raft.Barrier(timeout)
	if err := future.Error(); err != nil {
		return fmt.Errorf("error while waiting for the barrier: %w", err)
	}

	return nil
}

func (r *raft) AddServer(id, address string) error {
	future := r.raft.AddVoter(hcraft.ServerID(id), hcraft.ServerAddress(address), 0, 0)
	if err := future.Error(); err != nil {
		return fmt.Errorf("error adding server %s@%s: %w", id, address, err)
	}

	return nil
}

func (r *raft) RemoveServer(id string) error {
	future := r.raft.RemoveServer(hcraft.ServerID(id), 0, 0)
	if err := future.Error(); err != nil {
		return fmt.Errorf("error removing server %s: %w", id, err)
	}

	return nil
}

func (r *raft) LeadershipTransfer(id string) error {
	var future hcraft.Future

	if len(id) == 0 {
		future = r.raft.LeadershipTransfer()
	} else {
		servers, err := r.Servers()
		if err != nil {
			return err
		}

		for _, server := range servers {
			if server.ID != id {
				continue
			}

			future = r.raft.LeadershipTransferToServer(hcraft.ServerID(id), hcraft.ServerAddress(server.Address))
			break
		}
	}

	if err := future.Error(); err != nil {
		return fmt.Errorf("failed to transfer leadership: %w", err)
	}

	return nil
}

type readCloserWrapper struct {
	io.Reader
}

func (rcw *readCloserWrapper) Read(p []byte) (int, error) {
	return rcw.Reader.Read(p)
}

func (rcw *readCloserWrapper) Close() error {
	return nil
}

type Snapshot struct {
	Metadata *hcraft.SnapshotMeta
	Data     string
}

func (r *raft) Snapshot() (io.ReadCloser, error) {
	f := r.raft.Snapshot()
	err := f.Error()
	if err != nil {
		return nil, err
	}

	metadata, reader, err := f.Open()
	if err != nil {
		return nil, err
	}

	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to create snapshot: %w", err)
	}

	snapshot := Snapshot{
		Metadata: metadata,
		Data:     base64.StdEncoding.EncodeToString(data),
	}

	buffer := bytes.Buffer{}
	enc := json.NewEncoder(&buffer)
	err = enc.Encode(snapshot)
	if err != nil {
		return nil, err
	}

	return &readCloserWrapper{&buffer}, nil
}

func (r *raft) start(fsm hcraft.FSM, peers []Peer, inmem bool) error {
	defer func() {
		if r.raft == nil && r.raftStore != nil {
			r.raftStore.Close()
		}
	}()

	addr, err := gonet.ResolveTCPAddr("tcp", r.raftAddress)
	if err != nil {
		return err
	}

	r.logger.Debug().Log("Address: %s", addr)

	transport, err := hcraft.NewTCPTransportWithLogger(r.raftAddress, addr, 3, 10*time.Second, raftlogger.New(r.logger, hclog.Debug).Named("raft-transport"))
	if err != nil {
		return err
	}

	r.raftTransport = transport

	snapshotLogger := raftlogger.New(r.logger, hclog.Debug).Named("raft-snapshot")
	snapshots, err := hcraft.NewFileSnapshotStoreWithLogger(r.path, 3, snapshotLogger)
	if err != nil {
		return err
	}

	var logStore hcraft.LogStore
	var stableStore hcraft.StableStore
	if inmem {
		logStore = hcraft.NewInmemStore()
		stableStore = hcraft.NewInmemStore()
	} else {
		bolt, err := raftboltdb.New(raftboltdb.Options{
			Path: filepath.Join(r.path, "raftlog.db"),
			BoltOptions: &bbolt.Options{
				Timeout: 5 * time.Second,
			},
		})
		if err != nil {
			return fmt.Errorf("bolt: %w", err)
		}
		logStore = bolt
		stableStore = bolt

		cacheStore, err := hcraft.NewLogCache(512, logStore)
		if err != nil {
			return err
		}
		logStore = cacheStore

		r.raftStore = bolt
	}

	cfg := hcraft.DefaultConfig()
	cfg.LocalID = hcraft.ServerID(r.id)
	cfg.Logger = raftlogger.New(r.logger, hclog.Debug).Named("raft")

	hasState, err := hcraft.HasExistingState(logStore, stableStore, snapshots)
	if err != nil {
		return err
	}

	if !hasState {
		// Bootstrap cluster
		servers := []hcraft.Server{
			{
				Suffrage: hcraft.Voter,
				ID:       hcraft.ServerID(r.id),
				Address:  transport.LocalAddr(),
			},
		}

		for _, p := range peers {
			servers = append(servers, hcraft.Server{
				Suffrage: hcraft.Voter,
				ID:       hcraft.ServerID(p.ID),
				Address:  hcraft.ServerAddress(p.Address),
			})
		}

		configuration := hcraft.Configuration{
			Servers: servers,
		}

		if err := hcraft.BootstrapCluster(cfg, logStore, stableStore, snapshots, transport, configuration); err != nil {
			return fmt.Errorf("bootstrapping cluster: %w", err)
		}

		r.logger.Debug().Log("Raft node bootstrapped")
	} else {
		// Recover cluster
		fsm, err := store.NewStore(store.Config{})
		if err != nil {
			return err
		}

		servers := []hcraft.Server{
			{
				Suffrage: hcraft.Voter,
				ID:       hcraft.ServerID(r.id),
				Address:  transport.LocalAddr(),
			},
		}

		for _, p := range peers {
			servers = append(servers, hcraft.Server{
				Suffrage: hcraft.Voter,
				ID:       hcraft.ServerID(p.ID),
				Address:  hcraft.ServerAddress(p.Address),
			})
		}

		configuration := hcraft.Configuration{
			Servers: servers,
		}

		if err := hcraft.RecoverCluster(cfg, fsm, logStore, stableStore, snapshots, transport, configuration); err != nil {
			return fmt.Errorf("recovering cluster: %w", err)
		}

		r.logger.Debug().Log("Raft node recoverd")
	}

	// Set up a channel for reliable leader notifications.
	raftNotifyCh := make(chan bool, 10)
	cfg.NotifyCh = raftNotifyCh
	r.raftNotifyCh = raftNotifyCh

	node, err := hcraft.NewRaft(cfg, fsm, logStore, stableStore, snapshots, transport)
	if err != nil {
		return fmt.Errorf("creating raft: %w", err)
	}

	r.raft = node

	go r.trackLeaderChanges()
	go r.monitorLeadership()

	r.logger.Debug().Log("Raft started")

	return nil
}

func (r *raft) monitorLeadership() {
	for {
		select {
		case isLeader := <-r.raftNotifyCh:
			r.leaderLock.Lock()
			r.isLeader = isLeader
			r.leaderLock.Unlock()

			if r.leadershipNotifyCh != nil {
				select {
				case r.leadershipNotifyCh <- isLeader:
				default:
				}
			}

			r.logger.Debug().WithField("leader", isLeader).Log("leader notification")
		case <-r.shutdownCh:
			return
		}
	}
}

func (r *raft) trackLeaderChanges() {
	obsCh := make(chan hcraft.Observation, 16)
	observer := hcraft.NewObserver(obsCh, false, func(o *hcraft.Observation) bool {
		_, leaderOK := o.Data.(hcraft.LeaderObservation)
		return leaderOK
	})

	r.raft.RegisterObserver(observer)

	for {
		select {
		case obs := <-obsCh:
			if leaderObs, ok := obs.Data.(hcraft.LeaderObservation); ok {
				r.logger.Debug().WithFields(log.Fields{
					"id":      leaderObs.LeaderID,
					"address": leaderObs.LeaderAddr,
				}).Log("New leader observation")

				leaderAddress := string(leaderObs.LeaderAddr)

				if r.leaderObservationCh != nil {
					select {
					case r.leaderObservationCh <- leaderAddress:
					default:
					}
				}
			} else {
				r.logger.Debug().WithField("type", reflect.TypeOf(obs.Data)).Log("Unknown observation type from raft")
				continue
			}
		case <-r.shutdownCh:
			r.raft.DeregisterObserver(observer)
			return
		}
	}
}
