package url

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLookup(t *testing.T) {
	_, err := Lookup("https://www.google.com")

	require.NoError(t, err)
}

func TestLocalhost(t *testing.T) {
	ip, err := Lookup("http://localhost:8080/foobar")

	require.NoError(t, err)
	require.Subset(t, []string{"127.0.0.1", "::1"}, []string{ip})
}
