package zone

import (
	"math"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/datarhei/core/v16/io/fs"
	"github.com/datarhei/core/v16/restream/app"

	"github.com/stretchr/testify/require"
)

const mb = 1024 * 1024

func newMemFS(t *testing.T) fs.Filesystem {
	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	return memfs
}

func newManager(t *testing.T, memfs fs.Filesystem) Manager {
	m, err := New(Config{
		Filesystem: memfs,
		Filepath:   "/zones.json",
	})
	require.NoError(t, err)

	return m
}

func TestNew(t *testing.T) {
	_, err := New(Config{Filepath: "/zones.json"})
	require.Error(t, err)

	_, err = New(Config{Filesystem: newMemFS(t)})
	require.Error(t, err)

	m := newManager(t, newMemFS(t))

	// Only the default zone exists
	list := m.List()
	require.Len(t, list, 1)
	require.Equal(t, Info{Zone: Zone{ID: DefaultID}}, list[0])
}

func TestResourcesFromConfig(t *testing.T) {
	r := ResourcesFromConfig(&app.Config{
		LimitCPU:    10,
		LimitMemory: 20,
		LimitGPU: app.ConfigLimitGPU{
			Usage:   30,
			Encoder: 40,
			Decoder: 50,
			Memory:  60,
		},
	})

	require.Equal(t, Resources{CPU: 10, Memory: 20, GPUUsage: 30, GPUEncoder: 40, GPUDecoder: 50, GPUMemory: 60}, r)
}

func TestResourcesAddSub(t *testing.T) {
	a := Resources{CPU: 10, Memory: 20, GPUUsage: 30, GPUEncoder: 40, GPUDecoder: 50, GPUMemory: 60}
	b := Resources{CPU: 1, Memory: 2, GPUUsage: 3, GPUEncoder: 4, GPUDecoder: 5, GPUMemory: 6}

	require.Equal(t, Resources{CPU: 11, Memory: 22, GPUUsage: 33, GPUEncoder: 44, GPUDecoder: 55, GPUMemory: 66}, a.Add(b))
	require.Equal(t, Resources{CPU: 9, Memory: 18, GPUUsage: 27, GPUEncoder: 36, GPUDecoder: 45, GPUMemory: 54}, a.Sub(b))

	// The values don't drop below 0
	require.Equal(t, Resources{}, b.Sub(a))
}

func TestValidate(t *testing.T) {
	valid := []Zone{
		{ID: DefaultID},
		{ID: "abc-DEF_123"},
		{ID: "a", Limits: Resources{CPU: 100, Memory: 1, GPUUsage: 1, GPUEncoder: 1, GPUDecoder: 1, GPUMemory: 1}},
	}

	for _, z := range valid {
		require.NoError(t, Validate(z), z.ID)
	}

	invalid := []Zone{
		{ID: ""},
		{ID: "$foo"},
		{ID: "a$default"},
		{ID: "a b"},
		{ID: "a/b"},
		{ID: "ä"},
		{ID: "a", Limits: Resources{CPU: -1}},
		{ID: "a", Limits: Resources{GPUUsage: math.NaN()}},
		{ID: "a", Limits: Resources{GPUEncoder: math.Inf(1)}},
		{ID: "a", Limits: Resources{GPUDecoder: -0.1}},
	}

	for _, z := range invalid {
		require.ErrorIs(t, Validate(z), ErrInvalid, z)
	}
}

func TestCheck(t *testing.T) {
	z := Zone{ID: "a", Limits: Resources{CPU: 100, Memory: 100 * mb}}

	// Within the limits
	require.NoError(t, Check(z, Resources{CPU: 50, Memory: 50 * mb}, Resources{}, Resources{CPU: 50, Memory: 50 * mb}))

	// CPU exceeded
	err := Check(z, Resources{CPU: 50}, Resources{}, Resources{CPU: 60})
	require.ErrorIs(t, err, ErrExceeded)
	require.Equal(t, "zone limits exceeded: zone 'a': cpu_usage 110 of 100", err.Error())

	// Both exceeded, the memory is reported in megabytes
	err = Check(z, Resources{CPU: 50, Memory: 50 * mb}, Resources{}, Resources{CPU: 60, Memory: 60 * mb})
	require.ErrorIs(t, err, ErrExceeded)
	require.Equal(t, "zone limits exceeded: zone 'a': cpu_usage 110 of 100, memory_mbytes 110 of 100", err.Error())

	// Unlimited resources are not checked
	require.NoError(t, Check(z, Resources{}, Resources{}, Resources{GPUUsage: 1000, GPUMemory: 1000 * mb}))

	// The previous claim is replaced
	require.NoError(t, Check(z, Resources{CPU: 100}, Resources{CPU: 40}, Resources{CPU: 40}))
	require.NoError(t, Check(z, Resources{CPU: 100}, Resources{CPU: 40}, Resources{CPU: 30}))
	require.ErrorIs(t, Check(z, Resources{CPU: 100}, Resources{CPU: 40}, Resources{CPU: 41}), ErrExceeded)

	// Above the limit, but the resource doesn't increase
	require.NoError(t, Check(z, Resources{CPU: 150}, Resources{CPU: 40}, Resources{CPU: 40}))
	require.NoError(t, Check(z, Resources{CPU: 150}, Resources{CPU: 40}, Resources{CPU: 20}))
	require.ErrorIs(t, Check(z, Resources{CPU: 150}, Resources{CPU: 40}, Resources{CPU: 50}), ErrExceeded)

	// The memory doesn't increase, but the CPU does
	err = Check(z, Resources{CPU: 100, Memory: 150 * mb}, Resources{CPU: 10, Memory: 10 * mb}, Resources{CPU: 20, Memory: 10 * mb})
	require.Equal(t, "zone limits exceeded: zone 'a': cpu_usage 110 of 100", err.Error())
}

func TestAddGetUpdateRemove(t *testing.T) {
	m := newManager(t, newMemFS(t))

	z := Zone{ID: "a", Limits: Resources{CPU: 100}}

	require.NoError(t, m.Add(z))
	require.ErrorIs(t, m.Add(z), ErrExists)
	require.ErrorIs(t, m.Add(Zone{ID: DefaultID}), ErrExists)
	require.ErrorIs(t, m.Add(Zone{ID: ""}), ErrInvalid)
	require.ErrorIs(t, m.Add(Zone{ID: "a b"}), ErrInvalid)

	info, err := m.Get("a")
	require.NoError(t, err)
	require.Equal(t, Info{Zone: z}, info)

	_, err = m.Get("b")
	require.ErrorIs(t, err, ErrNotFound)

	z.Limits.CPU = 200
	require.NoError(t, m.Update(z))
	require.ErrorIs(t, m.Update(Zone{ID: "b"}), ErrNotFound)
	require.ErrorIs(t, m.Update(Zone{ID: "a", Limits: Resources{CPU: -1}}), ErrInvalid)

	// The limits of the default zone can be changed
	require.NoError(t, m.Update(Zone{ID: DefaultID, Limits: Resources{Memory: 1}}))

	require.NoError(t, m.Add(Zone{ID: "0"}))

	list := m.List()
	require.Equal(t, []Info{
		{Zone: Zone{ID: DefaultID, Limits: Resources{Memory: 1}}},
		{Zone: Zone{ID: "0"}},
		{Zone: Zone{ID: "a", Limits: Resources{CPU: 200}}},
	}, list)

	require.ErrorIs(t, m.Remove(DefaultID), ErrInvalid)
	require.ErrorIs(t, m.Remove(""), ErrNotFound)
	require.ErrorIs(t, m.Remove("b"), ErrNotFound)
	require.NoError(t, m.Remove("a"))

	_, err = m.Get("a")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestAcquireRelease(t *testing.T) {
	m := newManager(t, newMemFS(t))

	require.NoError(t, m.Add(Zone{ID: "a", Limits: Resources{CPU: 100}}))
	require.NoError(t, m.Add(Zone{ID: "b", Limits: Resources{CPU: 50}}))

	p1 := &Claim{Zone: "a", Resources: Resources{CPU: 60, Memory: 10}}
	p2 := &Claim{Zone: "a", Resources: Resources{CPU: 50}}

	require.NoError(t, m.Acquire(nil, p1))
	require.ErrorIs(t, m.Acquire(nil, p2), ErrExceeded)

	info, _ := m.Get("a")
	require.Equal(t, Resources{CPU: 60, Memory: 10}, info.Usage)
	require.Equal(t, 1, info.Processes)

	// Unknown zone
	require.ErrorIs(t, m.Acquire(nil, &Claim{Zone: "x"}), ErrNotFound)

	// A zone with processes can't be removed
	require.ErrorIs(t, m.Remove("a"), ErrInUse)

	// Update within the zone, the previous claim is replaced
	p1next := &Claim{Zone: "a", Resources: Resources{CPU: 90}}
	require.NoError(t, m.Acquire(p1, p1next))

	info, _ = m.Get("a")
	require.Equal(t, Resources{CPU: 90}, info.Usage)
	require.Equal(t, 1, info.Processes)

	// Moving to a zone that can't afford it, nothing changes
	require.ErrorIs(t, m.Acquire(p1next, &Claim{Zone: "b", Resources: Resources{CPU: 90}}), ErrExceeded)

	info, _ = m.Get("a")
	require.Equal(t, Resources{CPU: 90}, info.Usage)

	// Moving to another zone
	p1moved := &Claim{Zone: "b", Resources: Resources{CPU: 40}}
	require.NoError(t, m.Acquire(p1next, p1moved))

	info, _ = m.Get("a")
	require.Equal(t, Resources{}, info.Usage)
	require.Equal(t, 0, info.Processes)

	info, _ = m.Get("b")
	require.Equal(t, Resources{CPU: 40}, info.Usage)
	require.Equal(t, 1, info.Processes)

	require.NoError(t, m.Remove("a"))

	// Release
	m.Release(p1moved)

	info, _ = m.Get("b")
	require.Equal(t, Resources{}, info.Usage)
	require.Equal(t, 0, info.Processes)

	// Releasing again doesn't break anything
	m.Release(p1moved)

	info, _ = m.Get("b")
	require.Equal(t, 0, info.Processes)
}

func TestRegister(t *testing.T) {
	m := newManager(t, newMemFS(t))

	require.NoError(t, m.Add(Zone{ID: "a", Limits: Resources{CPU: 100}}))

	// Registering ignores the limits
	m.Register(&Claim{Zone: "a", Resources: Resources{CPU: 80}})
	m.Register(&Claim{Zone: "a", Resources: Resources{CPU: 80}})

	info, _ := m.Get("a")
	require.Equal(t, Resources{CPU: 160}, info.Usage)
	require.Equal(t, 2, info.Processes)

	// Above the limit, only claims that don't increase are allowed
	require.ErrorIs(t, m.Acquire(nil, &Claim{Zone: "a", Resources: Resources{CPU: 1}}), ErrExceeded)
	require.NoError(t, m.Acquire(&Claim{Zone: "a", Resources: Resources{CPU: 80}}, &Claim{Zone: "a", Resources: Resources{CPU: 70}}))

	info, _ = m.Get("a")
	require.Equal(t, Resources{CPU: 150}, info.Usage)

	// Processes in an unknown zone are counted, the zone appears once it is added
	m.Register(&Claim{Zone: "x", Resources: Resources{CPU: 10}})
	require.NoError(t, m.Add(Zone{ID: "x"}))

	info, _ = m.Get("x")
	require.Equal(t, Resources{CPU: 10}, info.Usage)
	require.Equal(t, 1, info.Processes)
}

func TestDefaultZone(t *testing.T) {
	m := newManager(t, newMemFS(t))

	// The default zone is unlimited, a claim without a zone belongs to the default zone
	p1 := &Claim{Zone: "", Resources: Resources{CPU: 10000}}
	require.NoError(t, m.Acquire(nil, p1))

	info, err := m.Get(DefaultID)
	require.NoError(t, err)
	require.Equal(t, Resources{CPU: 10000}, info.Usage)
	require.Equal(t, 1, info.Processes)

	require.NoError(t, m.Update(Zone{ID: DefaultID, Limits: Resources{CPU: 10100}}))
	require.ErrorIs(t, m.Acquire(nil, &Claim{Zone: "", Resources: Resources{CPU: 200}}), ErrExceeded)
	require.ErrorIs(t, m.Acquire(nil, &Claim{Zone: DefaultID, Resources: Resources{CPU: 200}}), ErrExceeded)

	// Moving from the empty ID to the default zone is the same zone, the previous claim is replaced
	p1next := &Claim{Zone: DefaultID, Resources: Resources{CPU: 10050}}
	require.NoError(t, m.Acquire(p1, p1next))

	info, _ = m.Get(DefaultID)
	require.Equal(t, Resources{CPU: 10050}, info.Usage)
	require.Equal(t, 1, info.Processes)

	m.Register(&Claim{Zone: "", Resources: Resources{CPU: 10}})

	info, _ = m.Get(DefaultID)
	require.Equal(t, 2, info.Processes)

	m.Release(&Claim{Zone: "", Resources: Resources{CPU: 10}})
	m.Release(p1next)

	info, _ = m.Get(DefaultID)
	require.Equal(t, Resources{}, info.Usage)
	require.Equal(t, 0, info.Processes)
}

func TestConcurrentAcquire(t *testing.T) {
	m := newManager(t, newMemFS(t))

	require.NoError(t, m.Add(Zone{ID: "a", Limits: Resources{CPU: 100}}))

	wg := sync.WaitGroup{}
	accepted := atomic.Int64{}

	for range 1000 {
		wg.Go(func() {
			if m.Acquire(nil, &Claim{Zone: "a", Resources: Resources{CPU: 1}}) == nil {
				accepted.Add(1)
			}
		})
	}

	wg.Wait()

	require.Equal(t, int64(100), accepted.Load())

	info, _ := m.Get("a")
	require.Equal(t, Resources{CPU: 100}, info.Usage)
	require.Equal(t, 100, info.Processes)
}

func TestPersistence(t *testing.T) {
	memfs := newMemFS(t)

	m := newManager(t, memfs)
	require.NoError(t, m.Add(Zone{ID: "a", Limits: Resources{CPU: 100, Memory: 200}}))
	require.NoError(t, m.Update(Zone{ID: DefaultID, Limits: Resources{GPUUsage: 50}}))

	// The usage is not persisted
	require.NoError(t, m.Acquire(nil, &Claim{Zone: "a", Resources: Resources{CPU: 10}}))

	m = newManager(t, memfs)

	require.Equal(t, []Info{
		{Zone: Zone{ID: DefaultID, Limits: Resources{GPUUsage: 50}}},
		{Zone: Zone{ID: "a", Limits: Resources{CPU: 100, Memory: 200}}},
	}, m.List())
}

func TestLoadInvalid(t *testing.T) {
	memfs := newMemFS(t)

	_, _, err := memfs.WriteFileSafe("/zones.json", []byte(`{"version":1,"zones":[
		{"id":"a","limits":{"cpu_usage":100}},
		{"id":"","limits":{"cpu_usage":300}},
		{"id":"$default","limits":{"memory_bytes":400}},
		{"id":"a b","limits":{}},
		{"id":"b","limits":{"cpu_usage":-1}},
		{"id":"a","limits":{"cpu_usage":200}}
	]}`))
	require.NoError(t, err)

	m := newManager(t, memfs)

	require.Equal(t, []Info{
		{Zone: Zone{ID: DefaultID, Limits: Resources{Memory: 400}}},
		{Zone: Zone{ID: "a", Limits: Resources{CPU: 100}}},
	}, m.List())

	_, _, err = memfs.WriteFileSafe("/zones.json", []byte(`{"version":2,"zones":[]}`))
	require.NoError(t, err)

	_, err = New(Config{Filesystem: memfs, Filepath: "/zones.json"})
	require.Error(t, err)

	_, _, err = memfs.WriteFileSafe("/zones.json", []byte(`{"version":1,`))
	require.NoError(t, err)

	_, err = New(Config{Filesystem: memfs, Filepath: "/zones.json"})
	require.Error(t, err)
}

type failingFS struct {
	fs.Filesystem
}

func (f *failingFS) WriteFileSafe(path string, data []byte) (int64, bool, error) {
	return 0, false, fs.ErrNotExist
}

func TestSaveFailure(t *testing.T) {
	m := newManager(t, &failingFS{Filesystem: newMemFS(t)})

	require.Error(t, m.Add(Zone{ID: "a"}))

	_, err := m.Get("a")
	require.ErrorIs(t, err, ErrNotFound)

	require.Error(t, m.Update(Zone{ID: DefaultID, Limits: Resources{CPU: 1}}))

	info, _ := m.Get(DefaultID)
	require.Equal(t, Resources{}, info.Limits)
}
