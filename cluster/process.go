package cluster

import (
	"fmt"

	"github.com/datarhei/core/v16/cluster/store"
	"github.com/datarhei/core/v16/restream/app"
)

func (c *cluster) ProcessAdd(origin string, config *app.Config) error {
	if !c.IsRaftLeader() {
		return c.forwarder.ProcessAdd(origin, config)
	}

	cmd := &store.Command{
		Operation: store.OpAddProcess,
		Data: &store.CommandAddProcess{
			Config: config,
		},
	}

	return c.applyCommand(cmd)
}

func (c *cluster) ProcessGet(origin string, id app.ProcessID, stale bool) (store.Process, string, error) {
	if !stale {
		if !c.IsRaftLeader() {
			return c.forwarder.ProcessGet(origin, id)
		}
	}

	process, nodeid, err := c.store.ProcessGet(id)
	if err != nil {
		return store.Process{}, "", err
	}

	return process, nodeid, nil
}

func (c *cluster) ProcessRemove(origin string, id app.ProcessID) error {
	if !c.IsRaftLeader() {
		return c.forwarder.ProcessRemove(origin, id)
	}

	cmd := &store.Command{
		Operation: store.OpRemoveProcess,
		Data: &store.CommandRemoveProcess{
			ID: id,
		},
	}

	return c.applyCommand(cmd)
}

func (c *cluster) ProcessUpdate(origin string, id app.ProcessID, config *app.Config) error {
	if !c.IsRaftLeader() {
		return c.forwarder.ProcessUpdate(origin, id, config)
	}

	cmd := &store.Command{
		Operation: store.OpUpdateProcess,
		Data: &store.CommandUpdateProcess{
			ID:     id,
			Config: config,
		},
	}

	return c.applyCommand(cmd)
}

func (c *cluster) ProcessSetCommand(origin string, id app.ProcessID, command string) error {
	if command == "start" || command == "stop" {
		if !c.IsRaftLeader() {
			return c.forwarder.ProcessSetCommand(origin, id, command)
		}

		cmd := &store.Command{
			Operation: store.OpSetProcessOrder,
			Data: &store.CommandSetProcessOrder{
				ID:    id,
				Order: command,
			},
		}

		return c.applyCommand(cmd)
	}

	nodeid, err := c.store.ProcessGetNode(id)
	if err != nil {
		return fmt.Errorf("the process '%s' is not registered with any node: %w", id.String(), err)
	}

	return c.manager.ProcessCommand(nodeid, id, command)
}

func (c *cluster) ProcessesRelocate(origin string, relocations map[app.ProcessID]string) error {
	if !c.IsRaftLeader() {
		return c.forwarder.ProcessesRelocate(origin, relocations)
	}

	cmd := &store.Command{
		Operation: store.OpSetRelocateProcess,
		Data: &store.CommandSetRelocateProcess{
			Map: relocations,
		},
	}

	return c.applyCommand(cmd)
}

func (c *cluster) ProcessSetMetadata(origin string, id app.ProcessID, key string, data interface{}) error {
	if !c.IsRaftLeader() {
		return c.forwarder.ProcessSetMetadata(origin, id, key, data)
	}

	cmd := &store.Command{
		Operation: store.OpSetProcessMetadata,
		Data: &store.CommandSetProcessMetadata{
			ID:   id,
			Key:  key,
			Data: data,
		},
	}

	return c.applyCommand(cmd)
}

func (c *cluster) ProcessGetMetadata(origin string, id app.ProcessID, key string) (interface{}, error) {
	p, _, err := c.store.ProcessGet(id)
	if err != nil {
		return nil, err
	}

	if len(key) == 0 {
		return p.Metadata, nil
	}

	data, ok := p.Metadata[key]
	if !ok {
		return nil, fmt.Errorf("unknown key")
	}

	return data, nil
}
