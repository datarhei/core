//go:build !linux
// +build !linux

package psutil

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
