package rewrite

import (
	"net/url"
	"testing"

	iamidentity "github.com/datarhei/core/v16/iam/identity"
	"github.com/datarhei/core/v16/io/fs"

	"github.com/stretchr/testify/require"
)

func getIdentityManager(enableBasic bool) (iamidentity.Manager, error) {
	dummyfs, _ := fs.NewMemFilesystem(fs.MemConfig{})

	adapter, err := iamidentity.NewJSONAdapter(dummyfs, "./users.json", nil)
	if err != nil {
		return nil, err
	}

	superuser := iamidentity.User{
		Name:      "foobar",
		Superuser: false,
		Auth: iamidentity.UserAuth{
			API: iamidentity.UserAuthAPI{},
			Services: iamidentity.UserAuthServices{
				Token: []string{"servicetoken"},
			},
		},
	}

	if enableBasic {
		superuser.Auth.Services.Basic = []string{"basicauthpassword"}
	}

	im, err := iamidentity.New(iamidentity.Config{
		Adapter:   adapter,
		Superuser: superuser,
		JWTRealm:  "",
		JWTSecret: "",
		Logger:    nil,
	})

	return im, err
}

func TestRewriteHTTP(t *testing.T) {
	im, err := getIdentityManager(false)
	require.NoError(t, err)

	rewrite, err := New(Config{
		HTTPBase: "http://localhost:8080/",
	})
	require.NoError(t, err)
	require.NotNil(t, rewrite)

	identity, err := im.GetVerifier("foobar")
	require.NoError(t, err)
	require.NotNil(t, identity)

	samples := [][3]string{
		{"http://example.com/live/stream.m3u8", "read", "http://example.com/live/stream.m3u8"},
		{"http://example.com/live/stream.m3u8", "write", "http://example.com/live/stream.m3u8"},
		{"http://localhost:8181/live/stream.m3u8", "read", "http://localhost:8181/live/stream.m3u8"},
		{"http://localhost:8181/live/stream.m3u8", "write", "http://localhost:8181/live/stream.m3u8"},
		{"http://localhost:8080/live/stream.m3u8", "read", "http://localhost:8080/live/stream.m3u8"},
		{"http://localhost:8080/live/stream.m3u8", "write", "http://localhost:8080/live/stream.m3u8"},
		{"http://admin:pass@localhost:8080/live/stream.m3u8", "read", "http://localhost:8080/live/stream.m3u8"},
		{"http://admin:pass@localhost:8080/live/stream.m3u8", "write", "http://localhost:8080/live/stream.m3u8"},
	}

	for _, e := range samples {
		rewritten := rewrite.RewriteAddress(e[0], identity, Access(e[1]))
		require.Equal(t, e[2], rewritten, "%s %s", e[0], e[1])
	}
}

func TestRewriteHTTPPassword(t *testing.T) {
	im, err := getIdentityManager(true)
	require.NoError(t, err)

	rewrite, err := New(Config{
		HTTPBase: "http://localhost:8080/",
	})
	require.NoError(t, err)
	require.NotNil(t, rewrite)

	identity, err := im.GetVerifier("foobar")
	require.NoError(t, err)
	require.NotNil(t, identity)

	samples := [][3]string{
		{"http://example.com/live/stream.m3u8", "read", "http://example.com/live/stream.m3u8"},
		{"http://example.com/live/stream.m3u8", "write", "http://example.com/live/stream.m3u8"},
		{"http://localhost:8181/live/stream.m3u8", "read", "http://localhost:8181/live/stream.m3u8"},
		{"http://localhost:8181/live/stream.m3u8", "write", "http://localhost:8181/live/stream.m3u8"},
		{"http://localhost:8080/live/stream.m3u8", "read", "http://foobar:basicauthpassword@localhost:8080/live/stream.m3u8"},
		{"http://localhost:8080/live/stream.m3u8", "write", "http://foobar:basicauthpassword@localhost:8080/live/stream.m3u8"},
		{"http://admin:pass@localhost:8080/live/stream.m3u8", "read", "http://foobar:basicauthpassword@localhost:8080/live/stream.m3u8"},
		{"http://admin:pass@localhost:8080/live/stream.m3u8", "write", "http://foobar:basicauthpassword@localhost:8080/live/stream.m3u8"},
	}

	for _, e := range samples {
		rewritten := rewrite.RewriteAddress(e[0], identity, Access(e[1]))
		require.Equal(t, e[2], rewritten, "%s %s", e[0], e[1])
	}
}

func TestRewriteRTMP(t *testing.T) {
	im, err := getIdentityManager(false)
	require.NoError(t, err)

	rewrite, err := New(Config{
		RTMPBase: "rtmp://localhost:1935/live",
	})
	require.NoError(t, err)
	require.NotNil(t, rewrite)

	identity, err := im.GetVerifier("foobar")
	require.NoError(t, err)
	require.NotNil(t, identity)

	samples := [][3]string{
		{"rtmp://example.com/live/stream", "read", "rtmp://example.com/live/stream"},
		{"rtmp://example.com/live/stream", "write", "rtmp://example.com/live/stream"},
		{"rtmp://localhost:1936/live/stream/token", "read", "rtmp://localhost:1936/live/stream/token"},
		{"rtmp://localhost:1936/live/stream?token=token", "write", "rtmp://localhost:1936/live/stream?token=token"},
		{"rtmp://localhost:1935/live/stream?token=token", "read", "rtmp://localhost:1935/live/stream?token=" + url.QueryEscape("foobar:servicetoken")},
		{"rtmp://localhost:1935/live/stream/token", "write", "rtmp://localhost:1935/live/stream?token=" + url.QueryEscape("foobar:servicetoken")},
	}

	for _, e := range samples {
		rewritten := rewrite.RewriteAddress(e[0], identity, Access(e[1]))
		require.Equal(t, e[2], rewritten, "%s %s", e[0], e[1])
	}
}

func TestRewriteSRT(t *testing.T) {
	im, err := getIdentityManager(false)
	require.NoError(t, err)

	rewrite, err := New(Config{
		SRTBase: "srt://localhost:6000/",
	})
	require.NoError(t, err)
	require.NotNil(t, rewrite)

	identity, err := im.GetVerifier("foobar")
	require.NoError(t, err)
	require.NotNil(t, identity)

	samples := [][3]string{
		{"srt://example.com/?streamid=stream", "read", "srt://example.com/?streamid=stream"},
		{"srt://example.com/?streamid=stream", "write", "srt://example.com/?streamid=stream"},
		{"srt://localhost:1936/?streamid=live/stream", "read", "srt://localhost:1936/?streamid=live/stream"},
		{"srt://localhost:1936/?streamid=live/stream", "write", "srt://localhost:1936/?streamid=live/stream"},
		{"srt://localhost:6000/?streamid=live/stream,mode:publish,token:token", "read", "srt://localhost:6000/?streamid=" + url.QueryEscape("live/stream,token:foobar:servicetoken")},
		{"srt://localhost:6000/?streamid=live/stream,mode:publish,token:token", "write", "srt://localhost:6000/?streamid=" + url.QueryEscape("live/stream,mode:publish,token:foobar:servicetoken")},
		{"srt://localhost:6000/?streamid=" + url.QueryEscape("#!:r=live/stream,m=publish,token=token"), "read", "srt://localhost:6000/?streamid=" + url.QueryEscape("live/stream,token:foobar:servicetoken")},
		{"srt://localhost:6000/?streamid=" + url.QueryEscape("#!:r=live/stream,m=publish,token=token"), "write", "srt://localhost:6000/?streamid=" + url.QueryEscape("live/stream,mode:publish,token:foobar:servicetoken")},
	}

	for _, e := range samples {
		rewritten := rewrite.RewriteAddress(e[0], identity, Access(e[1]))
		require.Equal(t, e[2], rewritten, "%s %s", e[0], e[1])
	}
}
