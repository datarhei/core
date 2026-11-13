package cluster

import (
	"fmt"

	"github.com/datarhei/core/v16/cluster/store"
	clusterzone "github.com/datarhei/core/v16/cluster/zone"
	"github.com/datarhei/core/v16/zone"
)

// Zones returns a zone manager that is backed by the cluster store. The manager is
// read-only, use ZoneAdd, ZoneUpdate, and ZoneRemove for modifications. The zone limits
// of the processes are checked by the cluster store.
func (c *cluster) Zones() (zone.Manager, error) {
	m, err := clusterzone.New(c.store)
	if err != nil {
		return nil, fmt.Errorf("cluster zones: %w", err)
	}

	return m, nil
}

func invalidZone(err error) error {
	return fmt.Errorf("%w%w", err, store.ErrBadRequest)
}

func (c *cluster) ZoneAdd(origin string, z zone.Zone) error {
	if err := zone.Validate(z); err != nil {
		return invalidZone(err)
	}

	if !c.IsRaftLeader() {
		return c.forwarder.ZoneAdd(origin, z)
	}

	cmd := &store.Command{
		Operation: store.OpAddZone,
		Data: &store.CommandAddZone{
			Zone: z,
		},
	}

	return c.applyCommand(cmd)
}

func (c *cluster) ZoneUpdate(origin string, z zone.Zone) error {
	if err := zone.Validate(z); err != nil {
		return invalidZone(err)
	}

	if !c.IsRaftLeader() {
		return c.forwarder.ZoneUpdate(origin, z)
	}

	cmd := &store.Command{
		Operation: store.OpUpdateZone,
		Data: &store.CommandUpdateZone{
			Zone: z,
		},
	}

	return c.applyCommand(cmd)
}

func (c *cluster) ZoneRemove(origin, id string) error {
	if !c.IsRaftLeader() {
		return c.forwarder.ZoneRemove(origin, id)
	}

	cmd := &store.Command{
		Operation: store.OpRemoveZone,
		Data: &store.CommandRemoveZone{
			ID: id,
		},
	}

	return c.applyCommand(cmd)
}
