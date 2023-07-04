package slices

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDiff(t *testing.T) {
	a := []string{"c", "d", "e", "f"}
	b := []string{"a", "b", "c", "d"}

	added, removed := Diff(a, b)

	require.ElementsMatch(t, []string{"e", "f"}, added)
	require.ElementsMatch(t, []string{"a", "b"}, removed)
}
