package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/http/handler/util"
	"github.com/datarhei/core/v16/io/fs"

	"github.com/labstack/echo/v4"
)

// ListFiles lists all files on a filesystem
// @Summary List all files on a filesystem
// @Description List all files on a filesystem. The listing can be ordered by name, size, or date of last modification in ascending or descending order.
// @Tags v16.?.?
// @ID cluster-3-list-files
// @Produce json
// @Param storage path string true "Name of the filesystem"
// @Param glob query string false "glob pattern for file names"
// @Param size_min query int64 false "minimal size of files"
// @Param size_max query int64 false "maximal size of files"
// @Param lastmod_start query int64 false "minimal last modification time"
// @Param lastmod_end query int64 false "maximal last modification time"
// @Param sort query string false "none, name, size, lastmod"
// @Param order query string false "asc, desc"
// @Success 200 {array} api.FileInfo
// @Success 500 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/fs/{storage} [get]
func (h *ClusterHandler) ListFiles(c echo.Context) error {
	//name := util.PathParam(c, "storage")
	pattern := util.DefaultQuery(c, "glob", "")
	sizeMin := util.DefaultQuery(c, "size_min", "0")
	sizeMax := util.DefaultQuery(c, "size_max", "0")
	modifiedStart := util.DefaultQuery(c, "lastmod_start", "")
	modifiedEnd := util.DefaultQuery(c, "lastmod_end", "")
	//sortby := util.DefaultQuery(c, "sort", "none")
	//order := util.DefaultQuery(c, "order", "asc")

	options := fs.ListOptions{
		Pattern: pattern,
	}

	if x, err := strconv.ParseInt(sizeMin, 10, 64); err != nil {
		return api.Err(http.StatusBadRequest, "", "size_min: %s", err.Error())
	} else {
		options.SizeMin = x
	}

	if x, err := strconv.ParseInt(sizeMax, 10, 64); err != nil {
		return api.Err(http.StatusBadRequest, "", "size_max: %s", err.Error())
	} else {
		options.SizeMax = x
	}

	if len(modifiedStart) != 0 {
		if x, err := strconv.ParseInt(modifiedStart, 10, 64); err != nil {
			return api.Err(http.StatusBadRequest, "", "lastmod_start: %s", err.Error())
		} else {
			t := time.Unix(x, 0)
			options.ModifiedStart = &t
		}
	}

	if len(modifiedEnd) != 0 {
		if x, err := strconv.ParseInt(modifiedEnd, 10, 64); err != nil {
			return api.Err(http.StatusBadRequest, "", "lastmode_end: %s", err.Error())
		} else {
			t := time.Unix(x+1, 0)
			options.ModifiedEnd = &t
		}
	}

	return api.Err(http.StatusNotImplemented, "", "not implemented")
}
