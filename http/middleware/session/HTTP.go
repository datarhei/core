package session

import (
	"github.com/labstack/echo/v4"
	"github.com/lithammer/shortuuid/v4"
)

func (h *handler) handleHTTP(c echo.Context, ctxuser string, data map[string]interface{}, next echo.HandlerFunc) error {
	req := c.Request()
	res := c.Response()

	if !h.httpCollector.IsCollectableIP(c.RealIP()) {
		return next(c)
	}

	path := req.URL.Path

	location := path + req.URL.RawQuery
	remote := req.RemoteAddr

	id := shortuuid.New()

	reader := req.Body
	r := &fakeReader{
		reader: req.Body,
	}
	req.Body = r

	writer := res.Writer
	w := &fakeWriter{
		ResponseWriter: res.Writer,
	}
	res.Writer = w

	h.httpCollector.RegisterAndActivate(id, "", location, remote)
	h.httpCollector.Extra(id, data)

	defer h.httpCollector.Close(id)

	defer func() {
		req.Body = reader
		h.httpCollector.Ingress(id, r.size+headerSize(req.Header))
	}()

	defer func() {
		res.Writer = writer

		h.httpCollector.Egress(id, w.size+headerSize(res.Header()))
		data["code"] = res.Status
		h.httpCollector.Extra(id, data)
	}()

	return next(c)
}
