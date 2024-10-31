package session

import (
	"net/url"

	"github.com/datarhei/core/v16/mem"
	"github.com/labstack/echo/v4"
	"github.com/lithammer/shortuuid/v4"
)

func (h *handler) handleHTTP(c echo.Context, _ string, data map[string]interface{}, next echo.HandlerFunc) error {
	req := c.Request()
	res := c.Response()

	if !h.httpCollector.IsCollectableIP(c.RealIP()) {
		return next(c)
	}

	path := req.URL.Path

	location := path
	if len(req.URL.RawQuery) != 0 {
		location += "?" + req.URL.RawQuery
	}

	referrer := req.Header.Get("Referer")
	if u, err := url.Parse(referrer); err == nil {
		referrer = u.Host
	}

	id := shortuuid.New()

	reader := req.Body
	r := &bodysizeReader{
		reader: req.Body,
	}
	req.Body = r

	writer := res.Writer
	w := &bodysizeWriter{
		ResponseWriter: res.Writer,
	}
	res.Writer = w

	h.httpCollector.RegisterAndActivate(id, "", location, referrer)
	h.httpCollector.Extra(id, data)

	defer func() {
		buffer := mem.Get()

		req.Body = reader
		h.httpCollector.Ingress(id, r.size+headerSize(req.Header, buffer))

		res.Writer = writer

		h.httpCollector.Egress(id, w.size+headerSize(res.Header(), buffer))
		data["code"] = res.Status
		h.httpCollector.Extra(id, data)

		h.httpCollector.Close(id)

		mem.Put(buffer)
	}()

	return next(c)
}
