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
	"github.com/datarhei/core/v16/restream"

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

// GetNodeVersion returns the proxy node version with the given ID
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
func (h *ClusterHandler) GetNodeVersion(c echo.Context) error {
	id := util.PathParam(c, "id")

	peer, err := h.proxy.GetNode(id)
	if err != nil {
		return api.Err(http.StatusNotFound, "Node not found", "%s", err)
	}

	version := peer.Version()

	return c.JSON(http.StatusOK, version)
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

// ListNodeProcesses returns the list of processes running on the nodes of the cluster
// @Summary List of processes in the cluster
// @Description List of processes in the cluster
// @Tags v16.?.?
// @ID cluster-3-list-node-processes
// @Produce json
// @Success 200 {array} api.ClusterProcess
// @Security ApiKeyAuth
// @Router /api/v3/cluster/node/process [get]
func (h *ClusterHandler) ListNodeProcesses(c echo.Context) error {
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

// ListStoreProcesses returns the list of processes stored in the DB of the cluster
// @Summary List of processes in the cluster
// @Description List of processes in the cluster
// @Tags v16.?.?
// @ID cluster-3-list-processes
// @Produce json
// @Success 200 {array} api.Process
// @Security ApiKeyAuth
// @Router /api/v3/cluster/process [get]
func (h *ClusterHandler) ListProcesses(c echo.Context) error {
	procs := h.cluster.ListProcesses()

	processes := []api.Process{}

	for _, p := range procs {
		process := api.Process{
			ID:        p.Config.ID,
			Type:      "ffmpeg",
			Reference: p.Config.Reference,
			CreatedAt: 0,
			UpdatedAt: p.UpdatedAt.Unix(),
		}

		config := &api.ProcessConfig{}
		config.Unmarshal(p.Config)

		process.Config = config

		processes = append(processes, process)
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

// Update replaces an existing process
// @Summary Replace an existing process
// @Description Replace an existing process.
// @Tags v16.?.?
// @ID cluster-3-update-process
// @Accept json
// @Produce json
// @Param id path string true "Process ID"
// @Param config body api.ProcessConfig true "Process config"
// @Success 200 {object} api.ProcessConfig
// @Failure 400 {object} api.Error
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/process/{id} [put]
func (h *ClusterHandler) UpdateProcess(c echo.Context) error {
	id := util.PathParam(c, "id")

	process := api.ProcessConfig{
		ID:        id,
		Type:      "ffmpeg",
		Autostart: true,
	}

	current, err := h.cluster.GetProcess(id)
	if err != nil {
		return api.Err(http.StatusNotFound, "Process not found", "%s", id)
	}

	// Prefill the config with the current values
	process.Unmarshal(current.Config)

	if err := util.ShouldBindJSON(c, &process); err != nil {
		return api.Err(http.StatusBadRequest, "Invalid JSON", "%s", err)
	}

	config := process.Marshal()

	if err := h.cluster.UpdateProcess("", id, config); err != nil {
		if err == restream.ErrUnknownProcess {
			return api.Err(http.StatusNotFound, "Process not found", "%s", id)
		}

		return api.Err(http.StatusBadRequest, "Process can't be updated", "%s", err)
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

// Add adds a new identity to the cluster
// @Summary Add a new identiy
// @Description Add a new identity
// @Tags v16.?.?
// @ID cluster-3-add-identity
// @Accept json
// @Produce json
// @Param config body api.IAMUser true "Identity"
// @Success 200 {object} api.IAMUser
// @Failure 400 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/iam/user [post]
func (h *ClusterHandler) AddIdentity(c echo.Context) error {
	user := api.IAMUser{}

	if err := util.ShouldBindJSON(c, &user); err != nil {
		return api.Err(http.StatusBadRequest, "Invalid JSON", "%s", err)
	}

	identity, _ := user.Unmarshal()

	if err := h.cluster.AddIdentity("", identity); err != nil {
		return api.Err(http.StatusBadRequest, "Invalid identity", "%s", err.Error())
	}

	return c.JSON(http.StatusOK, user)
}

// Delete deletes the identity with the given name from the cluster
// @Summary Delete an identity by its name
// @Description Delete an identity by its name
// @Tags v16.?.?
// @ID cluster-3-delete-identity
// @Produce json
// @Param name path string true "Identity name"
// @Success 200 {string} string
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/iam/user/{name} [delete]
func (h *ClusterHandler) RemoveIdentity(c echo.Context) error {
	name := util.PathParam(c, "name")

	if err := h.cluster.RemoveIdentity("", name); err != nil {
		return api.Err(http.StatusBadRequest, "Invalid identity", "%s", err.Error())
	}

	return c.JSON(http.StatusOK, "OK")
}
