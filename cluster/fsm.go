package cluster

import (
	"io"

	"github.com/hashicorp/raft"
)

// Implement a FSM
type fsm struct{}

func NewFSM() (raft.FSM, error) {
	return &fsm{}, nil
}

func (f *fsm) Apply(*raft.Log) interface{} {
	return nil
}

func (f *fsm) Snapshot() (raft.FSMSnapshot, error) {
	return &fsmSnapshot{}, nil
}

func (f *fsm) Restore(snapshot io.ReadCloser) error {
	return nil
}

type fsmSnapshot struct{}

func (s *fsmSnapshot) Persist(sink raft.SnapshotSink) error {
	return nil
}

func (s *fsmSnapshot) Release() {}
