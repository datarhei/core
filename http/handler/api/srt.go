package api

import (
	"net/http"

	"github.com/datarhei/core/v16/srt"

	"github.com/labstack/echo/v4"
)

// The SRTHandler type provides a handler for retrieving details from the SRTHandler server
type SRTHandler struct {
	srt srt.Server
}

// NewRTMP returns a new SRT type. You have to provide a SRT server instance.
func NewSRT(srt srt.Server) *SRTHandler {
	return &SRTHandler{
		srt: srt,
	}
}

// ListChannels lists all currently publishing SRT streams
// @Summary List all publishing SRT treams
// @Description List all currently publishing SRT streams
// @ID srt-3-list-channels
// @Produce json
// @Success 200 {array} api.SRTChannel
// @Security ApiKeyAuth
// @Router /api/v3/srt [get]
func (srth *SRTHandler) ListChannels(c echo.Context) error {
	channels := srth.srt.Channels()

	return c.JSON(http.StatusOK, channels)
}
