package replace

import (
	"testing"

	"github.com/datarhei/core/v16/restream/app"
	"github.com/stretchr/testify/require"
)

func TestReplace(t *testing.T) {
	foobar := ";:.,-_$\\£!^"

	samples := [][2]string{
		{"{foobar}", foobar},
		{"{foobar^:}", ";\\\\:.,-_$\\\\\\£!^"},
		{"{foobar^:}barfoo{foobar^:}", ";\\\\:.,-_$\\\\\\£!^barfoo;\\\\:.,-_$\\\\\\£!^"},
		{"{foobar^:.}", "{foobar^:.}"},
		{"{foobar^}", "{foobar^}"},
		{"{barfoo^:}", "{barfoo^:}"},
		{"{foobar^^}", ";:.,-_$\\\\\\£!\\\\^"},
		{`{foobar^\}`, ";:.,-_$\\\\\\£!^"},
		{`{barfoo}`, "{barfoo}"},
	}

	r := New()

	r.RegisterReplaceFunc(
		"foobar",
		func(params map[string]string, config *app.Config, section string) string {
			return ";:.,-_$\\£!^"
		},
		nil,
	)

	for _, e := range samples {
		replaced := r.Replace(e[0], "foobar", "", nil, nil, "")
		require.Equal(t, e[1], replaced, e[0])
	}

	r.RegisterReplaceFunc(
		"foobar",
		func(params map[string]string, config *app.Config, section string) string {
			return ""
		},
		nil,
	)

	replaced := r.Replace("{foobar}", "foobar", "", nil, nil, "")
	require.Equal(t, "", replaced)

	replaced = r.Replace("{foobar}", "barfoo", "", nil, nil, "")
	require.Equal(t, "{foobar}", replaced)

	replaced = r.Replace("{barfoo}", "barfoo", "barfoo", nil, nil, "")
	require.Equal(t, "barfoo", replaced)

	replaced = r.Replace("{barfoo}", "barfoo", "", nil, nil, "")
	require.Equal(t, "", replaced)
}

func TestReplacerFunc(t *testing.T) {
	r := New()
	r.RegisterReplaceFunc(
		"foo:bar",
		func(params map[string]string, config *app.Config, section string) string {
			return "Hello " + params["who"] + "! " + params["what"] + "?"
		},
		map[string]string{
			"who":  "defaultWho",
			"what": "defaultWhat",
		},
	)

	replaced := r.Replace("{foo:bar}", "foo:bar", "", nil, nil, "")
	require.Equal(t, "Hello defaultWho! defaultWhat?", replaced)

	replaced = r.Replace("{foo:bar,who=World}", "foo:bar", "", nil, nil, "")
	require.Equal(t, "Hello World! defaultWhat?", replaced)

	replaced = r.Replace("{foo:bar,who=World,what=E=mc^2}", "foo:bar", "", nil, nil, "")
	require.Equal(t, "Hello World! E=mc^2?", replaced)

	replaced = r.Replace("{foo:bar^:,who=World,what=E=mc:2}", "foo:bar", "", nil, nil, "")
	require.Equal(t, "Hello World! E=mc\\\\:2?", replaced)
}

func TestReplacerFuncWithVars(t *testing.T) {
	r := New()
	r.RegisterReplaceFunc(
		"foo:bar",
		func(params map[string]string, config *app.Config, section string) string {
			return "Hello " + params["who"] + "! " + params["what"] + "?"
		},
		map[string]string{
			"who":  "$processid_$location",
			"what": "$location",
		},
	)

	vars := map[string]string{
		"processid": "123456789",
		"location":  "World",
	}

	replaced := r.Replace("{foo:bar}", "foo:bar", "", vars, nil, "")
	require.Equal(t, "Hello 123456789_World! World?", replaced)

	replaced = r.Replace("{foo:bar,who=World}", "foo:bar", "", vars, nil, "")
	require.Equal(t, "Hello World! World?", replaced)

	replaced = r.Replace("{foo:bar,who=World,what=E=mc^2}", "foo:bar", "", vars, nil, "")
	require.Equal(t, "Hello World! E=mc^2?", replaced)

	replaced = r.Replace("{foo:bar^:,who=World,what=E=mc:2}", "foo:bar", "", vars, nil, "")
	require.Equal(t, "Hello World! E=mc\\\\:2?", replaced)

	replaced = r.Replace("{foo:bar,who=$location,what=$processid}", "foo:bar", "", vars, nil, "")
	require.Equal(t, "Hello World! 123456789?", replaced)
}

func TestReplaceGlob(t *testing.T) {
	r := New()
	r.RegisterReplaceFunc(
		"foo:bar",
		func(params map[string]string, config *app.Config, section string) string {
			return "Hello foobar"
		},
		nil,
	)
	r.RegisterReplaceFunc(
		"foo:baz",
		func(params map[string]string, config *app.Config, section string) string {
			return "Hello foobaz"
		},
		nil,
	)

	replaced := r.Replace("{foo:baz}, {foo:bar}", "foo:*", "", nil, nil, "")
	require.Equal(t, "Hello foobaz, Hello foobar", replaced)
}

func TestParseParams(t *testing.T) {
	r := New().(*replacer)

	tests := [][2]string{
		{"", "def"},
		{"foobar=hello", "hello"},
		{"foobar=%Y%m%d_%H%M%S", "%Y%m%d_%H%M%S"},
		{"foobar=foo\\,bar", "foo,bar"},
		{"barfoo=xxx,foobar=something", "something"},
	}

	for _, test := range tests {
		params := r.parseParametes(test[0], nil, map[string]string{"foobar": "def"})
		require.Equal(t, test[1], params["foobar"])
	}
}
