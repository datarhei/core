package resources

import (
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/datarhei/core/v16/psutil"

	"github.com/stretchr/testify/require"
)

type util struct {
	lock sync.Mutex

	cpu psutil.CPUInfo
	mem psutil.MemoryInfo
	gpu []psutil.GPUInfo
}

func newUtil(ngpu int) *util {
	u := &util{
		cpu: psutil.CPUInfo{
			System: 10,
			User:   50,
			Idle:   35,
			Other:  5,
		},
		mem: psutil.MemoryInfo{
			Total:     200,
			Available: 40,
			Used:      160,
		},
	}

	for i := 0; i < ngpu; i++ {
		u.gpu = append(u.gpu, psutil.GPUInfo{
			Index:       i,
			Name:        "L4",
			MemoryTotal: 24 * 1024 * 1024 * 1024,
			MemoryUsed:  uint64(12+i) * 1024 * 1024 * 1024,
			Usage:       50 - float64((i+1)*5),
			Encoder:     50 - float64((i+1)*10),
			Decoder:     50 - float64((i+1)*3),
		})
	}

	return u
}

func (u *util) Start() {}
func (u *util) Stop()  {}

func (u *util) CPUCounts() (float64, error) {
	return 2, nil
}

func (u *util) CPU() (*psutil.CPUInfo, error) {
	u.lock.Lock()
	defer u.lock.Unlock()

	cpu := u.cpu

	return &cpu, nil
}

func (u *util) Disk(path string) (*psutil.DiskInfo, error) {
	return &psutil.DiskInfo{}, nil
}

func (u *util) Memory() (*psutil.MemoryInfo, error) {
	u.lock.Lock()
	defer u.lock.Unlock()

	mem := u.mem

	return &mem, nil
}

func (u *util) Network() ([]psutil.NetworkInfo, error) {
	return nil, nil
}

func (u *util) GPU() ([]psutil.GPUInfo, error) {
	u.lock.Lock()
	defer u.lock.Unlock()

	gpu := []psutil.GPUInfo{}

	gpu = append(gpu, u.gpu...)

	return gpu, nil
}

func (u *util) Process(pid int32) (psutil.Process, error) {
	return &process{}, nil
}

type process struct{}

func (p *process) CPU() (*psutil.CPUInfo, error) {
	s := &psutil.CPUInfo{
		System: 1,
		User:   2,
		Idle:   0,
		Other:  3,
	}

	return s, nil
}

func (p *process) Memory() (uint64, error) { return 42, nil }
func (p *process) GPU() (*psutil.GPUInfo, error) {
	return &psutil.GPUInfo{
		Index:       0,
		Name:        "L4",
		MemoryTotal: 128,
		MemoryUsed:  42,
		Usage:       5,
		Encoder:     9,
		Decoder:     7,
	}, nil
}
func (p *process) Stop()          {}
func (p *process) Suspend() error { return nil }
func (p *process) Resume() error  { return nil }

func TestConfigNoLimits(t *testing.T) {
	_, err := New(Config{
		PSUtil: newUtil(0),
	})
	require.NoError(t, err)
}

func TestConfigWrongLimits(t *testing.T) {
	_, err := New(Config{
		MaxCPU:    102,
		MaxMemory: 573,
		PSUtil:    newUtil(0),
	})
	require.Error(t, err)

	_, err = New(Config{
		MaxCPU:       0,
		MaxMemory:    0,
		MaxGPU:       101,
		MaxGPUMemory: 103,
		PSUtil:       newUtil(0),
	})
	require.NoError(t, err)

	_, err = New(Config{
		MaxCPU:       0,
		MaxMemory:    0,
		MaxGPU:       101,
		MaxGPUMemory: 103,
		PSUtil:       newUtil(1),
	})
	require.Error(t, err)
}

func TestMemoryLimit(t *testing.T) {
	r, err := New(Config{
		MaxCPU:    100,
		MaxMemory: 150. / 200. * 100,
		PSUtil:    newUtil(0),
		Logger:    nil,
	})
	require.NoError(t, err)

	wg := sync.WaitGroup{}
	wg.Add(1)

	limit := false

	go func() {
		defer func() {
			wg.Done()
		}()

		timer := time.NewTimer(10 * time.Second)
		defer timer.Stop()

		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				_, limit, _ = r.ShouldLimit()
				if limit {
					return
				}
			case <-timer.C:
				return
			}
		}
	}()

	r.Start()

	wg.Wait()

	require.True(t, limit)

	_, err = r.Request(Request{CPU: 5, Memory: 10})
	require.Error(t, err)

	r.Stop()
}

func TestMemoryUnlimit(t *testing.T) {
	util := newUtil(0)

	r, err := New(Config{
		MaxCPU:    100,
		MaxMemory: 150. / 200. * 100,
		PSUtil:    util,
		Logger:    nil,
	})
	require.NoError(t, err)

	wg := sync.WaitGroup{}
	wg.Add(1)

	limit := false

	go func() {
		defer func() {
			wg.Done()
		}()

		timer := time.NewTimer(10 * time.Second)
		defer timer.Stop()

		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				_, limit, _ = r.ShouldLimit()
				if limit {
					return
				}
			case <-timer.C:
				return
			}
		}
	}()

	r.Start()

	wg.Wait()

	require.True(t, limit)

	_, limit, _ = r.ShouldLimit()
	require.True(t, limit)

	util.lock.Lock()
	util.mem.Used = 140
	util.lock.Unlock()

	wg.Add(1)

	go func() {
		defer func() {
			wg.Done()
		}()

		timer := time.NewTimer(10 * time.Second)
		defer timer.Stop()

		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				_, limit, _ = r.ShouldLimit()
				if !limit {
					return
				}
			case <-timer.C:
				return
			}
		}
	}()

	wg.Wait()

	require.False(t, limit)

	r.Stop()
}

func TestCPULimit(t *testing.T) {
	r, err := New(Config{
		MaxCPU:    50.,
		MaxMemory: 100,
		PSUtil:    newUtil(0),
		Logger:    nil,
	})
	require.NoError(t, err)

	wg := sync.WaitGroup{}
	wg.Add(1)

	limit := false

	go func() {
		defer func() {
			wg.Done()
		}()

		timer := time.NewTimer(10 * time.Second)
		defer timer.Stop()

		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				limit, _, _ = r.ShouldLimit()
				if limit {
					return
				}
			case <-timer.C:
				return
			}
		}
	}()

	r.Start()

	wg.Wait()

	require.True(t, limit)

	_, err = r.Request(Request{CPU: 5, Memory: 10})
	require.Error(t, err)

	r.Stop()
}

func TestCPUUnlimit(t *testing.T) {
	util := newUtil(0)

	r, err := New(Config{
		MaxCPU:    50.,
		MaxMemory: 100,
		PSUtil:    util,
		Logger:    nil,
	})
	require.NoError(t, err)

	wg := sync.WaitGroup{}
	wg.Add(1)

	limit := false

	go func() {
		defer func() {
			wg.Done()
		}()

		timer := time.NewTimer(10 * time.Second)
		defer timer.Stop()

		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				limit, _, _ = r.ShouldLimit()
				if limit {
					return
				}
			case <-timer.C:
				return
			}
		}
	}()

	r.Start()

	wg.Wait()

	require.True(t, limit)

	limit, _, _ = r.ShouldLimit()
	require.True(t, limit)

	util.lock.Lock()
	util.cpu.User = 20
	util.lock.Unlock()

	wg.Add(1)

	go func() {
		defer func() {
			wg.Done()
		}()

		timer := time.NewTimer(10 * time.Second)
		defer timer.Stop()

		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				limit, _, _ = r.ShouldLimit()
				if !limit {
					return
				}
			case <-timer.C:
				return
			}
		}
	}()

	wg.Wait()

	require.False(t, limit)

	r.Stop()
}

func TestGPULimitMemory(t *testing.T) {
	r, err := New(Config{
		MaxCPU:       100,
		MaxMemory:    100,
		MaxGPU:       100,
		MaxGPUMemory: 20,
		PSUtil:       newUtil(2),
		Logger:       nil,
	})
	require.NoError(t, err)

	wg := sync.WaitGroup{}
	wg.Add(1)

	limit := []bool{}

	go func() {
		defer func() {
			wg.Done()
		}()

		timer := time.NewTimer(10 * time.Second)
		defer timer.Stop()

		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				_, _, limit = r.ShouldLimit()
				if slices.Contains(limit, true) {
					return
				}
			case <-timer.C:
				return
			}
		}
	}()

	r.Start()

	wg.Wait()

	require.Contains(t, limit, true)

	_, err = r.Request(Request{CPU: 5, Memory: 10, GPUUsage: 10, GPUMemory: 10})
	require.Error(t, err)

	r.Stop()
}

func TestGPUUnlimitMemory(t *testing.T) {
	util := newUtil(2)

	r, err := New(Config{
		MaxCPU:       100,
		MaxMemory:    100,
		MaxGPU:       100,
		MaxGPUMemory: 20,
		PSUtil:       util,
		Logger:       nil,
	})
	require.NoError(t, err)

	wg := sync.WaitGroup{}
	wg.Add(1)

	limit := []bool{}

	go func() {
		defer func() {
			wg.Done()
		}()

		timer := time.NewTimer(10 * time.Second)
		defer timer.Stop()

		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				_, _, limit = r.ShouldLimit()
				if slices.Contains(limit, true) {
					return
				}
			case <-timer.C:
				return
			}
		}
	}()

	r.Start()

	wg.Wait()

	require.Contains(t, limit, true)

	util.lock.Lock()
	util.gpu[0].MemoryUsed = 10
	util.gpu[1].MemoryUsed = 10
	util.lock.Unlock()

	wg.Add(1)

	go func() {
		defer func() {
			wg.Done()
		}()

		timer := time.NewTimer(10 * time.Second)
		defer timer.Stop()

		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				_, _, limit = r.ShouldLimit()
				if !slices.Contains(limit, true) {
					return
				}
			case <-timer.C:
				return
			}
		}
	}()

	wg.Wait()

	require.NotContains(t, limit, true)

	r.Stop()
}

func TestGPULimitMemorySome(t *testing.T) {
	r, err := New(Config{
		MaxCPU:       100,
		MaxMemory:    100,
		MaxGPU:       100,
		MaxGPUMemory: 14. / 24. * 100.,
		PSUtil:       newUtil(4),
		Logger:       nil,
	})
	require.NoError(t, err)

	wg := sync.WaitGroup{}
	wg.Add(1)

	limit := []bool{}

	go func() {
		defer func() {
			wg.Done()
		}()

		timer := time.NewTimer(10 * time.Second)
		defer timer.Stop()

		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				_, _, limit = r.ShouldLimit()
				if slices.Contains(limit, true) {
					return
				}
			case <-timer.C:
				return
			}
		}
	}()

	r.Start()

	wg.Wait()

	require.Equal(t, []bool{false, false, true, true}, limit)

	_, err = r.Request(Request{CPU: 5, Memory: 10, GPUUsage: 10, GPUMemory: 10})
	require.NoError(t, err)

	r.Stop()
}

func TestGPULimitUsage(t *testing.T) {
	r, err := New(Config{
		MaxCPU:       100,
		MaxMemory:    100,
		MaxGPU:       40,
		MaxGPUMemory: 100,
		PSUtil:       newUtil(3),
		Logger:       nil,
	})
	require.NoError(t, err)

	wg := sync.WaitGroup{}
	wg.Add(1)

	limit := []bool{}

	go func() {
		defer func() {
			wg.Done()
		}()

		timer := time.NewTimer(10 * time.Second)
		defer timer.Stop()

		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				_, _, limit = r.ShouldLimit()
				if slices.Contains(limit, true) {
					return
				}
			case <-timer.C:
				return
			}
		}
	}()

	r.Start()

	wg.Wait()

	require.Equal(t, []bool{true, false, false}, limit)

	_, err = r.Request(Request{CPU: 5, Memory: 10, GPUUsage: 10, GPUMemory: 10})
	require.Error(t, err)

	_, err = r.Request(Request{CPU: 5, Memory: 10, GPUEncoder: 10, GPUMemory: 10})
	require.NoError(t, err)

	r.Stop()
}

func TestGPUUnlimitUsage(t *testing.T) {
	util := newUtil(3)

	r, err := New(Config{
		MaxCPU:       100,
		MaxMemory:    100,
		MaxGPU:       40,
		MaxGPUMemory: 100,
		PSUtil:       util,
		Logger:       nil,
	})
	require.NoError(t, err)

	wg := sync.WaitGroup{}
	wg.Add(1)

	limit := []bool{}

	go func() {
		defer func() {
			wg.Done()
		}()

		timer := time.NewTimer(10 * time.Second)
		defer timer.Stop()

		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				_, _, limit = r.ShouldLimit()
				if slices.Contains(limit, true) {
					return
				}
			case <-timer.C:
				return
			}
		}
	}()

	r.Start()

	wg.Wait()

	require.Equal(t, []bool{true, false, false}, limit)

	util.lock.Lock()
	util.gpu[0].Usage = 30
	util.gpu[0].Encoder = 30
	util.gpu[0].Decoder = 30
	util.lock.Unlock()

	wg.Add(1)

	go func() {
		defer func() {
			wg.Done()
		}()

		timer := time.NewTimer(10 * time.Second)
		defer timer.Stop()

		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				_, _, limit = r.ShouldLimit()
				if !slices.Contains(limit, true) {
					return
				}
			case <-timer.C:
				return
			}
		}
	}()

	wg.Wait()

	require.Equal(t, []bool{false, false, false}, limit)

	r.Stop()
}

func TestRequestCPU(t *testing.T) {
	r, err := New(Config{
		MaxCPU: 70.,
		PSUtil: newUtil(0),
	})
	require.NoError(t, err)

	_, err = r.Request(Request{CPU: 0, Memory: 0})
	require.Error(t, err)

	_, err = r.Request(Request{CPU: 5, Memory: 10})
	require.NoError(t, err)

	_, err = r.Request(Request{CPU: 30, Memory: 10})
	require.Error(t, err)
}

func TestRequestMemory(t *testing.T) {
	r, err := New(Config{
		MaxMemory: 170. / 200. * 100,
		PSUtil:    newUtil(0),
	})
	require.NoError(t, err)

	_, err = r.Request(Request{CPU: 5, Memory: 0})
	require.Error(t, err)

	_, err = r.Request(Request{CPU: 5, Memory: 10})
	require.NoError(t, err)

	_, err = r.Request(Request{CPU: 50, Memory: 20})
	require.Error(t, err)
}

func TestRequestNoGPU(t *testing.T) {
	r, err := New(Config{
		MaxCPU:    100,
		MaxMemory: 100,
		PSUtil:    newUtil(0),
	})
	require.NoError(t, err)

	_, err = r.Request(Request{CPU: 10, Memory: 10, GPUEncoder: 30, GPUMemory: 10})
	require.Error(t, err)
}

func TestRequestInvalidGPURequest(t *testing.T) {
	r, err := New(Config{
		MaxCPU:    100,
		MaxMemory: 100,
		PSUtil:    newUtil(1),
	})
	require.NoError(t, err)

	_, err = r.Request(Request{CPU: 10, Memory: 10, GPUEncoder: 30, GPUMemory: 0})
	require.Error(t, err)

	_, err = r.Request(Request{CPU: 10, Memory: 10, GPUUsage: -1, GPUEncoder: 30, GPUMemory: 0})
	require.Error(t, err)
}

func TestRequestGPULimitsOneGPU(t *testing.T) {
	r, err := New(Config{
		MaxCPU:       100,
		MaxMemory:    100,
		MaxGPU:       50,
		MaxGPUMemory: 60,
		PSUtil:       newUtil(1),
	})
	require.NoError(t, err)

	_, err = r.Request(Request{CPU: 10, Memory: 10, GPUUsage: 50, GPUMemory: 10})
	require.Error(t, err)

	_, err = r.Request(Request{CPU: 10, Memory: 10, GPUEncoder: 50, GPUMemory: 10})
	require.Error(t, err)

	_, err = r.Request(Request{CPU: 10, Memory: 10, GPUDecoder: 50, GPUMemory: 10})
	require.Error(t, err)

	_, err = r.Request(Request{CPU: 10, Memory: 10, GPUEncoder: 10, GPUMemory: 5 * 1024 * 1024 * 1024})
	require.Error(t, err)

	res, err := r.Request(Request{CPU: 10, Memory: 10, GPUEncoder: 10, GPUMemory: 10})
	require.NoError(t, err)
	require.Equal(t, 0, res.GPU)
}

func TestRequestGPULimitsMoreGPU(t *testing.T) {
	r, err := New(Config{
		MaxCPU:       100,
		MaxMemory:    100,
		MaxGPU:       60,
		MaxGPUMemory: 60,
		PSUtil:       newUtil(2),
	})
	require.NoError(t, err)

	_, err = r.Request(Request{CPU: 10, Memory: 10, GPUEncoder: 50, GPUMemory: 10})
	require.Error(t, err)

	res, err := r.Request(Request{CPU: 10, Memory: 10, GPUEncoder: 30, GPUMemory: 10})
	require.NoError(t, err)
	require.Equal(t, 1, res.GPU)
}

func TestHasLimits(t *testing.T) {
	r, err := New(Config{
		MaxCPU:    70.,
		MaxMemory: 170. / 200. * 100,
		PSUtil:    newUtil(0),
		Logger:    nil,
	})
	require.NoError(t, err)

	require.True(t, r.HasLimits())

	r, err = New(Config{
		MaxCPU:    100,
		MaxMemory: 100,
		PSUtil:    newUtil(0),
		Logger:    nil,
	})
	require.NoError(t, err)

	require.True(t, r.HasLimits())

	r, err = New(Config{
		MaxCPU:    0,
		MaxMemory: 0,
		PSUtil:    newUtil(0),
		Logger:    nil,
	})
	require.NoError(t, err)

	require.False(t, r.HasLimits())

	r, err = New(Config{
		MaxCPU:    0,
		MaxMemory: 0,
		MaxGPU:    10,
		PSUtil:    newUtil(1),
		Logger:    nil,
	})
	require.NoError(t, err)

	require.True(t, r.HasLimits())

	r, err = New(Config{
		MaxCPU:    0,
		MaxMemory: 0,
		MaxGPU:    10,
		PSUtil:    newUtil(0),
		Logger:    nil,
	})
	require.NoError(t, err)

	require.False(t, r.HasLimits())
}

func TestInfo(t *testing.T) {
	r, err := New(Config{
		MaxCPU:       90,
		MaxMemory:    90,
		MaxGPU:       11,
		MaxGPUMemory: 50,
		PSUtil:       newUtil(2),
	})
	require.NoError(t, err)

	info := r.Info()

	require.Equal(t, Info{
		Mem: MemoryInfo{
			Total:      200,
			Available:  40,
			Used:       160,
			Limit:      180,
			Core:       42,
			Throttling: false,
			Error:      nil,
		},
		CPU: CPUInfo{
			NCPU:       2,
			System:     10,
			User:       50,
			Idle:       35,
			Other:      5,
			Limit:      90,
			Core:       6,
			Throttling: false,
			Error:      nil,
		},
		GPU: GPUInfo{
			NGPU: 2,
			GPU: []GPUInfoStat{{
				Index:           0,
				Name:            "L4",
				MemoryTotal:     24 * 1024 * 1024 * 1024,
				MemoryUsed:      12 * 1024 * 1024 * 1024,
				MemoryAvailable: 12 * 1024 * 1024 * 1024,
				MemoryLimit:     12 * 1024 * 1024 * 1024,
				Usage:           45,
				Encoder:         40,
				Decoder:         47,
				UsageLimit:      11,
			}, {
				Index:           1,
				Name:            "L4",
				MemoryTotal:     24 * 1024 * 1024 * 1024,
				MemoryUsed:      13 * 1024 * 1024 * 1024,
				MemoryAvailable: 11 * 1024 * 1024 * 1024,
				MemoryLimit:     12 * 1024 * 1024 * 1024,
				Usage:           40,
				Encoder:         30,
				Decoder:         44,
				UsageLimit:      11,
			}},
			Error: nil,
		},
	}, info)
}
