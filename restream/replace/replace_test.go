package replace

import (
	"testing"

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

	for _, e := range samples {
		replaced := r.Replace(e[0], "foobar", foobar)
		require.Equal(t, e[1], replaced, e[0])
	}

	replaced := r.Replace("{foobar}", "foobar", "")
	require.Equal(t, "", replaced)
}

func TestReplaceTemplate(t *testing.T) {
	r := New()
	r.RegisterTemplate("foo:bar", "Hello {who}! {what}?")

	replaced := r.Replace("{foo:bar,who=World}", "foo:bar", "")
	require.Equal(t, "Hello World! {what}?", replaced)

	replaced = r.Replace("{foo:bar,who=World,what=E%3dmc^2}", "foo:bar", "")
	require.Equal(t, "Hello World! E=mc^2?", replaced)

	replaced = r.Replace("{foo:bar^:,who=World,what=E%3dmc:2}", "foo:bar", "")
	require.Equal(t, "Hello World! E=mc\\\\:2?", replaced)
}

func TestReplaceTemplateFunc(t *testing.T) {
	r := New()
	r.RegisterTemplateFunc("foo:bar", func() string { return "Hello {who}! {what}?" })

	replaced := r.Replace("{foo:bar,who=World}", "foo:bar", "")
	require.Equal(t, "Hello World! {what}?", replaced)

	replaced = r.Replace("{foo:bar,who=World,what=E%3dmc^2}", "foo:bar", "")
	require.Equal(t, "Hello World! E=mc^2?", replaced)

	replaced = r.Replace("{foo:bar^:,who=World,what=E%3dmc:2}", "foo:bar", "")
	require.Equal(t, "Hello World! E=mc\\\\:2?", replaced)
}

func TestReplaceCompileTemplate(t *testing.T) {
	samples := [][3]string{
		{"Hello {who}!", "who=World", "Hello World!"},
		{"Hello {who}! {what}?", "who=World", "Hello World! {what}?"},
		{"Hello {who}! {what}?", "who=World,what=Yeah", "Hello World! Yeah?"},
		{"Hello {who}! {what}?", "who=World,what=", "Hello World! ?"},
		{"Hello {who}!", "who=E%3dmc^2", "Hello E=mc^2!"},
	}

	r := New().(*replacer)

	for _, e := range samples {
		replaced := r.compileTemplate(e[0], e[1])
		require.Equal(t, e[2], replaced, e[0])
	}
}

func TestReplaceGlob(t *testing.T) {
	r := New()
	r.RegisterTemplate("foo:bar", "Hello foobar")
	r.RegisterTemplate("foo:baz", "Hello foobaz")

	replaced := r.Replace("{foo:baz}, {foo:bar}", "foo:*", "")
	require.Equal(t, "Hello foobaz, Hello foobar", replaced)
}
