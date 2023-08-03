package token

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMarshal(t *testing.T) {
	token := "xxx"

	data := [][2]string{
		{"", ""},
		{"foo", "foo"},
		{"foo:bar", "foo::bar"},
		{"foo::bar", "foo::::bar"},
	}

	for _, d := range data {
		encoded := Marshal(d[0], token)
		require.Equal(t, d[1]+":"+token, encoded, d[1])
	}
}

func TestUnmarshal(t *testing.T) {
	data := [][3]string{
		{"foo", "", "foo"},
		{"fo::o", "", "fo::o"},
		{"::foo", "", "::foo"},
		{":xxx", "", "xxx"},
		{"foo:xxx", "foo", "xxx"},
		{"foo::bar:xxx", "foo:bar", "xxx"},
		{"foo:::bar:xxx", "foo:", "bar:xxx"},
		{"foo::::bar:xxx", "foo::bar", "xxx"},
		{"foo-bar:%26%21%27", "foo-bar", "%26%21%27"},
	}

	for _, d := range data {
		username, token := Unmarshal(d[0])
		require.Equal(t, d[1], username, d[0])
		require.Equal(t, d[2], token)
	}
}
