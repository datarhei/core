package process

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/datarhei/core/v16/log"
	"github.com/datarhei/core/v16/resources/psutil"
)

type Usage struct {
	CPU struct {
		NCPU         float64 // number of logical processors
		Current      float64 // percent 0-100*ncpu
		Average      float64 // percent 0-100*ncpu
		Max          float64 // percent 0-100*ncpu
		Limit        float64 // percent 0-100*ncpu
		IsThrottling bool
	}
	Memory struct {
		Current uint64  // bytes
		Average float64 // bytes
		Max     uint64  // bytes
		Limit   uint64  // bytes
	}
	GPU struct {
		Index  int // number of the GPU
		Memory struct {
			Current uint64  // bytes
			Average float64 // bytes
			Max     uint64  // bytes
			Limit   uint64  // bytes
		}
		Usage struct {
			Current float64 // percent 0-100
			Average float64 // percent 0-100
			Max     float64 // percent 0-100
			Limit   float64 // percent 0-100
		}
		Encoder struct {
			Current float64 // percent 0-100
			Average float64 // percent 0-100
			Max     float64 // percent 0-100
			Limit   float64 // percent 0-100
		}
		Decoder struct {
			Current float64 // percent 0-100
			Average float64 // percent 0-100
			Max     float64 // percent 0-100
			Limit   float64 // percent 0-100
		}
	}
}

type LimitFunc func(cpu float64, memory uint64, gpuusage, gpuencoder, gpudecoder float64, gpumemory uint64)

type LimitMode int

func (m LimitMode) String() string {
	if m == LimitModeHard {
		return "hard"
	}

	if m == LimitModeSoft {
		return "soft"
	}

	return "undefined"
}

const (
	LimitModeHard LimitMode = 0 // Killing the process if either resource is above the limit for a certain time.
	LimitModeSoft LimitMode = 1 // If activated, will throttle the CPU, otherwise killing the process if resources are above the limit.
)

type LimiterConfig struct {
	CPU        float64       // Max. CPU usage in percent 0-100 in hard mode, 0-100*ncpu in soft mode.
	Memory     uint64        // Max. memory usage in bytes.
	GPUUsage   float64       // Max. GPU general usage in percent 0-100.
	GPUEncoder float64       // Max. GPU encoder usage in percent 0-100.
	GPUDecoder float64       // Max. GPU decoder usage in percent 0-100.
	GPUMemory  uint64        // Max. GPU memory usage in bytes.
	WaitFor    time.Duration // Duration for one of the limits has to be above the limit until OnLimit gets triggered.
	OnLimit    LimitFunc     // Function to be triggered if limits are exceeded.
	Mode       LimitMode     // How to limit CPU usage.
	PSUtil     psutil.Util
	Logger     log.Logger
}

type Limiter interface {
	// Start starts the limiter with a psutil.Process.
	Start(process psutil.Process) error

	// Stop stops the limiter. The limiter can be reused by calling Start() again
	Stop()

	// Usage returns the current state of the limiter, such as current, average, max, and
	// limit values for CPU and memory.
	Usage() Usage

	// Limit enables or disables the throttling of the CPU or killing because of to much
	// memory or GPU consumption.
	Limit(cpu, memory, gpu bool) error

	// Mode returns in which mode the limiter is running in.
	Mode() LimitMode
}

type numbers interface {
	~uint64 | ~float64
}

type metric[T numbers] struct {
	limit       T         // Limit
	current     T         // Current load value
	last        T         // Last load value
	max         T         // Max. load value
	top         T         // Decaying max. load value
	avg         float64   // Average load value
	avgCounter  uint64    // Counter for average calculation
	limitSince  time.Time // Time when the limit has been reached (hard limiter mode)
	limitEnable bool
}

func (x *metric[T]) Reset() {
	var zero T

	x.current = zero
	x.last = zero
	x.max = zero
	x.top = zero
	x.avg = 0
	x.avgCounter = 0
	x.limitEnable = false
}

func (x *metric[T]) Current() T {
	return x.current
}

func (x *metric[T]) Top() T {
	return x.top
}

func (x *metric[T]) Max() T {
	return x.max
}

func (x *metric[T]) Avg() float64 {
	return x.avg
}

func (x *metric[T]) SetLimit(limit T) {
	x.limit = limit
}

func (x *metric[T]) Limit() T {
	return x.limit
}

func (x *metric[T]) DoLimit(limit bool) (enabled, changed bool) {
	if x.limitEnable != limit {
		x.limitEnable = limit
		changed = true
	}

	enabled = x.limitEnable

	return
}

func (x *metric[T]) IsLimitEnabled() bool {
	return x.limitEnable
}

func (x *metric[T]) Update(value T) {
	x.last, x.current = x.current, value

	if x.current > x.max {
		x.max = x.current
	}

	if x.current > x.top {
		x.top = x.current
	} else {
		x.top = T(float64(x.top) * 0.95)
	}

	x.avgCounter++

	x.avg = ((x.avg * float64(x.avgCounter-1)) + float64(x.current)) / float64(x.avgCounter)
}

func (x *metric[T]) IsExceeded(waitFor time.Duration, mode LimitMode) bool {
	if x.limit <= 0 {
		return false
	}

	if mode == LimitModeSoft {
		// Check if we actually should limit.
		if !x.limitEnable {
			return false
		}

		// If we are currently above the limit, the limit is exceeded.
		if x.current > x.limit {
			return true
		}
	} else {
		if x.current > x.limit {
			// Current value is higher than the limit.
			if x.last <= x.limit {
				// If the previous value is below the limit, then we reached the limit as of now.
				x.limitSince = time.Now()
			}

			if time.Since(x.limitSince) >= waitFor {
				return true
			}
		}
	}

	return false
}

type limiter struct {
	psutil psutil.Util

	ncpu       float64
	ncpuFactor float64
	proc       psutil.Process
	lock       sync.RWMutex
	cancel     context.CancelFunc
	onLimit    LimitFunc

	lastUsage     Usage
	lastUsageLock sync.RWMutex

	cpu           metric[float64] // CPU limit
	cpuThrottling bool            // Whether CPU throttling is currently active (soft limiter mode)

	memory metric[uint64] // Memory limit (bytes)

	gpu struct {
		memory  metric[uint64]  // GPU memory limit (0-100 percent)
		usage   metric[float64] // GPU load limit (0-100 percent)
		encoder metric[float64] // GPU encoder limit (0-100 percent)
		decoder metric[float64] // GPU decoder limit (0-100 percent)
	}

	waitFor time.Duration
	mode    LimitMode

	logger log.Logger
}

// NewLimiter returns a new Limiter
func NewLimiter(config LimiterConfig) (Limiter, error) {
	l := &limiter{
		waitFor: config.WaitFor,
		onLimit: config.OnLimit,
		mode:    config.Mode,
		psutil:  config.PSUtil,
		logger:  config.Logger,
	}

	l.cpu.SetLimit(config.CPU / 100)
	l.memory.SetLimit(config.Memory)
	l.gpu.memory.SetLimit(config.GPUMemory)
	l.gpu.usage.SetLimit(config.GPUUsage / 100)
	l.gpu.encoder.SetLimit(config.GPUEncoder / 100)
	l.gpu.decoder.SetLimit(config.GPUDecoder / 100)

	if l.logger == nil {
		l.logger = log.New("")
	}

	if l.psutil == nil {
		return nil, fmt.Errorf("no psutil provided")
	}

	if ncpu, err := l.psutil.CPUCounts(); err != nil {
		l.ncpu = 1
	} else {
		l.ncpu = ncpu
	}

	l.lastUsage.CPU.NCPU = l.ncpu
	l.lastUsage.CPU.Limit = l.cpu.Limit() * 100 * l.ncpu
	l.lastUsage.Memory.Limit = l.memory.Limit()
	l.lastUsage.GPU.Memory.Limit = l.gpu.memory.Limit()
	l.lastUsage.GPU.Usage.Limit = l.gpu.usage.Limit() * 100
	l.lastUsage.GPU.Encoder.Limit = l.gpu.encoder.Limit() * 100
	l.lastUsage.GPU.Decoder.Limit = l.gpu.decoder.Limit() * 100

	l.ncpuFactor = 1

	mode := "hard"
	if l.mode == LimitModeSoft {
		mode = "soft"
		l.cpu.SetLimit(l.cpu.Limit() / l.ncpu)
		l.ncpuFactor = l.ncpu
	}

	if l.onLimit == nil {
		l.onLimit = func(float64, uint64, float64, float64, float64, uint64) {}
	}

	l.logger = l.logger.WithFields(log.Fields{
		"cpu":        l.cpu.Limit() * l.ncpuFactor,
		"memory":     l.memory.Limit(),
		"gpumemory":  l.gpu.memory.Limit(),
		"gpuusage":   l.gpu.usage.Limit(),
		"gpuencoder": l.gpu.encoder.Limit(),
		"gpudecoder": l.gpu.decoder.Limit(),
		"mode":       mode,
	})

	return l, nil
}

func (l *limiter) reset() {
	l.cpu.Reset()
	l.cpuThrottling = false

	l.memory.Reset()

	l.gpu.memory.Reset()
	l.gpu.usage.Reset()
	l.gpu.encoder.Reset()
	l.gpu.decoder.Reset()
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

	go l.ticker(ctx, time.Second)

	if l.mode == LimitModeSoft {
		go l.limitCPU(ctx, l.cpu.Limit(), time.Second)
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
		case <-ticker.C:
			l.collect()
		}
	}
}

func (l *limiter) collect() {
	l.lock.Lock()
	proc := l.proc
	l.lock.Unlock()

	if proc == nil {
		return
	}

	mstat, merr := proc.Memory()
	cpustat, cerr := proc.CPU()
	gstat, gerr := proc.GPU()
	gindex := -1

	l.lock.Lock()
	defer l.lock.Unlock()

	if merr == nil {
		l.memory.Update(mstat)
	}

	if cerr == nil {
		l.cpu.Update((cpustat.System + cpustat.User + cpustat.Other) / 100)
	}

	if gerr == nil {
		l.gpu.memory.Update(gstat.MemoryUsed)
		l.gpu.usage.Update(gstat.Usage / 100)
		l.gpu.encoder.Update(gstat.Encoder / 100)
		l.gpu.decoder.Update(gstat.Decoder / 100)
		gindex = gstat.Index
	}

	isLimitExceeded := false

	if l.mode == LimitModeHard {
		if l.cpu.IsExceeded(l.waitFor, l.mode) {
			l.logger.Warn().Log("CPU limit exceeded")
			isLimitExceeded = true
		}
	}

	if l.memory.IsExceeded(l.waitFor, l.mode) {
		l.logger.Warn().Log("Memory limit exceeded")
		isLimitExceeded = true
	}

	if l.gpu.memory.IsExceeded(l.waitFor, l.mode) {
		l.logger.Warn().Log("GPU memory limit exceeded")
		isLimitExceeded = true
	}

	if l.gpu.usage.IsExceeded(l.waitFor, l.mode) {
		l.logger.Warn().Log("GPU usage limit exceeded")
		isLimitExceeded = true
	}

	if l.gpu.encoder.IsExceeded(l.waitFor, l.mode) {
		l.logger.Warn().Log("GPU encoder limit exceeded")
		isLimitExceeded = true
	}

	if l.gpu.decoder.IsExceeded(l.waitFor, l.mode) {
		l.logger.Warn().Log("GPU decoder limit exceeded")
		isLimitExceeded = true
	}

	l.logger.Debug().WithFields(log.Fields{
		"cur_cpu":     l.cpu.Current() * l.ncpuFactor,
		"top_cpu":     l.cpu.Top() * l.ncpuFactor,
		"cur_mem":     l.memory.Current(),
		"top_mem":     l.memory.Top(),
		"cur_gpu_mem": l.gpu.memory.Current(),
		"top_gpu_mem": l.gpu.memory.Top(),
		"exceeded":    isLimitExceeded,
	}).Log("Observation")

	if isLimitExceeded {
		go l.onLimit(l.cpu.Current()*l.ncpuFactor*100, l.memory.Current(), l.gpu.usage.Current(), l.gpu.encoder.Current(), l.gpu.decoder.Current(), l.gpu.memory.Current())
	}

	l.lastUsageLock.Lock()
	l.lastUsage.CPU.Current = l.cpu.Current() * l.ncpu * 100
	l.lastUsage.CPU.Average = l.cpu.Avg() * l.ncpu * 100
	l.lastUsage.CPU.Max = l.cpu.Max() * l.ncpu * 100
	l.lastUsage.CPU.IsThrottling = l.cpuThrottling

	l.lastUsage.Memory.Current = l.memory.Current()
	l.lastUsage.Memory.Average = l.memory.Avg()
	l.lastUsage.Memory.Max = l.memory.Max()

	l.lastUsage.GPU.Index = gindex
	l.lastUsage.GPU.Memory.Current = l.gpu.memory.Current() * 100
	l.lastUsage.GPU.Memory.Average = l.gpu.memory.Avg() * 100
	l.lastUsage.GPU.Memory.Max = l.gpu.memory.Max() * 100

	l.lastUsage.GPU.Usage.Current = l.gpu.usage.Current() * 100
	l.lastUsage.GPU.Usage.Average = l.gpu.usage.Avg() * 100
	l.lastUsage.GPU.Usage.Max = l.gpu.usage.Max() * 100

	l.lastUsage.GPU.Encoder.Current = l.gpu.encoder.Current() * 100
	l.lastUsage.GPU.Encoder.Average = l.gpu.encoder.Avg() * 100
	l.lastUsage.GPU.Encoder.Max = l.gpu.encoder.Max() * 100

	l.lastUsage.GPU.Decoder.Current = l.gpu.decoder.Current() * 100
	l.lastUsage.GPU.Decoder.Average = l.gpu.decoder.Avg() * 100
	l.lastUsage.GPU.Decoder.Max = l.gpu.decoder.Max() * 100
	l.lastUsageLock.Unlock()
}

func (l *limiter) Limit(cpu, memory, gpu bool) error {
	l.lock.Lock()
	defer l.lock.Unlock()

	if l.mode == LimitModeHard {
		return nil
	}

	enabled, changed := l.cpu.DoLimit(cpu)
	if enabled && changed {
		l.logger.Debug().Log("CPU limiter enabled")
	} else if !enabled && changed {
		l.logger.Debug().Log("CPU limiter disabled")
	}

	enabled, changed = l.memory.DoLimit(memory)
	if enabled && changed {
		l.logger.Debug().Log("Memory limiter enabled")
	} else if !enabled && changed {
		l.logger.Debug().Log("Memory limiter disabled")
	}

	enabled, changed = l.gpu.memory.DoLimit(gpu)
	if enabled && changed {
		l.logger.Debug().Log("GPU limiter enabled")
	} else if !enabled && changed {
		l.logger.Debug().Log("GPU limiter disabled")
	}

	l.gpu.usage.DoLimit(gpu)
	l.gpu.encoder.DoLimit(gpu)
	l.gpu.decoder.DoLimit(gpu)

	return nil
}

// limitCPU will limit the CPU usage of this process. The limit is the max. CPU usage
// normed to 0-1. The interval defines how long a time slot is that will be splitted
// into sleeping and working.
// Inspired by https://github.com/opsengine/cpulimit
func (l *limiter) limitCPU(ctx context.Context, limit float64, interval time.Duration) {
	defer func() {
		l.lock.Lock()
		if l.proc != nil {
			l.proc.Resume()
		}
		l.cpuThrottling = false
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

		if !l.cpu.IsLimitEnabled() {
			if factorTopLimit > 0 {
				factorTopLimit -= 10
			} else {
				if l.cpuThrottling {
					if l.proc != nil {
						l.proc.Resume()
					}
					l.cpuThrottling = false
				}
				l.lock.Unlock()
				time.Sleep(100 * time.Millisecond)
				continue
			}
		} else {
			factorTopLimit = 100
			topLimit = l.cpu.Top() - limit
			l.cpuThrottling = true
		}

		lim := limit

		if topLimit > 0 {
			// After releasing the limiter, the process will not get the full CPU capacity back.
			// Instead the limit will be gradually lifted by increments until it reaches the
			// CPU top value. The CPU top value has to be larger than the actual limit.
			lim += (100 - factorTopLimit) / 100 * topLimit
		}

		pcpu := l.cpu.Current()

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

func (l *limiter) Usage() Usage {
	l.lastUsageLock.RLock()
	defer l.lastUsageLock.RUnlock()

	return l.lastUsage
}

func (l *limiter) Mode() LimitMode {
	return l.mode
}
