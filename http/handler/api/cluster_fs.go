package api

import (
	"net/http"
	"sort"

	"github.com/datarhei/core/v16/http/handler/util"

	"github.com/labstack/echo/v4"
)

// FilesystemListFiles lists all files on a filesystem
// @Summary List all files on a filesystem
// @Description List all files on a filesystem. The listing can be ordered by name, size, or date of last modification in ascending or descending order.
// @Tags v16.?.?
// @ID cluster-3-list-files
// @Produce json
// @Param storage path string true "Name of the filesystem"
// @Param glob query string false "glob pattern for file names"
// @Param sort query string false "none, name, size, lastmod"
// @Param order query string false "asc, desc"
// @Success 200 {array} api.FileInfo
// @Success 500 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/fs/{storage} [get]
func (h *ClusterHandler) FilesystemListFiles(c echo.Context) error {
	name := util.PathParam(c, "storage")
	pattern := util.DefaultQuery(c, "glob", "")
	sortby := util.DefaultQuery(c, "sort", "none")
	order := util.DefaultQuery(c, "order", "asc")

	files := h.proxy.FilesystemList(name, pattern)

	var sortFunc func(i, j int) bool

	switch sortby {
	case "name":
		if order == "desc" {
			sortFunc = func(i, j int) bool { return files[i].Name > files[j].Name }
		} else {
			sortFunc = func(i, j int) bool { return files[i].Name < files[j].Name }
		}
	case "size":
		if order == "desc" {
			sortFunc = func(i, j int) bool { return files[i].Size > files[j].Size }
		} else {
			sortFunc = func(i, j int) bool { return files[i].Size < files[j].Size }
		}
	default:
		if order == "asc" {
			sortFunc = func(i, j int) bool { return files[i].LastMod < files[j].LastMod }
		} else {
			sortFunc = func(i, j int) bool { return files[i].LastMod > files[j].LastMod }
		}
	}

	sort.Slice(files, sortFunc)

	return c.JSON(http.StatusOK, files)
}
