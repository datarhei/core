package restream

import (
	"testing"

	"github.com/datarhei/core/v16/io/fs"
	"github.com/datarhei/core/v16/log"
	"github.com/datarhei/core/v16/restream/app"
	jsonstore "github.com/datarhei/core/v16/restream/store/json"
	"github.com/datarhei/core/v16/zone"

	"github.com/stretchr/testify/require"
)

func newZoneManager(t *testing.T, zones ...zone.Zone) zone.Manager {
	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	m, err := zone.New(zone.Config{
		Filesystem: memfs,
		Filepath:   "/zones.json",
	})
	require.NoError(t, err)

	for _, z := range zones {
		require.NoError(t, m.Add(z))
	}

	return m
}

func getZoneProcess(id, zone string, cpu float64) *app.Config {
	process := getDummyProcess()
	process.ID = id
	process.Zone = zone
	process.LimitCPU = cpu

	return process
}

func requireZoneUsage(t *testing.T, zones zone.Manager, id string, cpu float64, processes int) {
	t.Helper()

	info, err := zones.Get(id)
	require.NoError(t, err)
	require.Equal(t, cpu, info.Usage.CPU, "cpu usage of zone '%s'", id)
	require.Equal(t, processes, info.Processes, "processes of zone '%s'", id)
}

func TestZoneAddProcess(t *testing.T) {
	zones := newZoneManager(t, zone.Zone{ID: "a", Limits: zone.Resources{CPU: 100}})

	rs, err := getDummyRestreamerWithZones(nil, nil, nil, nil, false, nil, zones)
	require.NoError(t, err)

	require.NoError(t, rs.AddProcess(getZoneProcess("p1", "a", 60)))
	requireZoneUsage(t, zones, "a", 60, 1)

	// Exceeds the limit
	err = rs.AddProcess(getZoneProcess("p2", "a", 60))
	require.ErrorIs(t, err, zone.ErrExceeded)

	_, err = rs.GetProcess(app.ProcessID{ID: "p2"})
	require.ErrorIs(t, err, ErrUnknownProcess)

	requireZoneUsage(t, zones, "a", 60, 1)

	// Unknown zone
	err = rs.AddProcess(getZoneProcess("p3", "x", 10))
	require.ErrorIs(t, err, zone.ErrNotFound)

	// The default zone is unlimited, both an empty zone and "$default" are the default zone
	require.NoError(t, rs.AddProcess(getZoneProcess("p4", "", 1000)))
	require.NoError(t, rs.AddProcess(getZoneProcess("p5", zone.DefaultID, 1000)))
	requireZoneUsage(t, zones, zone.DefaultID, 2000, 2)
}

func TestZoneAddProcessFailed(t *testing.T) {
	zones := newZoneManager(t, zone.Zone{ID: "a", Limits: zone.Resources{CPU: 100}})

	rs, err := getDummyRestreamerWithZones(nil, nil, nil, nil, false, nil, zones)
	require.NoError(t, err)

	require.NoError(t, rs.AddProcess(getZoneProcess("p1", "a", 60)))

	// The process exists already
	err = rs.AddProcess(getZoneProcess("p1", "a", 10))
	require.ErrorIs(t, err, ErrProcessExists)

	// Invalid process config
	err = rs.AddProcess(getZoneProcess("", "a", 10))
	require.Error(t, err)

	requireZoneUsage(t, zones, "a", 60, 1)
}

func TestZoneUpdateProcess(t *testing.T) {
	zones := newZoneManager(t,
		zone.Zone{ID: "a", Limits: zone.Resources{CPU: 100}},
		zone.Zone{ID: "b"},
	)

	rs, err := getDummyRestreamerWithZones(nil, nil, nil, nil, false, nil, zones)
	require.NoError(t, err)

	tid := app.ProcessID{ID: "p1"}

	require.NoError(t, rs.AddProcess(getZoneProcess("p1", "a", 60)))

	// Increase within the limit
	require.NoError(t, rs.UpdateProcess(tid, getZoneProcess("p1", "a", 90), false))
	requireZoneUsage(t, zones, "a", 90, 1)

	// Increase above the limit
	err = rs.UpdateProcess(tid, getZoneProcess("p1", "a", 110), false)
	require.ErrorIs(t, err, zone.ErrExceeded)
	requireZoneUsage(t, zones, "a", 90, 1)

	process, err := rs.GetProcess(tid)
	require.NoError(t, err)
	require.Equal(t, float64(90), process.Config.LimitCPU)

	// The zone is above its limit, an update that doesn't increase the resources is allowed
	require.NoError(t, zones.Update(zone.Zone{ID: "a", Limits: zone.Resources{CPU: 50}}))

	config := getZoneProcess("p1", "a", 90)
	config.ReconnectDelay = 42
	require.NoError(t, rs.UpdateProcess(tid, config, false))
	requireZoneUsage(t, zones, "a", 90, 1)

	// Move to another zone
	require.NoError(t, rs.UpdateProcess(tid, getZoneProcess("p1", "b", 200), false))
	requireZoneUsage(t, zones, "a", 0, 0)
	requireZoneUsage(t, zones, "b", 200, 1)

	// Moving back exceeds the limit
	err = rs.UpdateProcess(tid, getZoneProcess("p1", "a", 60), false)
	require.ErrorIs(t, err, zone.ErrExceeded)
	requireZoneUsage(t, zones, "b", 200, 1)

	// Changing the ID keeps the claim
	require.NoError(t, rs.UpdateProcess(tid, getZoneProcess("p2", "b", 200), false))
	requireZoneUsage(t, zones, "b", 200, 1)
}

func TestZoneDeleteProcess(t *testing.T) {
	zones := newZoneManager(t, zone.Zone{ID: "a", Limits: zone.Resources{CPU: 100}})

	rs, err := getDummyRestreamerWithZones(nil, nil, nil, nil, false, nil, zones)
	require.NoError(t, err)

	require.NoError(t, rs.AddProcess(getZoneProcess("p1", "a", 60)))
	require.ErrorIs(t, zones.Remove("a"), zone.ErrInUse)

	require.NoError(t, rs.DeleteProcess(app.ProcessID{ID: "p1"}, false))
	requireZoneUsage(t, zones, "a", 0, 0)

	require.NoError(t, zones.Remove("a"))
}

func TestZoneLoadProcesses(t *testing.T) {
	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	store, err := jsonstore.New(jsonstore.Config{
		Filesystem: memfs,
		Filepath:   "/db.json",
	})
	require.NoError(t, err)

	// Without a zone manager, the zones are not checked
	rs, err := getDummyRestreamerWithZones(nil, nil, nil, nil, false, store, nil)
	require.NoError(t, err)

	require.NoError(t, rs.AddProcess(getZoneProcess("p1", "a", 60)))
	require.NoError(t, rs.AddProcess(getZoneProcess("p2", "a", 60)))
	require.NoError(t, rs.AddProcess(getZoneProcess("p3", "", 10)))

	// The loaded processes claim their resources regardless of the limits
	zones := newZoneManager(t, zone.Zone{ID: "a", Limits: zone.Resources{CPU: 100}})

	rs, err = getDummyRestreamerWithZones(nil, nil, nil, nil, false, store, zones)
	require.NoError(t, err)

	process, err := rs.GetProcess(app.ProcessID{ID: "p1"})
	require.NoError(t, err)
	require.Equal(t, "a", process.Config.Zone)

	requireZoneUsage(t, zones, "a", 120, 2)
	requireZoneUsage(t, zones, zone.DefaultID, 10, 1)

	err = rs.AddProcess(getZoneProcess("p4", "a", 1))
	require.ErrorIs(t, err, zone.ErrExceeded)
}

// zoneLogs returns the zones of all log entries of the process with the given ID
// and the given message.
func zoneLogs(events []*log.Event, id, message string) []any {
	zones := []any{}

	for _, e := range events {
		if e.Data["id"] != id || e.Message != message {
			continue
		}

		zones = append(zones, e.Data["zone"])
	}

	return zones
}

func TestZoneLog(t *testing.T) {
	zones := newZoneManager(t, zone.Zone{ID: "a"}, zone.Zone{ID: "b"})

	rs, err := getDummyRestreamerWithZones(nil, nil, nil, nil, false, nil, zones)
	require.NoError(t, err)

	buffer := log.NewBufferWriter(log.Linfo, 100)
	rs.(*restream).logger = log.New("Process").WithOutput(buffer)

	require.NoError(t, rs.AddProcess(getZoneProcess("p1", "a", 10)))
	require.NoError(t, rs.AddProcess(getZoneProcess("p2", "", 10)))

	require.Equal(t, []any{"a"}, zoneLogs(buffer.Events(), "p1", "Created"))
	require.Equal(t, []any{zone.DefaultID}, zoneLogs(buffer.Events(), "p2", "Created"))

	// The process logs with the new zone after an update
	require.NoError(t, rs.UpdateProcess(app.ProcessID{ID: "p1"}, getZoneProcess("p1", "b", 10), false))
	require.Equal(t, []any{"a", "b"}, zoneLogs(buffer.Events(), "p1", "Created"))
}
