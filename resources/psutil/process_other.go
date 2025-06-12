//go:build !linux
// +build !linux

package psutil

import psprocess "github.com/shirou/gopsutil/v3/process"

func cpuTimes(pid int32) (*cpuTimesStat, error) {
	proc, err := psprocess.NewProcess(pid)
	if err != nil {
		return nil, err
	}

	times, err := proc.Times()
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
