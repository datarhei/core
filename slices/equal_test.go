package slices

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEqualComparableElements(t *testing.T) {
	a := []string{"a", "b", "c", "d"}
	b := []string{"b", "c", "a", "d"}

	err := EqualComparableElements(a, b)
	require.NoError(t, err)

	err = EqualComparableElements(b, a)
	require.NoError(t, err)

	a = append(a, "z")

	err = EqualComparableElements(a, b)
	require.Error(t, err)

	err = EqualComparableElements(b, a)
	require.Error(t, err)
}

type String string

func (a String) Equal(b String) error {
	if string(a) == string(b) {
		return nil
	}

	return fmt.Errorf("%s != %s", a, b)
}

func TestEqualEqualerElements(t *testing.T) {
	a := []String{"a", "b", "c", "d"}
	b := []String{"b", "c", "a", "d"}

	err := EqualEqualerElements(a, b)
	require.NoError(t, err)

	err = EqualEqualerElements(b, a)
	require.NoError(t, err)

	a = append(a, "z")

	err = EqualEqualerElements(a, b)
	require.Error(t, err)

	err = EqualEqualerElements(b, a)
	require.Error(t, err)
}
