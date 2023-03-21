package process

import (
	"fmt"
	"time"

	"github.com/adhocore/gronx"
)

type Scheduler interface {
	// Next returns the duration until the next scheduled time in reference
	// to time.Npw(). If there's no next scheduled time, a negative duration
	// and an error will be returned.
	Next() (time.Duration, error)

	// NextAfter returns the same as Next(), but with the given reference
	// time.
	NextAfter(after time.Time) (time.Duration, error)
}

type scheduler struct {
	pattern string
	pit     time.Time
	isCron  bool
}

func NewScheduler(pattern string) (Scheduler, error) {
	s := &scheduler{}

	t, err := time.Parse(time.RFC3339, pattern)
	if err == nil {
		s.pit = t
		s.isCron = false
	} else {
		cron := gronx.New()
		if !cron.IsValid(pattern) {
			return nil, err
		}
		s.pattern = pattern
		s.isCron = true
	}

	return s, nil
}

func (s *scheduler) Next() (time.Duration, error) {
	return s.NextAfter(time.Now())
}

func (s *scheduler) NextAfter(after time.Time) (time.Duration, error) {
	var t time.Time
	var err error

	if s.isCron {
		t, err = gronx.NextTickAfter(s.pattern, after, false)
		if err != nil {
			return time.Duration(-1), fmt.Errorf("no next time has been scheduled")
		}
	} else {
		t = s.pit
	}

	d := t.Sub(after)
	if d < time.Duration(0) {
		return d, fmt.Errorf("no next time has been scheduled")
	}

	return d, nil
}
