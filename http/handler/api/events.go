package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/datarhei/core/v16/encoding/json"
	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/http/handler/util"
	"github.com/datarhei/core/v16/log"

	"github.com/labstack/echo/v4"
)

// The EventsHandler type provides handler functions for retrieving event.
type EventsHandler struct {
	events log.ChannelWriter
}

// NewEvents returns a new EventsHandler type
func NewEvents(events log.ChannelWriter) *EventsHandler {
	return &EventsHandler{
		events: events,
	}
}

// Events returns a stream of event
// @Summary Stream of events
// @Description Stream of event of whats happening in the core
// @ID events
// @Tags v16.?.?
// @Accept json
// @Produce text/event-stream
// @Produce json-stream
// @Param filters body api.EventFilters false "Event filters"
// @Success 200 {object} api.Event
// @Security ApiKeyAuth
// @Router /api/v3/events [post]
func (h *EventsHandler) Events(c echo.Context) error {
	filters := api.EventFilters{}

	if err := util.ShouldBindJSON(c, &filters); err != nil {
		return api.Err(http.StatusBadRequest, "", "invalid JSON: %s", err.Error())
	}

	filter := map[string]*api.EventFilter{}

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

	evts, cancel := h.events.Subscribe()
	defer cancel()

	enc := json.NewEncoder(res)
	enc.SetIndent("", "")

	done := make(chan error, 1)

	filterEvent := func(event *api.Event) bool {
		if len(filter) == 0 {
			return true
		}

		f, ok := filter[event.Component]
		if !ok {
			return false
		}

		return event.Filter(f)
	}

	event := api.Event{}

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
