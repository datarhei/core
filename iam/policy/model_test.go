package policy

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizePolicy(t *testing.T) {
	p := Policy{
		Name:     "foobar",
		Domain:   "domain",
		Types:    []string{"fs", "API", "rtMp", "srt"},
		Resource: "/foo/**",
		Actions:  []string{"Head", "OPtionS", "GET"},
	}

	p = normalizePolicy(p)

	require.Equal(t, Policy{
		Name:     "foobar",
		Domain:   "domain",
		Types:    []string{"api", "fs", "rtmp", "srt"},
		Resource: "/foo/**",
		Actions:  []string{"get", "head", "options"},
	}, p)
}

func TestModelNew(t *testing.T) {
	m := NewModel("$superuser").(*model)
	require.Equal(t, m.superuser, "$superuser")
	require.NotNil(t, m.policies)
	require.NotNil(t, m.lock)
}

func TestModelAddPolicy(t *testing.T) {
	p := Policy{
		Name:     "foobar",
		Domain:   "domain",
		Types:    []string{"fs", "API", "rtMp", "srt"},
		Resource: "/foo/**",
		Actions:  []string{"Head", "OPtionS", "GET"},
	}

	m := NewModel("$superuser").(*model)

	err := m.AddPolicy(p)
	require.NoError(t, err)

	require.Equal(t, 1, len(m.policies))
	require.Equal(t, 1, len(m.policies["foobar@domain"]))
	require.Equal(t, Policy{
		Name:     "foobar",
		Domain:   "domain",
		Types:    []string{"api", "fs", "rtmp", "srt"},
		Resource: "/foo/**",
		Actions:  []string{"get", "head", "options"},
	}, m.policies["foobar@domain"][0])

	m.AddPolicies([]Policy{p, p, p})

	require.Equal(t, 1, len(m.policies))
	require.Equal(t, 1, len(m.policies["foobar@domain"]))

	p.Resource = "/bar/*"
	m.AddPolicy(p)

	require.Equal(t, 2, len(m.policies["foobar@domain"]))

	p.Name = "foobaz"
	m.AddPolicy(p)

	require.Equal(t, 2, len(m.policies))
	require.Equal(t, 1, len(m.policies["foobaz@domain"]))
}

func TestModelHasPolicy(t *testing.T) {
	p := Policy{
		Name:     "foobar",
		Domain:   "domain",
		Types:    []string{"fs", "API", "rtMp", "srt"},
		Resource: "/foo/**",
		Actions:  []string{"Head", "OPtionS", "GET"},
	}

	m := NewModel("$superuser").(*model)

	ok := m.HasPolicy(p)
	require.False(t, ok)

	m.AddPolicy(p)

	ok = m.HasPolicy(p)
	require.True(t, ok)

	ok = m.HasPolicy(Policy{
		Name:     "foobaz",
		Domain:   "domain",
		Types:    []string{"fs", "API", "rtMp", "srt"},
		Resource: "/foo/**",
		Actions:  []string{"Head", "OPtionS", "GET", "put"},
	})
	require.False(t, ok)

	ok = m.HasPolicy(Policy{
		Name:     "foobar",
		Domain:   "domaim",
		Types:    []string{"fs", "API", "rtMp", "srt"},
		Resource: "/foo/**",
		Actions:  []string{"Head", "OPtionS", "GET", "put"},
	})
	require.False(t, ok)

	ok = m.HasPolicy(Policy{
		Name:     "foobar",
		Domain:   "domain",
		Types:    []string{"API", "rtMp", "srt"},
		Resource: "/foo/**",
		Actions:  []string{"Head", "OPtionS", "GET", "put"},
	})
	require.False(t, ok)

	ok = m.HasPolicy(Policy{
		Name:     "foobar",
		Domain:   "domain",
		Types:    []string{"fs", "API", "rtMp", "srt"},
		Resource: "/foo/*",
		Actions:  []string{"Head", "OPtionS", "GET", "put"},
	})
	require.False(t, ok)

	ok = m.HasPolicy(Policy{
		Name:     "foobar",
		Domain:   "domain",
		Types:    []string{"fs", "API", "rtMp", "srt"},
		Resource: "/foo/**",
		Actions:  []string{"Head", "OPtionS", "GET", "pot"},
	})
	require.False(t, ok)
}

func TestModelRemovePolicy(t *testing.T) {
	p := Policy{
		Name:     "foobar",
		Domain:   "domain",
		Types:    []string{"fs", "API", "rtMp", "srt"},
		Resource: "/foo/**",
		Actions:  []string{"Head", "OPtionS", "GET"},
	}

	m := NewModel("$superuser").(*model)
	m.AddPolicy(p)

	p.Resource = "/bar/*"
	m.AddPolicy(p)

	require.Equal(t, 2, len(m.policies["foobar@domain"]))

	err := m.RemovePolicy(Policy{
		Name:     "foobar",
		Domain:   "domain",
		Types:    []string{"fs", "API", "rtMp", "srt"},
		Resource: "/foo/**",
		Actions:  []string{"Head", "OPtionS", "GET", "put"},
	})
	require.NoError(t, err)

	require.Equal(t, 2, len(m.policies["foobar@domain"]))

	err = m.RemovePolicy(p)
	require.NoError(t, err)

	require.Equal(t, 1, len(m.policies))
	require.Equal(t, 1, len(m.policies["foobar@domain"]))
}

func TestModelListPolicies(t *testing.T) {
	p := Policy{
		Name:     "foobar",
		Domain:   "domain",
		Types:    []string{"fs", "API", "rtMp", "srt"},
		Resource: "/foo/**",
		Actions:  []string{"Head", "OPtionS", "GET"},
	}

	m := NewModel("$superuser").(*model)

	policies := m.GetFilteredPolicy("", "")
	require.Equal(t, 0, len(policies))

	m.addPolicy(p)

	p.Resource = "/bar/*"
	m.addPolicy(p)

	p.Name = "foobaz"
	m.addPolicy(p)

	p.Domain = "group"
	m.addPolicy(p)

	policies = m.GetFilteredPolicy("", "")
	require.Equal(t, 4, len(policies))
	require.ElementsMatch(t, []Policy{
		{
			Name:     "foobar",
			Domain:   "domain",
			Types:    []string{"api", "fs", "rtmp", "srt"},
			Resource: "/foo/**",
			Actions:  []string{"get", "head", "options"},
		},
		{
			Name:     "foobar",
			Domain:   "domain",
			Types:    []string{"api", "fs", "rtmp", "srt"},
			Resource: "/bar/*",
			Actions:  []string{"get", "head", "options"},
		},
		{
			Name:     "foobaz",
			Domain:   "domain",
			Types:    []string{"api", "fs", "rtmp", "srt"},
			Resource: "/bar/*",
			Actions:  []string{"get", "head", "options"},
		},
		{
			Name:     "foobaz",
			Domain:   "group",
			Types:    []string{"api", "fs", "rtmp", "srt"},
			Resource: "/bar/*",
			Actions:  []string{"get", "head", "options"},
		},
	}, policies)

	policies = m.GetFilteredPolicy("foobar", "")
	require.Equal(t, 2, len(policies))
	require.ElementsMatch(t, []Policy{
		{
			Name:     "foobar",
			Domain:   "domain",
			Types:    []string{"api", "fs", "rtmp", "srt"},
			Resource: "/foo/**",
			Actions:  []string{"get", "head", "options"},
		},
		{
			Name:     "foobar",
			Domain:   "domain",
			Types:    []string{"api", "fs", "rtmp", "srt"},
			Resource: "/bar/*",
			Actions:  []string{"get", "head", "options"},
		},
	}, policies)

	policies = m.GetFilteredPolicy("", "group")
	require.Equal(t, 1, len(policies))
	require.ElementsMatch(t, []Policy{
		{
			Name:     "foobaz",
			Domain:   "group",
			Types:    []string{"api", "fs", "rtmp", "srt"},
			Resource: "/bar/*",
			Actions:  []string{"get", "head", "options"},
		},
	}, policies)

	policies = m.GetFilteredPolicy("foobaz", "domain")
	require.Equal(t, 1, len(policies))
	require.ElementsMatch(t, []Policy{
		{
			Name:     "foobaz",
			Domain:   "domain",
			Types:    []string{"api", "fs", "rtmp", "srt"},
			Resource: "/bar/*",
			Actions:  []string{"get", "head", "options"},
		},
	}, policies)

	policies = m.GetFilteredPolicy("foobar", "group")
	require.Equal(t, 0, len(policies))
}

func TestModelEnforce(t *testing.T) {
	p := Policy{
		Name:     "foobar",
		Domain:   "domain",
		Types:    []string{"fs", "API", "rtMp", "srt"},
		Resource: "/foo/**",
		Actions:  []string{"Head", "OPtionS", "GET", "play"},
	}

	m := NewModel("$superuser").(*model)

	policies := m.GetFilteredPolicy("", "")
	require.Equal(t, 0, len(policies))

	m.addPolicy(p)

	ok, _ := m.Enforce("$superuser", "xxx", "something", "/nothing", "anything")
	require.True(t, ok)

	ok, _ = m.Enforce("foobar", "domain", "rtmp", "/foo/bar/baz", "play")
	require.True(t, ok)

	ok, _ = m.Enforce("foobar", "domain", "rtmp", "/foo/bar/baz", "publish")
	require.False(t, ok)

	ok, _ = m.Enforce("foobar", "domain", "rtmp", "/fo/bar/baz", "play")
	require.False(t, ok)

	ok, _ = m.Enforce("foobar", "domain", "rtsp", "/foo/bar/baz", "play")
	require.False(t, ok)

	ok, _ = m.Enforce("foobar", "group", "rtmp", "/foo/bar/baz", "play")
	require.False(t, ok)

	ok, _ = m.Enforce("foobaz", "domain", "rtmp", "/foo/bar/baz", "play")
	require.False(t, ok)
}

func TestModelClear(t *testing.T) {
	p := Policy{
		Name:     "foobar",
		Domain:   "domain",
		Types:    []string{"fs", "API", "rtMp", "srt"},
		Resource: "/foo/**",
		Actions:  []string{"Head", "OPtionS", "GET"},
	}

	m := NewModel("$superuser").(*model)

	policies := m.GetFilteredPolicy("", "")
	require.Equal(t, 0, len(policies))

	m.addPolicy(p)

	p.Resource = "/bar/*"
	m.addPolicy(p)

	p.Name = "foobaz"
	m.addPolicy(p)

	p.Domain = "group"
	m.addPolicy(p)

	policies = m.GetFilteredPolicy("", "")
	require.Equal(t, 4, len(policies))

	m.ClearPolicy()

	policies = m.GetFilteredPolicy("", "")
	require.Equal(t, 0, len(policies))

	require.Empty(t, m.policies)
}
