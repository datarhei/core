package node

import (
	"testing"
	"time"

	timesrc "github.com/datarhei/core/v16/time"

	"github.com/stretchr/testify/require"
)

func TestCache(t *testing.T) {
	ts := &timesrc.TestSource{
		N: time.Unix(0, 0),
	}

	c := NewCache[string](ts)

	_, err := c.Get("foo")
	require.Error(t, err)

	c.Put("foo", "bar", 10*time.Second)

	v, err := c.Get("foo")
	require.NoError(t, err)
	require.Equal(t, "bar", v)

	ts.Set(10, 0)

	v, err = c.Get("foo")
	require.NoError(t, err)
	require.Equal(t, "bar", v)

	ts.Set(11, 0)

	_, err = c.Get("foo")
	require.Error(t, err)
}

func TestCachePurge(t *testing.T) {
	ts := &timesrc.TestSource{
		N: time.Unix(0, 0),
	}

	c := NewCache[string](ts)

	c.Put("foo", "bar", 10*time.Second)

	v, err := c.Get("foo")
	require.NoError(t, err)
	require.Equal(t, "bar", v)

	ts.Set(59, 0)

	c.Put("foz", "boz", 10*time.Second)

	_, ok := c.entries["foo"]
	require.True(t, ok)

	ts.Set(61, 0)

	c.Put("foz", "boz", 10*time.Second)

	_, ok = c.entries["foo"]
	require.False(t, ok)
}
