package api

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEventFilter(t *testing.T) {
	event := Event{
		Timestamp: 1234,
		Level:     0,
		Component: "foobar",
		Message:   "none",
		Data: map[string]string{
			"foo": "bar",
		},
	}

	foobarfilter := EventFilter{
		Component: "foobar",
		Data: map[string]string{
			"foo": "^b.*$",
		},
	}

	err := foobarfilter.Compile()
	require.NoError(t, err)

	foobazfilter := EventFilter{
		Component: "foobaz",
		Data: map[string]string{
			"foo": "baz",
		},
	}

	err = foobazfilter.Compile()
	require.NoError(t, err)

	res := event.Filter(&foobarfilter)
	require.True(t, res)

	res = event.Filter(&foobazfilter)
	require.False(t, res)
}
