package psutil

import (
	"context"
	"sync"
	"time"

	"github.com/datarhei/core/v16/resources/psutil/gpu"
	psprocess "github.com/shirou/gopsutil/v3/process"
)

type Process interface {
	// CPU returns the current CPU load for this process only. The values
	// are normed to the range of 0 to 100.
	CPU() (*CPUInfo, error)

	// Memory returns the current memory usage in bytes of this process only.
	Memory() (uint64, error)

	// GPU returns the current GPU memory in bytes and usage in percent (0-100) of this process only.
	GPU() (*GPUInfo, error)

	// Stop will stop collecting CPU and memory data for this process.
	Stop()

	// Suspend will send SIGSTOP to the process.
	Suspend() error

	// Resume will send SIGCONT to the process.
	Resume() error
}

type process struct {
	pid       int32
	hasCgroup bool
	cpuLimit  uint64
	ncpu      float64
	proc      *psprocess.Process

	stopTicker context.CancelFunc

	lock             sync.RWMutex
	statCurrent      cpuTimesStat
	statCurrentTime  time.Time
	statPrevious     cpuTimesStat
	statPreviousTime time.Time
	nTicks           uint64
	memRSS           uint64

	gpu gpu.GPU
}

func (u *util) Process(pid int32) (Process, error) {
	p := &process{
		pid:       pid,
		hasCgroup: u.hasCgroup,
		cpuLimit:  u.cpuLimit,
		ncpu:      u.ncpu,
		gpu:       u.gpu,
	}

	proc, err := psprocess.NewProcess(pid)
	if err != nil {
		return nil, err
	}

	p.proc = proc

	ctx, cancel := context.WithCancel(context.Background())
	p.stopTicker = cancel
	go p.tickCPU(ctx, time.Second)
	go p.tickMemory(ctx, time.Second)

	return p, nil
}

func (p *process) tickCPU(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			stat := p.collectCPU()

			p.lock.Lock()
			p.statPrevious, p.statCurrent = p.statCurrent, stat
			p.statPreviousTime, p.statCurrentTime = p.statCurrentTime, t
			p.nTicks++
			p.lock.Unlock()
		}
	}
}

func (p *process) collectCPU() cpuTimesStat {
	stat, err := p.cpuTimes()
	if err != nil {
		return cpuTimesStat{
			total: float64(time.Now().Unix()),
			idle:  float64(time.Now().Unix()),
		}
	}

	return *stat
}

func (p *process) tickMemory(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rss := p.collectMemory()

			p.lock.Lock()
			p.memRSS = rss
			p.lock.Unlock()
		}
	}
}

func (p *process) collectMemory() uint64 {
	info, err := p.proc.MemoryInfo()
	if err != nil {
		return 0
	}

	return info.RSS
}

func (p *process) Stop() {
	p.stopTicker()
}

func (p *process) Suspend() error {
	return p.proc.Suspend()
}

func (p *process) Resume() error {
	return p.proc.Resume()
}

func (p *process) CPU() (*CPUInfo, error) {
	var diff float64

	for {
		p.lock.RLock()
		nTicks := p.nTicks
		p.lock.RUnlock()

		if nTicks < 2 {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		break
	}

	p.lock.RLock()
	defer p.lock.RUnlock()

	if p.hasCgroup && p.cpuLimit > 0 {
		diff = float64(p.cpuLimit) * (p.statCurrentTime.Sub(p.statPreviousTime)).Seconds() / 1e9
	} else {
		diff = p.statCurrentTime.Sub(p.statPreviousTime).Seconds() * p.ncpu
	}

	s := &CPUInfo{
		System: 0,
		User:   0,
		Idle:   0,
		Other:  0,
	}

	if diff <= 0 {
		return s, nil
	}

	s.System = 100 * (p.statCurrent.system - p.statPrevious.system) / diff
	s.User = 100 * (p.statCurrent.user - p.statPrevious.user) / diff
	s.Idle = 100 * (p.statCurrent.idle - p.statPrevious.idle) / diff
	s.Other = 100 * (p.statCurrent.other - p.statPrevious.other) / diff

	return s, nil
}

func (p *process) Memory() (uint64, error) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	return p.memRSS, nil
}

func (p *process) GPU() (*GPUInfo, error) {
	info := &GPUInfo{
		Index: -1,
	}

	proc, err := p.gpu.Process(p.pid)
	if err != nil {
		return info, nil
	}

	info.Index = proc.Index
	info.MemoryUsed = proc.Memory
	info.Usage = proc.Usage
	info.Encoder = proc.Encoder
	info.Decoder = proc.Decoder

	return info, nil
}
