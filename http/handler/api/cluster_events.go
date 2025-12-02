package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/datarhei/core/v16/encoding/json"
	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/http/handler/util"

	"github.com/labstack/echo/v4"
)

// Events returns a stream of log event
// @Summary Stream of log events
// @Description Stream of log events of whats happening on each node in the cluster
// @ID cluster-3-events
// @Tags v16.?.?
// @Accept json
// @Produce text/event-stream
// @Produce json-stream
// @Param filters body api.LogEventFilters false "Event filters"
// @Success 200 {object} api.LogEvent
// @Security ApiKeyAuth
// @Router /api/v3/cluster/events [post]
func (h *ClusterHandler) Events(c echo.Context) error {
	filters := api.LogEventFilters{}

	if err := util.ShouldBindJSON(c, &filters); err != nil {
		return api.Err(http.StatusBadRequest, "", "invalid JSON: %s", err.Error())
	}

	filter := map[string]*api.LogEventFilter{}

	for _, f := range filters.Filters {
		f := f

		if err := f.Compile(); err != nil {
			return api.Err(http.StatusBadRequest, "", "invalid filter: %s: %s", f.Component, err.Error())
		}

		component := strings.ToLower(f.Component)
		filter[component] = &f
	}

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	req := c.Request()
	reqctx := req.Context()

	contentType := "text/event-stream"
	accept := req.Header.Get(echo.HeaderAccept)
	if strings.Contains(accept, "application/x-json-stream") {
		contentType = "application/x-json-stream"
	}

	res := c.Response()

	res.Header().Set(echo.HeaderContentType, contentType+"; charset=UTF-8")
	res.Header().Set(echo.HeaderCacheControl, "no-store")
	res.Header().Set(echo.HeaderConnection, "close")
	res.WriteHeader(http.StatusOK)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	evts, err := h.proxy.LogEvents(ctx, filters)
	if err != nil {
		return api.Err(http.StatusInternalServerError, "", "%s", err.Error())
	}

	enc := json.NewEncoder(res)
	enc.SetIndent("", "")

	done := make(chan error, 1)

	filterEvent := func(event *api.LogEvent) bool {
		if len(filter) == 0 {
			return true
		}

		f, ok := filter[event.Component]
		if !ok {
			return false
		}

		return event.Filter(f)
	}

	if contentType == "text/event-stream" {
		res.Write([]byte(":keepalive\n\n"))
		res.Flush()

		for {
			select {
			case err := <-done:
				return err
			case <-reqctx.Done():
				done <- nil
			case <-ticker.C:
				res.Write([]byte(":keepalive\n\n"))
				res.Flush()
			case event := <-evts:
				if !filterEvent(&event) {
					continue
				}

				res.Write([]byte("event: " + event.Component + "\ndata: "))
				if err := enc.Encode(event); err != nil {
					done <- err
				}
				res.Write([]byte("\n"))
				res.Flush()
			}
		}
	} else {
		res.Write([]byte("{\"event\": \"keepalive\"}\n"))
		res.Flush()

		for {
			select {
			case err := <-done:
				return err
			case <-reqctx.Done():
				done <- nil
			case <-ticker.C:
				res.Write([]byte("{\"event\": \"keepalive\"}\n"))
				res.Flush()
			case event := <-evts:
				if !filterEvent(&event) {
					continue
				}

				if err := enc.Encode(event); err != nil {
					done <- err
				}
				res.Flush()
			}
		}
	}
}
