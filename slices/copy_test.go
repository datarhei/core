package slices

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCopy(t *testing.T) {
	a := []string{"a", "b", "c"}

	b := Copy(a)

	require.Equal(t, []string{"a", "b", "c"}, b)
}
