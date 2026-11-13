package zone

import (
	"testing"

	"github.com/datarhei/core/v16/cluster/store"
	"github.com/datarhei/core/v16/encoding/json"
	"github.com/datarhei/core/v16/restream/app"
	"github.com/datarhei/core/v16/zone"

	"github.com/hashicorp/raft"
	"github.com/stretchr/testify/require"
)

func apply(t *testing.T, s store.Store, op store.Operation, data any) {
	b, err := json.Marshal(store.Command{Operation: op, Data: data})
	require.NoError(t, err)

	res := s.Apply(&raft.Log{Data: b})
	if err, ok := res.(error); ok {
		require.NoError(t, err)
	}
}

func TestZones(t *testing.T) {
	s, err := store.NewStore(store.Config{})
	require.NoError(t, err)

	_, err = New(nil)
	require.Error(t, err)

	m, err := New(s)
	require.NoError(t, err)

	require.Equal(t, []zone.Info{{Zone: zone.Zone{ID: zone.DefaultID}}}, m.List())

	apply(t, s, store.OpAddZone, store.CommandAddZone{Zone: zone.Zone{ID: "a", Limits: zone.Resources{CPU: 100}}})
	apply(t, s, store.OpAddProcess, store.CommandAddProcess{Config: &app.Config{ID: "p1", Zone: "a", LimitCPU: 60, LimitMemory: 1}})

	info, err := m.Get("a")
	require.NoError(t, err)
	require.Equal(t, zone.Info{
		Zone:      zone.Zone{ID: "a", Limits: zone.Resources{CPU: 100}},
		Usage:     zone.Resources{CPU: 60, Memory: 1},
		Processes: 1,
	}, info)

	require.Len(t, m.List(), 2)

	_, err = m.Get("x")
	require.ErrorIs(t, err, zone.ErrNotFound)

	// Read-only
	require.ErrorIs(t, m.Add(zone.Zone{ID: "b"}), zone.ErrReadOnly)
	require.ErrorIs(t, m.Update(zone.Zone{ID: "a"}), zone.ErrReadOnly)
	require.ErrorIs(t, m.Remove("a"), zone.ErrReadOnly)
	require.ErrorIs(t, m.Acquire(nil, &zone.Claim{Zone: "a"}), zone.ErrReadOnly)

	m.Register(&zone.Claim{Zone: "a", Resources: zone.Resources{CPU: 10}})
	m.Release(&zone.Claim{Zone: "a", Resources: zone.Resources{CPU: 60}})

	info, err = m.Get("a")
	require.NoError(t, err)
	require.Equal(t, float64(60), info.Usage.CPU)
}
