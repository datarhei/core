package policy

import (
	"fmt"
	"math/rand/v2"
	"sync"
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

	policies := am.ListPolicies("", "")
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

	policies = am.ListPolicies("", "")
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

	am.RemovePolicy("ingo", "")

	policies = am.ListPolicies("", "")
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

	names := []string{}

	for i := 0; i < 1000; i++ {
		name := fmt.Sprintf("user%d", i)
		names = append(names, name)
		am.AddPolicy(name, "$none", []string{"foobar"}, "**", []string{"ANY"})
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		name := names[rand.IntN(1000)]
		ok, _ := am.Enforce(name, "$none", "foobar", "baz", "read")
		require.True(b, ok)
	}
}

func BenchmarkConcurrentEnforce(b *testing.B) {
	adapter, err := createAdapter()
	require.NoError(b, err)

	am, err := New(Config{
		Adapter: adapter,
		Logger:  nil,
	})
	require.NoError(b, err)

	names := []string{}

	for i := 0; i < 1000; i++ {
		name := fmt.Sprintf("user%d", i)
		names = append(names, name)
		am.AddPolicy(name, "$none", []string{"foobar"}, "**", []string{"ANY"})
	}

	b.ResetTimer()

	readerWg := sync.WaitGroup{}

	for i := 0; i < 1000; i++ {
		readerWg.Add(1)
		go func() {
			defer readerWg.Done()

			for i := 0; i < b.N; i++ {
				name := names[rand.IntN(1000)]
				ok, _ := am.Enforce(name, "$none", "foobar", "baz", "read")
				require.True(b, ok)
			}
		}()
	}

	readerWg.Wait()
}
