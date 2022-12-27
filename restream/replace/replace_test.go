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
	r.RegisterTemplate("foobar", "Hello {who}! {what}?", nil)

	replaced := r.Replace("{foobar,who=World}", "foobar", "")
	require.Equal(t, "Hello World! {what}?", replaced)

	replaced = r.Replace("{foobar,who=World,what=E%3dmc^2}", "foobar", "")
	require.Equal(t, "Hello World! E=mc^2?", replaced)

	replaced = r.Replace("{foobar^:,who=World,what=E%3dmc:2}", "foobar", "")
	require.Equal(t, "Hello World! E=mc\\\\:2?", replaced)
}

func TestReplaceTemplateDefaults(t *testing.T) {
	r := New()
	r.RegisterTemplate("foobar", "Hello {who}! {what}?", map[string]string{
		"who":  "someone",
		"what": "something",
	})

	replaced := r.Replace("{foobar}", "foobar", "")
	require.Equal(t, "Hello someone! something?", replaced)

	replaced = r.Replace("{foobar,who=World}", "foobar", "")
	require.Equal(t, "Hello World! something?", replaced)
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
		replaced := r.compileTemplate(e[0], e[1], nil)
		require.Equal(t, e[2], replaced, e[0])
	}
}

func TestReplaceCompileTemplateDefaults(t *testing.T) {
	samples := [][3]string{
		{"Hello {who}!", "", "Hello someone!"},
		{"Hello {who}!", "who=World", "Hello World!"},
		{"Hello {who}! {what}?", "who=World", "Hello World! something?"},
		{"Hello {who}! {what}?", "who=World,what=Yeah", "Hello World! Yeah?"},
		{"Hello {who}! {what}?", "who=World,what=", "Hello World! ?"},
	}

	r := New().(*replacer)

	for _, e := range samples {
		replaced := r.compileTemplate(e[0], e[1], map[string]string{
			"who":  "someone",
			"what": "something",
		})
		require.Equal(t, e[2], replaced, e[0])
	}
}
