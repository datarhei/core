package url

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	srturl := "srt://127.0.0.1:6000?mode=caller&passphrase=foobar&streamid=" + url.QueryEscape("#!:m=publish,r=123456,token=bla")

	u, err := Parse(srturl)
	require.NoError(t, err)

	require.Equal(t, "srt", u.Scheme)
	require.Equal(t, "127.0.0.1:6000", u.Host)
	require.Equal(t, "#!:m=publish,r=123456,token=bla", u.StreamId)

	si, err := u.StreamInfo()
	require.NoError(t, err)
	require.Equal(t, "publish", si.Mode)
	require.Equal(t, "123456", si.Resource)
	require.Equal(t, "bla", si.Token)

	require.Equal(t, srturl, u.String())

	srturl = "srt://127.0.0.1:6000?mode=caller&passphrase=foobar&streamid=" + url.QueryEscape("123456,mode:publish,token:bla")

	u, err = Parse(srturl)
	require.NoError(t, err)

	require.Equal(t, "srt", u.Scheme)
	require.Equal(t, "127.0.0.1:6000", u.Host)
	require.Equal(t, "123456,mode:publish,token:bla", u.StreamId)

	si, err = u.StreamInfo()
	require.NoError(t, err)
	require.Equal(t, "publish", si.Mode)
	require.Equal(t, "123456", si.Resource)
	require.Equal(t, "bla", si.Token)

	require.Equal(t, srturl, u.String())
}

func TestParseStreamId(t *testing.T) {
	streamids := map[string]StreamInfo{
		"":                                      {Mode: "request"},
		"bla":                                   {Mode: "request", Resource: "bla"},
		"bla,token=foobar":                      {Mode: "request", Resource: "bla,token=foobar"},
		"bla,token:foobar":                      {Mode: "request", Resource: "bla", Token: "foobar"},
		"bla,token:foobar,mode:publish":         {Mode: "publish", Resource: "bla", Token: "foobar"},
		"#!:":                                   {Mode: "request"},
		"#!:key=value":                          {Mode: "request"},
		"#!:m=publish":                          {Mode: "publish"},
		"#!:r=123456789":                        {Mode: "request", Resource: "123456789"},
		"#!:token=foobar":                       {Mode: "request", Token: "foobar"},
		"#!:token=foo,bar":                      {Mode: "request", Token: "foo"},
		"#!:m=publish,r=123456789,token=foobar": {Mode: "publish", Resource: "123456789", Token: "foobar"},
	}

	for streamid, wantsi := range streamids {
		si, err := ParseStreamId(streamid)
		require.NoError(t, err)
		require.Equal(t, wantsi, si, streamid)
	}
}
