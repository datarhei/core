package monitor

import (
	"github.com/datarhei/core/v16/monitor/metric"
	"github.com/datarhei/core/v16/psutil"
	"github.com/datarhei/core/v16/resources"
)

type memCollector struct {
	totalDescr    *metric.Description
	freeDescr     *metric.Description
	limitDescr    *metric.Description
	throttleDescr *metric.Description

	resources resources.Resources
}

func NewMemCollector(rsc resources.Resources) metric.Collector {
	c := &memCollector{
		resources: rsc,
	}

	c.totalDescr = metric.NewDesc("mem_total", "Total available memory in bytes", nil)
	c.freeDescr = metric.NewDesc("mem_free", "Free memory in bytes", nil)
	c.limitDescr = metric.NewDesc("mem_limit", "Memory limit in bytes", nil)
	c.throttleDescr = metric.NewDesc("mem_throttling", "Whether the memory is currently throttled", nil)

	return c
}

func (c *memCollector) Prefix() string {
	return "mem"
}

func (c *memCollector) Describe() []*metric.Description {
	return []*metric.Description{
		c.totalDescr,
		c.freeDescr,
		c.limitDescr,
		c.throttleDescr,
	}
}

func (c *memCollector) Collect() metric.Metrics {
	metrics := metric.NewMetrics()

	_, limit := c.resources.Limits()

	metrics.Add(metric.NewValue(c.limitDescr, float64(limit)))

	_, memory := c.resources.ShouldLimit()
	throttling := .0
	if memory {
		throttling = 1
	}

	metrics.Add(metric.NewValue(c.throttleDescr, throttling))

	stat, err := psutil.VirtualMemory()
	if err != nil {
		return metrics
	}

	metrics.Add(metric.NewValue(c.totalDescr, float64(stat.Total)))
	metrics.Add(metric.NewValue(c.freeDescr, float64(stat.Available)))

	return metrics
}

func (c *memCollector) Stop() {}
