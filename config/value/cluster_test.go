package value

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClusterPeerValue(t *testing.T) {
	var x string

	val := NewClusterPeer(&x, "abc@foobar:8080")

	require.Equal(t, "abc@foobar:8080", val.String())
	require.Equal(t, nil, val.Validate())
	require.Equal(t, false, val.IsEmpty())

	x = "xyz@foobaz:9090"

	require.Equal(t, "xyz@foobaz:9090", val.String())
	require.Equal(t, nil, val.Validate())
	require.Equal(t, false, val.IsEmpty())

	val.Set("mno@fooboz:7070")

	require.Equal(t, "mno@fooboz:7070", x)

	err := val.Set("foobar:7070")
	require.Error(t, err)
}

func TestClusterPeerListValue(t *testing.T) {
	var x []string

	val := NewClusterPeerList(&x, []string{"abc@foobar:8080"}, ",")

	require.Equal(t, "abc@foobar:8080", val.String())
	require.Equal(t, nil, val.Validate())
	require.Equal(t, false, val.IsEmpty())

	x = []string{"abc@foobar:8080", "xyz@foobaz:9090"}

	require.Equal(t, "abc@foobar:8080,xyz@foobaz:9090", val.String())
	require.Equal(t, nil, val.Validate())
	require.Equal(t, false, val.IsEmpty())

	val.Set("mno@fooboz:8080,rst@foobax:9090")

	require.Equal(t, []string{"mno@fooboz:8080", "rst@foobax:9090"}, x)

	err := val.Set("mno@:8080")
	require.Error(t, err)

	err = val.Set("foobax:9090")
	require.Error(t, err)
}
