package url

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLookup(t *testing.T) {
	ip, err := Lookup("/localhost:8080/foobar")

	require.NoError(t, err)
	require.Equal(t, "", ip)

	ip, err = Lookup("http://")

	require.NoError(t, err)
	require.Equal(t, "", ip)

	ip, err = Lookup("https://www.google.com")

	require.NoError(t, err)
	require.NotEmpty(t, ip)
}

func TestLocalhost(t *testing.T) {
	ip, err := Lookup("http://localhost:8080/foobar")

	require.NoError(t, err)
	require.Subset(t, []string{"127.0.0.1", "::1"}, []string{ip})
}

func TestValidate(t *testing.T) {
	err := Validate("http://localhost/foobar")
	require.NoError(t, err)

	err = Validate("foobar")
	require.NoError(t, err)
}

func TestScheme(t *testing.T) {
	r := HasScheme("http://localhost/foobar")
	require.True(t, r)

	r = HasScheme("iueriherfd://localhost/foobar")
	require.True(t, r)

	r = HasScheme("//localhost/foobar")
	require.False(t, r)
}
