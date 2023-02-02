package value

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAbsolutePathValue(t *testing.T) {
	var x string

	val := NewAbsolutePath(&x, "foobar")

	require.Equal(t, "foobar", val.String())
	require.Error(t, val.Validate())
	require.Equal(t, false, val.IsEmpty())

	x = "/foobaz"

	require.Equal(t, "/foobaz", val.String())
	require.Equal(t, nil, val.Validate())
	require.Equal(t, false, val.IsEmpty())

	val.Set("/fooboz")

	require.Equal(t, "/fooboz", x)
}
