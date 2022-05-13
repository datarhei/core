package monitor

import (
	"github.com/datarhei/core/monitor/metric"
	"github.com/datarhei/core/psutil"
)

type netCollector struct {
	rxDescr *metric.Description
	txDescr *metric.Description
}

func NewNetCollector() metric.Collector {
	c := &netCollector{}

	c.rxDescr = metric.NewDesc("net_rx", "", []string{"interface"})
	c.txDescr = metric.NewDesc("net_tx", "", []string{"interface"})

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

	devs, err := psutil.NetIOCounters(true)
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
