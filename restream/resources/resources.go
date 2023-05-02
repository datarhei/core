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
	psutil psutil.Util

	ncpu      float64
	maxCPU    float64
	maxMemory uint64

	limitCPUCh    chan bool
	isCPULimiting bool

	limitMemoryCh    chan bool
	isMemoryLimiting bool

	cancelObserver context.CancelFunc

	lock      sync.Mutex
	startOnce sync.Once
	stopOnce  sync.Once

	logger log.Logger
}

type Resources interface {
	Start()
	Stop()

	LimitCPU() <-chan bool
	LimitMemory() <-chan bool

	Request(cpu float64, memory uint64) error
}

type Config struct {
	MaxCPU    float64
	MaxMemory float64
	PSUtil    psutil.Util
	Logger    log.Logger
}

func New(config Config) (Resources, error) {
	r := &resources{
		maxCPU: config.MaxCPU,
		psutil: config.PSUtil,
		logger: config.Logger,
	}

	if r.logger == nil {
		r.logger = log.New("")
	}

	if r.psutil == nil {
		r.psutil = psutil.DefaultUtil
	}

	vmstat, err := r.psutil.VirtualMemory()
	if err != nil {
		return nil, fmt.Errorf("unable to determine available memory: %w", err)
	}

	ncpu, err := r.psutil.CPUCounts(true)
	if err != nil {
		return nil, fmt.Errorf("unable to determine number of logical CPUs: %w", err)
	}

	r.ncpu = ncpu

	r.maxCPU *= r.ncpu
	r.maxMemory = uint64(float64(vmstat.Total) * config.MaxMemory / 100)

	r.limitCPUCh = make(chan bool, 10)
	r.limitMemoryCh = make(chan bool, 10)

	r.logger = r.logger.WithFields(log.Fields{
		"ncpu":       r.ncpu,
		"max_cpu":    r.maxCPU,
		"max_memory": r.maxMemory,
	})

	r.logger.Debug().Log("Created")

	r.stopOnce.Do(func() {})

	return r, nil
}

func (r *resources) Start() {
	r.startOnce.Do(func() {
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

func (r *resources) LimitCPU() <-chan bool {
	return r.limitCPUCh
}

func (r *resources) LimitMemory() <-chan bool {
	return r.limitMemoryCh
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
			cpustat, err := r.psutil.CPUPercent()
			if err != nil {
				r.logger.Warn().WithError(err).Log("Failed to determine system CPU usage")
				continue
			}

			cpuload := (cpustat.User + cpustat.System + cpustat.Other) * r.ncpu

			vmstat, err := r.psutil.VirtualMemory()
			if err != nil {
				r.logger.Warn().WithError(err).Log("Failed to determine system memory usage")
				continue
			}

			r.logger.Debug().WithFields(log.Fields{
				"cur_cpu":    cpuload,
				"cur_memory": vmstat.Used,
			}).Log("Observation")

			doCPULimit := false

			if !r.isCPULimiting {
				if cpuload > r.maxCPU {
					r.logger.Debug().WithField("cpu", cpuload).Log("CPU limit reached")
					doCPULimit = true
				}
			} else {
				doCPULimit = true
				if cpuload <= r.maxCPU {
					r.logger.Debug().WithField("cpu", cpuload).Log("CPU limit released")
					doCPULimit = false
				}
			}

			doMemoryLimit := false

			if !r.isMemoryLimiting {
				if vmstat.Used > r.maxMemory {
					r.logger.Debug().WithField("memory", vmstat.Used).Log("Memory limit reached")
					doMemoryLimit = true
				}
			} else {
				doMemoryLimit = true
				if vmstat.Used <= r.maxMemory {
					r.logger.Debug().WithField("memory", vmstat.Used).Log("Memory limit released")
					doMemoryLimit = false
				}
			}

			r.lock.Lock()
			if r.isCPULimiting != doCPULimit {
				r.logger.Debug().WithFields(log.Fields{
					"enabled": doCPULimit,
				}).Log("Limiting CPU")

				r.isCPULimiting = doCPULimit
				select {
				case r.limitCPUCh <- doCPULimit:
				default:
				}
			}

			if r.isMemoryLimiting != doMemoryLimit {
				r.logger.Debug().WithFields(log.Fields{
					"enabled": doMemoryLimit,
				}).Log("Limiting memory")

				r.isMemoryLimiting = doMemoryLimit
				select {
				case r.limitMemoryCh <- doMemoryLimit:
				default:
				}
			}
			r.lock.Unlock()
		}
	}
}

func (r *resources) Request(cpu float64, memory uint64) error {
	r.lock.Lock()
	defer r.lock.Unlock()

	logger := r.logger.WithFields(log.Fields{
		"req_cpu":    cpu,
		"req_memory": memory,
	})

	logger.Debug().Log("Request for acquiring resources")

	if r.isCPULimiting || r.isMemoryLimiting {
		logger.Debug().Log("Rejected, currently limiting")
		return fmt.Errorf("resources are currenlty actively limited")
	}

	if cpu <= 0 || memory == 0 {
		logger.Debug().Log("Rejected, invalid values")
		return fmt.Errorf("the cpu and/or memory values are invalid: cpu=%f, memory=%d", cpu, memory)
	}

	cpustat, err := r.psutil.CPUPercent()
	if err != nil {
		r.logger.Warn().WithError(err).Log("Failed to determine system CPU usage")
		return fmt.Errorf("the system CPU usage couldn't be determined")
	}

	cpuload := (cpustat.User + cpustat.System + cpustat.Other) * r.ncpu

	vmstat, err := r.psutil.VirtualMemory()
	if err != nil {
		r.logger.Warn().WithError(err).Log("Failed to determine system memory usage")
		return fmt.Errorf("the system memory usage couldn't be determined")
	}

	if cpuload+cpu > r.maxCPU {
		logger.Debug().WithField("cur_cpu", cpuload).Log("Rejected, CPU limit exceeded")
		return fmt.Errorf("the CPU limit would be exceeded: %f + %f > %f", cpuload, cpu, r.maxCPU)
	}

	if vmstat.Used+memory > r.maxMemory {
		logger.Debug().WithField("cur_memory", vmstat.Used).Log("Rejected, memory limit exceeded")
		return fmt.Errorf("the memory limit would be exceeded: %d + %d > %d", vmstat.Used, memory, r.maxMemory)
	}

	logger.Debug().WithFields(log.Fields{
		"cur_cpu":    cpuload,
		"cur_memory": vmstat.Used,
	}).Log("Acquiring approved")

	return nil
}
