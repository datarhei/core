package api

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEventFilter(t *testing.T) {
	event := Event{
		Timestamp: 1234,
		Level:     3,
		Component: "foobar",
		Message:   "none",
		Data: map[string]string{
			"foo": "bar",
		},
	}

	filter := EventFilter{
		Component: "foobar",
		Level:     "info",
		Message:   "none",
	}

	err := filter.Compile()
	require.NoError(t, err)

	res := event.Filter(&filter)
	require.True(t, res)

	filter = EventFilter{
		Component: "foobar",
		Level:     "warn",
		Message:   "none",
	}

	err = filter.Compile()
	require.NoError(t, err)

	res = event.Filter(&filter)
	require.False(t, res)

	filter = EventFilter{
		Component: "foobar",
		Level:     "info",
		Message:   "done",
	}

	err = filter.Compile()
	require.NoError(t, err)

	res = event.Filter(&filter)
	require.False(t, res)

	foobarfilter := EventFilter{
		Component: "foobar",
		Data: map[string]string{
			"foo": "^b.*$",
		},
	}

	err = foobarfilter.Compile()
	require.NoError(t, err)

	res = event.Filter(&foobarfilter)
	require.True(t, res)

	foobazfilter := EventFilter{
		Component: "foobaz",
		Data: map[string]string{
			"foo": "baz",
		},
	}

	err = foobazfilter.Compile()
	require.NoError(t, err)

	res = event.Filter(&foobazfilter)
	require.False(t, res)
}

func TestEventFilterDataKey(t *testing.T) {
	event := Event{
		Timestamp: 1234,
		Level:     3,
		Component: "foobar",
		Message:   "none",
		Data: map[string]string{
			"foo": "bar",
		},
	}

	filter := EventFilter{
		Component: "foobar",
		Level:     "info",
		Message:   "none",
	}

	err := filter.Compile()
	require.NoError(t, err)

	res := event.Filter(&filter)
	require.True(t, res)

	filter = EventFilter{
		Component: "foobar",
		Level:     "info",
		Message:   "none",
		Data: map[string]string{
			"bar": "foo",
		},
	}

	err = filter.Compile()
	require.NoError(t, err)

	res = event.Filter(&filter)
	require.False(t, res)

	filter = EventFilter{
		Component: "foobar",
		Level:     "info",
		Message:   "none",
		Data: map[string]string{
			"foo": "bar",
		},
	}

	err = filter.Compile()
	require.NoError(t, err)

	res = event.Filter(&filter)
	require.True(t, res)
}

func BenchmarkEventFilters(b *testing.B) {
	event := Event{
		Timestamp: 1234,
		Level:     3,
		Component: "foobar",
		Message:   "none",
		Data: map[string]string{
			"foo": "bar",
		},
	}

	levelfilter := EventFilter{
		Component: "foobar",
		Level:     "info",
		Data: map[string]string{
			"foo": "^b.*$",
		},
	}

	err := levelfilter.Compile()
	require.NoError(b, err)

	res := event.Filter(&levelfilter)
	require.True(b, res)

	for i := 0; i < b.N; i++ {
		event.Filter(&levelfilter)
	}
}
