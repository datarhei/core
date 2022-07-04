package app

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReplace(t *testing.T) {
	foobar := `;:.,-_$£!^`

	samples := [][2]string{
		{"{foobar}", foobar},
		{"{foobar^:}", `;\:.,-_$£!^`},
		{"{foobar^:}barfoo{foobar^:}", `;\:.,-_$£!^barfoo;\:.,-_$£!^`},
		{"{foobar^:.}", "{foobar^:.}"},
		{"{foobar^}", "{foobar^}"},
		{"{barfoo^:}", "{barfoo^:}"},
		{"{foobar^^}", `;:.,-_$£!\^`},
	}

	for _, e := range samples {
		replaced := replace(e[0], "foobar", foobar)
		require.Equal(t, e[1], replaced)
	}
}

func TestCreateCommand(t *testing.T) {
	config := &Config{
		Options: []string{"-global", "global"},
		Input: []ConfigIO{
			{Address: "inputAddress", Options: []string{"-input", "inputoption"}},
		},
		Output: []ConfigIO{
			{Address: "outputAddress", Options: []string{"-output", "oututoption"}},
		},
	}

	command := config.CreateCommand()
	require.Equal(t, []string{
		"-global", "global",
		"-input", "inputoption", "-i", "inputAddress",
		"-output", "oututoption", "outputAddress",
	}, command)
}
