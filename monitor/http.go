package monitor

import (
	"strconv"

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

	c.statusDescr = metric.NewDesc("http_status", "Total return status", []string{"name", "code"})

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

	for code, count := range status {
		metrics.Add(metric.NewValue(c.statusDescr, float64(count), c.name, strconv.Itoa(code)))
	}

	return metrics
}

func (c *httpCollector) Stop() {}
