package store

import (
	"bytes"
	"io"
	"testing"

	"github.com/datarhei/core/v16/encoding/json"
	"github.com/datarhei/core/v16/restream/app"
	"github.com/datarhei/core/v16/zone"

	"github.com/stretchr/testify/require"
)

func zoneProcessConfig(id, zone string, cpu float64) *app.Config {
	return &app.Config{
		ID:          id,
		Zone:        zone,
		LimitCPU:    cpu,
		LimitMemory: 1,
	}
}

func TestAddZoneCommand(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	err = s.applyCommand(Command{
		Operation: OpAddZone,
		Data: CommandAddZone{
			Zone: zone.Zone{ID: "a", Limits: zone.Resources{CPU: 100}},
		},
	})
	require.NoError(t, err)

	info, err := s.ZoneGet("a")
	require.NoError(t, err)
	require.Equal(t, zone.Info{Zone: zone.Zone{ID: "a", Limits: zone.Resources{CPU: 100}}}, info)
}

func TestAddUpdateRemoveZone(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	// The default zone always exists
	require.Equal(t, []zone.Info{{Zone: zone.Zone{ID: zone.DefaultID}}}, s.ZoneList())

	require.NoError(t, s.addZone(CommandAddZone{Zone: zone.Zone{ID: "a", Limits: zone.Resources{CPU: 100}}}))

	err = s.addZone(CommandAddZone{Zone: zone.Zone{ID: "a"}})
	require.ErrorIs(t, err, zone.ErrExists)
	require.ErrorIs(t, err, ErrConflict)

	err = s.addZone(CommandAddZone{Zone: zone.Zone{ID: zone.DefaultID}})
	require.ErrorIs(t, err, ErrConflict)

	err = s.addZone(CommandAddZone{Zone: zone.Zone{ID: "a b"}})
	require.ErrorIs(t, err, zone.ErrInvalid)
	require.ErrorIs(t, err, ErrBadRequest)

	require.NoError(t, s.updateZone(CommandUpdateZone{Zone: zone.Zone{ID: "a", Limits: zone.Resources{CPU: 200}}}))
	require.NoError(t, s.updateZone(CommandUpdateZone{Zone: zone.Zone{ID: zone.DefaultID, Limits: zone.Resources{CPU: 300}}}))

	err = s.updateZone(CommandUpdateZone{Zone: zone.Zone{ID: "x"}})
	require.ErrorIs(t, err, ErrNotFound)

	err = s.updateZone(CommandUpdateZone{Zone: zone.Zone{ID: "a", Limits: zone.Resources{CPU: -1}}})
	require.ErrorIs(t, err, ErrBadRequest)

	require.Equal(t, []zone.Info{
		{Zone: zone.Zone{ID: zone.DefaultID, Limits: zone.Resources{CPU: 300}}},
		{Zone: zone.Zone{ID: "a", Limits: zone.Resources{CPU: 200}}},
	}, s.ZoneList())

	_, err = s.ZoneGet("x")
	require.ErrorIs(t, err, zone.ErrNotFound)

	_, err = s.ZoneGet("")
	require.ErrorIs(t, err, zone.ErrNotFound)

	// A zone with processes can't be removed
	require.NoError(t, s.addProcess(CommandAddProcess{Config: zoneProcessConfig("p1", "a", 10)}))

	err = s.removeZone(CommandRemoveZone{ID: "a"})
	require.ErrorIs(t, err, zone.ErrInUse)
	require.ErrorIs(t, err, ErrConflict)

	require.NoError(t, s.removeProcess(CommandRemoveProcess{ID: app.ProcessID{ID: "p1"}}))
	require.NoError(t, s.removeZone(CommandRemoveZone{ID: "a"}))

	err = s.removeZone(CommandRemoveZone{ID: "a"})
	require.ErrorIs(t, err, ErrNotFound)

	err = s.removeZone(CommandRemoveZone{ID: zone.DefaultID})
	require.ErrorIs(t, err, ErrBadRequest)

	err = s.removeZone(CommandRemoveZone{ID: ""})
	require.ErrorIs(t, err, ErrNotFound)
}

func TestZoneAddProcess(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	require.NoError(t, s.addZone(CommandAddZone{Zone: zone.Zone{ID: "a", Limits: zone.Resources{CPU: 100}}}))

	require.NoError(t, s.addProcess(CommandAddProcess{Config: zoneProcessConfig("p1", "a", 60)}))

	err = s.addProcess(CommandAddProcess{Config: zoneProcessConfig("p2", "a", 60)})
	require.ErrorIs(t, err, zone.ErrExceeded)
	require.ErrorIs(t, err, ErrBadRequest)

	err = s.addProcess(CommandAddProcess{Config: zoneProcessConfig("p3", "x", 10)})
	require.ErrorIs(t, err, zone.ErrNotFound)
	require.ErrorIs(t, err, ErrBadRequest)

	// The default zone is unlimited
	require.NoError(t, s.addProcess(CommandAddProcess{Config: zoneProcessConfig("p4", "", 1000)}))
	require.NoError(t, s.addProcess(CommandAddProcess{Config: zoneProcessConfig("p5", zone.DefaultID, 1000)}))

	info, err := s.ZoneGet("a")
	require.NoError(t, err)
	require.Equal(t, float64(60), info.Usage.CPU)
	require.Equal(t, 1, info.Processes)

	info, err = s.ZoneGet(zone.DefaultID)
	require.NoError(t, err)
	require.Equal(t, float64(2000), info.Usage.CPU)
	require.Equal(t, 2, info.Processes)

	require.Len(t, s.ProcessList(), 3)
}

func TestZoneUpdateProcess(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	require.NoError(t, s.addZone(CommandAddZone{Zone: zone.Zone{ID: "a", Limits: zone.Resources{CPU: 100}}}))
	require.NoError(t, s.addZone(CommandAddZone{Zone: zone.Zone{ID: "b"}}))

	pid := app.ProcessID{ID: "p1"}

	require.NoError(t, s.addProcess(CommandAddProcess{Config: zoneProcessConfig("p1", "a", 60)}))

	// Increase within the limit
	require.NoError(t, s.updateProcess(CommandUpdateProcess{ID: pid, Config: zoneProcessConfig("p1", "a", 90)}))

	// Increase above the limit
	err = s.updateProcess(CommandUpdateProcess{ID: pid, Config: zoneProcessConfig("p1", "a", 110)})
	require.ErrorIs(t, err, zone.ErrExceeded)

	p, _, err := s.ProcessGet(pid)
	require.NoError(t, err)
	require.Equal(t, float64(90), p.Config.LimitCPU)

	// Above the limit, an update that doesn't increase the resources is allowed
	require.NoError(t, s.updateZone(CommandUpdateZone{Zone: zone.Zone{ID: "a", Limits: zone.Resources{CPU: 50}}}))
	require.NoError(t, s.updateProcess(CommandUpdateProcess{ID: pid, Config: zoneProcessConfig("p1", "a", 80)}))

	// Move to another zone
	require.NoError(t, s.updateProcess(CommandUpdateProcess{ID: pid, Config: zoneProcessConfig("p1", "b", 200)}))

	info, _ := s.ZoneGet("a")
	require.Equal(t, 0, info.Processes)

	info, _ = s.ZoneGet("b")
	require.Equal(t, float64(200), info.Usage.CPU)
	require.Equal(t, 1, info.Processes)

	// Moving back exceeds the limit
	err = s.updateProcess(CommandUpdateProcess{ID: pid, Config: zoneProcessConfig("p1", "a", 60)})
	require.ErrorIs(t, err, zone.ErrExceeded)

	// Unknown zone
	err = s.updateProcess(CommandUpdateProcess{ID: pid, Config: zoneProcessConfig("p1", "x", 10)})
	require.ErrorIs(t, err, zone.ErrNotFound)
	require.ErrorIs(t, err, ErrBadRequest)

	// Changing the ID keeps the claim
	require.NoError(t, s.updateProcess(CommandUpdateProcess{ID: pid, Config: zoneProcessConfig("p2", "b", 200)}))

	info, _ = s.ZoneGet("b")
	require.Equal(t, float64(200), info.Usage.CPU)
	require.Equal(t, 1, info.Processes)
}

func TestZoneCheckDeterministic(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	require.NoError(t, s.addZone(CommandAddZone{Zone: zone.Zone{ID: "a", Limits: zone.Resources{CPU: 1}}}))

	// The sum of these values depends on the order: 0.1+0.2+0.3 != 0.3+0.2+0.1
	require.NoError(t, s.addProcess(CommandAddProcess{Config: zoneProcessConfig("p1", "a", 0.1)}))
	require.NoError(t, s.addProcess(CommandAddProcess{Config: zoneProcessConfig("p2", "a", 0.2)}))
	require.NoError(t, s.addProcess(CommandAddProcess{Config: zoneProcessConfig("p3", "a", 0.3)}))

	configs := s.zoneConfigs()
	require.Equal(t, "p1", configs[0].ID)
	require.Equal(t, "p2", configs[1].ID)
	require.Equal(t, "p3", configs[2].ID)

	next := zoneProcessConfig("p4", "a", 0.4)

	first := s.zoneCheck(nil, next)

	for range 100 {
		err := s.zoneCheck(nil, next)
		if first == nil {
			require.NoError(t, err)
		} else {
			require.EqualError(t, err, first.Error())
		}
	}
}

func TestZoneSnapshotRestore(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	require.NoError(t, s.addZone(CommandAddZone{Zone: zone.Zone{ID: "a", Limits: zone.Resources{CPU: 100}}}))
	require.NoError(t, s.addProcess(CommandAddProcess{Config: zoneProcessConfig("p1", "a", 80)}))

	// Lower the limit below the usage
	require.NoError(t, s.updateZone(CommandUpdateZone{Zone: zone.Zone{ID: "a", Limits: zone.Resources{CPU: 50}}}))

	snapshot, err := s.Snapshot()
	require.NoError(t, err)

	data := snapshot.(*fsmSnapshot).data

	ops := []Operation{}

	r, err := createStore()
	require.NoError(t, err)

	r.OnApply(func(op Operation) { ops = append(ops, op) })

	require.NoError(t, r.Restore(io.NopCloser(bytes.NewReader(data))))
	require.Equal(t, s.ZoneList(), r.ZoneList())
	require.Contains(t, ops, OpUpdateZone)

	info, err := r.ZoneGet("a")
	require.NoError(t, err)
	require.Equal(t, float64(80), info.Usage.CPU)
	require.Equal(t, float64(50), info.Limits.CPU)

	// Snapshot from before zones existed
	old := map[string]any{}
	require.NoError(t, json.Unmarshal(data, &old))
	delete(old, "Zones")

	data, err = json.Marshal(old)
	require.NoError(t, err)

	require.NoError(t, r.Restore(io.NopCloser(bytes.NewReader(data))))
	require.Equal(t, []zone.Info{{Zone: zone.Zone{ID: zone.DefaultID}}}, r.ZoneList())

	// The zones can be modified after the restore
	require.NoError(t, r.addZone(CommandAddZone{Zone: zone.Zone{ID: "b"}}))

	// Snapshot with null zones
	old["Zones"] = nil

	data, err = json.Marshal(old)
	require.NoError(t, err)

	require.NoError(t, r.Restore(io.NopCloser(bytes.NewReader(data))))
	require.NoError(t, r.addZone(CommandAddZone{Zone: zone.Zone{ID: "b"}}))
}
