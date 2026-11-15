package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/datarhei/core/v16/encoding/json"
	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/http/mock"
	"github.com/datarhei/core/v16/iam"
	"github.com/datarhei/core/v16/iam/identity"
	"github.com/datarhei/core/v16/iam/policy"
	"github.com/datarhei/core/v16/internal/mock/restream"
	"github.com/datarhei/core/v16/io/fs"
	"github.com/datarhei/core/v16/zone"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

func newZoneIAM(t *testing.T) iam.IAM {
	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	policyAdapter, err := policy.NewJSONAdapter(memfs, "./policy.json", nil)
	require.NoError(t, err)

	identityAdapter, err := identity.NewJSONAdapter(memfs, "./users.json", nil)
	require.NoError(t, err)

	i, err := iam.New(iam.Config{
		PolicyAdapter:   policyAdapter,
		IdentityAdapter: identityAdapter,
		Superuser: identity.User{
			Name: "foobar",
		},
	})
	require.NoError(t, err)

	return i
}

func newZoneManager(t *testing.T, zones ...zone.Zone) zone.Manager {
	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	m, err := zone.New(zone.Config{
		Filesystem: memfs,
		Filepath:   "/zones.json",
	})
	require.NoError(t, err)

	for _, z := range zones {
		require.NoError(t, m.Add(z))
	}

	return m
}

// withUser sets the user and the superuser flag from the X-User header, the user
// "foobar" is the superuser.
func withUser(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		user := c.Request().Header.Get("X-User")
		c.Set("user", user)
		c.Set("superuser", user == "foobar")
		return next(c)
	}
}

func getDummyZoneRouter(t *testing.T, zones zone.Manager) *echo.Echo {
	router := mock.DummyEcho()
	router.Use(withUser)

	handler := NewZone(zones)

	router.GET("/zone", handler.List)
	router.POST("/zone", handler.Add)
	router.GET("/zone/:id", handler.Get)
	router.PUT("/zone/:id", handler.Update)
	router.DELETE("/zone/:id", handler.Delete)

	return router
}

func zoneRequest(t *testing.T, router *echo.Echo, user, method, path string, body any) *httptest.ResponseRecorder {
	var data *bytes.Reader

	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		data = bytes.NewReader(b)
	} else {
		data = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(method, path, data)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User", user)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	return rec
}

func zoneList(t *testing.T, router *echo.Echo, user string) []string {
	rec := zoneRequest(t, router, user, "GET", "/zone", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	zones := []api.ZoneInfo{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &zones))

	ids := []string{}
	for _, z := range zones {
		ids = append(ids, z.ID)
	}

	return ids
}

func TestZoneAddGetUpdateDelete(t *testing.T) {
	zones := newZoneManager(t)
	router := getDummyZoneRouter(t, zones)

	z := api.Zone{ID: "a", Limits: api.ZoneResources{CPU: 100, Memory: 512}}

	rec := zoneRequest(t, router, "foobar", "POST", "/zone", z)
	require.Equal(t, http.StatusOK, rec.Code)

	rec = zoneRequest(t, router, "foobar", "POST", "/zone", z)
	require.Equal(t, http.StatusConflict, rec.Code)

	rec = zoneRequest(t, router, "foobar", "POST", "/zone", api.Zone{ID: "a b"})
	require.Equal(t, http.StatusBadRequest, rec.Code)

	rec = zoneRequest(t, router, "foobar", "POST", "/zone", api.Zone{ID: zone.DefaultID})
	require.Equal(t, http.StatusConflict, rec.Code)

	zones.Register(&zone.Claim{Zone: "a", Resources: zone.Resources{CPU: 60, Memory: 256 * 1024 * 1024}})

	rec = zoneRequest(t, router, "foobar", "GET", "/zone/a", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	info := api.ZoneInfo{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &info))
	require.Equal(t, api.ZoneInfo{
		Zone:      z,
		Usage:     api.ZoneResources{CPU: 60, Memory: 256},
		Processes: 1,
	}, info)

	rec = zoneRequest(t, router, "foobar", "GET", "/zone/$default", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	rec = zoneRequest(t, router, "foobar", "GET", "/zone/%24default", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	rec = zoneRequest(t, router, "foobar", "GET", "/zone/x", nil)
	require.Equal(t, http.StatusNotFound, rec.Code)

	// Update, the ID is taken from the path
	rec = zoneRequest(t, router, "foobar", "PUT", "/zone/a", api.Zone{Limits: api.ZoneResources{CPU: 200}})
	require.Equal(t, http.StatusOK, rec.Code)

	updated, err := zones.Get("a")
	require.NoError(t, err)
	require.Equal(t, float64(200), updated.Limits.CPU)

	rec = zoneRequest(t, router, "foobar", "PUT", "/zone/a", api.Zone{ID: "b"})
	require.Equal(t, http.StatusBadRequest, rec.Code)

	rec = zoneRequest(t, router, "foobar", "PUT", "/zone/x", api.Zone{})
	require.Equal(t, http.StatusNotFound, rec.Code)

	rec = zoneRequest(t, router, "foobar", "PUT", "/zone/$default", api.Zone{Limits: api.ZoneResources{CPU: 300}})
	require.Equal(t, http.StatusOK, rec.Code)

	// Delete
	rec = zoneRequest(t, router, "foobar", "DELETE", "/zone/$default", nil)
	require.Equal(t, http.StatusBadRequest, rec.Code)

	rec = zoneRequest(t, router, "foobar", "DELETE", "/zone/a", nil)
	require.Equal(t, http.StatusConflict, rec.Code)

	zones.Release(&zone.Claim{Zone: "a", Resources: zone.Resources{CPU: 60, Memory: 256 * 1024 * 1024}})

	rec = zoneRequest(t, router, "foobar", "DELETE", "/zone/a", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	rec = zoneRequest(t, router, "foobar", "DELETE", "/zone/a", nil)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestZoneAccess(t *testing.T) {
	zones := newZoneManager(t, zone.Zone{ID: "a"}, zone.Zone{ID: "b"})
	router := getDummyZoneRouter(t, zones)

	// Everybody can read all zones
	require.Equal(t, []string{zone.DefaultID, "a", "b"}, zoneList(t, router, "foobar"))
	require.Equal(t, []string{zone.DefaultID, "a", "b"}, zoneList(t, router, "alice"))
	require.Equal(t, []string{zone.DefaultID, "a", "b"}, zoneList(t, router, ""))

	require.Equal(t, http.StatusOK, zoneRequest(t, router, "alice", "GET", "/zone/a", nil).Code)
	require.Equal(t, http.StatusOK, zoneRequest(t, router, "alice", "GET", "/zone/b", nil).Code)
	require.Equal(t, http.StatusOK, zoneRequest(t, router, "alice", "GET", "/zone/$default", nil).Code)

	// Only superusers can modify zones
	require.Equal(t, http.StatusForbidden, zoneRequest(t, router, "alice", "POST", "/zone", api.Zone{ID: "c"}).Code)
	require.Equal(t, http.StatusForbidden, zoneRequest(t, router, "alice", "PUT", "/zone/a", api.Zone{}).Code)
	require.Equal(t, http.StatusForbidden, zoneRequest(t, router, "alice", "DELETE", "/zone/a", nil).Code)
}

func getDummyZoneProcessRouter(t *testing.T, zones zone.Manager, i iam.IAM) *echo.Echo {
	rs, err := restream.NewWithZones(nil, nil, nil, nil, zones)
	require.NoError(t, err)

	handler := NewProcess(rs, i)

	router := mock.DummyEcho()
	router.Use(withUser)

	router.GET("/", handler.GetAll)
	router.POST("/", handler.Add)
	router.PUT("/:id", handler.Update)

	return router
}

func zoneProcess(t *testing.T, id, domain, zone string, cpu float64) api.ProcessConfig {
	process := api.ProcessConfig{}
	require.NoError(t, json.NewDecoder(mock.Read(t, "./fixtures/addProcess.json")).Decode(&process))

	process.ID = id
	process.Domain = domain
	process.Zone = zone
	process.Limits.CPU = cpu

	return process
}

func TestZoneProcess(t *testing.T) {
	i := newZoneIAM(t)
	i.AddPolicy("alice", "$none", []string{"process"}, "**", []string{"ANY"})
	i.AddPolicy("alice", "foo", []string{"process"}, "**", []string{"ANY"})
	i.AddPolicy("alice", "foo", []string{"zone"}, "a", []string{"write"})
	i.AddPolicy("alice", "$none", []string{"zone"}, "b", []string{"write"})

	zones := newZoneManager(t,
		zone.Zone{ID: "a", Limits: zone.Resources{CPU: 100}},
		zone.Zone{ID: "b"},
	)

	router := getDummyZoneProcessRouter(t, zones, i)

	// Allowed to write to zone "a" in domain "foo"
	rec := zoneRequest(t, router, "alice", "POST", "/", zoneProcess(t, "p1", "foo", "a", 60))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	config := api.ProcessConfig{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &config))
	require.Equal(t, "a", config.Zone)

	// The limit of zone "a" is exceeded
	rec = zoneRequest(t, router, "alice", "POST", "/", zoneProcess(t, "p2", "foo", "a", 60))
	require.Equal(t, http.StatusBadRequest, rec.Code)

	// Not allowed to write to zone "a" without a domain
	rec = zoneRequest(t, router, "alice", "POST", "/", zoneProcess(t, "p3", "", "a", 10))
	require.Equal(t, http.StatusForbidden, rec.Code)

	// Not allowed to write to zone "b" in domain "foo"
	rec = zoneRequest(t, router, "alice", "POST", "/", zoneProcess(t, "p3", "foo", "b", 10))
	require.Equal(t, http.StatusForbidden, rec.Code)

	// Allowed to write to zone "b" without a domain
	rec = zoneRequest(t, router, "alice", "POST", "/", zoneProcess(t, "p4", "", "b", 10))
	require.Equal(t, http.StatusOK, rec.Code)

	// Unknown zone
	rec = zoneRequest(t, router, "foobar", "POST", "/", zoneProcess(t, "p5", "", "x", 10))
	require.Equal(t, http.StatusBadRequest, rec.Code)

	// The default zone needs no permission
	rec = zoneRequest(t, router, "alice", "POST", "/", zoneProcess(t, "p5", "foo", "", 10))
	require.Equal(t, http.StatusOK, rec.Code)

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &config))
	require.Equal(t, zone.DefaultID, config.Zone)

	rec = zoneRequest(t, router, "alice", "POST", "/", zoneProcess(t, "p6", "", zone.DefaultID, 10))
	require.Equal(t, http.StatusOK, rec.Code)

	// Moving p1 from zone "a" in domain "foo" to zone "b" without a domain is allowed, the
	// old zone is checked in domain "foo", the new zone without a domain
	rec = zoneRequest(t, router, "alice", "PUT", "/p1?domain=foo", zoneProcess(t, "p1", "", "b", 60))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// p1 is now in zone "b" without a domain, moving it to zone "a" without a domain is not allowed
	rec = zoneRequest(t, router, "alice", "PUT", "/p1", zoneProcess(t, "p1", "", "a", 60))
	require.Equal(t, http.StatusForbidden, rec.Code)

	// Moving it to zone "a" in domain "foo" is allowed, the old zone is checked in the old domain
	rec = zoneRequest(t, router, "alice", "PUT", "/p1", zoneProcess(t, "p1", "foo", "a", 60))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	info, err := zones.Get("a")
	require.NoError(t, err)
	require.Equal(t, 1, info.Processes)

	// Moving a process out of zone "b" in domain "foo" requires write permission on zone "b" in domain "foo"
	rec = zoneRequest(t, router, "foobar", "POST", "/", zoneProcess(t, "p7", "foo", "b", 10))
	require.Equal(t, http.StatusOK, rec.Code)

	rec = zoneRequest(t, router, "alice", "PUT", "/p7?domain=foo", zoneProcess(t, "p7", "foo", "", 10))
	require.Equal(t, http.StatusForbidden, rec.Code)

	// An update without a zone change needs no zone permission
	rec = zoneRequest(t, router, "alice", "PUT", "/p7?domain=foo", zoneProcess(t, "p7", "foo", "b", 20))
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestZoneProcessList(t *testing.T) {
	zones := newZoneManager(t, zone.Zone{ID: "a"}, zone.Zone{ID: "b"})

	// The processes are listed by a user without an identity. Listing them as a superuser
	// triggers a data race in the identity manager while enforcing the policies in parallel.
	i := newZoneIAM(t)
	i.AddPolicy("alice", "$none", []string{"process"}, "**", []string{"read"})

	router := getDummyZoneProcessRouter(t, zones, i)

	for _, p := range []api.ProcessConfig{
		zoneProcess(t, "p1", "", "a", 10),
		zoneProcess(t, "p2", "", "b", 10),
		zoneProcess(t, "p3", "", "", 10),
	} {
		rec := zoneRequest(t, router, "foobar", "POST", "/", p)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	}

	list := func(query string) []string {
		rec := zoneRequest(t, router, "alice", "GET", "/?filter=config&"+query, nil)
		require.Equal(t, http.StatusOK, rec.Code)

		processes := []api.Process{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &processes))

		ids := []string{}
		for _, p := range processes {
			ids = append(ids, p.ID)
		}

		return ids
	}

	require.ElementsMatch(t, []string{"p1", "p2", "p3"}, list(""))
	require.ElementsMatch(t, []string{"p1"}, list("zonepattern=a"))
	require.ElementsMatch(t, []string{"p3"}, list("zonepattern=$default"))
	require.ElementsMatch(t, []string{"p1", "p2"}, list("zonepattern=[ab]"))
	require.ElementsMatch(t, []string{"p2"}, list("zonepattern=b&idpattern=p*"))
	require.ElementsMatch(t, []string{}, list("zonepattern=b&idpattern=x*"))
}
