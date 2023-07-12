package maps

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEqual(t *testing.T) {
	a := map[string]string{
		"foo": "bar",
		"boz": "foz",
	}

	b := map[string]string{
		"boz": "foz",
		"foo": "bar",
	}

	require.True(t, Equal(a, b))
	require.True(t, Equal(b, a))
}

func TestNotEqual(t *testing.T) {
	a := map[string]string{
		"foo": "bar",
		"boz": "foz",
	}

	b := map[string]string{
		"boz": "foz",
	}

	require.False(t, Equal(a, b))
	require.False(t, Equal(b, a))

	c := map[string]string{
		"foo": "baz",
		"boz": "foz",
	}

	require.False(t, Equal(a, c))
	require.False(t, Equal(c, a))

	d := map[string]string{
		"foo": "bar",
		"baz": "foz",
	}

	require.False(t, Equal(a, d))
	require.False(t, Equal(d, a))
}
