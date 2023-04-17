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
	case "addNode":
		b, _ := json.Marshal(c.Data)
		cmd := addNodeCommand{}
		json.Unmarshal(b, &cmd)

		fmt.Printf("addNode: %+v\n", cmd)
	case "removeNode":
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

type fsmSnapshot struct{}

func (s *fsmSnapshot) Persist(sink raft.SnapshotSink) error {
	return nil
}

func (s *fsmSnapshot) Release() {}
