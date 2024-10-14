package monitor

import (
	"runtime"

	"github.com/datarhei/core/v16/mem"
	"github.com/datarhei/core/v16/monitor/metric"
)

type selfCollector struct {
	allocDescr       *metric.Description
	recycleDescr     *metric.Description
	dumpDescr        *metric.Description
	defaultSizeDescr *metric.Description
	maxSizeDescr     *metric.Description
}

func NewSelfCollector() metric.Collector {
	c := &selfCollector{}

	c.allocDescr = metric.NewDesc("bufferpool_alloc", "Number of buffer allocations", nil)
	c.recycleDescr = metric.NewDesc("bufferpool_recycle", "Number of buffer recycles", nil)
	c.dumpDescr = metric.NewDesc("bufferpool_dump", "Number of buffer dumps", nil)
	c.defaultSizeDescr = metric.NewDesc("bufferpool_default_size", "Default buffer size", nil)
	c.maxSizeDescr = metric.NewDesc("bufferpool_max_size", "Max. buffer size for recycling", nil)

	return c
}

func (c *selfCollector) Stop() {}

func (c *selfCollector) Prefix() string {
	return "bufferpool"
}

func (c *selfCollector) Describe() []*metric.Description {
	return []*metric.Description{
		c.allocDescr,
		c.recycleDescr,
		c.dumpDescr,
		c.defaultSizeDescr,
		c.maxSizeDescr,
	}
}

func (c *selfCollector) Collect() metric.Metrics {
	stats := mem.Stats()

	metrics := metric.NewMetrics()

	metrics.Add(metric.NewValue(c.allocDescr, float64(stats.Alloc)))
	metrics.Add(metric.NewValue(c.recycleDescr, float64(stats.Recycle)))
	metrics.Add(metric.NewValue(c.dumpDescr, float64(stats.Dump)))
	metrics.Add(metric.NewValue(c.defaultSizeDescr, float64(stats.DefaultSize)))
	metrics.Add(metric.NewValue(c.maxSizeDescr, float64(stats.MaxSize)))

	memstats := runtime.MemStats{}
	runtime.ReadMemStats(&memstats)

	//metrics.Add(metric.NewValue())

	return metrics
}
