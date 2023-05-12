package api

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/datarhei/core/v16/cluster"
	"github.com/datarhei/core/v16/cluster/proxy"
	"github.com/datarhei/core/v16/encoding/json"
	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/http/handler/util"

	"github.com/labstack/echo/v4"
	"github.com/lithammer/shortuuid/v4"
)

// The ClusterHandler type provides handler functions for manipulating the cluster config.
type ClusterHandler struct {
	cluster cluster.Cluster
	proxy   proxy.ProxyReader
}

// NewCluster return a new ClusterHandler type. You have to provide a cluster.
func NewCluster(cluster cluster.Cluster) *ClusterHandler {
	return &ClusterHandler{
		cluster: cluster,
		proxy:   cluster.ProxyReader(),
	}
}

// GetNodes returns the list of proxy nodes in the cluster
// @Summary List of proxy nodes in the cluster
// @Description List of proxy nodes in the cluster
// @Tags v16.?.?
// @ID cluster-3-get-nodes
// @Produce json
// @Success 200 {array} api.ClusterNode
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/node [get]
func (h *ClusterHandler) GetNodes(c echo.Context) error {
	nodes := h.proxy.ListNodes()

	list := []api.ClusterNode{}

	for _, node := range nodes {
		about := node.About()
		n := api.ClusterNode{
			ID:          about.ID,
			Name:        about.Name,
			Address:     about.Address,
			CreatedAt:   about.CreatedAt.Format(time.RFC3339),
			Uptime:      int64(about.Uptime.Seconds()),
			LastContact: about.LastContact.Unix(),
			Latency:     about.Latency.Seconds() * 1000,
			State:       about.State,
			Resources:   api.ClusterNodeResources(about.Resources),
		}

		list = append(list, n)
	}

	return c.JSON(http.StatusOK, list)
}

// GetNode returns the proxy node with the given ID
// @Summary List a proxy node by its ID
// @Description List a proxy node by its ID
// @Tags v16.?.?
// @ID cluster-3-get-node
// @Produce json
// @Param id path string true "Node ID"
// @Success 200 {object} api.ClusterNode
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/node/{id} [get]
func (h *ClusterHandler) GetNode(c echo.Context) error {
	id := util.PathParam(c, "id")

	peer, err := h.proxy.GetNode(id)
	if err != nil {
		return api.Err(http.StatusNotFound, "Node not found", "%s", err)
	}

	about := peer.About()

	node := api.ClusterNode{
		ID:          about.ID,
		Name:        about.Name,
		Address:     about.Address,
		CreatedAt:   about.CreatedAt.Format(time.RFC3339),
		Uptime:      int64(about.Uptime.Seconds()),
		LastContact: about.LastContact.Unix(),
		Latency:     about.Latency.Seconds() * 1000,
		State:       about.State,
		Resources:   api.ClusterNodeResources(about.Resources),
	}

	return c.JSON(http.StatusOK, node)
}

// GetNodeFiles returns the files from the proxy node with the given ID
// @Summary List the files of a proxy node by its ID
// @Description List the files of a proxy node by its ID
// @Tags v16.?.?
// @ID cluster-3-get-node-files
// @Produce json
// @Param id path string true "Node ID"
// @Success 200 {object} api.ClusterNodeFiles
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/node/{id}/files [get]
func (h *ClusterHandler) GetNodeFiles(c echo.Context) error {
	id := util.PathParam(c, "id")

	peer, err := h.proxy.GetNode(id)
	if err != nil {
		return api.Err(http.StatusNotFound, "Node not found", "%s", err)
	}

	files := api.ClusterNodeFiles{
		Files: make(map[string][]string),
	}

	peerFiles := peer.Files()

	files.LastUpdate = peerFiles.LastUpdate.Unix()

	sort.Strings(peerFiles.Files)

	for _, path := range peerFiles.Files {
		prefix, path, found := strings.Cut(path, ":")
		if !found {
			continue
		}

		files.Files[prefix] = append(files.Files[prefix], path)
	}

	return c.JSON(http.StatusOK, files)
}

// GetCluster returns the list of nodes in the cluster
// @Summary List of nodes in the cluster
// @Description List of nodes in the cluster
// @Tags v16.?.?
// @ID cluster-3-get-cluster
// @Produce json
// @Success 200 {object} api.ClusterAbout
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster [get]
func (h *ClusterHandler) About(c echo.Context) error {
	state, _ := h.cluster.About()

	about := api.ClusterAbout{
		ID:                state.ID,
		Address:           state.Address,
		ClusterAPIAddress: state.ClusterAPIAddress,
		CoreAPIAddress:    state.CoreAPIAddress,
		Server:            []api.ClusterServer{},
		Stats: api.ClusterStats{
			State:       state.Stats.State,
			LastContact: state.Stats.LastContact.Seconds() * 1000,
			NumPeers:    state.Stats.NumPeers,
		},
	}

	for _, n := range state.Nodes {
		about.Server = append(about.Server, api.ClusterServer{
			ID:      n.ID,
			Address: n.Address,
			Voter:   n.Voter,
			Leader:  n.Leader,
		})
	}

	return c.JSON(http.StatusOK, about)
}

// ListProcesses returns the list of processes in the cluster
// @Summary List of processes in the cluster
// @Description List of processes in the cluster
// @Tags v16.?.?
// @ID cluster-3-list-processes
// @Produce json
// @Success 200 {array} api.ClusterProcess
// @Security ApiKeyAuth
// @Router /api/v3/cluster/process [get]
func (h *ClusterHandler) ListProcesses(c echo.Context) error {
	procs := h.proxy.ListProcesses()

	processes := []api.ClusterProcess{}

	for _, p := range procs {
		processes = append(processes, api.ClusterProcess{
			ProcessID: p.Config.ID,
			NodeID:    p.NodeID,
			Reference: p.Config.Reference,
			Order:     p.Order,
			State:     p.State,
			CPU:       json.ToNumber(p.CPU),
			Memory:    p.Mem,
			Runtime:   int64(p.Runtime.Seconds()),
		})
	}

	return c.JSON(http.StatusOK, processes)
}

// Add adds a new process to the cluster
// @Summary Add a new process
// @Description Add a new FFmpeg process
// @Tags v16.?.?
// @ID cluster-3-add-process
// @Accept json
// @Produce json
// @Param config body api.ProcessConfig true "Process config"
// @Success 200 {object} api.ProcessConfig
// @Failure 400 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/process [post]
func (h *ClusterHandler) AddProcess(c echo.Context) error {
	process := api.ProcessConfig{
		ID:        shortuuid.New(),
		Type:      "ffmpeg",
		Autostart: true,
	}

	if err := util.ShouldBindJSON(c, &process); err != nil {
		return api.Err(http.StatusBadRequest, "Invalid JSON", "%s", err)
	}

	if process.Type != "ffmpeg" {
		return api.Err(http.StatusBadRequest, "Unsupported process type", "Supported process types are: ffmpeg")
	}

	if len(process.Input) == 0 || len(process.Output) == 0 {
		return api.Err(http.StatusBadRequest, "At least one input and one output need to be defined")
	}

	config := process.Marshal()

	if err := h.cluster.AddProcess("", config); err != nil {
		return api.Err(http.StatusBadRequest, "Invalid process config", "%s", err.Error())
	}

	return c.JSON(http.StatusOK, process)
}

// Delete deletes the process with the given ID from the cluster
// @Summary Delete a process by its ID
// @Description Delete a process by its ID
// @Tags v16.?.?
// @ID cluster-3-delete-process
// @Produce json
// @Param id path string true "Process ID"
// @Success 200 {string} string
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/process/{id} [delete]
func (h *ClusterHandler) DeleteProcess(c echo.Context) error {
	id := util.PathParam(c, "id")

	if err := h.cluster.RemoveProcess("", id); err != nil {
		return api.Err(http.StatusInternalServerError, "Process can't be deleted", "%s", err)
	}

	return c.JSON(http.StatusOK, "OK")
}

/*
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

	id, err := h.cluster.AddNodeX(node.Address, "", "")
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
// @Failure 404 {object} api.Error
// @Failure 500 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/node/{id} [delete]
func (h *ClusterHandler) DeleteNode(c echo.Context) error {
	id := util.PathParam(c, "id")

	if err := h.cluster.RemoveNodeX(id); err != nil {
		if err == cluster.ErrNodeNotFound {
			return api.Err(http.StatusNotFound, err.Error(), "%s", id)
		}

		return api.Err(http.StatusInternalServerError, "Failed to remove node", "%s", err)
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

	peer, err := h.cluster.GetNodeX(id)
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

	peer, err := h.cluster.GetNodeX(id)
	if err != nil {
		return api.Err(http.StatusNotFound, "Node not found", "%s", err)
	}

	files := api.ClusterNodeFiles{}

	state := peer.State()

	sort.Strings(state.Files)

	for _, path := range state.Files {
		prefix := strings.TrimSuffix(h.prefix.FindString(path), ":")
		path = h.prefix.ReplaceAllString(path, "")

		files[prefix] = append(files[prefix], path)
	}

	return c.JSON(http.StatusOK, files)
}

// UpdateNode replaces an existing node
// @Summary Replaces an existing node
// @Description Replaces an existing node and returns the new node ID
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
		if err == cluster.ErrNodeNotFound {
			return api.Err(http.StatusNotFound, err.Error(), "%s", id)
		}
		return api.Err(http.StatusBadRequest, "Failed to remove node", "%s", err)
	}

	id, err := h.cluster.AddNodeX(node.Address, "", "")
	if err != nil {
		return api.Err(http.StatusBadRequest, "Failed to add node", "%s", err)
	}

	return c.JSON(http.StatusOK, id)
}
*/
