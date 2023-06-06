package monitor

import (
	"github.com/datarhei/core/v16/monitor/metric"
	"github.com/datarhei/core/v16/psutil"
)

type memCollector struct {
	totalDescr *metric.Description
	freeDescr  *metric.Description
	limitDescr *metric.Description

	limit float64
}

func NewMemCollector(limit float64) metric.Collector {
	c := &memCollector{
		limit: limit / 100,
	}

	if limit <= 0 || limit > 1 {
		c.limit = 1
	}

	c.totalDescr = metric.NewDesc("mem_total", "Total available memory in bytes", nil)
	c.freeDescr = metric.NewDesc("mem_free", "Free memory in bytes", nil)
	c.limitDescr = metric.NewDesc("mem_limit", "Memory limit in bytes", nil)

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
	}
}

func (c *memCollector) Collect() metric.Metrics {
	metrics := metric.NewMetrics()

	stat, err := psutil.VirtualMemory()
	if err != nil {
		return metrics
	}

	metrics.Add(metric.NewValue(c.totalDescr, float64(stat.Total)))
	metrics.Add(metric.NewValue(c.freeDescr, float64(stat.Available)))
	metrics.Add(metric.NewValue(c.limitDescr, float64(stat.Total)*c.limit))

	return metrics
}

func (c *memCollector) Stop() {}
