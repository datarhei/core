package prometheus

import (
	"github.com/datarhei/core/v16/monitor/metric"

	"github.com/prometheus/client_golang/prometheus"
)

type memCollector struct {
	core      string
	collector metric.Reader

	memTotalDesc *prometheus.Desc
	memFreeDesc  *prometheus.Desc
	memLimitDesc *prometheus.Desc
}

func NewMemCollector(core string, c metric.Reader) prometheus.Collector {
	return &memCollector{
		core:      core,
		collector: c,
		memTotalDesc: prometheus.NewDesc(
			"mem_total_bytes",
			"Total available memory in bytes",
			[]string{"core"}, nil),
		memFreeDesc: prometheus.NewDesc(
			"mem_free_bytes",
			"Free memory in bytes",
			[]string{"core"}, nil),
		memLimitDesc: prometheus.NewDesc(
			"mem_limit_bytes",
			"Configured memory limit in bytes",
			[]string{"core"}, nil),
	}
}

func (c *memCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.memLimitDesc
	ch <- c.memFreeDesc
	ch <- c.memLimitDesc
}

func (c *memCollector) Collect(ch chan<- prometheus.Metric) {
	metrics := c.collector.Collect([]metric.Pattern{
		metric.NewPattern("mem_total"),
		metric.NewPattern("mem_free"),
		metric.NewPattern("mem_limit"),
	})

	ch <- prometheus.MustNewConstMetric(c.memTotalDesc, prometheus.GaugeValue, metrics.Value("mem_total").Val(), c.core)
	ch <- prometheus.MustNewConstMetric(c.memFreeDesc, prometheus.GaugeValue, metrics.Value("mem_free").Val(), c.core)
	ch <- prometheus.MustNewConstMetric(c.memLimitDesc, prometheus.GaugeValue, metrics.Value("mem_limit").Val(), c.core)
}
