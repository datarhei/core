package store

import "time"

func (s *store) setNodeState(cmd CommandSetNodeState) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if cmd.State == "online" {
		delete(s.data.Nodes, cmd.NodeID)
		return nil
	}

	node := s.data.Nodes[cmd.NodeID]

	node.State = cmd.State
	node.UpdatedAt = time.Now()

	s.data.Nodes[cmd.NodeID] = node

	return nil
}

func (s *store) NodeList() map[string]Node {
	s.lock.RLock()
	defer s.lock.RUnlock()

	m := map[string]Node{}

	for id, node := range s.data.Nodes {
		m[id] = node
	}

	return m
}
