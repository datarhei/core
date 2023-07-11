package cluster

import (
	"fmt"

	"github.com/datarhei/core/v16/cluster/store"
	"github.com/datarhei/core/v16/restream/app"
)

func (c *cluster) ListProcesses() []store.Process {
	return c.store.ListProcesses()
}

func (c *cluster) GetProcess(id app.ProcessID) (store.Process, error) {
	return c.store.GetProcess(id)
}

func (c *cluster) AddProcess(origin string, config *app.Config) error {
	if ok, _ := c.IsDegraded(); ok {
		return ErrDegraded
	}

	if !c.IsRaftLeader() {
		return c.forwarder.AddProcess(origin, config)
	}

	cmd := &store.Command{
		Operation: store.OpAddProcess,
		Data: &store.CommandAddProcess{
			Config: config,
		},
	}

	return c.applyCommand(cmd)
}

func (c *cluster) RemoveProcess(origin string, id app.ProcessID) error {
	if ok, _ := c.IsDegraded(); ok {
		return ErrDegraded
	}

	if !c.IsRaftLeader() {
		return c.forwarder.RemoveProcess(origin, id)
	}

	cmd := &store.Command{
		Operation: store.OpRemoveProcess,
		Data: &store.CommandRemoveProcess{
			ID: id,
		},
	}

	return c.applyCommand(cmd)
}

func (c *cluster) UpdateProcess(origin string, id app.ProcessID, config *app.Config) error {
	if ok, _ := c.IsDegraded(); ok {
		return ErrDegraded
	}

	if !c.IsRaftLeader() {
		return c.forwarder.UpdateProcess(origin, id, config)
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

func (c *cluster) SetProcessCommand(origin string, id app.ProcessID, command string) error {
	if ok, _ := c.IsDegraded(); ok {
		return ErrDegraded
	}

	if command == "start" || command == "stop" {
		if !c.IsRaftLeader() {
			return c.forwarder.SetProcessCommand(origin, id, command)
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

	nodeid, err := c.proxy.FindNodeFromProcess(id)
	if err != nil {
		return fmt.Errorf("the process '%s' is not registered with any node: %w", id.String(), err)
	}

	return c.proxy.CommandProcess(nodeid, id, command)
}

func (c *cluster) SetProcessMetadata(origin string, id app.ProcessID, key string, data interface{}) error {
	if ok, _ := c.IsDegraded(); ok {
		return ErrDegraded
	}

	if !c.IsRaftLeader() {
		return c.forwarder.SetProcessMetadata(origin, id, key, data)
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

func (c *cluster) GetProcessNodeMap() map[string]string {
	return c.store.GetProcessNodeMap()
}
