package alias

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/datarhei/core/v16/http/errorhandler"
	httpfs "github.com/datarhei/core/v16/http/fs"
	"github.com/datarhei/core/v16/http/handler"
	mwiam "github.com/datarhei/core/v16/http/middleware/iam"
	"github.com/datarhei/core/v16/iam"
	iamidentity "github.com/datarhei/core/v16/iam/identity"
	"github.com/datarhei/core/v16/iam/policy"
	"github.com/datarhei/core/v16/io/fs"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

func getIAM(t *testing.T) iam.IAM {
	dummyfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	policyAdapter, err := policy.NewJSONAdapter(dummyfs, "./policy.json", nil)
	require.NoError(t, err)

	identityAdapter, err := iamidentity.NewJSONAdapter(dummyfs, "./users.json", nil)
	require.NoError(t, err)

	i, err := iam.New(iam.Config{
		PolicyAdapter:   policyAdapter,
		IdentityAdapter: identityAdapter,
		Superuser: iamidentity.User{
			Name: "admin",
		},
		JWTRealm:  "datarhei-core",
		JWTSecret: "1234567890",
		Logger:    nil,
	})
	require.NoError(t, err)

	for _, name := range []string{"usera", "userb", "userc"} {
		err := i.CreateIdentity(iamidentity.User{
			Name: name,
			Auth: iamidentity.UserAuth{
				Services: iamidentity.UserAuthServices{
					Basic: []string{"secret"},
				},
			},
		})
		require.NoError(t, err)
	}

	return i
}

func TestAliasWithIAM(t *testing.T) {
	i := getIAM(t)

	// usera owns /memfs/a, userb and userc only have access to their own directories.
	i.AddPolicy("usera", "$none", []string{"fs"}, "/memfs/a/**", []string{"ANY"})
	i.AddPolicy("userb", "$none", []string{"fs"}, "/memfs/b/**", []string{"GET", "HEAD"})
	i.AddPolicy("userc", "$none", []string{"fs"}, "/memfs/c/**", []string{"GET", "HEAD"})

	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	_, _, err = memfs.WriteFileReader("/a/x.txt", strings.NewReader("hello from a"), -1)
	require.NoError(t, err)

	fshandler := handler.NewFS(httpfs.FS{
		Name:       "memfs",
		Mountpoint: "/memfs",
		Filesystem: memfs,
	})

	e := echo.New()
	e.HTTPErrorHandler = errorhandler.HTTPErrorHandler
	e.Use(mwiam.NewWithConfig(mwiam.Config{
		IAM:    i,
		Mounts: []string{"/", "/memfs"},
	}))
	e.Use(NewWithConfig(Config{
		Mounts: []string{"/", "/memfs"},
		Rules: []Rule{
			{Pattern: regexp.MustCompile(`^/memfs/b/(.*)$`), Target: "/memfs/a/$1"},
		},
	}))

	g := e.Group("/memfs/*")
	g.GET("", fshandler.GetFile)
	g.HEAD("", fshandler.GetFile)

	get := func(user, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if len(user) != 0 {
			req.Header.Set(echo.HeaderAuthorization, "Basic "+base64.StdEncoding.EncodeToString([]byte(user+":secret")))
		}
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}

	// The owner reads the original path
	rec := get("usera", "/memfs/a/x.txt")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "hello from a", rec.Body.String())

	// userb reads the file through the alias
	rec = get("userb", "/memfs/b/x.txt")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "hello from a", rec.Body.String())

	// userb has no access to the original path
	rec = get("userb", "/memfs/a/x.txt")
	require.Equal(t, http.StatusForbidden, rec.Code)

	// userc has no access to the alias path
	rec = get("userc", "/memfs/b/x.txt")
	require.Equal(t, http.StatusForbidden, rec.Code)

	// Anonymous has no access to the alias path
	rec = get("", "/memfs/b/x.txt")
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}
