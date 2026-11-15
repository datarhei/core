package client

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProcessListOptionsQuery(t *testing.T) {
	opts := ProcessListOptions{
		ID:            []string{"a", "b"},
		IDPattern:     "id*",
		DomainPattern: "domain*",
		ZonePattern:   "zone*",
	}

	values := opts.Query()

	require.Equal(t, "a,b", values.Get("id"))
	require.Equal(t, "id*", values.Get("idpattern"))
	require.Equal(t, "domain*", values.Get("domainpattern"))
	require.Equal(t, "zone*", values.Get("zonepattern"))
}
