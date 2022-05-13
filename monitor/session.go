package monitor

import (
	"github.com/datarhei/core/monitor/metric"
	"github.com/datarhei/core/session"
)

type sessionCollector struct {
	prefix            string
	r                 session.Registry
	collectors        []string
	totalDescr        *metric.Description
	limitDescr        *metric.Description
	activeDescr       *metric.Description
	rxBytesDescr      *metric.Description
	txBytesDescr      *metric.Description
	rxBitrateDescr    *metric.Description
	txBitrateDescr    *metric.Description
	maxTxBitrateDescr *metric.Description
	maxRxBitrateDescr *metric.Description
}

func NewSessionCollector(r session.Registry, collectors []string) metric.Collector {
	c := &sessionCollector{
		prefix:     "session",
		r:          r,
		collectors: collectors,
	}

	if len(collectors) == 0 {
		c.collectors = r.Collectors()
	}

	c.totalDescr = metric.NewDesc("session_total", "", []string{"collector"})
	c.limitDescr = metric.NewDesc("session_limit", "", []string{"collector"})
	c.activeDescr = metric.NewDesc("session_active", "", []string{"collector"})
	c.rxBytesDescr = metric.NewDesc("session_rxbytes", "", []string{"collector"})
	c.txBytesDescr = metric.NewDesc("session_txbytes", "", []string{"collector"})

	c.rxBitrateDescr = metric.NewDesc("session_rxbitrate", "", []string{"collector"})
	c.txBitrateDescr = metric.NewDesc("session_txbitrate", "", []string{"collector"})

	c.maxTxBitrateDescr = metric.NewDesc("session_maxtxbitrate", "", []string{"collector"})
	c.maxRxBitrateDescr = metric.NewDesc("session_maxrxbitrate", "", []string{"collector"})

	return c
}

func (c *sessionCollector) Prefix() string {
	return c.prefix
}

func (c *sessionCollector) Describe() []*metric.Description {
	return []*metric.Description{
		c.totalDescr,
		c.limitDescr,
		c.activeDescr,
		c.rxBytesDescr,
		c.txBytesDescr,
		c.rxBitrateDescr,
		c.txBitrateDescr,
		c.maxTxBitrateDescr,
		c.maxRxBitrateDescr,
	}
}

func (c *sessionCollector) Collect() metric.Metrics {
	metrics := metric.NewMetrics()

	for _, name := range c.collectors {
		s := c.r.Summary(name)

		metrics.Add(metric.NewValue(c.totalDescr, float64(s.Summary.TotalSessions), name))
		metrics.Add(metric.NewValue(c.limitDescr, float64(s.MaxSessions), name))
		metrics.Add(metric.NewValue(c.activeDescr, float64(s.CurrentSessions), name))
		metrics.Add(metric.NewValue(c.rxBytesDescr, float64(s.Summary.TotalRxBytes), name))
		metrics.Add(metric.NewValue(c.txBytesDescr, float64(s.Summary.TotalTxBytes), name))
		metrics.Add(metric.NewValue(c.rxBitrateDescr, s.CurrentRxBitrate, name))
		metrics.Add(metric.NewValue(c.txBitrateDescr, s.CurrentTxBitrate, name))
		metrics.Add(metric.NewValue(c.maxTxBitrateDescr, s.MaxTxBitrate, name))
		metrics.Add(metric.NewValue(c.maxRxBitrateDescr, 0, name))
	}

	return metrics
}

func (c *sessionCollector) Stop() {}
