package api

import (
	"net"
	"net/http"

	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/http/handler/util"
	"github.com/datarhei/core/v16/rtmp"

	"github.com/labstack/echo/v4"
)

// The RTMPHandler type provides a handler for retrieving details from the RTMPHandler server
type RTMPHandler struct {
	rtmp rtmp.Server
}

// NewRTMP returns a new RTMP type. You have to provide a RTMP server instance.
func NewRTMP(rtmp rtmp.Server) *RTMPHandler {
	return &RTMPHandler{
		rtmp: rtmp,
	}
}

// ListChannels lists all currently publishing RTMP streams
// @Summary List all publishing RTMP streams
// @Description List all currently publishing RTMP streams.
// @Tags v16.7.2
// @ID rtmp-3-list-channels
// @Produce json
// @Success 200 {array} api.RTMPChannel
// @Security ApiKeyAuth
// @Router /api/v3/rtmp [get]
func (rtmph *RTMPHandler) ListChannels(c echo.Context) error {
	channels := rtmph.rtmp.Channels()

	list := []api.RTMPChannel{}

	for _, c := range channels {
		list = append(list, api.RTMPChannel{
			Name: c,
		})
	}

	return c.JSON(http.StatusOK, list)
}

// Play plays a RTMP stream over HTTP
// @Summary Plays a RTMP stream over HTTP
// @Description Plays a RTMP stream over HTTP
// @Tags v16.?.?
// @ID rtmp-3-play
// @Produce video/x-flv
// @Produce json
// @Success 200 {file} byte
// @Success 403 {object} api.Error
// @Success 404 {object} api.Error
// @Success 500 {object} api.Error
// @Router /rtmp/{path} [get]
func (rtmph *RTMPHandler) Play(c echo.Context) error {
	path := util.PathWildcardParam(c)
	addr, err := net.ResolveIPAddr("ip", c.RealIP())
	if err != nil {
		return api.Err(http.StatusBadRequest, "", "%s", err.Error())
	}

	u := c.Request().URL
	u.Path = path

	r, err := rtmph.rtmp.PlayFLV(addr, u)
	if err != nil {
		rtmperr, ok := err.(*rtmp.PlayError)
		if ok {
			status := http.StatusInternalServerError
			switch rtmperr.Message {
			case "FORBIDDEN":
				status = http.StatusForbidden
			case "NOTFOUND":
				status = http.StatusNotFound
			}

			return api.Err(status, "", "%s", err.Error())
		} else {
			return err
		}
	}

	defer r.Close()

	c.Response().Header().Set("Transfer-Encoding", "chunked")
	c.Response().Header().Set("Access-Control-Allow-Origin", "*") // important for browser player

	return c.Stream(200, "video/x-flv", r)
}
