package process

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/datarhei/core/v16/log"
	"github.com/datarhei/core/v16/psutil"
)

type Usage struct {
	CPU struct {
		NCPU    float64 // number of logical processors
		Current float64 // percent 0-100*ncpu
		Average float64 // percent 0-100*ncpu
		Max     float64 // percent 0-100*ncpu
		Limit   float64 // percent 0-100*ncpu
	}
	Memory struct {
		Current uint64  // bytes
		Average float64 // bytes
		Max     uint64  // bytes
		Limit   uint64  // bytes
	}
}

type LimitFunc func(cpu float64, memory uint64)

type LimitMode int

const (
	LimitModeHard LimitMode = 0 // Killing the process if either CPU or memory is above the limit (for a certain time)
	LimitModeSoft LimitMode = 1 // Throttling the CPU if activated, killing the process if memory is above the limit
)

type LimiterConfig struct {
	CPU     float64       // Max. CPU usage in percent 0-100 in hard mode, 0-100*ncpu in softmode
	Memory  uint64        // Max. memory usage in bytes
	WaitFor time.Duration // Duration for one of the limits has to be above the limit until OnLimit gets triggered
	OnLimit LimitFunc     // Function to be triggered if limits are exceeded
	Mode    LimitMode     // How to limit CPU usage
	PSUtil  psutil.Util
	Logger  log.Logger
}

type Limiter interface {
	// Start starts the limiter with a psutil.Process.
	Start(process psutil.Process) error

	// Stop stops the limiter. The limiter can be reused by calling Start() again
	Stop()

	// Current returns the current CPU and memory values
	// Deprecated: use Usage()
	Current() (cpu float64, memory uint64)

	// Limits returns the defined CPU and memory limits. Values <= 0 means no limit
	// Deprecated: use Usage()
	Limits() (cpu float64, memory uint64)

	// Usage returns the current state of the limiter, such as current, average, max, and
	// limit values for CPU and memory.
	Usage() Usage

	// Limit enables or disables the throttling of the CPU or killing because of to much
	// memory consumption.
	Limit(cpu, memory bool) error
}

type limiter struct {
	psutil psutil.Util

	ncpu       float64
	ncpuFactor float64
	proc       psutil.Process
	lock       sync.Mutex
	cancel     context.CancelFunc
	onLimit    LimitFunc

	cpu            float64   // CPU limit
	cpuCurrent     float64   // Current CPU load of this process
	cpuLast        float64   // Last CPU load of this process
	cpuMax         float64   // Max. CPU load of this process
	cpuTop         float64   // Decaying max. CPU load of this process
	cpuAvg         float64   // Average CPU load of this process
	cpuAvgCounter  uint64    // Counter for average calculation
	cpuLimitSince  time.Time // Time when the CPU limit has been reached (hard limiter mode)
	cpuLimitEnable bool      // Whether CPU limiting is enabled (soft limiter mode)

	memory            uint64    // Memory limit (bytes)
	memoryCurrent     uint64    // Current memory usage
	memoryLast        uint64    // Last memory usage
	memoryMax         uint64    // Max. memory usage
	memoryTop         uint64    // Decaying max. memory usage
	memoryAvg         float64   // Average memory usage
	memoryAvgCounter  uint64    // Counter for average memory calculation
	memoryLimitSince  time.Time // Time when the memory limit has been reached (hard limiter mode)
	memoryLimitEnable bool      // Whether memory limiting is enabled (soft limiter mode)

	waitFor time.Duration
	mode    LimitMode

	cancelLimit context.CancelFunc

	logger log.Logger
}

// NewLimiter returns a new Limiter
func NewLimiter(config LimiterConfig) Limiter {
	l := &limiter{
		cpu:     config.CPU,
		memory:  config.Memory,
		waitFor: config.WaitFor,
		onLimit: config.OnLimit,
		mode:    config.Mode,
		psutil:  config.PSUtil,
		logger:  config.Logger,
	}

	if l.logger == nil {
		l.logger = log.New("")
	}

	if l.psutil == nil {
		l.psutil = psutil.DefaultUtil
	}

	if ncpu, err := l.psutil.CPUCounts(true); err != nil {
		l.ncpu = 1
	} else {
		l.ncpu = ncpu
	}

	l.ncpuFactor = 1

	mode := "hard"
	if l.mode == LimitModeSoft {
		mode = "soft"
		l.cpu /= l.ncpu
		l.ncpuFactor = l.ncpu
	}

	l.cpu /= 100

	if l.onLimit == nil {
		l.onLimit = func(float64, uint64) {}
	}

	l.logger = l.logger.WithFields(log.Fields{
		"cpu":    l.cpu * l.ncpuFactor,
		"memory": l.memory,
		"mode":   mode,
	})

	return l
}

func (l *limiter) reset() {
	l.cpuCurrent = 0
	l.cpuLast = 0
	l.cpuAvg = 0
	l.cpuAvgCounter = 0
	l.cpuMax = 0
	l.cpuTop = 0
	l.cpuLimitEnable = false

	l.memoryCurrent = 0
	l.memoryLast = 0
	l.memoryAvg = 0
	l.memoryAvgCounter = 0
	l.memoryMax = 0
	l.memoryTop = 0
	l.memoryLimitEnable = false
}

func (l *limiter) Start(process psutil.Process) error {
	l.lock.Lock()
	defer l.lock.Unlock()

	if l.proc != nil {
		return fmt.Errorf("limiter is already running")
	}

	l.reset()

	l.proc = process

	ctx, cancel := context.WithCancel(context.Background())
	l.cancel = cancel

	go l.ticker(ctx, 1000*time.Millisecond)

	if l.mode == LimitModeSoft {
		ctx, cancel = context.WithCancel(context.Background())
		l.cancelLimit = cancel

		go l.limitCPU(ctx, l.cpu, time.Second)
	}

	return nil
}

func (l *limiter) Stop() {
	l.lock.Lock()
	defer l.lock.Unlock()

	if l.proc == nil {
		return
	}

	l.cancel()

	if l.cancelLimit != nil {
		l.cancelLimit()
		l.cancelLimit = nil
	}

	l.proc.Stop()
	l.proc = nil

	l.reset()
}

func (l *limiter) ticker(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			l.collect(t)
		}
	}
}

func (l *limiter) collect(t time.Time) {
	l.lock.Lock()
	defer l.lock.Unlock()

	if l.proc == nil {
		return
	}

	if mstat, err := l.proc.VirtualMemory(); err == nil {
		l.memoryLast, l.memoryCurrent = l.memoryCurrent, mstat

		if l.memoryCurrent > l.memoryMax {
			l.memoryMax = l.memoryCurrent
		}

		if l.memoryCurrent > l.memoryTop {
			l.memoryTop = l.memoryCurrent
		} else {
			l.memoryTop = uint64(float64(l.memoryTop) * 0.95)
		}

		l.memoryAvgCounter++

		l.memoryAvg = ((l.memoryAvg * float64(l.memoryAvgCounter-1)) + float64(l.memoryCurrent)) / float64(l.memoryAvgCounter)
	}

	if cpustat, err := l.proc.CPUPercent(); err == nil {
		l.cpuLast, l.cpuCurrent = l.cpuCurrent, (cpustat.System+cpustat.User+cpustat.Other)/100

		if l.cpuCurrent > l.cpuMax {
			l.cpuMax = l.cpuCurrent
		}

		if l.cpuCurrent > l.cpuTop {
			l.cpuTop = l.cpuCurrent
		} else {
			l.cpuTop = l.cpuTop * 0.95
		}

		l.cpuAvgCounter++

		l.cpuAvg = ((l.cpuAvg * float64(l.cpuAvgCounter-1)) + l.cpuCurrent) / float64(l.cpuAvgCounter)
	}

	isLimitExceeded := false

	if l.mode == LimitModeHard {
		if l.cpu > 0 {
			if l.cpuCurrent > l.cpu {
				// Current value is higher than the limit
				if l.cpuLast <= l.cpu {
					// If the previous value is below the limit, then we reached the
					// limit as of now
					l.cpuLimitSince = time.Now()
				}

				if time.Since(l.cpuLimitSince) >= l.waitFor {
					l.logger.Warn().Log("CPU limit exceeded")
					isLimitExceeded = true
				}
			}
		}

		if l.memory > 0 {
			if l.memoryCurrent > l.memory {
				// Current value is higher than the limit
				if l.memoryLast <= l.memory {
					// If the previous value is below the limit, then we reached the
					// limit as of now
					l.memoryLimitSince = time.Now()
				}

				if time.Since(l.memoryLimitSince) >= l.waitFor {
					l.logger.Warn().Log("Memory limit exceeded")
					isLimitExceeded = true
				}
			}
		}
	} else {
		if l.memory > 0 && l.memoryLimitEnable {
			if l.memoryCurrent > l.memory {
				// Current value is higher than the limit
				l.logger.Warn().Log("Memory limit exceeded")
				isLimitExceeded = true
			}
		}
	}

	l.logger.Debug().WithFields(log.Fields{
		"cur_cpu":  l.cpuCurrent * l.ncpuFactor,
		"top_cpu":  l.cpuTop * l.ncpuFactor,
		"cur_mem":  l.memoryCurrent,
		"top_mem":  l.memoryTop,
		"exceeded": isLimitExceeded,
	}).Log("Observation")

	if isLimitExceeded {
		go l.onLimit(l.cpuCurrent*l.ncpuFactor*100, l.memoryCurrent)
	}
}

func (l *limiter) Limit(cpu, memory bool) error {
	l.lock.Lock()
	defer l.lock.Unlock()

	if l.mode == LimitModeHard {
		return nil
	}

	if memory {
		if !l.memoryLimitEnable {
			l.memoryLimitEnable = true

			l.logger.Debug().Log("Memory limiter enabled")
		}
	} else {
		if l.memoryLimitEnable {
			l.memoryLimitEnable = false

			l.logger.Debug().Log("Memory limiter disabled")
		}
	}

	if cpu {
		if !l.cpuLimitEnable {
			l.cpuLimitEnable = true

			l.logger.Debug().Log("CPU limiter enabled")
		}
	} else {
		if l.cpuLimitEnable {
			l.cpuLimitEnable = false

			l.logger.Debug().Log("CPU limiter disabled")
		}

	}

	return nil
}

// limitCPU will limit the CPU usage of this process. The limit is the max. CPU usage
// normed to 0-1. The interval defines how long a time slot is that will be splitted
// into sleeping and working.
func (l *limiter) limitCPU(ctx context.Context, limit float64, interval time.Duration) {
	defer func() {
		l.lock.Lock()
		if l.proc != nil {
			l.proc.Resume()
		}
		l.lock.Unlock()

		l.logger.Debug().Log("CPU throttler disabled")
	}()

	var workingrate float64 = -1
	var factorTopLimit float64 = 0
	var topLimit float64 = 0

	l.logger.Debug().WithField("limit", limit*l.ncpu).Log("CPU throttler enabled")

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		l.lock.Lock()

		if !l.cpuLimitEnable {
			if factorTopLimit > 0 {
				factorTopLimit -= 10
			} else {
				if l.proc != nil {
					l.proc.Resume()
				}
				l.lock.Unlock()
				time.Sleep(100 * time.Millisecond)
				continue
			}
		} else {
			factorTopLimit = 100
			topLimit = l.cpuTop - limit
		}

		lim := limit

		if topLimit > 0 {
			// After releasing the limiter, the process will not get the full CPU capacity back.
			// Instead the limit will be gradually lifted by increments until it reaches the
			// CPU top value. The CPU top value has to be larger than the actual limit.
			lim += (100 - factorTopLimit) / 100 * topLimit
		}

		pcpu := l.cpuCurrent

		l.lock.Unlock()

		if workingrate < 0 {
			workingrate = limit
		}
		// else {
		//	workingrate = math.Min(workingrate/pcpu*limit, 1)
		//}

		workingrate = lim

		worktime := float64(interval.Nanoseconds()) * workingrate
		sleeptime := float64(interval.Nanoseconds()) - worktime

		l.logger.Debug().WithFields(log.Fields{
			"limit":     lim * l.ncpu,
			"pcpu":      pcpu,
			"factor":    factorTopLimit,
			"worktime":  (time.Duration(worktime) * time.Nanosecond).String(),
			"sleeptime": (time.Duration(sleeptime) * time.Nanosecond).String(),
		}).Log("Throttler")

		l.lock.Lock()
		if l.proc != nil {
			l.proc.Resume()
		}
		l.lock.Unlock()

		time.Sleep(time.Duration(worktime) * time.Nanosecond)

		if sleeptime > 0 {
			l.lock.Lock()
			if l.proc != nil {
				l.proc.Suspend()
			}
			l.lock.Unlock()

			time.Sleep(time.Duration(sleeptime) * time.Nanosecond)
		}
	}
}

func (l *limiter) Current() (cpu float64, memory uint64) {
	l.lock.Lock()
	defer l.lock.Unlock()

	cpu = l.cpuCurrent * 100
	memory = l.memoryCurrent * 100

	return
}

func (l *limiter) Usage() Usage {
	l.lock.Lock()
	defer l.lock.Unlock()

	usage := Usage{}

	usage.CPU.NCPU = l.ncpu
	usage.CPU.Limit = l.cpu * l.ncpu * 100
	usage.CPU.Current = l.cpuCurrent * l.ncpu * 100
	usage.CPU.Average = l.cpuAvg * l.ncpu * 100
	usage.CPU.Max = l.cpuMax * l.ncpu * 100

	usage.Memory.Limit = l.memory
	usage.Memory.Current = l.memoryCurrent
	usage.Memory.Average = l.memoryAvg
	usage.Memory.Max = l.memoryMax

	return usage
}

func (l *limiter) Limits() (cpu float64, memory uint64) {
	return l.cpu * 100, l.memory
}
