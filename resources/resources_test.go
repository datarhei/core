package resources

import (
	"sync"
	"testing"
	"time"

	"github.com/datarhei/core/v16/psutil"

	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/net"
	"github.com/stretchr/testify/require"
)

type util struct{}

func (u *util) Start() {}
func (u *util) Stop()  {}

func (u *util) CPUCounts(logical bool) (float64, error) {
	return 2, nil
}

func (u *util) CPUPercent() (*psutil.CPUInfoStat, error) {
	return &psutil.CPUInfoStat{
		System: 10,
		User:   50,
		Idle:   35,
		Other:  5,
	}, nil
}

func (u *util) DiskUsage(path string) (*disk.UsageStat, error) {
	return &disk.UsageStat{}, nil
}

func (u *util) VirtualMemory() (*psutil.MemoryInfoStat, error) {
	return &psutil.MemoryInfoStat{
		Total:     200,
		Available: 40,
		Used:      160,
	}, nil
}

func (u *util) NetIOCounters(pernic bool) ([]net.IOCountersStat, error) {
	return nil, nil
}

func (u *util) Process(pid int32) (psutil.Process, error) {
	return nil, nil
}

func TestMemoryLimit(t *testing.T) {
	r, err := New(Config{
		MaxCPU:    100,
		MaxMemory: 150. / 200. * 100,
		PSUtil:    &util{},
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
				_, limit = r.ShouldLimit()
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

	r.Stop()
}

func TestCPULimit(t *testing.T) {
	r, err := New(Config{
		MaxCPU:    50.,
		MaxMemory: 100,
		PSUtil:    &util{},
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
				limit, _ = r.ShouldLimit()
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

	r.Stop()
}

func TestRequest(t *testing.T) {
	r, err := New(Config{
		MaxCPU:    70.,
		MaxMemory: 170. / 200. * 100,
		PSUtil:    &util{},
		Logger:    nil,
	})
	require.NoError(t, err)

	err = r.Request(-1, 0)
	require.Error(t, err)

	err = r.Request(5, 10)
	require.NoError(t, err)

	err = r.Request(5, 20)
	require.Error(t, err)

	err = r.Request(10, 10)
	require.NoError(t, err)
}

func TestHasLimits(t *testing.T) {
	r, err := New(Config{
		MaxCPU:    70.,
		MaxMemory: 170. / 200. * 100,
		PSUtil:    &util{},
		Logger:    nil,
	})
	require.NoError(t, err)

	require.True(t, r.HasLimits())

	r, err = New(Config{
		MaxCPU:    0,
		MaxMemory: 0,
		PSUtil:    &util{},
		Logger:    nil,
	})
	require.NoError(t, err)

	require.False(t, r.HasLimits())
}
