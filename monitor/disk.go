package monitor

import (
	"github.com/datarhei/core/v16/monitor/metric"
	"github.com/datarhei/core/v16/resources"
)

type diskCollector struct {
	path      string
	resources resources.Resources

	totalDescr *metric.Description
	usageDescr *metric.Description
}

func NewDiskCollector(path string, rsc resources.Resources) metric.Collector {
	c := &diskCollector{
		path:      path,
		resources: rsc,
	}

	c.totalDescr = metric.NewDesc("disk_total", "Total size of the disk in bytes", []string{"path"})
	c.usageDescr = metric.NewDesc("disk_usage", "Number of used bytes on the disk", []string{"path"})

	return c
}

func (c *diskCollector) Prefix() string {
	return "disk"
}

func (c *diskCollector) Describe() []*metric.Description {
	return []*metric.Description{
		c.totalDescr,
		c.usageDescr,
	}
}

func (c *diskCollector) Collect() metric.Metrics {
	metrics := metric.NewMetrics()

	stat, err := c.resources.Disk(c.path)
	if err != nil {
		return metrics
	}

	metrics.Add(metric.NewValue(c.totalDescr, float64(stat.Total), c.path))
	metrics.Add(metric.NewValue(c.usageDescr, float64(stat.Used), c.path))

	return metrics
}

func (c *diskCollector) Stop() {}
