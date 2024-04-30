package access

import (
	"testing"

	"github.com/datarhei/core/v16/io/fs"
	"github.com/stretchr/testify/require"
)

func createAdapter() (Adapter, error) {
	memfs, err := fs.NewMemFilesystemFromDir("./fixtures", fs.MemConfig{})
	if err != nil {
		return nil, err
	}

	return NewJSONAdapter(memfs, "./policy.json", nil)
}

func TestAccessManager(t *testing.T) {
	adapter, err := createAdapter()
	require.NoError(t, err)

	am, err := New(Config{
		Adapter: adapter,
		Logger:  nil,
	})
	require.NoError(t, err)

	policies := am.ListPolicies("", "", nil, "", nil)
	require.ElementsMatch(t, []Policy{
		{
			Name:     "ingo",
			Domain:   "$none",
			Types:    []string{"rtmp"},
			Resource: "/bla-*",
			Actions:  []string{"play", "publish"},
		},
		{
			Name:     "ingo",
			Domain:   "igelcamp",
			Types:    []string{"rtmp"},
			Resource: "/igelcamp/**",
			Actions:  []string{"publish"},
		},
	}, policies)

	am.AddPolicy("foobar", "group", []string{"bla", "blubb"}, "/", []string{"write"})

	policies = am.ListPolicies("", "", nil, "", nil)
	require.ElementsMatch(t, []Policy{
		{
			Name:     "ingo",
			Domain:   "$none",
			Types:    []string{"rtmp"},
			Resource: "/bla-*",
			Actions:  []string{"play", "publish"},
		},
		{
			Name:     "ingo",
			Domain:   "igelcamp",
			Types:    []string{"rtmp"},
			Resource: "/igelcamp/**",
			Actions:  []string{"publish"},
		},
		{
			Name:     "foobar",
			Domain:   "group",
			Types:    []string{"bla", "blubb"},
			Resource: "/",
			Actions:  []string{"write"},
		},
	}, policies)

	require.True(t, am.HasDomain("igelcamp"))
	require.True(t, am.HasDomain("group"))
	require.False(t, am.HasDomain("$none"))

	am.RemovePolicy("ingo", "", nil, "", nil)

	policies = am.ListPolicies("", "", nil, "", nil)
	require.ElementsMatch(t, []Policy{
		{
			Name:     "foobar",
			Domain:   "group",
			Types:    []string{"bla", "blubb"},
			Resource: "/",
			Actions:  []string{"write"},
		},
	}, policies)

	require.False(t, am.HasDomain("igelcamp"))
	require.True(t, am.HasDomain("group"))
	require.False(t, am.HasDomain("$none"))

	ok, _ := am.Enforce("foobar", "group", "bla", "/", "read")
	require.False(t, ok)

	ok, _ = am.Enforce("foobar", "group", "bla", "/", "write")
	require.True(t, ok)

	ok, _ = am.Enforce("foobar", "group", "blubb", "/", "write")
	require.True(t, ok)
}

func BenchmarkEnforce(b *testing.B) {
	adapter, err := createAdapter()
	require.NoError(b, err)

	am, err := New(Config{
		Adapter: adapter,
		Logger:  nil,
	})
	require.NoError(b, err)

	am.AddPolicy("$anon", "$none", []string{"foobar"}, "**", []string{"ANY"})

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ok, _ := am.Enforce("$anon", "$none", "foobar", "baz", "read")
		require.True(b, ok)
	}
}
