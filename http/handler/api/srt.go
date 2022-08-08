package api

import (
	"net/http"

	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/srt"

	"github.com/labstack/echo/v4"
)

// The SRTHandler type provides a handler for retrieving details from the SRTHandler server
type SRTHandler struct {
	srt srt.Server
}

// NewSRT returns a new SRT type. You have to provide a SRT server instance.
func NewSRT(srt srt.Server) *SRTHandler {
	return &SRTHandler{
		srt: srt,
	}
}

// ListChannels lists all currently publishing SRT streams
// @Summary List all publishing SRT treams
// @Description List all currently publishing SRT streams. This endpoint is EXPERIMENTAL and may change in future.
// @ID srt-3-list-channels
// @Produce json
// @Success 200 {array} []api.SRTChannel
// @Security ApiKeyAuth
// @Router /api/v3/srt [get]
func (srth *SRTHandler) ListChannels(c echo.Context) error {
	channels := srth.srt.Channels()

	srtchannels := []api.SRTChannel{}

	for _, channel := range channels {
		srtchannels = append(srtchannels, srth.unmarshalChannel(channel))
	}

	return c.JSON(http.StatusOK, srtchannels)
}

// Unmarshal converts the SRT channels into API representation
func (srth *SRTHandler) unmarshalChannel(ss srt.Channel) api.SRTChannel {
	s := api.SRTChannel{
		Name:        ss.Name,
		SocketId:    ss.SocketId,
		Connections: map[uint32]api.SRTConnection{},
		Log:         make(map[string][]api.SRTLog),
	}

	s.Subscriber = make([]uint32, len(ss.Subscriber))
	copy(s.Subscriber, ss.Subscriber)

	for k, v := range ss.Connections {
		c := s.Connections[k]
		c.Log = make(map[string][]api.SRTLog)
		c.Stats.Unmarshal(&v.Stats)

		for lk, lv := range ss.Log {
			s.Log[lk] = make([]api.SRTLog, len(lv))
			for i, l := range lv {
				s.Log[lk][i].Timestamp = l.Timestamp.UnixMilli()
				s.Log[lk][i].Message = l.Message
			}
		}

		s.Connections[k] = c
	}

	for k, v := range ss.Log {
		s.Log[k] = make([]api.SRTLog, len(v))
		for i, l := range v {
			s.Log[k][i].Timestamp = l.Timestamp.UnixMilli()
			s.Log[k][i].Message = l.Message
		}
	}

	return s
}
