package average

import (
	"testing"
	"time"

	timesrc "github.com/datarhei/core/v16/time"
	"github.com/stretchr/testify/require"
)

func TestNewSMA(t *testing.T) {
	_, err := NewSMA(time.Second, time.Second)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrMultiplier)

	_, err = NewSMA(time.Second, 2*time.Second)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrMultiplier)

	_, err = NewSMA(3*time.Second, 2*time.Second)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrMultiplier)

	_, err = NewSMA(0, time.Second)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrWindow)

	_, err = NewSMA(time.Second, 0)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrGranularity)

	sme, err := NewSMA(10*time.Second, time.Second)
	require.NoError(t, err)
	require.NotNil(t, sme)
}

func TestAddSMA(t *testing.T) {
	ts := &timesrc.TestSource{
		N: time.Unix(0, 0),
	}

	sme := &SMA{
		ts:          ts,
		window:      time.Second.Nanoseconds(),
		granularity: time.Millisecond.Nanoseconds(),
	}
	sme.init()

	sme.Add(42)

	total, samplecount := sme.Total()
	require.Equal(t, float64(42), total)
	require.Equal(t, int(time.Second/time.Millisecond), samplecount)

	sme.Add(5)

	total, samplecount = sme.Total()
	require.Equal(t, float64(47), total)
	require.Equal(t, int(time.Second/time.Millisecond), samplecount)

	ts.Set(5, 0)

	total, samplecount = sme.Total()
	require.Equal(t, float64(0), total)
	require.Equal(t, int(time.Second/time.Millisecond), samplecount)
}

func TestAverageSMA(t *testing.T) {
	ts := &timesrc.TestSource{
		N: time.Unix(0, 0),
	}

	sme := &SMA{
		ts:          ts,
		window:      time.Second.Nanoseconds(),
		granularity: time.Millisecond.Nanoseconds(),
	}
	sme.init()

	sme.Add(42)

	avg := sme.Average()
	require.Equal(t, 42.0/1000, avg)

	sme.Add(5)

	avg = sme.Average()
	require.Equal(t, 47.0/1000, avg)

	ts.Set(5, 0)

	avg = sme.Average()
	require.Equal(t, .0/1000, avg)
}

func TestAddAndAverageSMA(t *testing.T) {
	ts := &timesrc.TestSource{
		N: time.Unix(0, 0),
	}

	sme := &SMA{
		ts:          ts,
		window:      time.Second.Nanoseconds(),
		granularity: time.Millisecond.Nanoseconds(),
	}
	sme.init()

	avg := sme.AddAndAverage(42)
	require.Equal(t, 42.0/1000, avg)

	avg = sme.AddAndAverage(5)
	require.Equal(t, 47.0/1000, avg)

	ts.Set(5, 0)

	avg = sme.Average()
	require.Equal(t, .0/1000, avg)
}

func TestAverageSeriesSMA(t *testing.T) {
	ts := &timesrc.TestSource{
		N: time.Unix(0, 0),
	}

	sme := &SMA{
		ts:          ts,
		window:      10 * time.Second.Nanoseconds(),
		granularity: time.Second.Nanoseconds(),
	}
	sme.init()

	sme.Add(42) // [42, 0, 0, 0, 0, 0, 0, 0, 0, 0]

	ts.Set(1, 0)

	sme.Add(5) // [5, 42, 0, 0, 0, 0, 0, 0, 0, 0]

	ts.Set(2, 0)

	sme.Add(18) // [18, 5, 42, 0, 0, 0, 0, 0, 0, 0]

	ts.Set(3, 0)

	sme.Add(47) // [47, 18, 5, 42, 0, 0, 0, 0, 0, 0]

	ts.Set(4, 0)

	sme.Add(92) // [92, 47, 18, 5, 42, 0, 0, 0, 0, 0]

	ts.Set(5, 0)

	sme.Add(2) // [2, 92, 47, 18, 5, 42, 0, 0, 0, 0]

	ts.Set(6, 0)

	sme.Add(75) // [75, 2, 92, 47, 18, 5, 42, 0, 0, 0]

	ts.Set(7, 0)

	sme.Add(33) // [33, 75, 2, 92, 47, 18, 5, 42, 0, 0]

	ts.Set(8, 0)

	sme.Add(89) // [89, 33, 75, 2, 92, 47, 18, 5, 42, 0]

	ts.Set(9, 0)

	sme.Add(12) // [12, 89, 33, 75, 2, 92, 47, 18, 5, 42]

	avg := sme.Average()
	require.Equal(t, (12+89+33+75+2+92+47+18+5+42)/10., avg)

	ts.Set(10, 0)

	avg = sme.Average()
	require.Equal(t, (12+89+33+75+2+92+47+18+5)/10., avg)

	ts.Set(15, 0)

	avg = sme.Average()
	require.Equal(t, (12+89+33+75)/10., avg)

	ts.Set(19, 0)

	avg = sme.Average()
	require.Equal(t, (0)/10., avg)
}

func TestResetSMA(t *testing.T) {
	ts := &timesrc.TestSource{
		N: time.Unix(0, 0),
	}

	sme := &SMA{
		ts:          ts,
		window:      10 * time.Second.Nanoseconds(),
		granularity: time.Second.Nanoseconds(),
	}
	sme.init()

	sme.Add(42) // [42, 0, 0, 0, 0, 0, 0, 0, 0, 0]

	ts.Set(1, 0)

	sme.Add(5) // [5, 42, 0, 0, 0, 0, 0, 0, 0, 0]

	ts.Set(2, 0)

	sme.Add(18) // [18, 5, 42, 0, 0, 0, 0, 0, 0, 0]

	ts.Set(3, 0)

	sme.Add(47) // [47, 18, 5, 42, 0, 0, 0, 0, 0, 0]

	ts.Set(4, 0)

	sme.Add(92) // [92, 47, 18, 5, 42, 0, 0, 0, 0, 0]

	ts.Set(5, 0)

	sme.Add(2) // [2, 92, 47, 18, 5, 42, 0, 0, 0, 0]

	ts.Set(6, 0)

	sme.Add(75) // [75, 2, 92, 47, 18, 5, 42, 0, 0, 0]

	ts.Set(7, 0)

	sme.Add(33) // [33, 75, 2, 92, 47, 18, 5, 42, 0, 0]

	ts.Set(8, 0)

	sme.Add(89) // [89, 33, 75, 2, 92, 47, 18, 5, 42, 0]

	ts.Set(9, 0)

	sme.Add(12) // [12, 89, 33, 75, 2, 92, 47, 18, 5, 42]

	avg := sme.Average()
	require.Equal(t, (12+89+33+75+2+92+47+18+5+42)/10., avg)

	sme.Reset()

	avg = sme.Average()
	require.Equal(t, 0/10., avg)
}

func TestTotalSMA(t *testing.T) {
	ts := &timesrc.TestSource{
		N: time.Unix(0, 0),
	}

	sme := &SMA{
		ts:          ts,
		window:      10 * time.Second.Nanoseconds(),
		granularity: time.Second.Nanoseconds(),
	}
	sme.init()

	sme.Add(42) // [42, 0, 0, 0, 0, 0, 0, 0, 0, 0]

	ts.Set(1, 0)

	sme.Add(5) // [5, 42, 0, 0, 0, 0, 0, 0, 0, 0]

	ts.Set(2, 0)

	sme.Add(18) // [18, 5, 42, 0, 0, 0, 0, 0, 0, 0]

	ts.Set(3, 0)

	sme.Add(47) // [47, 18, 5, 42, 0, 0, 0, 0, 0, 0]

	ts.Set(4, 0)

	sme.Add(92) // [92, 47, 18, 5, 42, 0, 0, 0, 0, 0]

	ts.Set(5, 0)

	sme.Add(2) // [2, 92, 47, 18, 5, 42, 0, 0, 0, 0]

	ts.Set(6, 0)

	sme.Add(75) // [75, 2, 92, 47, 18, 5, 42, 0, 0, 0]

	ts.Set(7, 0)

	sme.Add(33) // [33, 75, 2, 92, 47, 18, 5, 42, 0, 0]

	ts.Set(8, 0)

	sme.Add(89) // [89, 33, 75, 2, 92, 47, 18, 5, 42, 0]

	ts.Set(9, 0)

	sme.Add(12) // [12, 89, 33, 75, 2, 92, 47, 18, 5, 42]

	total, nsamples := sme.Total()
	require.Equal(t, float64(12+89+33+75+2+92+47+18+5+42), total)
	require.Equal(t, 10, nsamples)
}
