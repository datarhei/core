package api

import (
	"net/http"
	"sort"

	"github.com/datarhei/core/v16/cluster"
	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/http/handler/util"

	"github.com/labstack/echo/v4"
)

// The ClusterHandler type provides handler functions for manipulating the cluster config.
type ClusterHandler struct {
	cluster cluster.Cluster
}

// NewCluster return a new ClusterHandler type. You have to provide a cluster.
func NewCluster(cluster cluster.Cluster) *ClusterHandler {
	return &ClusterHandler{
		cluster: cluster,
	}
}

// GetCluster returns the list of nodes in the cluster
// @Summary List of nodes in the cluster
// @Description List of nodes in the cluster
// @ID cluster-3-get-cluster
// @Produce json
// @Success 200 {array} api.ClusterNode
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster [get]
func (h *ClusterHandler) GetCluster(c echo.Context) error {
	nodes := h.cluster.ListNodes()

	list := []api.ClusterNode{}

	for _, node := range nodes {
		state := node.State()
		n := api.ClusterNode{
			Address:    node.Address(),
			ID:         state.ID,
			LastUpdate: state.LastUpdate.Unix(),
			State:      state.State,
		}

		list = append(list, n)
	}

	return c.JSON(http.StatusOK, list)
}

// AddNode adds a new node
// @Summary Add a new node
// @Description Add a new node to the cluster
// @ID cluster-3-add-node
// @Accept json
// @Produce json
// @Param config body api.ClusterNodeConfig true "Node config"
// @Success 200 {string} string
// @Failure 400 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/node [post]
func (h *ClusterHandler) AddNode(c echo.Context) error {
	node := api.ClusterNodeConfig{}

	if err := util.ShouldBindJSON(c, &node); err != nil {
		return api.Err(http.StatusBadRequest, "Invalid JSON", "%s", err)
	}

	id, err := h.cluster.AddNode(node.Address, "", "")
	if err != nil {
		return api.Err(http.StatusBadRequest, "Failed to add node", "%s", err)
	}

	return c.JSON(http.StatusOK, id)
}

// DeleteNode deletes the node with the given ID
// @Summary Delete a node by its ID
// @Description Delete a node by its ID
// @ID cluster-3-delete-node
// @Produce json
// @Param id path string true "Node ID"
// @Success 200 {string} string
// @Failure 400 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/node/{id} [delete]
func (h *ClusterHandler) DeleteNode(c echo.Context) error {
	id := util.PathParam(c, "id")

	if err := h.cluster.RemoveNode(id); err != nil {
		return api.Err(http.StatusBadRequest, "Failed to remove node", "%s", err)
	}

	return c.JSON(http.StatusOK, "OK")
}

// GetNode returns the node with the given ID
// @Summary List a node by its ID
// @Description List a node by its ID
// @ID cluster-3-get-node
// @Produce json
// @Param id path string true "Node ID"
// @Success 200 {object} api.ClusterNode
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/node/{id} [get]
func (h *ClusterHandler) GetNode(c echo.Context) error {
	id := util.PathParam(c, "id")

	peer, err := h.cluster.GetNode(id)
	if err != nil {
		return api.Err(http.StatusNotFound, "Node not found", "%s", err)
	}

	state := peer.State()

	node := api.ClusterNode{
		Address:    peer.Address(),
		ID:         state.ID,
		LastUpdate: state.LastUpdate.Unix(),
		State:      state.State,
	}

	return c.JSON(http.StatusOK, node)
}

// GetNodeProxy returns the files from the node with the given ID
// @Summary List the files of a node by its ID
// @Description List the files of a node by its ID
// @ID cluster-3-get-node-proxy
// @Produce json
// @Param id path string true "Node ID"
// @Success 200 {object} api.ClusterNodeFiles
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/node/{id}/proxy [get]
func (h *ClusterHandler) GetNodeProxy(c echo.Context) error {
	id := util.PathParam(c, "id")

	peer, err := h.cluster.GetNode(id)
	if err != nil {
		return api.Err(http.StatusNotFound, "Node not found", "%s", err)
	}

	state := peer.State()

	sort.Strings(state.Files)

	return c.JSON(http.StatusOK, state.Files)
}

// UpdateNode replaces an existing node
// @Summary Replace an existing Node
// @Description Replace an existing Node
// @ID cluster-3-update-node
// @Accept json
// @Produce json
// @Param id path string true "Node ID"
// @Param config body api.ClusterNodeConfig true "Node config"
// @Success 200 {string} string
// @Failure 400 {object} api.Error
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/node/{id} [put]
func (h *ClusterHandler) UpdateNode(c echo.Context) error {
	id := util.PathParam(c, "id")

	node := api.ClusterNodeConfig{}

	if err := util.ShouldBindJSON(c, &node); err != nil {
		return api.Err(http.StatusBadRequest, "Invalid JSON", "%s", err)
	}

	if err := h.cluster.RemoveNode(id); err != nil {
		return api.Err(http.StatusBadRequest, "Failed to remove node", "%s", err)
	}

	id, err := h.cluster.AddNode(node.Address, "", "")
	if err != nil {
		return api.Err(http.StatusBadRequest, "Failed to add node", "%s", err)
	}

	return c.JSON(http.StatusOK, id)
}
