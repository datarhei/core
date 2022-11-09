package vars

import (
	"testing"

	"github.com/datarhei/core/v16/config/value"

	"github.com/stretchr/testify/require"
)

func TestVars(t *testing.T) {
	v1 := Variables{}

	s := ""

	v1.Register(value.NewString(&s, "foobar"), "string", "", nil, "a string", false, false)

	require.Equal(t, "foobar", s)
	x, _ := v1.Get("string")
	require.Equal(t, "foobar", x)

	v := v1.findVariable("string")
	v.value.Set("barfoo")

	require.Equal(t, "barfoo", s)
	x, _ = v1.Get("string")
	require.Equal(t, "barfoo", x)

	v1.Set("string", "foobaz")

	require.Equal(t, "foobaz", s)
	x, _ = v1.Get("string")
	require.Equal(t, "foobaz", x)

	v1.SetDefault("string")

	require.Equal(t, "foobar", s)
	x, _ = v1.Get("string")
	require.Equal(t, "foobar", x)
}
