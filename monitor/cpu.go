package monitor

import (
	"github.com/datarhei/core/v16/monitor/metric"
	"github.com/datarhei/core/v16/resources"
)

type cpuCollector struct {
	ncpuDescr     *metric.Description
	systemDescr   *metric.Description
	userDescr     *metric.Description
	idleDescr     *metric.Description
	otherDescr    *metric.Description
	limitDescr    *metric.Description
	throttleDescr *metric.Description

	resources resources.Resources
}

func NewCPUCollector(rsc resources.Resources) metric.Collector {
	c := &cpuCollector{
		resources: rsc,
	}

	c.ncpuDescr = metric.NewDesc("cpu_ncpu", "Number of logical CPUs in the system", nil)
	c.systemDescr = metric.NewDesc("cpu_system", "Percentage of CPU used for the system", nil)
	c.userDescr = metric.NewDesc("cpu_user", "Percentage of CPU used for the user", nil)
	c.idleDescr = metric.NewDesc("cpu_idle", "Percentage of idle CPU", nil)
	c.otherDescr = metric.NewDesc("cpu_other", "Percentage of CPU used for other subsystems", nil)
	c.limitDescr = metric.NewDesc("cpu_limit", "Percentage of CPU to be consumed", nil)
	c.throttleDescr = metric.NewDesc("cpu_throttling", "Whether the CPU is currently throttled", nil)

	return c
}

func (c *cpuCollector) Stop() {}

func (c *cpuCollector) Prefix() string {
	return "cpu"
}

func (c *cpuCollector) Describe() []*metric.Description {
	return []*metric.Description{
		c.ncpuDescr,
		c.systemDescr,
		c.userDescr,
		c.idleDescr,
		c.otherDescr,
		c.limitDescr,
		c.throttleDescr,
	}
}

func (c *cpuCollector) Collect() metric.Metrics {
	metrics := metric.NewMetrics()

	rinfo := c.resources.Info()

	metrics.Add(metric.NewValue(c.ncpuDescr, rinfo.CPU.NCPU))

	metrics.Add(metric.NewValue(c.limitDescr, rinfo.CPU.Limit))

	throttling := .0
	if rinfo.CPU.Throttling {
		throttling = 1
	}

	metrics.Add(metric.NewValue(c.throttleDescr, throttling))

	metrics.Add(metric.NewValue(c.systemDescr, rinfo.CPU.System))
	metrics.Add(metric.NewValue(c.userDescr, rinfo.CPU.User))
	metrics.Add(metric.NewValue(c.idleDescr, rinfo.CPU.Idle))
	metrics.Add(metric.NewValue(c.otherDescr, rinfo.CPU.Other))

	return metrics
}
