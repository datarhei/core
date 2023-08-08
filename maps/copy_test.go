package maps

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCopy(t *testing.T) {
	a := map[string]string{
		"foo": "bar",
		"bar": "foo",
	}

	b := Copy(a)

	require.Equal(t, a, b)
}
