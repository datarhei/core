package cluster

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/hashicorp/raft"
)

type Store interface {
	raft.FSM

	ListNodes() []string
	GetNode(id string) string
}

type operation string

const (
	opAddNode       operation = "addNode"
	opRemoveNode    operation = "removeNode"
	opAddProcess    operation = "addProcess"
	opRemoveProcess operation = "removeProcess"
)

type command struct {
	Operation operation
	Data      interface{}
}

type addNodeCommand struct {
	ID       string
	Address  string
	Username string
	Password string
}

type removeNodeCommand struct {
	ID string
}

type addProcessCommand struct {
	Config []byte
}

// Implement a FSM
type store struct{}

func NewStore() (Store, error) {
	return &store{}, nil
}

func (s *store) Apply(log *raft.Log) interface{} {
	fmt.Printf("a log entry came in (index=%d, term=%d): %s\n", log.Index, log.Term, string(log.Data))

	c := command{}

	err := json.Unmarshal(log.Data, &c)
	if err != nil {
		fmt.Printf("invalid log entry\n")
		return nil
	}

	fmt.Printf("op: %s\n", c.Operation)
	fmt.Printf("op: %+v\n", c)

	switch c.Operation {
	case opAddNode:
		b, _ := json.Marshal(c.Data)
		cmd := addNodeCommand{}
		json.Unmarshal(b, &cmd)

		fmt.Printf("addNode: %+v\n", cmd)
	case opRemoveNode:
		b, _ := json.Marshal(c.Data)
		cmd := removeNodeCommand{}
		json.Unmarshal(b, &cmd)

		fmt.Printf("removeNode: %+v\n", cmd)
	}
	return nil
}

func (s *store) Snapshot() (raft.FSMSnapshot, error) {
	fmt.Printf("a snapshot is requested\n")
	return &fsmSnapshot{}, nil
}

func (s *store) Restore(snapshot io.ReadCloser) error {
	fmt.Printf("a snapshot is restored\n")
	return nil
}

func (s *store) ListNodes() []string {
	return nil
}

func (s *store) GetNode(id string) string {
	return ""
}

type fsmSnapshot struct{}

func (s *fsmSnapshot) Persist(sink raft.SnapshotSink) error {
	return nil
}

func (s *fsmSnapshot) Release() {}
