package api

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/datarhei/core/v16/cluster"
	"github.com/datarhei/core/v16/cluster/proxy"
	"github.com/datarhei/core/v16/encoding/json"
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
		ID:      state.ID,
		Domains: state.Domains,
		Leader: api.ClusterAboutLeader{
			ID:           state.Leader.ID,
			Address:      state.Leader.Address,
			ElectedSince: uint64(state.Leader.ElectedSince.Seconds()),
		},
		Status: state.Status,
		Raft: api.ClusterRaft{
			Address:     state.Raft.Address,
			State:       state.Raft.State,
			LastContact: state.Raft.LastContact.Seconds() * 1000,
			NumPeers:    state.Raft.NumPeers,
			LogTerm:     state.Raft.LogTerm,
			LogIndex:    state.Raft.LogIndex,
		},
		Nodes:    []api.ClusterNode{},
		Version:  state.Version.String(),
		Degraded: state.Degraded,
	}

	if state.DegradedErr != nil {
		about.DegradedErr = state.DegradedErr.Error()
	}

	for _, node := range state.Nodes {
		about.Nodes = append(about.Nodes, h.marshalClusterNode(node))
	}

	return c.JSON(http.StatusOK, about)
}

func (h *ClusterHandler) marshalClusterNode(node cluster.ClusterNode) api.ClusterNode {
	n := api.ClusterNode{
		ID:          node.ID,
		Name:        node.Name,
		Version:     node.Version,
		Status:      node.Status,
		Voter:       node.Voter,
		Leader:      node.Leader,
		Address:     node.Address,
		CreatedAt:   node.CreatedAt.Format(time.RFC3339),
		Uptime:      int64(node.Uptime.Seconds()),
		LastContact: node.LastContact.Seconds() * 1000,
		Latency:     node.Latency.Seconds() * 1000,
		Core: api.ClusterNodeCore{
			Address:     node.Core.Address,
			Status:      node.Core.Status,
			LastContact: node.Core.LastContact.Seconds() * 1000,
			Latency:     node.Core.Latency.Seconds() * 1000,
			Version:     node.Core.Version,
		},
		Resources: api.ClusterNodeResources{
			IsThrottling: node.Resources.IsThrottling,
			NCPU:         node.Resources.NCPU,
			CPU:          node.Resources.CPU,
			CPULimit:     node.Resources.CPULimit,
			Mem:          node.Resources.Mem,
			MemLimit:     node.Resources.MemLimit,
		},
	}

	if node.Error != nil {
		n.Error = node.Error.Error()
	}

	if node.Core.Error != nil {
		n.Core.Error = node.Core.Error.Error()
	}

	if node.Resources.Error != nil {
		n.Resources.Error = node.Resources.Error.Error()
	}

	return n
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
// @Param nodeid body api.ClusterNodeID true "Node ID"
// @Success 200 {string} string
// @Failure 500 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/leave [put]
func (h *ClusterHandler) Leave(c echo.Context) error {
	nodeid := api.ClusterNodeID{}

	req := c.Request()

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return err
	}

	if len(body) != 0 {
		if err := json.Unmarshal(body, &nodeid); err != nil {
			return api.Err(http.StatusBadRequest, "", "invalid JSON: %s", json.FormatError(body, err).Error())
		}
	}

	h.cluster.Leave("", nodeid.ID)

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
