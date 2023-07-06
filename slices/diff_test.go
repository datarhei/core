package slices

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDiffComparable(t *testing.T) {
	a := []string{"c", "d", "e", "f"}
	b := []string{"a", "a", "b", "c", "d"}

	added, removed := DiffComparable(a, b)

	require.ElementsMatch(t, []string{"e", "f"}, added)
	require.ElementsMatch(t, []string{"a", "a", "b"}, removed)
}

func TestDiffEqualer(t *testing.T) {
	a := []String{"c", "d", "e", "f"}
	b := []String{"a", "a", "b", "c", "d"}

	added, removed := DiffComparable(a, b)

	require.ElementsMatch(t, []String{"e", "f"}, added)
	require.ElementsMatch(t, []String{"a", "a", "b"}, removed)
}
