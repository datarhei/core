package process

import (
	"sync"
	"testing"
	"time"

	"github.com/datarhei/core/v16/resources"

	"github.com/stretchr/testify/require"
)

type proc struct{}

func (p *proc) Info() (resources.ProcessInfo, error) {
	info := resources.ProcessInfo{
		CPU: resources.ProcessInfoCPU{
			System: 50,
			User:   0,
			Idle:   0,
			Other:  0,
		},
		Memory: 197,
		GPU: resources.ProcessInfoGPU{
			Index:      0,
			MemoryUsed: 91,
			Usage:      3,
			Encoder:    9,
			Decoder:    5,
		},
	}

	return info, nil
}

func (p *proc) Cancel()        {}
func (p *proc) Suspend() error { return nil }
func (p *proc) Resume() error  { return nil }

func TestCPULimit(t *testing.T) {
	lock := sync.Mutex{}

	lock.Lock()
	done := false
	lock.Unlock()

	go func() {
		wg := sync.WaitGroup{}
		wg.Add(1)

		l, _ := NewLimiter(LimiterConfig{
			CPU: 42,
			OnLimit: func(float64, uint64, float64, float64, float64, uint64) {
				wg.Done()
			},
		})

		l.Start(&proc{})
		defer l.Stop()

		wg.Wait()

		lock.Lock()
		done = true
		lock.Unlock()
	}()

	require.Eventually(t, func() bool {
		lock.Lock()
		defer lock.Unlock()

		return done
	}, 2*time.Second, 100*time.Millisecond)
}

func TestCPULimitWaitFor(t *testing.T) {
	lock := sync.Mutex{}

	lock.Lock()
	done := false
	lock.Unlock()

	go func() {
		wg := sync.WaitGroup{}
		wg.Add(1)

		l, _ := NewLimiter(LimiterConfig{
			CPU:     42,
			WaitFor: 3 * time.Second,
			OnLimit: func(float64, uint64, float64, float64, float64, uint64) {
				wg.Done()
			},
		})

		l.Start(&proc{})
		defer l.Stop()

		wg.Wait()

		lock.Lock()
		done = true
		lock.Unlock()
	}()

	require.Eventually(t, func() bool {
		lock.Lock()
		defer lock.Unlock()

		return done
	}, 10*time.Second, 1*time.Second)
}

func TestMemoryLimit(t *testing.T) {
	lock := sync.Mutex{}

	lock.Lock()
	done := false
	lock.Unlock()

	go func() {
		wg := sync.WaitGroup{}
		wg.Add(1)

		l, _ := NewLimiter(LimiterConfig{
			Memory: 42,
			OnLimit: func(float64, uint64, float64, float64, float64, uint64) {
				wg.Done()
			},
		})

		l.Start(&proc{})
		defer l.Stop()

		wg.Wait()

		lock.Lock()
		done = true
		lock.Unlock()
	}()

	require.Eventually(t, func() bool {
		lock.Lock()
		defer lock.Unlock()

		return done
	}, 2*time.Second, 100*time.Millisecond)
}

func TestMemoryLimitWaitFor(t *testing.T) {
	lock := sync.Mutex{}

	lock.Lock()
	done := false
	lock.Unlock()

	go func() {
		wg := sync.WaitGroup{}
		wg.Add(1)

		l, _ := NewLimiter(LimiterConfig{
			Memory:  42,
			WaitFor: 3 * time.Second,
			OnLimit: func(float64, uint64, float64, float64, float64, uint64) {
				wg.Done()
			},
		})

		l.Start(&proc{})
		defer l.Stop()

		wg.Wait()

		lock.Lock()
		done = true
		lock.Unlock()
	}()

	require.Eventually(t, func() bool {
		lock.Lock()
		defer lock.Unlock()

		return done
	}, 10*time.Second, 1*time.Second)
}

func TestGPUMemoryLimit(t *testing.T) {
	lock := sync.Mutex{}

	lock.Lock()
	done := false
	lock.Unlock()

	go func() {
		wg := sync.WaitGroup{}
		wg.Add(1)

		l, _ := NewLimiter(LimiterConfig{
			GPUMemory: 42,
			OnLimit: func(float64, uint64, float64, float64, float64, uint64) {
				wg.Done()
			},
		})

		l.Start(&proc{})
		defer l.Stop()

		wg.Wait()

		lock.Lock()
		done = true
		lock.Unlock()
	}()

	require.Eventually(t, func() bool {
		lock.Lock()
		defer lock.Unlock()

		return done
	}, 2*time.Second, 100*time.Millisecond)
}

func TestGPUMemoryLimitWaitFor(t *testing.T) {
	lock := sync.Mutex{}

	lock.Lock()
	done := false
	lock.Unlock()

	go func() {
		wg := sync.WaitGroup{}
		wg.Add(1)

		l, _ := NewLimiter(LimiterConfig{
			GPUMemory: 42,
			WaitFor:   3 * time.Second,
			OnLimit: func(float64, uint64, float64, float64, float64, uint64) {
				wg.Done()
			},
		})

		l.Start(&proc{})
		defer l.Stop()

		wg.Wait()

		lock.Lock()
		done = true
		lock.Unlock()
	}()

	require.Eventually(t, func() bool {
		lock.Lock()
		defer lock.Unlock()

		return done
	}, 10*time.Second, 1*time.Second)
}

func TestMemoryLimitSoftMode(t *testing.T) {
	lock := sync.Mutex{}

	lock.Lock()
	done := false
	lock.Unlock()

	go func() {
		wg := sync.WaitGroup{}
		wg.Add(1)

		l, _ := NewLimiter(LimiterConfig{
			Memory: 42,
			Mode:   LimitModeSoft,
			OnLimit: func(float64, uint64, float64, float64, float64, uint64) {
				wg.Done()
			},
		})

		l.Start(&proc{})
		defer l.Stop()

		l.Limit(false, true, false)

		wg.Wait()

		lock.Lock()
		done = true
		lock.Unlock()
	}()

	require.Eventually(t, func() bool {
		lock.Lock()
		defer lock.Unlock()

		return done
	}, 2*time.Second, 100*time.Millisecond)
}

func TestGPUMemoryLimitSoftMode(t *testing.T) {
	lock := sync.Mutex{}

	lock.Lock()
	done := false
	lock.Unlock()

	go func() {
		wg := sync.WaitGroup{}
		wg.Add(1)

		l, _ := NewLimiter(LimiterConfig{
			GPUMemory: 42,
			Mode:      LimitModeSoft,
			OnLimit: func(float64, uint64, float64, float64, float64, uint64) {
				wg.Done()
			},
		})

		l.Start(&proc{})
		defer l.Stop()

		l.Limit(false, false, true)

		wg.Wait()

		lock.Lock()
		done = true
		lock.Unlock()
	}()

	require.Eventually(t, func() bool {
		lock.Lock()
		defer lock.Unlock()

		return done
	}, 2*time.Second, 100*time.Millisecond)
}
