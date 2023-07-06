package slices

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEqualComparableElements(t *testing.T) {
	a := []string{"a", "b", "c", "d"}
	b := []string{"b", "c", "a", "d"}

	ok := EqualComparableElements(a, b)
	require.True(t, ok)

	ok = EqualComparableElements(b, a)
	require.True(t, ok)

	a = append(a, "z")

	ok = EqualComparableElements(a, b)
	require.False(t, ok)

	ok = EqualComparableElements(b, a)
	require.False(t, ok)
}

type String string

func (a String) Equal(b String) bool {
	return string(a) == string(b)
}

func TestEqualEqualerElements(t *testing.T) {
	a := []String{"a", "b", "c", "d"}
	b := []String{"b", "c", "a", "d"}

	ok := EqualEqualerElements(a, b)
	require.True(t, ok)

	ok = EqualEqualerElements(b, a)
	require.True(t, ok)

	a = append(a, "z")

	ok = EqualEqualerElements(a, b)
	require.False(t, ok)

	ok = EqualEqualerElements(b, a)
	require.False(t, ok)
}
