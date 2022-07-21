package srt

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseStreamId(t *testing.T) {
	streamids := map[string]streamInfo{
		"bla":                                   {},
		"#!:":                                   {},
		"#!:key=value":                          {},
		"#!:m=publish":                          {mode: "publish"},
		"#!:r=123456789":                        {resource: "123456789"},
		"#!:token=foobar":                       {token: "foobar"},
		"#!:token=foo,bar":                      {token: "foo"},
		"#!:m=publish,r=123456789,token=foobar": {mode: "publish", resource: "123456789", token: "foobar"},
	}

	for streamid, wantsi := range streamids {
		si, _ := parseStreamId(streamid)

		require.Equal(t, wantsi, si)
	}
}
