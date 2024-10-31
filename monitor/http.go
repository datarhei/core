package monitor

import (
	"strings"

	"github.com/datarhei/core/v16/http/server"
	"github.com/datarhei/core/v16/monitor/metric"
)

type httpCollector struct {
	handler     server.Server
	name        string
	statusDescr *metric.Description
}

func NewHTTPCollector(name string, handler server.Server) metric.Collector {
	c := &httpCollector{
		handler: handler,
		name:    name,
	}

	c.statusDescr = metric.NewDesc("http_status", "Total return status count", []string{"name", "code", "method", "path"})

	return c
}

func (c *httpCollector) Prefix() string {
	return "filesystem"
}

func (c *httpCollector) Describe() []*metric.Description {
	return []*metric.Description{
		c.statusDescr,
	}
}

func (c *httpCollector) Collect() metric.Metrics {
	status := c.handler.HTTPStatus()

	metrics := metric.NewMetrics()

	for key, count := range status {
		vals := strings.SplitN(key, ":", 3)
		metrics.Add(metric.NewValue(c.statusDescr, float64(count), c.name, vals[0], vals[1], vals[2]))
	}

	return metrics
}

func (c *httpCollector) Stop() {}
