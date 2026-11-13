// Package zone provides a zone manager that is backed by the cluster store.
package zone

import (
	"fmt"

	"github.com/datarhei/core/v16/cluster/store"
	"github.com/datarhei/core/v16/zone"
)

// ErrClusterMode is returned for any modification. In cluster mode the zones can only
// be modified through the cluster, and the zone limits are checked by the cluster.
var ErrClusterMode = fmt.Errorf("%w: not available in cluster mode", zone.ErrReadOnly)

type manager struct {
	store store.Store
}

// New returns a zone manager that reads the zones and their usage from the cluster store.
// All modifications and claims return ErrClusterMode. Release and Register do nothing.
func New(store store.Store) (zone.Manager, error) {
	if store == nil {
		return nil, fmt.Errorf("no store provided")
	}

	return &manager{
		store: store,
	}, nil
}

func (m *manager) List() []zone.Info {
	return m.store.ZoneList()
}

func (m *manager) Get(id string) (zone.Info, error) {
	return m.store.ZoneGet(id)
}

func (m *manager) Add(z zone.Zone) error {
	return ErrClusterMode
}

func (m *manager) Update(z zone.Zone) error {
	return ErrClusterMode
}

func (m *manager) Remove(id string) error {
	return ErrClusterMode
}

func (m *manager) Acquire(prev, next *zone.Claim) error {
	return ErrClusterMode
}

func (m *manager) Release(c *zone.Claim) {}

func (m *manager) Register(c *zone.Claim) {}
