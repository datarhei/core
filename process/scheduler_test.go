package process

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSchedulerPointInTime(t *testing.T) {
	s, err := NewScheduler("2023-03-20T11:06:39Z")
	require.NoError(t, err)

	p, err := time.Parse(time.RFC3339, "2023-03-20T11:05:39Z")
	require.NoError(t, err)

	d, err := s.NextAfter(p)
	require.NoError(t, err)
	require.Equal(t, time.Minute, d)

	p, err = time.Parse(time.RFC3339, "2023-03-20T11:07:39Z")
	require.NoError(t, err)

	_, err = s.NextAfter(p)
	require.Error(t, err)
}

func TestSchedulerCron(t *testing.T) {
	s, err := NewScheduler("* * * * *")
	require.NoError(t, err)

	sc := s.(*scheduler)
	require.True(t, sc.isCron)

	p, err := time.Parse(time.RFC3339, "2023-03-20T11:05:39Z")
	require.NoError(t, err)

	d, err := s.NextAfter(p)
	require.NoError(t, err)
	require.Equal(t, 21*time.Second, d)

	d, err = s.NextAfter(p.Add(21 * time.Second))
	require.NoError(t, err)
	require.Equal(t, 60*time.Second, d)
}
