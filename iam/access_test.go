package iam

import (
	"testing"

	"github.com/datarhei/core/v16/io/fs"
	"github.com/stretchr/testify/require"
)

func TestAccessManager(t *testing.T) {
	memfs, err := fs.NewMemFilesystemFromDir("./fixtures", fs.MemConfig{})
	require.NoError(t, err)

	am, err := NewAccessManager(AccessConfig{
		FS:     memfs,
		Logger: nil,
	})
	require.NoError(t, err)

	policies := am.ListPolicies("", "", "", nil)
	require.ElementsMatch(t, []Policy{
		{
			Name:     "ingo",
			Domain:   "$none",
			Resource: "rtmp:/bla-*",
			Actions:  []string{"play", "publish"},
		},
		{
			Name:     "ingo",
			Domain:   "igelcamp",
			Resource: "rtmp:/igelcamp/**",
			Actions:  []string{"publish"},
		},
	}, policies)

	am.AddPolicy("foobar", "group", "bla:/", []string{"write"})

	policies = am.ListPolicies("", "", "", nil)
	require.ElementsMatch(t, []Policy{
		{
			Name:     "ingo",
			Domain:   "$none",
			Resource: "rtmp:/bla-*",
			Actions:  []string{"play", "publish"},
		},
		{
			Name:     "ingo",
			Domain:   "igelcamp",
			Resource: "rtmp:/igelcamp/**",
			Actions:  []string{"publish"},
		},
		{
			Name:     "foobar",
			Domain:   "group",
			Resource: "bla:/",
			Actions:  []string{"write"},
		},
	}, policies)

	require.True(t, am.HasDomain("igelcamp"))
	require.True(t, am.HasDomain("group"))
	require.False(t, am.HasDomain("$none"))

	am.RemovePolicy("ingo", "", "", nil)

	policies = am.ListPolicies("", "", "", nil)
	require.ElementsMatch(t, []Policy{
		{
			Name:     "foobar",
			Domain:   "group",
			Resource: "bla:/",
			Actions:  []string{"write"},
		},
	}, policies)

	require.False(t, am.HasDomain("igelcamp"))
	require.True(t, am.HasDomain("group"))
	require.False(t, am.HasDomain("$none"))

	ok, _ := am.Enforce("foobar", "group", "bla:/", "read")
	require.False(t, ok)

	ok, _ = am.Enforce("foobar", "group", "bla:/", "write")
	require.True(t, ok)
}
