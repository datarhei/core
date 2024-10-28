package monitor

import (
	"github.com/datarhei/core/v16/monitor/metric"
	"github.com/datarhei/core/v16/resources"
)

type netCollector struct {
	rxDescr *metric.Description
	txDescr *metric.Description

	resources resources.Resources
}

func NewNetCollector(rsc resources.Resources) metric.Collector {
	c := &netCollector{
		resources: rsc,
	}

	c.rxDescr = metric.NewDesc("net_rx", "Number of received bytes", []string{"interface"})
	c.txDescr = metric.NewDesc("net_tx", "Number of transmitted bytes", []string{"interface"})

	return c
}

func (c *netCollector) Prefix() string {
	return "net"
}

func (c *netCollector) Describe() []*metric.Description {
	return []*metric.Description{
		c.rxDescr,
		c.txDescr,
	}
}

func (c *netCollector) Collect() metric.Metrics {
	metrics := metric.NewMetrics()

	devs, err := c.resources.Network()
	if err != nil {
		return metrics
	}

	for _, dev := range devs {
		metrics.Add(metric.NewValue(c.rxDescr, float64(dev.BytesRecv), dev.Name))
		metrics.Add(metric.NewValue(c.txDescr, float64(dev.BytesSent), dev.Name))
	}

	return metrics
}

func (c *netCollector) Stop() {}
