package api

import (
	"net/http"
	"sort"
	"strings"
	"time"

	clientapi "github.com/datarhei/core-client-go/v16/api"
	"github.com/datarhei/core/v16/cluster/proxy"
	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/http/handler/util"
	"github.com/labstack/echo/v4"
)

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
	about, _ := h.cluster.About()

	list := []api.ClusterNode{}

	for _, node := range about.Nodes {
		list = append(list, h.marshalClusterNode(node))
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

	about, _ := h.cluster.About()

	for _, node := range about.Nodes {
		if node.ID != id {
			continue
		}

		return c.JSON(http.StatusOK, h.marshalClusterNode(node))
	}

	return api.Err(http.StatusNotFound, "", "node not found")
}

// GetNodeVersion returns the proxy node version with the given ID
// @Summary List a proxy node by its ID
// @Description List a proxy node by its ID
// @Tags v16.?.?
// @ID cluster-3-get-node-version
// @Produce json
// @Param id path string true "Node ID"
// @Success 200 {object} api.Version
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/node/{id}/version [get]
func (h *ClusterHandler) GetNodeVersion(c echo.Context) error {
	id := util.PathParam(c, "id")

	peer, err := h.proxy.GetNodeReader(id)
	if err != nil {
		return api.Err(http.StatusNotFound, "", "node not found: %s", err.Error())
	}

	v := peer.Version()

	version := api.Version{
		Number:   v.Number,
		Commit:   v.Commit,
		Branch:   v.Branch,
		Build:    v.Build.Format(time.RFC3339),
		Arch:     v.Arch,
		Compiler: v.Compiler,
	}

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

	peer, err := h.proxy.GetNodeReader(id)
	if err != nil {
		return api.Err(http.StatusNotFound, "", "node not found: %s", err.Error())
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

// ListNodeProcesses returns the list of processes running on a node of the cluster
// @Summary List of processes in the cluster on a node
// @Description List of processes in the cluster on a node
// @Tags v16.?.?
// @ID cluster-3-list-node-processes
// @Produce json
// @Param id path string true "Node ID"
// @Param domain query string false "Domain to act on"
// @Param filter query string false "Comma separated list of fields (config, state, report, metadata) that will be part of the output. If empty, all fields will be part of the output."
// @Param reference query string false "Return only these process that have this reference value. If empty, the reference will be ignored."
// @Param id query string false "Comma separated list of process ids to list. Overrides the reference. If empty all IDs will be returned."
// @Param idpattern query string false "Glob pattern for process IDs. If empty all IDs will be returned. Intersected with results from other pattern matches."
// @Param refpattern query string false "Glob pattern for process references. If empty all IDs will be returned. Intersected with results from other pattern matches."
// @Param ownerpattern query string false "Glob pattern for process owners. If empty all IDs will be returned. Intersected with results from other pattern matches."
// @Param domainpattern query string false "Glob pattern for process domain. If empty all IDs will be returned. Intersected with results from other pattern matches."
// @Success 200 {array} api.Process
// @Failure 404 {object} api.Error
// @Failure 500 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/node/{id}/process [get]
func (h *ClusterHandler) ListNodeProcesses(c echo.Context) error {
	id := util.PathParam(c, "id")
	ctxuser := util.DefaultContext(c, "user", "")
	filter := strings.FieldsFunc(util.DefaultQuery(c, "filter", ""), func(r rune) bool {
		return r == rune(',')
	})
	reference := util.DefaultQuery(c, "reference", "")
	wantids := strings.FieldsFunc(util.DefaultQuery(c, "id", ""), func(r rune) bool {
		return r == rune(',')
	})
	domain := util.DefaultQuery(c, "domain", "")
	idpattern := util.DefaultQuery(c, "idpattern", "")
	refpattern := util.DefaultQuery(c, "refpattern", "")
	ownerpattern := util.DefaultQuery(c, "ownerpattern", "")
	domainpattern := util.DefaultQuery(c, "domainpattern", "")

	peer, err := h.proxy.GetNodeReader(id)
	if err != nil {
		return api.Err(http.StatusNotFound, "", "node not found: %s", err.Error())
	}

	procs, err := peer.ProcessList(proxy.ProcessListOptions{
		ID:            wantids,
		Filter:        filter,
		Domain:        domain,
		Reference:     reference,
		IDPattern:     idpattern,
		RefPattern:    refpattern,
		OwnerPattern:  ownerpattern,
		DomainPattern: domainpattern,
	})
	if err != nil {
		return api.Err(http.StatusInternalServerError, "", "node not available: %s", err.Error())
	}

	processes := []clientapi.Process{}

	for _, p := range procs {
		if !h.iam.Enforce(ctxuser, domain, "process:"+p.Config.ID, "read") {
			continue
		}

		processes = append(processes, p)
	}

	return c.JSON(http.StatusOK, processes)
}
