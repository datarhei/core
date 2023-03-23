package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/log"

	"github.com/labstack/echo/v4"
)

// The EventsHandler type provides handler functions for retrieving details
// about the API version and build infos.
type EventsHandler struct {
	events log.ChannelWriter
}

// NewEvents returns a new About type
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
// @Accept text/event-stream
// @Accept json-stream
// @Produce text/event-stream
// @Produce json-stream
// @Param filters body []api.EventFilter false "Event filters"
// @Success 200 {object} api.Event
// @Security ApiKeyAuth
// @Router /api/v3/events [post]
func (h *EventsHandler) Events(c echo.Context) error {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	req := c.Request()

	contentType := "text/event-stream"
	accept := req.Header.Get(echo.HeaderAccept)
	if strings.Contains(accept, "application/x-json-stream") {
		contentType = "application/x-json-stream"
	}

	res := c.Response()

	res.Header().Set(echo.HeaderContentType, contentType+"; charset=UTF-8")
	res.Header().Set(echo.HeaderCacheControl, "no-store")
	res.WriteHeader(http.StatusOK)

	evts, cancel := h.events.Subscribe()
	defer cancel()

	enc := json.NewEncoder(res)
	enc.SetIndent("", "")

	done := make(chan struct{})

	event := api.Event{}

	if contentType == "text/event-stream" {
		res.Write([]byte(":keepalive\n\n"))
		res.Flush()

		for {
			select {
			case <-done:
				return nil
			case <-ticker.C:
				res.Write([]byte(":keepalive\n\n"))
				res.Flush()
			case e := <-evts:
				event.Marshal(&e)
				res.Write([]byte("event: " + strings.ToLower(event.Component) + "\ndata: "))
				if err := enc.Encode(event); err != nil {
					close(done)
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
			case <-done:
				return nil
			case <-ticker.C:
				res.Write([]byte("{\"event\": \"keepalive\"}\n"))
				res.Flush()
			case e := <-evts:
				event.Marshal(&e)
				if err := enc.Encode(event); err != nil {
					close(done)
				}
				res.Flush()
			}
		}
	}
}
