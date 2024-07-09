package resources

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/datarhei/core/v16/log"
	"github.com/datarhei/core/v16/psutil"
)

type Info struct {
	Mem MemoryInfo
	CPU CPUInfo
}

type MemoryInfo struct {
	Total      uint64 // bytes
	Available  uint64 // bytes
	Used       uint64 // bytes
	Limit      uint64 // bytes
	Throttling bool
	Error      error
}

type CPUInfo struct {
	NCPU       float64 // number of cpus
	System     float64 // percent 0-100
	User       float64 // percent 0-100
	Idle       float64 // percent 0-100
	Other      float64 // percent 0-100
	Limit      float64 // percent 0-100
	Throttling bool
	Error      error
}

type resources struct {
	psutil psutil.Util

	ncpu      float64
	maxCPU    float64 // percent 0-100*ncpu
	maxMemory uint64  // bytes

	isUnlimited      bool
	isCPULimiting    bool
	isMemoryLimiting bool

	cancelObserver context.CancelFunc

	lock      sync.RWMutex
	startOnce sync.Once
	stopOnce  sync.Once

	logger log.Logger
}

type Resources interface {
	Start()
	Stop()

	// HasLimits returns whether any limits have been set.
	HasLimits() bool

	// Limits returns the CPU (percent 0-100) and memory (bytes) limits.
	Limits() (float64, uint64)

	// ShouldLimit returns whether cpu and/or memory is currently limited.
	ShouldLimit() (bool, bool)

	// Request checks whether the requested resources are available.
	Request(cpu float64, memory uint64) error

	// Info returns the current resource usage
	Info() Info
}

type Config struct {
	MaxCPU    float64 // percent 0-100
	MaxMemory float64 // percent 0-100
	PSUtil    psutil.Util
	Logger    log.Logger
}

func New(config Config) (Resources, error) {
	isUnlimited := false

	if config.MaxCPU <= 0 && config.MaxMemory <= 0 {
		isUnlimited = true
	}

	if config.MaxCPU <= 0 {
		config.MaxCPU = 100
	}

	if config.MaxMemory <= 0 {
		config.MaxMemory = 100
	}

	if config.MaxCPU > 100 || config.MaxMemory > 100 {
		return nil, fmt.Errorf("both MaxCPU and MaxMemory must have a range of 0-100")
	}

	r := &resources{
		maxCPU:      config.MaxCPU,
		psutil:      config.PSUtil,
		isUnlimited: isUnlimited,
		logger:      config.Logger,
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

		r.logger.Info().Log("Started")
	})
}

func (r *resources) Stop() {
	r.stopOnce.Do(func() {
		r.cancelObserver()

		r.startOnce = sync.Once{}

		r.logger.Info().Log("Stopped")
	})
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

			if !r.isUnlimited {
				if !r.isCPULimiting {
					if cpuload >= r.maxCPU {
						r.logger.Debug().WithField("cpu", cpuload).Log("CPU limit reached")
						doCPULimit = true
					}
				} else {
					doCPULimit = true
					if cpuload < r.maxCPU {
						r.logger.Debug().WithField("cpu", cpuload).Log("CPU limit released")
						doCPULimit = false
					}
				}
			}

			doMemoryLimit := false

			if !r.isUnlimited {
				if !r.isMemoryLimiting {
					if vmstat.Used >= r.maxMemory {
						r.logger.Debug().WithField("memory", vmstat.Used).Log("Memory limit reached")
						doMemoryLimit = true
					}
				} else {
					doMemoryLimit = true
					if vmstat.Used < r.maxMemory {
						r.logger.Debug().WithField("memory", vmstat.Used).Log("Memory limit released")
						doMemoryLimit = false
					}
				}
			}

			r.lock.Lock()
			if r.isCPULimiting != doCPULimit {
				r.logger.Warn().WithFields(log.Fields{
					"enabled": doCPULimit,
				}).Log("Limiting CPU")

				r.isCPULimiting = doCPULimit
			}

			if r.isMemoryLimiting != doMemoryLimit {
				r.logger.Warn().WithFields(log.Fields{
					"enabled": doMemoryLimit,
				}).Log("Limiting memory")

				r.isMemoryLimiting = doMemoryLimit
			}
			r.lock.Unlock()
		}
	}
}

func (r *resources) HasLimits() bool {
	return !r.isUnlimited
}

func (r *resources) Limits() (float64, uint64) {
	return r.maxCPU / r.ncpu, r.maxMemory
}

func (r *resources) ShouldLimit() (bool, bool) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	return r.isCPULimiting, r.isMemoryLimiting
}

func (r *resources) Request(cpu float64, memory uint64) error {
	r.lock.RLock()
	defer r.lock.RUnlock()

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

func (r *resources) Info() Info {
	cpulimit, memlimit := r.Limits()
	cputhrottling, memthrottling := r.ShouldLimit()

	cpustat, cpuerr := r.psutil.CPUPercent()
	memstat, memerr := r.psutil.VirtualMemory()

	cpuinfo := CPUInfo{
		NCPU:       r.ncpu,
		System:     cpustat.System,
		User:       cpustat.User,
		Idle:       cpustat.Idle,
		Other:      cpustat.Other,
		Limit:      cpulimit,
		Throttling: cputhrottling,
		Error:      cpuerr,
	}

	meminfo := MemoryInfo{
		Total:      memstat.Total,
		Available:  memstat.Available,
		Used:       memstat.Used,
		Limit:      memlimit,
		Throttling: memthrottling,
		Error:      memerr,
	}

	i := Info{
		CPU: cpuinfo,
		Mem: meminfo,
	}

	return i
}
