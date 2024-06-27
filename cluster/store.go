package cluster

import (
	"github.com/datarhei/core/v16/cluster/store"
	"github.com/datarhei/core/v16/restream/app"
)

func (c *cluster) StoreListProcesses() []store.Process {
	return c.store.ListProcesses()
}

func (c *cluster) StoreGetProcess(id app.ProcessID) (store.Process, error) {
	return c.store.GetProcess(id)
}
