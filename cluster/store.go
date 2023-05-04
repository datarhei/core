package cluster

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/hashicorp/raft"
)

type Store interface {
	raft.FSM

	ListNodes() []StoreNode
	GetNode(id string) (StoreNode, error)
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
	ID      string
	Address string
}

type removeNodeCommand struct {
	ID string
}

type StoreNode struct {
	ID      string
	Address string
}

type addProcessCommand struct {
	Config []byte
}

// Implement a FSM
type store struct {
	lock  sync.RWMutex
	Nodes map[string]string
}

func NewStore() (Store, error) {
	return &store{
		Nodes: map[string]string{},
	}, nil
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

		s.lock.Lock()
		s.Nodes[cmd.ID] = cmd.Address
		s.lock.Unlock()
	case opRemoveNode:
		b, _ := json.Marshal(c.Data)
		cmd := removeNodeCommand{}
		json.Unmarshal(b, &cmd)

		fmt.Printf("removeNode: %+v\n", cmd)

		s.lock.Lock()
		delete(s.Nodes, cmd.ID)
		s.lock.Unlock()
	}

	s.lock.RLock()
	fmt.Printf("\n==> %+v\n\n", s.Nodes)
	s.lock.RUnlock()
	return nil
}

func (s *store) Snapshot() (raft.FSMSnapshot, error) {
	fmt.Printf("a snapshot is requested\n")

	s.lock.Lock()
	defer s.lock.Unlock()

	data, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}

	return &fsmSnapshot{
		data: data,
	}, nil
}

func (s *store) Restore(snapshot io.ReadCloser) error {
	fmt.Printf("a snapshot is restored\n")

	defer snapshot.Close()

	s.lock.Lock()
	defer s.lock.Unlock()

	dec := json.NewDecoder(snapshot)
	if err := dec.Decode(s); err != nil {
		return err
	}

	return nil
}

func (s *store) ListNodes() []StoreNode {
	nodes := []StoreNode{}

	s.lock.Lock()
	defer s.lock.Unlock()

	for id, address := range s.Nodes {
		nodes = append(nodes, StoreNode{
			ID:      id,
			Address: address,
		})
	}

	return nodes
}

func (s *store) GetNode(id string) (StoreNode, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	address, ok := s.Nodes[id]
	if !ok {
		return StoreNode{}, fmt.Errorf("not found")
	}

	return StoreNode{
		ID:      id,
		Address: address,
	}, nil
}

type fsmSnapshot struct {
	data []byte
}

func (s *fsmSnapshot) Persist(sink raft.SnapshotSink) error {
	if _, err := sink.Write(s.data); err != nil {
		sink.Cancel()
		return err
	}

	sink.Close()
	return nil
}

func (s *fsmSnapshot) Release() {
	s.data = nil
}
