package monitor

import (
	"fmt"

	"github.com/datarhei/core/v16/monitor/metric"
	"github.com/datarhei/core/v16/resources"
)

type gpuCollector struct {
	ngpuDescr        *metric.Description
	usageDescr       *metric.Description
	encoderDescr     *metric.Description
	decoderDescr     *metric.Description
	memoryTotalDescr *metric.Description
	memoryFreeDescr  *metric.Description
	memoryLimitDescr *metric.Description
	limitDescr       *metric.Description

	resources resources.Resources
}

func NewGPUCollector(rsc resources.Resources) metric.Collector {
	c := &gpuCollector{
		resources: rsc,
	}

	c.ngpuDescr = metric.NewDesc("gpu_ngpu", "Number of GPUs in the system", nil)
	c.usageDescr = metric.NewDesc("gpu_usage", "Percentage of GPU used ", []string{"index"})
	c.encoderDescr = metric.NewDesc("gpu_encoder", "Percentage of GPU encoder used", []string{"index"})
	c.decoderDescr = metric.NewDesc("gpu_decoder", "Percentage of GPU decoder used", []string{"index"})
	c.memoryTotalDescr = metric.NewDesc("gpu_mem_total", "GPU memory total in bytes", []string{"index"})
	c.memoryFreeDescr = metric.NewDesc("gpu_mem_free", "GPU memory available in bytes", []string{"index"})
	c.memoryLimitDescr = metric.NewDesc("gpu_mem_limit", "GPU memory limit in bytes", []string{"index"})
	c.limitDescr = metric.NewDesc("gpu_limit", "Percentage of GPU to be consumed", []string{"index"})

	return c
}

func (c *gpuCollector) Stop() {}

func (c *gpuCollector) Prefix() string {
	return "cpu"
}

func (c *gpuCollector) Describe() []*metric.Description {
	return []*metric.Description{
		c.ngpuDescr,
		c.usageDescr,
		c.encoderDescr,
		c.decoderDescr,
		c.memoryTotalDescr,
		c.memoryFreeDescr,
		c.memoryLimitDescr,
		c.limitDescr,
	}
}

func (c *gpuCollector) Collect() metric.Metrics {
	metrics := metric.NewMetrics()

	rinfo := c.resources.Info()

	metrics.Add(metric.NewValue(c.ngpuDescr, rinfo.GPU.NGPU))

	for i, gpu := range rinfo.GPU.GPU {
		index := fmt.Sprintf("%d", i)
		metrics.Add(metric.NewValue(c.usageDescr, gpu.Usage, index))
		metrics.Add(metric.NewValue(c.encoderDescr, gpu.Encoder, index))
		metrics.Add(metric.NewValue(c.decoderDescr, gpu.Decoder, index))
		metrics.Add(metric.NewValue(c.limitDescr, gpu.UsageLimit, index))

		metrics.Add(metric.NewValue(c.memoryTotalDescr, float64(gpu.MemoryTotal), index))
		metrics.Add(metric.NewValue(c.memoryFreeDescr, float64(gpu.MemoryAvailable), index))
		metrics.Add(metric.NewValue(c.memoryLimitDescr, float64(gpu.MemoryLimit), index))
	}

	return metrics
}
