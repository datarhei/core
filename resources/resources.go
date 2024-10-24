package resources

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/datarhei/core/v16/log"
	"github.com/datarhei/core/v16/psutil"
	"github.com/datarhei/core/v16/slices"
)

type Info struct {
	Mem MemoryInfo
	CPU CPUInfo
	GPU GPUInfo
}

type MemoryInfo struct {
	Total      uint64 // bytes
	Available  uint64 // bytes
	Used       uint64 // bytes
	Limit      uint64 // bytes
	Core       uint64 // bytes
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
	Core       float64 // percent 0-100
	Throttling bool
	Error      error
}

type GPUInfo struct {
	NGPU  float64 // number of gpus
	GPU   []GPUInfoStat
	Error error
}

type GPUInfoStat struct {
	Index int
	Name  string

	// Memory
	MemoryTotal     uint64 // bytes
	MemoryUsed      uint64 // bytes
	MemoryAvailable uint64 // bytes
	MemoryLimit     uint64 // bytes

	// GPU
	Usage      float64 // percent 0-100
	Encoder    float64 // percent 0-100
	Decoder    float64 // percent 0-100
	UsageLimit float64 // percent 0-100

	Throttling bool
}

type Request struct {
	CPU        float64 // percent 0-100*ncpu
	Memory     uint64  // bytes
	GPUUsage   float64 // percent 0-100
	GPUEncoder float64 // percent 0-100
	GPUDecoder float64 // percent 0-100
	GPUMemory  uint64  // bytes
}

type Response struct {
	GPU int // GPU number, hwdevice
}

type resources struct {
	psutil psutil.Util

	ncpu      float64
	maxCPU    float64 // percent 0-100*ncpu
	maxMemory uint64  // bytes

	ngpu         int
	maxGPU       float64 // general usage, percent 0-100
	maxGPUMemory float64 // memory usage, percent 0-100

	isUnlimited      bool
	isCPULimiting    bool
	isMemoryLimiting bool
	isGPULimiting    []bool

	self psutil.Process

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

	// Limits returns the CPU (percent 0-100), memory (bytes) limits, and GPU limits (usage and memory each in percent 0-100).
	Limits() (float64, uint64, float64, float64)

	// ShouldLimit returns whether cpu, memory, and/or GPU is currently limited.
	ShouldLimit() (bool, bool, []bool)

	// Request checks whether the requested resources are available.
	Request(req Request) (Response, error)

	// Info returns the current resource usage.
	Info() Info
}

type Config struct {
	MaxCPU       float64 // percent 0-100
	MaxMemory    float64 // percent 0-100
	MaxGPU       float64 // general,encoder,decoder usage, percent 0-100
	MaxGPUMemory float64 // memory usage, percent 0-100
	PSUtil       psutil.Util
	Logger       log.Logger
}

func New(config Config) (Resources, error) {
	if config.PSUtil == nil {
		config.PSUtil = psutil.DefaultUtil
	}

	gpu, err := config.PSUtil.GPU()
	if err != nil {
		return nil, fmt.Errorf("unable to determine number of GPUs: %w", err)
	}

	if len(gpu) == 0 {
		config.MaxGPU = 0
		config.MaxGPUMemory = 0
	}

	isUnlimited := false

	if config.MaxCPU <= 0 && config.MaxMemory <= 0 && config.MaxGPU <= 0 && config.MaxGPUMemory <= 0 {
		isUnlimited = true
	}

	if config.MaxCPU <= 0 {
		config.MaxCPU = 100
	}

	if config.MaxMemory <= 0 {
		config.MaxMemory = 100
	}

	if config.MaxGPU <= 0 {
		config.MaxGPU = 100
	}

	if config.MaxGPUMemory <= 0 {
		config.MaxGPUMemory = 100
	}

	if config.MaxCPU > 100 || config.MaxMemory > 100 || config.MaxGPU > 100 || config.MaxGPUMemory > 100 {
		return nil, fmt.Errorf("all Max... values must have a range of 0-100")
	}

	r := &resources{
		maxCPU:        config.MaxCPU,
		maxGPU:        config.MaxGPU,
		maxGPUMemory:  config.MaxGPUMemory,
		psutil:        config.PSUtil,
		isUnlimited:   isUnlimited,
		ngpu:          len(gpu),
		isGPULimiting: make([]bool, len(gpu)),
		logger:        config.Logger,
	}

	if r.logger == nil {
		r.logger = log.New("")
	}

	vmstat, err := r.psutil.Memory()
	if err != nil {
		return nil, fmt.Errorf("unable to determine available memory: %w", err)
	}

	ncpu, err := r.psutil.CPUCounts()
	if err != nil {
		return nil, fmt.Errorf("unable to determine number of logical CPUs: %w", err)
	}

	r.ncpu = ncpu

	r.maxCPU *= r.ncpu
	r.maxMemory = uint64(float64(vmstat.Total) * config.MaxMemory / 100)

	r.logger = r.logger.WithFields(log.Fields{
		"ncpu":           r.ncpu,
		"max_cpu":        r.maxCPU,
		"max_memory":     r.maxMemory,
		"ngpu":           len(gpu),
		"max_gpu":        r.maxGPU,
		"max_gpu_memory": r.maxGPUMemory,
	})

	r.self, err = r.psutil.Process(int32(os.Getpid()))
	if err != nil {
		return nil, fmt.Errorf("unable to create process observer for self: %w", err)
	}

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
		r.self.Stop()

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
			if r.isUnlimited {
				// If there aren't any limits imposed, don't do anything
				continue
			}

			cpustat, err := r.psutil.CPU()
			if err != nil {
				r.logger.Warn().WithError(err).Log("Failed to determine system CPU usage")
				continue
			}

			cpuload := (cpustat.User + cpustat.System + cpustat.Other) * r.ncpu

			vmstat, err := r.psutil.Memory()
			if err != nil {
				r.logger.Warn().WithError(err).Log("Failed to determine system memory usage")
				continue
			}

			gpustat, err := r.psutil.GPU()
			if err != nil {
				r.logger.Warn().WithError(err).Log("Failed to determine GPU usage")
				continue
			}

			r.logger.Debug().WithFields(log.Fields{
				"cur_cpu":    cpuload,
				"cur_memory": vmstat.Used,
			}).Log("Observation")

			doCPULimit := false

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

			doMemoryLimit := false

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

			doGPULimit := make([]bool, r.ngpu)

			for i, limiting := range r.isGPULimiting {
				maxMemory := uint64(r.maxGPUMemory * float64(gpustat[i].MemoryTotal) / 100)
				if !limiting {
					if gpustat[i].MemoryUsed >= maxMemory || (gpustat[i].Usage >= r.maxGPU && gpustat[i].Encoder >= r.maxGPU && gpustat[i].Decoder >= r.maxGPU) {
						doGPULimit[i] = true
					}
				} else {
					doGPULimit[i] = true
					if gpustat[i].MemoryUsed < maxMemory && (gpustat[i].Usage < r.maxGPU || gpustat[i].Encoder < r.maxGPU || gpustat[i].Decoder < r.maxGPU) {
						doGPULimit[i] = false
					}
				}
			}

			r.lock.Lock()
			if r.isCPULimiting != doCPULimit {
				r.logger.Warn().WithFields(log.Fields{
					"enabled": doCPULimit,
				}).Log("Limiting CPU")
			}
			r.isCPULimiting = doCPULimit

			if r.isMemoryLimiting != doMemoryLimit {
				r.logger.Warn().WithFields(log.Fields{
					"enabled": doMemoryLimit,
				}).Log("Limiting memory")
			}
			r.isMemoryLimiting = doMemoryLimit

			for i, limiting := range r.isGPULimiting {
				if limiting != doGPULimit[i] {
					r.logger.Warn().WithFields(log.Fields{
						"enabled": doGPULimit,
						"index":   i,
					}).Log("Limiting GPU")
				}
			}
			r.isGPULimiting = doGPULimit

			r.lock.Unlock()
		}
	}
}

func (r *resources) HasLimits() bool {
	return !r.isUnlimited
}

func (r *resources) Limits() (float64, uint64, float64, float64) {
	return r.maxCPU / r.ncpu, r.maxMemory, r.maxGPU, r.maxGPUMemory
}

func (r *resources) ShouldLimit() (bool, bool, []bool) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	return r.isCPULimiting, r.isMemoryLimiting, slices.Copy(r.isGPULimiting)
}

func (r *resources) Request(req Request) (Response, error) {
	res := Response{
		GPU: -1,
	}

	r.lock.RLock()
	defer r.lock.RUnlock()

	logger := r.logger.WithFields(log.Fields{
		"req_cpu":         req.CPU,
		"req_memory":      req.Memory,
		"req_gpu":         req.GPUUsage,
		"req_gpu_encoder": req.GPUEncoder,
		"req_gpu_decoder": req.GPUDecoder,
		"req_gpu_memory":  req.GPUMemory,
	})

	logger.Debug().Log("Request for acquiring resources")

	// Check if anything is currently limiting.
	if r.isCPULimiting || r.isMemoryLimiting {
		logger.Debug().Log("Rejected, currently limiting")
		return res, fmt.Errorf("resources are currenlty actively limited")
	}

	// Check if the requested resources are valid.
	if req.CPU <= 0 || req.Memory == 0 {
		logger.Debug().Log("Rejected, invalid values")
		return res, fmt.Errorf("the cpu and/or memory values are invalid: cpu=%f, memory=%d", req.CPU, req.Memory)
	}

	// Get current CPU and memory values.
	cpustat, err := r.psutil.CPU()
	if err != nil {
		r.logger.Warn().WithError(err).Log("Failed to determine system CPU usage")
		return res, fmt.Errorf("the system CPU usage couldn't be determined")
	}

	cpuload := (cpustat.User + cpustat.System + cpustat.Other) * r.ncpu

	vmstat, err := r.psutil.Memory()
	if err != nil {
		r.logger.Warn().WithError(err).Log("Failed to determine system memory usage")
		return res, fmt.Errorf("the system memory usage couldn't be determined")
	}

	// Check if enough resources are available
	if cpuload+req.CPU > r.maxCPU {
		logger.Debug().WithField("cur_cpu", cpuload).Log("Rejected, CPU limit exceeded")
		return res, fmt.Errorf("the CPU limit would be exceeded: %f + %f > %f", cpuload, req.CPU, r.maxCPU)
	}

	if vmstat.Used+req.Memory > r.maxMemory {
		logger.Debug().WithField("cur_memory", vmstat.Used).Log("Rejected, memory limit exceeded")
		return res, fmt.Errorf("the memory limit would be exceeded: %d + %d > %d", vmstat.Used, req.Memory, r.maxMemory)
	}

	// Check if any GPU resources are requested
	if req.GPUUsage > 0 || req.GPUEncoder > 0 || req.GPUDecoder > 0 || req.GPUMemory > 0 {
		if req.GPUUsage < 0 || req.GPUEncoder < 0 || req.GPUDecoder < 0 || req.GPUMemory == 0 {
			logger.Debug().Log("Rejected, invalid values")
			return res, fmt.Errorf("the gpu usage and memory values are invalid: usage=%f, encoder=%f, decoder=%f, memory=%d", req.GPUUsage, req.GPUEncoder, req.GPUDecoder, req.GPUMemory)
		}

		// Get current GPU values
		gpustat, err := r.psutil.GPU()
		if err != nil {
			r.logger.Warn().WithError(err).Log("Failed to determine GPU usage")
			return res, fmt.Errorf("the GPU usage couldn't be determined")
		}

		if len(gpustat) == 0 {
			r.logger.Debug().WithError(err).Log("GPU resources requested but no GPU available")
			return res, fmt.Errorf("some GPU resources requested but no GPU available")
		}

		foundGPU := -1
		for _, g := range gpustat {
			if req.GPUUsage > 0 && g.Usage+req.GPUUsage > r.maxGPU {
				logger.Debug().WithFields(log.Fields{"id": g.Index, "cur_gpu": g.Usage}).Log("Rejected, GPU usage limit exceeded")
				continue
			}

			if req.GPUEncoder > 0 && g.Encoder+req.GPUEncoder > r.maxGPU {
				logger.Debug().WithFields(log.Fields{"id": g.Index, "cur_gpu_encoder": g.Usage}).Log("Rejected, GPU encoder usage limit exceeded")
				continue
			}

			if req.GPUDecoder > 0 && g.Decoder+req.GPUDecoder > r.maxGPU {
				logger.Debug().WithFields(log.Fields{"id": g.Index, "cur_gpu_decoder": g.Usage}).Log("Rejected, GPU decoder usage limit exceeded")
				continue
			}

			gpuMemoryUsage := float64(g.MemoryUsed) / float64(g.MemoryTotal) * 100
			requestedGPUMemoryUsage := float64(req.GPUMemory) / float64(g.MemoryTotal) * 100

			if gpuMemoryUsage+requestedGPUMemoryUsage > r.maxGPUMemory {
				logger.Debug().WithFields(log.Fields{"id": g.Index, "cur_gpu_memory": gpuMemoryUsage}).Log("Rejected, GPU memory usage limit exceeded")
				continue
			}

			foundGPU = g.Index

			logger = logger.Debug().WithFields(log.Fields{
				"cur_gpu":         foundGPU,
				"cur_gpu_general": g.Usage,
				"cur_gpu_encoder": g.Encoder,
				"cur_gpu_decoder": g.Decoder,
				"cur_gpu_memory":  gpuMemoryUsage,
			})

			break
		}

		if foundGPU < 0 {
			return res, fmt.Errorf("all GPU usage limits are exceeded")
		}

		res.GPU = foundGPU
	}

	logger.Debug().WithFields(log.Fields{
		"cur_cpu":    cpuload,
		"cur_memory": vmstat.Used,
	}).Log("Acquiring approved")

	return res, nil
}

func (r *resources) Info() Info {
	cpulimit, memlimit, gpulimit, gpumemlimit := r.Limits()
	cputhrottling, memthrottling, gputhrottling := r.ShouldLimit()

	cpustat, cpuerr := r.psutil.CPU()
	memstat, memerr := r.psutil.Memory()
	gpustat, gpuerr := r.psutil.GPU()
	selfcpu, _ := r.self.CPU()
	selfmem, _ := r.self.Memory()

	cpuinfo := CPUInfo{
		NCPU:       r.ncpu,
		System:     cpustat.System,
		User:       cpustat.User,
		Idle:       cpustat.Idle,
		Other:      cpustat.Other,
		Limit:      cpulimit,
		Core:       selfcpu.System + selfcpu.User + selfcpu.Other,
		Throttling: cputhrottling,
		Error:      cpuerr,
	}

	meminfo := MemoryInfo{
		Total:      memstat.Total,
		Available:  memstat.Available,
		Used:       memstat.Used,
		Limit:      memlimit,
		Core:       selfmem,
		Throttling: memthrottling,
		Error:      memerr,
	}

	gpuinfo := GPUInfo{
		NGPU:  float64(len(gpustat)),
		Error: gpuerr,
	}

	for i, g := range gpustat {
		gpuinfo.GPU = append(gpuinfo.GPU, GPUInfoStat{
			Index:           g.Index,
			Name:            g.Name,
			MemoryTotal:     g.MemoryTotal,
			MemoryUsed:      g.MemoryUsed,
			MemoryAvailable: g.MemoryTotal - g.MemoryUsed,
			MemoryLimit:     uint64(float64(g.MemoryTotal) * gpumemlimit / 100),
			Usage:           g.Usage,
			Encoder:         g.Encoder,
			Decoder:         g.Decoder,
			UsageLimit:      gpulimit,
			Throttling:      gputhrottling[i],
		})
	}

	i := Info{
		CPU: cpuinfo,
		Mem: meminfo,
		GPU: gpuinfo,
	}

	return i
}
