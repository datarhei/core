package psutil

import (
	"context"
	"math"
	"sync"
	"time"

	psprocess "github.com/shirou/gopsutil/v3/process"
)

type Process interface {
	// CPUPercent returns the current CPU load for this process only. The values
	// are normed to the range of 0 to 100.
	CPUPercent() (*CPUInfoStat, error)

	// VirtualMemory returns the current memory usage in bytes of this process only.
	VirtualMemory() (uint64, error)

	// Stop will stop collecting CPU and memory data for this process.
	Stop()
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

	imposeLimit bool
}

func (u *util) Process(pid int32, limit bool) (Process, error) {
	p := &process{
		pid:         pid,
		hasCgroup:   u.hasCgroup,
		cpuLimit:    u.cpuLimit,
		ncpu:        u.ncpu,
		imposeLimit: limit,
	}

	proc, err := psprocess.NewProcess(pid)
	if err != nil {
		return nil, err
	}

	p.proc = proc

	ctx, cancel := context.WithCancel(context.Background())
	p.stopTicker = cancel
	go p.tick(ctx, 1000*time.Millisecond)

	return p, nil
}

func NewProcess(pid int32, limit bool) (Process, error) {
	return DefaultUtil.Process(pid, limit)
}

func (p *process) tick(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	if p.imposeLimit {
		go p.limit(ctx, interval)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			stat := p.collect()

			p.lock.Lock()
			p.statPrevious, p.statCurrent = p.statCurrent, stat
			p.statPreviousTime, p.statCurrentTime = p.statCurrentTime, t
			p.lock.Unlock()
			/*
				pct, _ := p.CPUPercent()
				pcpu := (pct.System + pct.User + pct.Other) / 100

				fmt.Printf("%d\t%0.2f%%\n", p.pid, pcpu*100*p.ncpu)
			*/
		}
	}
}

func (p *process) limit(ctx context.Context, interval time.Duration) {
	var limit float64 = 50.0 / 100.0 / p.ncpu
	var workingrate float64 = -1

	counter := 0

	for {
		select {
		case <-ctx.Done():
			return
		default:
			pct, _ := p.CPUPercent()
			/*
				pct.System *= p.ncpu
				pct.Idle *= p.ncpu
				pct.User *= p.ncpu
				pct.Other *= p.ncpu
			*/

			pcpu := (pct.System + pct.User + pct.Other) / 100

			if workingrate < 0 {
				workingrate = limit
			} else {
				workingrate = math.Min(workingrate/pcpu*limit, 1)
			}

			worktime := float64(interval.Nanoseconds()) * workingrate
			sleeptime := float64(interval.Nanoseconds()) - worktime
			/*
				if counter%20 == 0 {
					fmt.Printf("\nPID\t%%CPU\twork quantum\tsleep quantum\tactive rate\n")
					counter = 0
				}

				fmt.Printf("%d\t%0.2f%%\t%.2f us\t%.2f us\t%0.2f%%\n", p.pid, pcpu*100*p.ncpu, worktime/1000, sleeptime/1000, workingrate*100)
			*/
			if p.imposeLimit {
				p.proc.Resume()
			}
			time.Sleep(time.Duration(worktime) * time.Nanosecond)

			if sleeptime > 0 {
				if p.imposeLimit {
					p.proc.Suspend()
				}
				time.Sleep(time.Duration(sleeptime) * time.Nanosecond)
			}

			counter++
		}
	}
}

func (p *process) collect() cpuTimesStat {
	stat, err := p.cpuTimes()
	if err != nil {
		return cpuTimesStat{
			total: float64(time.Now().Unix()),
			idle:  float64(time.Now().Unix()),
		}
	}

	return *stat
}

func (p *process) Stop() {
	p.stopTicker()
}

func (p *process) cpuTimes() (*cpuTimesStat, error) {
	times, err := p.proc.Times()
	if err != nil {
		return nil, err
	}

	s := &cpuTimesStat{
		total:  cpuTotal(times),
		system: times.System,
		user:   times.User,
	}

	s.other = s.total - s.system - s.user
	if s.other < 0.0001 {
		s.other = 0
	}

	return s, nil
}

func (p *process) CPUPercent() (*CPUInfoStat, error) {
	var diff float64

	p.lock.RLock()
	defer p.lock.RUnlock()

	if p.hasCgroup && p.cpuLimit > 0 {
		diff = float64(p.cpuLimit) * (p.statCurrentTime.Sub(p.statPreviousTime)).Seconds() / 1e9
	} else {
		diff = p.statCurrentTime.Sub(p.statPreviousTime).Seconds() * p.ncpu
	}

	s := &CPUInfoStat{
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

func (p *process) VirtualMemory() (uint64, error) {
	info, err := p.proc.MemoryInfo()
	if err != nil {
		return 0, err
	}

	return info.RSS, nil
}
