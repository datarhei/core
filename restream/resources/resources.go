package resources

import (
	"context"
	"sync"
	"time"

	"github.com/datarhei/core/v16/psutil"
)

type resources struct {
	ncpu      float64
	maxCPU    float64
	maxMemory uint64

	consumerCPU    float64
	consumerMemory uint64

	limit      chan bool
	isLimiting bool

	cancelObserver context.CancelFunc

	lock      sync.Mutex
	startOnce sync.Once
	stopOnce  sync.Once
}

type Resources interface {
	Start()
	Stop()

	Limit() <-chan bool

	Add(cpu float64, memory uint64) bool
	Remove(cpu float64, memory uint64)
}

func New(maxCPU, maxMemory float64) (Resources, error) {
	r := &resources{
		maxCPU: maxCPU,
	}

	vmstat, err := psutil.VirtualMemory()
	if err != nil {
		return nil, err
	}

	ncpu, err := psutil.CPUCounts(true)
	if err != nil {
		ncpu = 1
	}

	r.ncpu = ncpu

	r.maxMemory = uint64(float64(vmstat.Total) * maxMemory / 100)

	r.stopOnce.Do(func() {})

	return r, nil
}

func (r *resources) Start() {
	r.startOnce.Do(func() {
		r.limit = make(chan bool, 10)

		ctx, cancel := context.WithCancel(context.Background())
		r.cancelObserver = cancel

		go r.observe(ctx, time.Second)

		r.stopOnce = sync.Once{}
	})
}

func (r *resources) Stop() {
	r.stopOnce.Do(func() {
		r.cancelObserver()

		r.startOnce = sync.Once{}
	})
}

func (r *resources) Limit() <-chan bool {
	return r.limit
}

func (r *resources) observe(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			limit := false

			cpustat, err := psutil.CPUPercent()
			if err != nil {
				continue
			}

			cpuload := cpustat.User + cpustat.System + cpustat.Other

			if cpuload > r.maxCPU {
				limit = true
			}

			vmstat, err := psutil.VirtualMemory()
			if err != nil {
				continue
			}

			if vmstat.Used > r.maxMemory {
				limit = true
			}

			r.lock.Lock()
			if r.isLimiting != limit {
				r.isLimiting = limit
				select {
				case r.limit <- limit:
				default:
				}
			}
			r.lock.Unlock()
		}
	}
}

func (r *resources) Add(cpu float64, memory uint64) bool {
	r.lock.Lock()
	defer r.lock.Unlock()

	if r.isLimiting {
		return false
	}

	if r.consumerCPU+cpu > r.maxCPU {
		return false
	}

	if r.consumerMemory+memory > r.maxMemory {
		return false
	}

	r.consumerCPU += cpu
	r.consumerMemory += memory

	return true
}

func (r *resources) Remove(cpu float64, memory uint64) {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.consumerCPU -= cpu
	r.consumerMemory -= memory
}
