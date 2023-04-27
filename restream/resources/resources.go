package resources

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/datarhei/core/v16/log"
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

	logger log.Logger
}

type Resources interface {
	Start()
	Stop()

	Limit() <-chan bool

	Add(cpu float64, memory uint64) bool
	Remove(cpu float64, memory uint64)
}

type Config struct {
	MaxCPU    float64
	MaxMemory float64
	Logger    log.Logger
}

func New(config Config) (Resources, error) {
	r := &resources{
		maxCPU: config.MaxCPU,
		logger: config.Logger,
	}

	vmstat, err := psutil.VirtualMemory()
	if err != nil {
		return nil, fmt.Errorf("unable to determine available memory: %w", err)
	}

	ncpu, err := psutil.CPUCounts(true)
	if err != nil {
		return nil, fmt.Errorf("unable to determine number of logical CPUs: %w", err)
	}

	r.ncpu = ncpu

	r.maxMemory = uint64(float64(vmstat.Total) * config.MaxMemory / 100)

	if r.logger == nil {
		r.logger = log.New("")
	}

	r.logger = r.logger.WithFields(log.Fields{
		"max_cpu":    r.maxCPU,
		"max_memory": r.maxMemory,
	})

	r.logger.Debug().Log("Created")

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

		r.logger.Debug().Log("Started")
	})
}

func (r *resources) Stop() {
	r.stopOnce.Do(func() {
		r.cancelObserver()

		r.startOnce = sync.Once{}

		r.logger.Debug().Log("Stopped")
	})
}

func (r *resources) Limit() <-chan bool {
	return r.limit
}

func (r *resources) observe(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	r.logger.Debug().Log("Observer started")

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cpustat, err := psutil.CPUPercent()
			if err != nil {
				r.logger.Warn().WithError(err).Log("Failed to determine CPU load")
				continue
			}

			cpuload := (cpustat.User + cpustat.System + cpustat.Other) * r.ncpu

			vmstat, err := psutil.VirtualMemory()
			if err != nil {
				continue
			}

			r.logger.Debug().WithFields(log.Fields{
				"cur_cpu":    cpuload,
				"cur_memory": vmstat.Used,
			}).Log("Observation")

			limit := false

			if !r.isLimiting {
				if cpuload > r.maxCPU {
					r.logger.Debug().WithField("cpu", cpuload).Log("CPU limit reached")
					limit = true
				}

				if vmstat.Used > r.maxMemory {
					r.logger.Debug().WithField("memory", vmstat.Used).Log("Memory limit reached")
					limit = true
				}
			} else {
				limit = true
				if cpuload <= r.maxCPU*0.8 {
					r.logger.Debug().WithField("cpu", cpuload).Log("CPU limit released")
					limit = false
				}

				if vmstat.Used <= uint64(float64(r.maxMemory)*0.8) {
					r.logger.Debug().WithField("memory", vmstat.Used).Log("Memory limit reached")
					limit = false
				}
			}

			r.lock.Lock()
			if r.isLimiting != limit {
				r.logger.Debug().WithField("enabled", limit).Log("Limiting")
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

	logger := r.logger.WithFields(log.Fields{
		"cpu":    cpu,
		"memory": memory,
	})

	logger.Debug().WithFields(log.Fields{
		"used_cpu":    r.consumerCPU,
		"used_memory": r.consumerMemory,
	}).Log("Request for acquiring resources")

	if r.isLimiting {
		logger.Debug().Log("Rejected, currently limiting")
		return false
	}

	if cpu <= 0 || memory == 0 {
		logger.Debug().Log("Rejected, invalid values")
		return false
	}

	if r.consumerCPU+cpu > r.maxCPU {
		logger.Debug().Log("Rejected, CPU limit exceeded")
		return false
	}

	if r.consumerMemory+memory > r.maxMemory {
		logger.Debug().Log("Rejected, memory limit exceeded")
		return false
	}

	r.consumerCPU += cpu
	r.consumerMemory += memory

	logger.Debug().WithFields(log.Fields{
		"used_cpu":    r.consumerCPU,
		"used_memory": r.consumerMemory,
	}).Log("Acquiring approved")

	return true
}

func (r *resources) Remove(cpu float64, memory uint64) {
	r.lock.Lock()
	defer r.lock.Unlock()

	logger := r.logger.WithFields(log.Fields{
		"cpu":    cpu,
		"memory": memory,
	})

	logger.Debug().WithFields(log.Fields{
		"used_cpu":    r.consumerCPU,
		"used_memory": r.consumerMemory,
	}).Log("Request for releasing resources")

	r.consumerCPU -= cpu
	r.consumerMemory -= memory

	if r.consumerCPU < 0 {
		logger.Warn().WithField("used_cpu", r.consumerCPU).Log("Used CPU resources below 0")
		r.consumerCPU = 0
	}

	logger.Debug().WithFields(log.Fields{
		"used_cpu":    r.consumerCPU,
		"used_memory": r.consumerMemory,
	}).Log("Releasing approved")
}
