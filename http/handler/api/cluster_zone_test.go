package api

import (
	"net/http"
	"testing"

	"github.com/datarhei/core/v16/cluster"
	clusternode "github.com/datarhei/core/v16/cluster/node"
	"github.com/datarhei/core/v16/cluster/store"
	clusterzone "github.com/datarhei/core/v16/cluster/zone"
	"github.com/datarhei/core/v16/encoding/json"
	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/http/mock"
	"github.com/datarhei/core/v16/iam"
	"github.com/datarhei/core/v16/restream/app"
	"github.com/datarhei/core/v16/zone"

	"github.com/hashicorp/raft"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

// zoneCluster is a cluster that applies the commands directly to a store. Only the
// methods that are required for the zone and process handlers are implemented.
type zoneCluster struct {
	cluster.Cluster

	store   store.Store
	manager *clusternode.Manager
}

func newZoneCluster(t *testing.T) *zoneCluster {
	s, err := store.NewStore(store.Config{})
	require.NoError(t, err)

	m, err := clusternode.NewManager(clusternode.ManagerConfig{ID: "node"})
	require.NoError(t, err)

	return &zoneCluster{
		store:   s,
		manager: m,
	}
}

func (c *zoneCluster) apply(op store.Operation, data any) error {
	b, err := json.Marshal(store.Command{Operation: op, Data: data})
	if err != nil {
		return err
	}

	res := c.store.Apply(&raft.Log{Data: b})
	if err, ok := res.(error); ok {
		return err
	}

	return nil
}

func (c *zoneCluster) Store() store.Store            { return c.store }
func (c *zoneCluster) Manager() *clusternode.Manager { return c.manager }
func (c *zoneCluster) ZoneAdd(origin string, z zone.Zone) error {
	return c.apply(store.OpAddZone, store.CommandAddZone{Zone: z})
}
func (c *zoneCluster) ZoneUpdate(origin string, z zone.Zone) error {
	return c.apply(store.OpUpdateZone, store.CommandUpdateZone{Zone: z})
}
func (c *zoneCluster) ZoneRemove(origin, id string) error {
	return c.apply(store.OpRemoveZone, store.CommandRemoveZone{ID: id})
}
func (c *zoneCluster) ProcessAdd(origin string, config *app.Config) error {
	return c.apply(store.OpAddProcess, store.CommandAddProcess{Config: config})
}
func (c *zoneCluster) ProcessUpdate(origin string, id app.ProcessID, config *app.Config, force bool) error {
	return c.apply(store.OpUpdateProcess, store.CommandUpdateProcess{ID: id, Config: config, Force: force})
}
func (c *zoneCluster) ProcessGet(origin string, id app.ProcessID, stale bool) (store.Process, string, error) {
	return c.store.ProcessGet(id)
}
func (c *zoneCluster) ProcessSetMetadata(origin string, id app.ProcessID, key string, data interface{}) error {
	return nil
}

func getDummyClusterZoneRouter(t *testing.T, i iam.IAM) (*echo.Echo, *zoneCluster) {
	c := newZoneCluster(t)

	handler, err := NewCluster(c, i)
	require.NoError(t, err)

	router := mock.DummyEcho()
	router.Use(withUser)

	router.GET("/zone", handler.ZoneList)
	router.POST("/zone", handler.ZoneAdd)
	router.GET("/zone/:id", handler.ZoneGet)
	router.PUT("/zone/:id", handler.ZoneUpdate)
	router.DELETE("/zone/:id", handler.ZoneRemove)

	router.POST("/process", handler.ProcessAdd)
	router.PUT("/process/:id", handler.ProcessUpdate)

	return router, c
}

func clusterZoneProcess(t *testing.T, id, domain, zone string, cpu float64) api.ProcessConfig {
	process := zoneProcess(t, id, domain, zone, cpu)
	process.Limits.Memory = 10

	return process
}

func TestClusterZoneAddGetUpdateDelete(t *testing.T) {
	router, _ := getDummyClusterZoneRouter(t, newZoneIAM(t))

	z := api.Zone{ID: "a", Limits: api.ZoneResources{CPU: 100, Memory: 512}}

	rec := zoneRequest(t, router, "foobar", "POST", "/zone", z)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	rec = zoneRequest(t, router, "foobar", "POST", "/zone", z)
	require.Equal(t, http.StatusConflict, rec.Code)

	rec = zoneRequest(t, router, "foobar", "POST", "/zone", api.Zone{ID: "a b"})
	require.Equal(t, http.StatusBadRequest, rec.Code)

	rec = zoneRequest(t, router, "foobar", "POST", "/zone", api.Zone{ID: zone.DefaultID})
	require.Equal(t, http.StatusConflict, rec.Code)

	// Everybody can read the zones
	require.Equal(t, []string{zone.DefaultID, "a"}, zoneList(t, router, "alice"))

	rec = zoneRequest(t, router, "alice", "GET", "/zone/a", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	info := api.ZoneInfo{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &info))
	require.Equal(t, api.ZoneInfo{Zone: z}, info)

	rec = zoneRequest(t, router, "alice", "GET", "/zone/$default", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	rec = zoneRequest(t, router, "alice", "GET", "/zone/x", nil)
	require.Equal(t, http.StatusNotFound, rec.Code)

	// Update, the ID is taken from the path
	rec = zoneRequest(t, router, "foobar", "PUT", "/zone/a", api.Zone{Limits: api.ZoneResources{CPU: 200}})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = zoneRequest(t, router, "foobar", "PUT", "/zone/a", api.Zone{ID: "b"})
	require.Equal(t, http.StatusBadRequest, rec.Code)

	rec = zoneRequest(t, router, "foobar", "PUT", "/zone/x", api.Zone{})
	require.Equal(t, http.StatusNotFound, rec.Code)

	rec = zoneRequest(t, router, "foobar", "PUT", "/zone/$default", api.Zone{Limits: api.ZoneResources{CPU: 300}})
	require.Equal(t, http.StatusOK, rec.Code)

	// Only superusers can modify zones
	require.Equal(t, http.StatusForbidden, zoneRequest(t, router, "alice", "POST", "/zone", api.Zone{ID: "c"}).Code)
	require.Equal(t, http.StatusForbidden, zoneRequest(t, router, "alice", "PUT", "/zone/a", api.Zone{}).Code)
	require.Equal(t, http.StatusForbidden, zoneRequest(t, router, "alice", "DELETE", "/zone/a", nil).Code)

	// Delete
	rec = zoneRequest(t, router, "foobar", "DELETE", "/zone/$default", nil)
	require.Equal(t, http.StatusBadRequest, rec.Code)

	rec = zoneRequest(t, router, "foobar", "POST", "/process", clusterZoneProcess(t, "p1", "", "a", 10))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	rec = zoneRequest(t, router, "foobar", "DELETE", "/zone/a", nil)
	require.Equal(t, http.StatusConflict, rec.Code)

	rec = zoneRequest(t, router, "foobar", "GET", "/zone/a", nil)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &info))
	require.Equal(t, uint64(1), info.Processes)
	require.Equal(t, float64(10), info.Usage.CPU)
	require.Equal(t, uint64(10), info.Usage.Memory)

	rec = zoneRequest(t, router, "foobar", "DELETE", "/zone/x", nil)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestClusterZoneProcess(t *testing.T) {
	i := newZoneIAM(t)
	i.AddPolicy("alice", "$none", []string{"process"}, "**", []string{"ANY"})
	i.AddPolicy("alice", "foo", []string{"process"}, "**", []string{"ANY"})
	i.AddPolicy("alice", "foo", []string{"zone"}, "a", []string{"write"})
	i.AddPolicy("alice", "$none", []string{"zone"}, "b", []string{"write"})

	router, c := getDummyClusterZoneRouter(t, i)

	require.NoError(t, c.ZoneAdd("", zone.Zone{ID: "a", Limits: zone.Resources{CPU: 100}}))
	require.NoError(t, c.ZoneAdd("", zone.Zone{ID: "b"}))

	// Allowed to write to zone "a" in domain "foo"
	rec := zoneRequest(t, router, "alice", "POST", "/process", clusterZoneProcess(t, "p1", "foo", "a", 60))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// The limit of zone "a" is exceeded
	rec = zoneRequest(t, router, "alice", "POST", "/process", clusterZoneProcess(t, "p2", "foo", "a", 60))
	require.Equal(t, http.StatusBadRequest, rec.Code)

	// Not allowed to write to zone "a" without a domain
	rec = zoneRequest(t, router, "alice", "POST", "/process", clusterZoneProcess(t, "p3", "", "a", 10))
	require.Equal(t, http.StatusForbidden, rec.Code)

	// Unknown zone
	rec = zoneRequest(t, router, "foobar", "POST", "/process", clusterZoneProcess(t, "p3", "", "x", 10))
	require.Equal(t, http.StatusBadRequest, rec.Code)

	// The default zone needs no permission
	rec = zoneRequest(t, router, "alice", "POST", "/process", clusterZoneProcess(t, "p4", "foo", "", 10))
	require.Equal(t, http.StatusOK, rec.Code)

	// Moving p1 from zone "a" in domain "foo" to zone "b" without a domain is allowed
	rec = zoneRequest(t, router, "alice", "PUT", "/process/p1?domain=foo", clusterZoneProcess(t, "p1", "", "b", 60))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// Moving it to zone "a" without a domain is not allowed
	rec = zoneRequest(t, router, "alice", "PUT", "/process/p1", clusterZoneProcess(t, "p1", "", "a", 60))
	require.Equal(t, http.StatusForbidden, rec.Code)

	info, err := c.Store().ZoneGet("b")
	require.NoError(t, err)
	require.Equal(t, 1, info.Processes)

	// Moving a process out of zone "b" in domain "foo" is not allowed
	rec = zoneRequest(t, router, "foobar", "POST", "/process", clusterZoneProcess(t, "p5", "foo", "b", 10))
	require.Equal(t, http.StatusOK, rec.Code)

	rec = zoneRequest(t, router, "alice", "PUT", "/process/p5?domain=foo", clusterZoneProcess(t, "p5", "foo", "", 10))
	require.Equal(t, http.StatusForbidden, rec.Code)

	// An update without a zone change needs no zone permission
	rec = zoneRequest(t, router, "alice", "PUT", "/process/p5?domain=foo", clusterZoneProcess(t, "p5", "foo", "b", 20))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

func TestZoneReadOnly(t *testing.T) {
	s, err := store.NewStore(store.Config{})
	require.NoError(t, err)

	zones, err := clusterzone.New(s)
	require.NoError(t, err)

	router := getDummyZoneRouter(t, zones)

	require.Equal(t, []string{zone.DefaultID}, zoneList(t, router, "foobar"))

	rec := zoneRequest(t, router, "foobar", "POST", "/zone", api.Zone{ID: "a"})
	require.Equal(t, http.StatusBadRequest, rec.Code)

	rec = zoneRequest(t, router, "foobar", "PUT", "/zone/$default", api.Zone{})
	require.Equal(t, http.StatusBadRequest, rec.Code)

	rec = zoneRequest(t, router, "foobar", "DELETE", "/zone/a", nil)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestClusterZoneFilterStoreProcesses(t *testing.T) {
	processes := []store.Process{
		{Config: &app.Config{ID: "p1", Zone: "a"}},
		{Config: &app.Config{ID: "p2", Zone: "ab"}},
		{Config: &app.Config{ID: "p3", Zone: ""}},
		{Config: &app.Config{ID: "x4", Zone: "a"}},
	}

	h := &ClusterHandler{}

	ids := func(idpattern, zonepattern string) []string {
		list := []string{}
		for _, p := range h.getFilteredStoreProcesses(processes, nil, "", idpattern, "", "", "", zonepattern) {
			list = append(list, p.Config.ID)
		}
		return list
	}

	require.ElementsMatch(t, []string{"p1", "p2", "p3", "x4"}, ids("", ""))
	require.ElementsMatch(t, []string{"p1", "x4"}, ids("", "a"))
	require.ElementsMatch(t, []string{"p1", "p2", "x4"}, ids("", "a*"))
	require.ElementsMatch(t, []string{"p3"}, ids("", zone.DefaultID))
	require.ElementsMatch(t, []string{"p1", "p2"}, ids("p*", "a*"))
}
