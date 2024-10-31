package average

import (
	"container/ring"
	"errors"
	gotime "time"

	"github.com/datarhei/core/v16/time"
)

type SMA struct {
	ts          time.Source
	window      int64
	granularity int64
	size        int
	last        int64
	samples     *ring.Ring
}

var ErrWindow = errors.New("window size must be positive")
var ErrGranularity = errors.New("granularity must be positive")
var ErrMultiplier = errors.New("window size has to be a multiplier of the granularity size")

func NewSMA(window, granularity gotime.Duration) (*SMA, error) {
	if window <= 0 {
		return nil, ErrWindow
	}

	if granularity <= 0 {
		return nil, ErrGranularity
	}

	if window <= granularity || window%granularity != 0 {
		return nil, ErrMultiplier
	}

	s := &SMA{
		ts:          &time.StdSource{},
		window:      window.Nanoseconds(),
		granularity: granularity.Nanoseconds(),
	}

	s.init()

	return s, nil
}

func (s *SMA) init() {
	s.size = int(s.window / s.granularity)
	s.samples = ring.New(s.size)

	s.Reset()

	now := s.ts.Now().UnixNano()
	s.last = now - now%s.granularity
}

func (s *SMA) Add(v float64) {
	now := s.ts.Now().UnixNano()
	now -= now % s.granularity

	n := (now - s.last) / s.granularity

	if n >= int64(s.samples.Len()) {
		// zero everything
		s.Reset()
	} else {
		for i := n; i > 0; i-- {
			s.samples = s.samples.Next()
			s.samples.Value = float64(0)
		}
	}

	s.samples.Value = s.samples.Value.(float64) + v

	s.last = now
}

func (s *SMA) AddAndAverage(v float64) float64 {
	s.Add(v)

	total := float64(0)

	s.samples.Do(func(v any) {
		total += v.(float64)
	})

	return total / float64(s.samples.Len())
}

func (s *SMA) Average() float64 {
	total, samplecount := s.Total()

	return total / float64(samplecount)
}

func (s *SMA) Reset() {
	n := s.samples.Len()

	// Initialize the ring buffer with 0 values.
	for i := 0; i < n; i++ {
		s.samples.Value = float64(0)
		s.samples = s.samples.Next()
	}
}

func (s *SMA) Total() (float64, int) {
	// Propagate the ringbuffer
	s.Add(0)

	total := float64(0)

	s.samples.Do(func(v any) {
		total += v.(float64)
	})

	return total, s.samples.Len()
}
