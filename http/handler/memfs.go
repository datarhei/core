package handler

import (
	"net/http"
	"path/filepath"

	"github.com/datarhei/core/http/api"
	"github.com/datarhei/core/http/handler/util"
	"github.com/datarhei/core/io/fs"

	"github.com/labstack/echo/v4"
)

// The MemFSHandler type provides handlers for manipulating a filesystem
type MemFSHandler struct {
	filesystem fs.Filesystem
}

// NewMemFS return a new MemFS type. You have to provide a filesystem to act on.
func NewMemFS(fs fs.Filesystem) *MemFSHandler {
	return &MemFSHandler{
		filesystem: fs,
	}
}

// GetFile returns the file at the given path
// @Summary Fetch a file from the memory filesystem
// @Description Fetch a file from the memory filesystem
// @ID memfs-get-file
// @Produce application/data
// @Produce json
// @Param path path string true "Path to file"
// @Success 200 {file} byte
// @Success 301 {string} string
// @Failure 404 {object} api.Error
// @Router /memfs/{path} [get]
func (h *MemFSHandler) GetFile(c echo.Context) error {
	path := util.PathWildcardParam(c)

	mimeType := c.Response().Header().Get(echo.HeaderContentType)
	c.Response().Header().Del(echo.HeaderContentType)

	file := h.filesystem.Open(path)
	if file == nil {
		return api.Err(http.StatusNotFound, "File not found", path)
	}

	defer file.Close()

	stat, _ := file.Stat()

	c.Response().Header().Set("Last-Modified", stat.ModTime().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT"))

	if path, ok := stat.IsLink(); ok {
		path = filepath.Clean("/" + path)

		if path[0] == '/' {
			path = path[1:]
		}

		return c.Redirect(http.StatusMovedPermanently, path)
	}

	c.Response().Header().Set(echo.HeaderContentType, mimeType)

	if c.Request().Method == "HEAD" {
		return c.Blob(http.StatusOK, "application/data", nil)
	}

	return c.Stream(http.StatusOK, "application/data", file)
}

// PutFile adds or overwrites a file at the given path
// @Summary Add a file to the memory filesystem
// @Description Writes or overwrites a file on the memory filesystem
// @ID memfs-put-file
// @Accept application/data
// @Produce text/plain
// @Produce json
// @Param path path string true "Path to file"
// @Param data body []byte true "File data"
// @Success 201 {string} string
// @Success 204 {string} string
// @Failure 507 {object} api.Error
// @Security BasicAuth
// @Router /memfs/{path} [put]
func (h *MemFSHandler) PutFile(c echo.Context) error {
	path := util.PathWildcardParam(c)

	c.Response().Header().Del(echo.HeaderContentType)

	req := c.Request()

	_, created, err := h.filesystem.Store(path, req.Body)
	if err != nil {
		return api.Err(http.StatusBadRequest, "%s", err)
	}

	c.Response().Header().Set("Content-Location", req.URL.RequestURI())

	if created {
		return c.String(http.StatusCreated, "")
	}

	return c.NoContent(http.StatusNoContent)
}

// DeleteFile removes a file from the filesystem
// @Summary Remove a file from the memory filesystem
// @Description Remove a file from the memory filesystem
// @ID memfs-delete-file
// @Produce text/plain
// @Param path path string true "Path to file"
// @Success 200 {string} string
// @Failure 404 {object} api.Error
// @Security BasicAuth
// @Router /memfs/{path} [delete]
func (h *MemFSHandler) DeleteFile(c echo.Context) error {
	path := util.PathWildcardParam(c)

	c.Response().Header().Del(echo.HeaderContentType)

	size := h.filesystem.Delete(path)

	if size < 0 {
		return api.Err(http.StatusNotFound, "File not found", path)
	}

	return c.String(http.StatusOK, "Deleted: "+path)
}
