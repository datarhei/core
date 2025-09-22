package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/textproto"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/datarhei/core/v16/glob"
	"github.com/datarhei/core/v16/http/api"
	httpfs "github.com/datarhei/core/v16/http/fs"
	"github.com/datarhei/core/v16/http/handler/util"
	"github.com/datarhei/core/v16/io/fs"

	"github.com/labstack/echo/v4"
)

// The FSHandler type provides handlers for manipulating a filesystem
type FSHandler struct {
	FS httpfs.FS
}

// NewFS return a new FSHandler type. You have to provide a filesystem to act on.
func NewFS(fs httpfs.FS) *FSHandler {
	return &FSHandler{
		FS: fs,
	}
}

func (h *FSHandler) GetFile(c echo.Context) error {
	path := util.PathWildcardParam(c)

	mimeType := c.Response().Header().Get(echo.HeaderContentType)
	c.Response().Header().Del(echo.HeaderContentType)

	file := h.FS.Filesystem.Open(path)
	if file == nil {
		return api.Err(http.StatusNotFound, "", "file not found: %s", path)
	}

	stat, _ := file.Stat()

	if len(h.FS.DefaultFile) != 0 {
		if stat.IsDir() {
			path = filepath.Join(path, h.FS.DefaultFile)

			file.Close()

			file = h.FS.Filesystem.Open(path)
			if file == nil {
				return api.Err(http.StatusNotFound, "", "file not found: %s", path)
			}

			stat, _ = file.Stat()
		}
	}

	defer file.Close()

	c.Response().Header().Set("Last-Modified", stat.ModTime().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT"))

	if path, ok := stat.IsLink(); ok {
		path = filepath.Clean("/" + path)

		if path[0] == '/' {
			path = path[1:]
		}

		return c.Redirect(http.StatusMovedPermanently, path)
	}

	c.Response().Header().Set(echo.HeaderContentType, mimeType)
	c.Response().Header().Set("Accept-Ranges", "bytes")

	if c.Request().Method == "HEAD" {
		c.Response().Header().Set(echo.HeaderContentLength, strconv.FormatInt(stat.Size(), 10))
		return c.Blob(http.StatusOK, "application/data", nil)
	}

	var streamFile io.Reader = file
	status := http.StatusOK

	ifRange := c.Request().Header.Get("If-Range")
	if len(ifRange) != 0 {
		ifTime, err := time.Parse("Mon, 02 Jan 2006 15:04:05 MST", ifRange)
		if err != nil {
			return api.Err(http.StatusBadRequest, "", "%s", err)
		}

		if ifTime.Unix() != stat.ModTime().Unix() {
			c.Request().Header.Del("Range")
		}
	}

	byteRange := c.Request().Header.Get("Range")
	if len(byteRange) != 0 {
		ranges, err := parseRange(byteRange, stat.Size())
		if err != nil {
			return api.Err(http.StatusRequestedRangeNotSatisfiable, "", "%s", err.Error())
		}

		if len(ranges) > 1 {
			return api.Err(http.StatusNotImplemented, "", "multipart range requests are not supported")
		}

		if len(ranges) == 1 {
			_, err := file.Seek(ranges[0].start, io.SeekStart)
			if err != nil {
				return api.Err(http.StatusRequestedRangeNotSatisfiable, "", "%s", err.Error())
			}

			c.Response().Header().Set("Content-Range", ranges[0].contentRange(stat.Size()))
			streamFile = &io.LimitedReader{
				R: streamFile,
				N: ranges[0].length,
			}

			status = http.StatusPartialContent
		}
	}

	return c.Stream(status, "application/data", streamFile)
}

func (h *FSHandler) PutFile(c echo.Context) error {
	path := util.PathWildcardParam(c)

	c.Response().Header().Del(echo.HeaderContentType)

	req := c.Request()

	_, created, err := h.FS.Filesystem.WriteFileReader(path, req.Body, -1)
	if err != nil {
		return api.Err(http.StatusBadRequest, "", "%s", err.Error())
	}

	if h.FS.Cache != nil {
		h.FS.Cache.Delete(path)

		if len(h.FS.DefaultFile) != 0 {
			if strings.HasSuffix(path, "/"+h.FS.DefaultFile) {
				path := strings.TrimSuffix(path, h.FS.DefaultFile)
				h.FS.Cache.Delete(path)
			}
		}
	}

	c.Response().Header().Set("Content-Location", req.URL.RequestURI())

	if created {
		return c.String(http.StatusCreated, "")
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *FSHandler) DeleteFile(c echo.Context) error {
	path := util.PathWildcardParam(c)

	c.Response().Header().Del(echo.HeaderContentType)

	size := h.FS.Filesystem.Remove(path)

	if h.FS.Cache != nil {
		h.FS.Cache.Delete(path)

		if len(h.FS.DefaultFile) != 0 {
			if strings.HasSuffix(path, "/"+h.FS.DefaultFile) {
				path := strings.TrimSuffix(path, h.FS.DefaultFile)
				h.FS.Cache.Delete(path)
			}
		}
	}

	if size < 0 {
		return api.Err(http.StatusNotFound, "", "file not found: %s", path)
	}

	return c.String(http.StatusOK, "Deleted: "+path)
}

func (h *FSHandler) DeleteFiles(c echo.Context) error {
	pattern := util.DefaultQuery(c, "glob", "")
	sizeMin := util.DefaultQuery(c, "size_min", "0")
	sizeMax := util.DefaultQuery(c, "size_max", "0")
	modifiedStart := util.DefaultQuery(c, "lastmod_start", "")
	modifiedEnd := util.DefaultQuery(c, "lastmod_end", "")

	if len(pattern) == 0 {
		return api.Err(http.StatusBadRequest, "", "a glob pattern is required")
	}

	path := "/"

	if len(pattern) != 0 {
		prefix := glob.Prefix(pattern)
		index := strings.LastIndex(prefix, "/")
		path = prefix[:index+1]
	}

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
			return api.Err(http.StatusBadRequest, "", "lastmod_end: %s", err.Error())
		} else {
			t := time.Unix(x+1, 0)
			options.ModifiedEnd = &t
		}
	}

	paths, _ := h.FS.Filesystem.RemoveList(path, options)

	if h.FS.Cache != nil {
		for _, path := range paths {
			h.FS.Cache.Delete(path)

			if len(h.FS.DefaultFile) != 0 {
				if strings.HasSuffix(path, "/"+h.FS.DefaultFile) {
					path := strings.TrimSuffix(path, h.FS.DefaultFile)
					h.FS.Cache.Delete(path)
				}
			}
		}
	}

	return c.JSON(http.StatusOK, paths)
}

func (h *FSHandler) ListFiles(c echo.Context) error {
	pattern := util.DefaultQuery(c, "glob", "")
	sizeMin := util.DefaultQuery(c, "size_min", "0")
	sizeMax := util.DefaultQuery(c, "size_max", "0")
	modifiedStart := util.DefaultQuery(c, "lastmod_start", "")
	modifiedEnd := util.DefaultQuery(c, "lastmod_end", "")
	sortby := util.DefaultQuery(c, "sort", "none")
	order := util.DefaultQuery(c, "order", "asc")

	accept := c.Request().Header.Get(echo.HeaderAccept)
	if strings.Contains(accept, "application/x-json-stream") || strings.Contains(accept, "text/event-stream") {
		return h.ListFilesEvent(c)
	}

	path := "/"

	if len(pattern) != 0 {
		prefix := glob.Prefix(pattern)
		index := strings.LastIndex(prefix, "/")
		path = prefix[:index+1]
	}

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
			return api.Err(http.StatusBadRequest, "", "lastmod_end: %s", err.Error())
		} else {
			t := time.Unix(x+1, 0)
			options.ModifiedEnd = &t
		}
	}

	files := h.FS.Filesystem.List(path, options)

	var sortFunc func(i, j int) bool

	switch sortby {
	case "name":
		if order == "desc" {
			sortFunc = func(i, j int) bool { return files[i].Name() > files[j].Name() }
		} else {
			sortFunc = func(i, j int) bool { return files[i].Name() < files[j].Name() }
		}
	case "size":
		if order == "desc" {
			sortFunc = func(i, j int) bool { return files[i].Size() > files[j].Size() }
		} else {
			sortFunc = func(i, j int) bool { return files[i].Size() < files[j].Size() }
		}
	default:
		if order == "asc" {
			sortFunc = func(i, j int) bool { return files[i].ModTime().Before(files[j].ModTime()) }
		} else {
			sortFunc = func(i, j int) bool { return files[i].ModTime().After(files[j].ModTime()) }
		}
	}

	sort.Slice(files, sortFunc)

	var fileinfos []api.FileInfo = make([]api.FileInfo, len(files))

	for i, f := range files {
		fileinfos[i] = api.FileInfo{
			Name:    f.Name(),
			Size:    f.Size(),
			LastMod: f.ModTime().Unix(),
		}
	}

	return c.JSON(http.StatusOK, fileinfos)
}

func (h *FSHandler) ListFilesEvent(c echo.Context) error {
	keepaliveTicker := time.NewTicker(5 * time.Second)
	defer keepaliveTicker.Stop()

	listTicker := time.NewTicker(30 * time.Second)
	defer listTicker.Stop()

	req := c.Request()
	reqctx := req.Context()

	contentType := "text/event-stream"
	accept := req.Header.Get(echo.HeaderAccept)
	if strings.Contains(accept, "application/x-json-stream") {
		contentType = "application/x-json-stream"
	}

	evts, cancel, err := h.FS.Filesystem.Events()
	if err != nil {
		return api.Err(http.StatusNotImplemented, "", "events are not implemented for this filesystem")
	}
	defer cancel()

	res := c.Response()

	res.Header().Set(echo.HeaderContentType, contentType+"; charset=UTF-8")
	res.Header().Set(echo.HeaderCacheControl, "no-store")
	res.Header().Set(echo.HeaderConnection, "close")
	res.WriteHeader(http.StatusOK)

	enc := json.NewEncoder(res)
	enc.SetIndent("", "")

	done := make(chan error, 1)

	createList := func() api.FilesystemEvent {
		files := h.FS.Filesystem.List("/", fs.ListOptions{})
		event := api.FilesystemEvent{
			Action: "list",
			Names:  make([]string, 0, len(files)),
		}
		for _, file := range files {
			event.Names = append(event.Names, file.Name())
		}

		return event
	}

	if err := enc.Encode(createList()); err != nil {
		done <- err
	}
	res.Flush()

	for {
		select {
		case err := <-done:
			return err
		case <-reqctx.Done():
			done <- nil
		case <-keepaliveTicker.C:
			res.Write([]byte("{\"action\": \"keepalive\"}\n"))
			res.Flush()
		case <-listTicker.C:
			if err := enc.Encode(createList()); err != nil {
				done <- err
			}
			res.Flush()
		case e := <-evts:
			if err := enc.Encode(api.FilesystemEvent{
				Action: e.Action,
				Name:   e.Name,
			}); err != nil {
				done <- err
			}
			res.Flush()
		}
	}
}

// From: github.com/golang/go/net/http/fs.go@7dc9fcb

// errNoOverlap is returned by serveContent's parseRange if first-byte-pos of
// all of the byte-range-spec values is greater than the content size.
var errNoOverlap = errors.New("invalid range: failed to overlap")

// httpRange specifies the byte range to be sent to the client.
type httpRange struct {
	start, length int64
}

func (r httpRange) contentRange(size int64) string {
	return fmt.Sprintf("bytes %d-%d/%d", r.start, r.start+r.length-1, size)
}

/*
func (r httpRange) mimeHeader(contentType string, size int64) textproto.MIMEHeader {
	return textproto.MIMEHeader{
		"Content-Range": {r.contentRange(size)},
		"Content-Type":  {contentType},
	}
}
*/

// parseRange parses a Range header string as per RFC 7233.
// errNoOverlap is returned if none of the ranges overlap.
func parseRange(s string, size int64) ([]httpRange, error) {
	if s == "" {
		return nil, nil // header not present
	}
	const b = "bytes="
	if !strings.HasPrefix(s, b) {
		return nil, errors.New("invalid range")
	}
	var ranges []httpRange
	noOverlap := false
	for _, ra := range strings.Split(s[len(b):], ",") {
		ra = textproto.TrimString(ra)
		if ra == "" {
			continue
		}
		start, end, ok := strings.Cut(ra, "-")
		if !ok {
			return nil, errors.New("invalid range")
		}
		start, end = textproto.TrimString(start), textproto.TrimString(end)
		var r httpRange
		if start == "" {
			// If no start is specified, end specifies the
			// range start relative to the end of the file,
			// and we are dealing with <suffix-length>
			// which has to be a non-negative integer as per
			// RFC 7233 Section 2.1 "Byte-Ranges".
			if end == "" || end[0] == '-' {
				return nil, errors.New("invalid range")
			}
			i, err := strconv.ParseInt(end, 10, 64)
			if i < 0 || err != nil {
				return nil, errors.New("invalid range")
			}
			if i > size {
				i = size
			}
			r.start = size - i
			r.length = size - r.start
		} else {
			i, err := strconv.ParseInt(start, 10, 64)
			if err != nil || i < 0 {
				return nil, errors.New("invalid range")
			}
			if i >= size {
				// If the range begins after the size of the content,
				// then it does not overlap.
				noOverlap = true
				continue
			}
			r.start = i
			if end == "" {
				// If no end is specified, range extends to end of the file.
				r.length = size - r.start
			} else {
				i, err := strconv.ParseInt(end, 10, 64)
				if err != nil || r.start > i {
					return nil, errors.New("invalid range")
				}
				if i >= size {
					i = size - 1
				}
				r.length = i - r.start + 1
			}
		}
		ranges = append(ranges, r)
	}
	if noOverlap && len(ranges) == 0 {
		// The specified ranges did not overlap with the content.
		return nil, errNoOverlap
	}
	return ranges, nil
}
