package api

import (
	"net/http"
	goslices "slices"
	"strings"
	"sync"
	"time"

	"github.com/datarhei/core/v16/encoding/json"
	"github.com/datarhei/core/v16/event"
	"github.com/datarhei/core/v16/glob"
	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/http/handler/util"
	"github.com/datarhei/core/v16/log"
	"github.com/datarhei/core/v16/slices"

	"github.com/labstack/echo/v4"
)

// The EventsHandler type provides handler functions for retrieving event.
type EventsHandler struct {
	logs    log.ChannelWriter
	media   map[string]event.MediaSource
	process event.EventSource
	lock    sync.Mutex
}

// NewEvents returns a new EventsHandler type
func NewEvents(logs log.ChannelWriter) *EventsHandler {
	return &EventsHandler{
		logs:  logs,
		media: map[string]event.MediaSource{},
	}
}

func (h *EventsHandler) AddMediaSource(name string, source event.MediaSource) {
	if source == nil {
		return
	}

	h.lock.Lock()
	defer h.lock.Unlock()

	h.media[name] = source
}

func (h *EventsHandler) SetProcessSource(source event.EventSource) {
	if source == nil {
		return
	}

	h.lock.Lock()
	defer h.lock.Unlock()

	h.process = source
}

// LogEvents returns a stream of event
// @Summary Stream of log events
// @Description Stream of log event of whats happening in the core
// @ID events-3-log
// @Tags v16.?.?
// @Accept json
// @Produce text/event-stream
// @Produce json-stream
// @Param filters body api.LogEventFilters false "Event filters"
// @Success 200 {object} api.LogEvent
// @Security ApiKeyAuth
// @Router /api/v3/events [post]
func (h *EventsHandler) LogEvents(c echo.Context) error {
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

	evts, cancel := h.logs.Subscribe()
	defer cancel()

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

	event := api.LogEvent{}

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
			case e := <-evts:
				event.Unmarshal(&e)

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
			case e := <-evts:
				event.Unmarshal(&e)

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

// MediaEvents returns a stream of media event
// @Summary Stream of media events
// @Description Stream of media event of whats happening in the filesystems
// @ID events-3-media
// @Tags v16.?.?
// @Accept json
// @Param glob query string false "glob pattern for media names"
// @Produce json-stream
// @Success 200 {object} api.MediaEvent
// @Security ApiKeyAuth
// @Router /api/v3/events/media/{type} [post]
func (h *EventsHandler) MediaEvents(c echo.Context) error {
	pattern := util.DefaultQuery(c, "glob", "")

	var compiledPattern glob.Glob = nil

	if len(pattern) != 0 {
		var err error
		compiledPattern, err = glob.Compile(pattern, '/')
		if err != nil {
			return api.Err(http.StatusBadRequest, "", "invalid pattern: %w", err)
		}
	}

	mediaType := util.PathParam(c, "type")

	keepaliveTicker := time.NewTicker(5 * time.Second)
	defer keepaliveTicker.Stop()

	listTicker := time.NewTicker(30 * time.Second)
	defer listTicker.Stop()

	req := c.Request()
	reqctx := req.Context()

	contentType := "application/x-json-stream"

	h.lock.Lock()
	mediaSource, ok := h.media[mediaType]
	h.lock.Unlock()

	if !ok {
		return api.Err(http.StatusNotFound, "", "media source not found")
	}

	evts, cancel, err := mediaSource.Events()
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

	done := make(chan error, 1)

	createList := func() api.MediaEvent {
		list := mediaSource.MediaList()

		if compiledPattern != nil {
			names := []string{}

			for _, l := range list {
				if !compiledPattern.Match(l) {
					continue
				}

				names = append(names, l)
			}

			list = names
		}

		event := api.MediaEvent{
			Action:    "list",
			Names:     slices.Copy(list),
			Timestamp: time.Now().UnixMilli(),
		}

		return event
	}

	if err := enc.Encode(createList()); err != nil {
		done <- err
	}
	res.Flush()

	event := api.MediaEvent{}

	for {
		select {
		case err := <-done:
			return err
		case <-reqctx.Done():
			done <- nil
		case <-keepaliveTicker.C:
			res.Write([]byte("{\"action\":\"keepalive\"}\n"))
			res.Flush()
		case <-listTicker.C:
			if err := enc.Encode(createList()); err != nil {
				done <- err
			}
			res.Flush()
		case e := <-evts:
			if !event.Unmarshal(e) {
				continue
			}

			if compiledPattern != nil {
				if !compiledPattern.Match(event.Name) {
					continue
				}
			}

			if err := enc.Encode(event); err != nil {
				done <- err
			}
			res.Flush()
		}
	}
}

// ProcessEvents returns a stream of process event
// @Summary Stream of process events
// @Description Stream of process event of whats happening in the processes
// @ID events-3-process
// @Tags v16.?.?
// @Accept json
// @Produce json-stream
// @Param filters body api.ProcessEventFilters false "Event filters"
// @Success 200 {object} api.ProcessEvent
// @Security ApiKeyAuth
// @Router /api/v3/events/process [post]
func (h *EventsHandler) ProcessEvents(c echo.Context) error {
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

	evts, cancel, err := h.process.Events()
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

	done := make(chan error, 1)

	filterEvent := func(event *api.ProcessEvent) bool {
		if len(filter) == 0 {
			return true
		}

		return goslices.ContainsFunc(filter, event.Filter)
	}

	event := api.ProcessEvent{}

	for {
		select {
		case err := <-done:
			return err
		case <-reqctx.Done():
			done <- nil
		case <-keepaliveTicker.C:
			res.Write([]byte("{\"type\":\"keepalive\"}\n"))
			res.Flush()
		case e := <-evts:
			if !event.Unmarshal(e) {
				continue
			}

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
