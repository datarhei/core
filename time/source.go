package time

import "time"

type Source interface {
	Now() time.Time
}

type StdSource struct{}

func (s *StdSource) Now() time.Time {
	return time.Now()
}

type TestSource struct {
	N time.Time
}

func (t *TestSource) Now() time.Time {
	return t.N
}

func (t *TestSource) Set(sec int64, nsec int64) {
	t.N = time.Unix(sec, nsec)
}
