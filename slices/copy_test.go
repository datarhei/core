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

func (a String) Clone() String {
	return String(string(a))
}

func TestCopyDeep(t *testing.T) {
	a := []String{"a", "b", "c"}

	b := CopyDeep[String](a)

	require.Equal(t, []String{"a", "b", "c"}, b)
}
