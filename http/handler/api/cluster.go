package api

import (
	"fmt"
	"net/http"

	"github.com/datarhei/core/v16/cluster"
	"github.com/datarhei/core/v16/cluster/proxy"
	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/http/handler/util"
	"github.com/datarhei/core/v16/iam"

	"github.com/labstack/echo/v4"
)

// The ClusterHandler type provides handler functions for manipulating the cluster config.
type ClusterHandler struct {
	cluster cluster.Cluster
	proxy   proxy.ProxyReader
	iam     iam.IAM
}

// NewCluster return a new ClusterHandler type. You have to provide a cluster.
func NewCluster(cluster cluster.Cluster, iam iam.IAM) (*ClusterHandler, error) {
	h := &ClusterHandler{
		cluster: cluster,
		proxy:   cluster.ProxyReader(),
		iam:     iam,
	}

	if h.cluster == nil {
		return nil, fmt.Errorf("no cluster provided")
	}

	if h.proxy == nil {
		return nil, fmt.Errorf("proxy reader from cluster is not available")
	}

	if h.iam == nil {
		return nil, fmt.Errorf("no IAM provided")
	}

	return h, nil
}

// GetCluster returns the list of nodes in the cluster
// @Summary List of nodes in the cluster
// @Description List of nodes in the cluster
// @Tags v16.?.?
// @ID cluster-3-get-cluster
// @Produce json
// @Success 200 {object} api.ClusterAbout
// @Security ApiKeyAuth
// @Router /api/v3/cluster [get]
func (h *ClusterHandler) About(c echo.Context) error {
	state, _ := h.cluster.About()

	about := api.ClusterAbout{
		ID:                state.ID,
		Address:           state.Address,
		ClusterAPIAddress: state.ClusterAPIAddress,
		CoreAPIAddress:    state.CoreAPIAddress,
		Raft: api.ClusterRaft{
			Server: []api.ClusterRaftServer{},
			Stats: api.ClusterRaftStats{
				State:       state.Raft.Stats.State,
				LastContact: state.Raft.Stats.LastContact.Seconds() * 1000,
				NumPeers:    state.Raft.Stats.NumPeers,
			},
		},
		Version:  state.Version.String(),
		Degraded: state.Degraded,
	}

	if state.DegradedErr != nil {
		about.DegradedErr = state.DegradedErr.Error()
	}

	for _, n := range state.Raft.Server {
		about.Raft.Server = append(about.Raft.Server, api.ClusterRaftServer{
			ID:      n.ID,
			Address: n.Address,
			Voter:   n.Voter,
			Leader:  n.Leader,
		})
	}

	for _, node := range state.Nodes {
		n := api.ClusterNode{}
		n.Marshal(node)

		about.Nodes = append(about.Nodes, n)
	}

	return c.JSON(http.StatusOK, about)
}

// Healthy returns whether the cluster is healthy
// @Summary Whether the cluster is healthy
// @Description Whether the cluster is healthy
// @Tags v16.?.?
// @ID cluster-3-healthy
// @Produce json
// @Success 200 {bool} bool
// @Security ApiKeyAuth
// @Router /api/v3/cluster/healthy [get]
func (h *ClusterHandler) Healthy(c echo.Context) error {
	degraded, _ := h.cluster.IsDegraded()

	return c.JSON(http.StatusOK, !degraded)
}

// Transfer the leadership to another node
// @Summary Transfer the leadership to another node
// @Description Transfer the leadership to another node
// @Tags v16.?.?
// @ID cluster-3-transfer-leadership
// @Produce json
// @Success 200 {string} string
// @Failure 500 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/transfer/{id} [put]
func (h *ClusterHandler) TransferLeadership(c echo.Context) error {
	id := util.PathParam(c, "id")

	h.cluster.TransferLeadership("", id)

	return c.JSON(http.StatusOK, "OK")
}

// Leave the cluster gracefully
// @Summary Leave the cluster gracefully
// @Description Leave the cluster gracefully
// @Tags v16.?.?
// @ID cluster-3-leave
// @Produce json
// @Success 200 {string} string
// @Failure 500 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/leave [put]
func (h *ClusterHandler) Leave(c echo.Context) error {
	h.cluster.Leave("", "")
	h.cluster.Shutdown()

	return c.JSON(http.StatusOK, "OK")
}

// GetSnapshot returns a current snapshot of the cluster DB
// @Summary Retrieve snapshot of the cluster DB
// @Description Retrieve snapshot of the cluster DB
// @Tags v16.?.?
// @ID cluster-3-snapshot
// @Produce application/octet-stream
// @Success 200 {file} byte
// @Security ApiKeyAuth
// @Router /api/v3/cluster/snapshot [get]
func (h *ClusterHandler) GetSnapshot(c echo.Context) error {
	r, err := h.cluster.Snapshot("")
	if err != nil {
		return api.Err(http.StatusInternalServerError, "", "failed to retrieve snapshot: %w", err)
	}

	defer r.Close()

	return c.Stream(http.StatusOK, "application/octet-stream", r)
}
