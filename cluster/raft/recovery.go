package raft

import (
	"io"
	"sync"
	"time"
)

type RaftRecoverer interface {
	Raft

	Recover([]Peer, time.Duration) error
}

type raftRecoverer struct {
	raft   Raft
	config Config

	lock sync.RWMutex
}

func NewRecoverer(config Config) (RaftRecoverer, error) {
	r := &raftRecoverer{
		config: config,
	}

	raft, err := New(config)
	if err != nil {
		return nil, err
	}

	r.raft = raft

	return r, nil
}

func (r *raftRecoverer) Recover(peers []Peer, cooldown time.Duration) error {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.raft.Shutdown()

	if r.config.Logger != nil {
		r.config.Logger.Warn().WithField("duration", cooldown).Log("Cooling down")
	}

	time.Sleep(cooldown)

	r.config.Peers = peers

	raft, err := New(r.config)
	if err != nil {
		return err
	}

	r.raft = raft

	return nil
}

func (r *raftRecoverer) Shutdown() {
	r.lock.RLock()
	defer r.lock.RUnlock()

	r.raft.Shutdown()
}

func (r *raftRecoverer) IsLeader() bool {
	r.lock.RLock()
	defer r.lock.RUnlock()

	return r.raft.IsLeader()
}

func (r *raftRecoverer) Leader() (string, string) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	return r.raft.Leader()
}

func (r *raftRecoverer) Servers() ([]Server, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	return r.raft.Servers()
}

func (r *raftRecoverer) Stats() Stats {
	r.lock.RLock()
	defer r.lock.RUnlock()

	return r.raft.Stats()
}

func (r *raftRecoverer) Apply(data []byte) error {
	r.lock.RLock()
	defer r.lock.RUnlock()

	return r.raft.Apply(data)
}

func (r *raftRecoverer) Barrier(timeout time.Duration) error {
	r.lock.RLock()
	defer r.lock.RUnlock()

	return r.raft.Barrier(timeout)
}

func (r *raftRecoverer) AddServer(id, address string) error {
	r.lock.RLock()
	defer r.lock.RUnlock()

	return r.raft.AddServer(id, address)
}

func (r *raftRecoverer) RemoveServer(id string) error {
	r.lock.RLock()
	defer r.lock.RUnlock()

	return r.raft.RemoveServer(id)
}

func (r *raftRecoverer) LeadershipTransfer(id string) error {
	r.lock.RLock()
	defer r.lock.RUnlock()

	return r.raft.LeadershipTransfer(id)
}

func (r *raftRecoverer) Snapshot() (io.ReadCloser, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	return r.raft.Snapshot()
}
