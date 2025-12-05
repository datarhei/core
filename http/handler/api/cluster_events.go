package api

import (
	"fmt"
	"net/http"
	goslices "slices"
	"strings"
	"time"

	"github.com/datarhei/core/v16/encoding/json"
	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/http/handler/util"

	"github.com/labstack/echo/v4"
)

// LogEvents returns a stream of log event
// @Summary Stream of log events
// @Description Stream of log events of whats happening on each node in the cluster
// @ID cluster-3-events-log
// @Tags v16.?.?
// @Accept json
// @Produce text/event-stream
// @Produce json-stream
// @Param filters body api.LogEventFilters false "Event filters"
// @Success 200 {object} api.LogEvent
// @Security ApiKeyAuth
// @Router /api/v3/cluster/events [post]
func (h *ClusterHandler) LogEvents(c echo.Context) error {
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

	evts, cancel, err := h.proxy.LogEvents()
	if err != nil {
		return api.Err(http.StatusInternalServerError, "", "%s", err.Error())
	}
	defer cancel()

	enc := json.NewEncoder(res)
	enc.SetIndent("", "")

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

	event := api.LogEvent{}

	if contentType == "text/event-stream" {
		res.Write([]byte(":keepalive\n\n"))
		res.Flush()

		for {
			select {
			case <-reqctx.Done():
				return nil
			case <-ticker.C:
				res.Write([]byte(":keepalive\n\n"))
				res.Flush()
			case e, ok := <-evts:
				if !ok {
					return fmt.Errorf("channel closed")
				}

				event.Unmarshal(e)

				if !filterEvent(&event) {
					continue
				}

				res.Write([]byte("event: " + event.Component + "\ndata: "))
				if err := enc.Encode(event); err != nil {
					return err
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
			case <-reqctx.Done():
				return nil
			case <-ticker.C:
				res.Write([]byte("{\"event\": \"keepalive\"}\n"))
				res.Flush()
			case e, ok := <-evts:
				if !ok {
					return fmt.Errorf("channel closed")
				}

				event.Unmarshal(e)

				if !filterEvent(&event) {
					continue
				}

				if err := enc.Encode(event); err != nil {
					return err
				}
				res.Flush()
			}
		}
	}
}

// ProcessEvents returns a stream of process event
// @Summary Stream of process events
// @Description Stream of process events of whats happening on each node in the cluster
// @ID cluster-3-events-process
// @Tags v16.?.?
// @Accept json
// @Produce json-stream
// @Param filters body api.ProcessEventFilters false "Event filters"
// @Success 200 {object} api.ProcessEvent
// @Security ApiKeyAuth
// @Router /api/v3/cluster/events/process [post]
func (h *ClusterHandler) ProcessEvents(c echo.Context) error {
	filters := api.ProcessEventFilters{}

	if err := util.ShouldBindJSON(c, &filters); err != nil {
		return api.Err(http.StatusBadRequest, "", "invalid JSON: %s", err.Error())
	}

	filter := []*api.ProcessEventFilter{}

	for _, f := range filters.Filters {
		f := f

		if err := f.Compile(); err != nil {
			return api.Err(http.StatusBadRequest, "", "invalid filter: %s", err.Error())
		}

		filter = append(filter, &f)
	}

	keepaliveTicker := time.NewTicker(5 * time.Second)
	defer keepaliveTicker.Stop()

	req := c.Request()
	reqctx := req.Context()

	contentType := "application/x-json-stream"

	evts, cancel, err := h.proxy.ProcessEvents()
	if err != nil {
		return api.Err(http.StatusNotImplemented, "", "events are not implemented for this server")
	}
	defer cancel()

	res := c.Response()

	res.Header().Set(echo.HeaderContentType, contentType+"; charset=UTF-8")
	res.Header().Set(echo.HeaderCacheControl, "no-store")
	res.Header().Set(echo.HeaderConnection, "close")
	res.WriteHeader(http.StatusOK)

	enc := json.NewEncoder(res)
	enc.SetIndent("", "")

	filterEvent := func(event *api.ProcessEvent) bool {
		if len(filter) == 0 {
			return true
		}

		return goslices.ContainsFunc(filter, event.Filter)
	}

	event := api.ProcessEvent{}

	for {
		select {
		case <-reqctx.Done():
			return nil
		case <-keepaliveTicker.C:
			_, err := res.Write([]byte("{\"type\":\"keepalive\"}\n"))
			if err != nil {
				return err
			}
			res.Flush()
		case e, ok := <-evts:
			if !ok {
				return fmt.Errorf("channel closed")
			}

			if !event.Unmarshal(e) {
				continue
			}

			if !filterEvent(&event) {
				continue
			}

			if err := enc.Encode(event); err != nil {
				return err
			}
			res.Flush()
		}
	}
}
