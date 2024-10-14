package monitor

import (
	"runtime"

	"github.com/datarhei/core/v16/mem"
	"github.com/datarhei/core/v16/monitor/metric"
)

type selfCollector struct {
	bufferAllocDescr       *metric.Description
	bufferReuseDescr       *metric.Description
	bufferRecycleDescr     *metric.Description
	bufferDumpDescr        *metric.Description
	bufferDefaultSizeDescr *metric.Description
	bufferMaxSizeDescr     *metric.Description

	heapAllocDescr   *metric.Description
	totalAllocDescr  *metric.Description
	heapSysDescr     *metric.Description
	heapIdleDescr    *metric.Description
	heapInuseDescr   *metric.Description
	heapObjectsDescr *metric.Description
	mallocsDescr     *metric.Description
	freesDescr       *metric.Description
}

func NewSelfCollector() metric.Collector {
	c := &selfCollector{}

	c.bufferAllocDescr = metric.NewDesc("self_bufferpool_alloc", "Number of buffer allocations", nil)
	c.bufferReuseDescr = metric.NewDesc("self_bufferpool_reuse", "Number of buffer reuses", nil)
	c.bufferRecycleDescr = metric.NewDesc("self_bufferpool_recycle", "Number of buffer recycles", nil)
	c.bufferDumpDescr = metric.NewDesc("self_bufferpool_dump", "Number of buffer dumps", nil)
	c.bufferDefaultSizeDescr = metric.NewDesc("self_bufferpool_default_size", "Default buffer size", nil)
	c.bufferMaxSizeDescr = metric.NewDesc("self_bufferpool_max_size", "Max. buffer size for recycling", nil)

	c.heapAllocDescr = metric.NewDesc("self_mem_heap_alloc_bytes", "Number of bytes allocated on the heap", nil)
	c.totalAllocDescr = metric.NewDesc("self_mem_total_alloc_bytes", "Cumulative count of bytes allocated since start", nil)
	c.heapSysDescr = metric.NewDesc("self_mem_heap_sys_bytes", "Number of bytes obtained from OS", nil)
	c.heapIdleDescr = metric.NewDesc("self_mem_heap_idle_bytes", "Number of unused bytes", nil)
	c.heapInuseDescr = metric.NewDesc("self_mem_heap_inuse_bytes", "Number of used bytes", nil)
	c.heapObjectsDescr = metric.NewDesc("self_mem_heap_objects", "Number of objects in heap", nil)
	c.mallocsDescr = metric.NewDesc("self_mem_mallocs", "Cumulative count of heap objects allocated", nil)
	c.freesDescr = metric.NewDesc("self_mem_frees", "Cumulative count of heap objects freed", nil)

	return c
}

func (c *selfCollector) Stop() {}

func (c *selfCollector) Prefix() string {
	return "bufferpool"
}

func (c *selfCollector) Describe() []*metric.Description {
	return []*metric.Description{
		c.bufferAllocDescr,
		c.bufferReuseDescr,
		c.bufferRecycleDescr,
		c.bufferDumpDescr,
		c.bufferDefaultSizeDescr,
		c.bufferMaxSizeDescr,
		c.heapAllocDescr,
		c.totalAllocDescr,
		c.heapSysDescr,
		c.heapIdleDescr,
		c.heapInuseDescr,
		c.heapObjectsDescr,
		c.mallocsDescr,
		c.freesDescr,
	}
}

func (c *selfCollector) Collect() metric.Metrics {
	bufferstats := mem.Stats()

	metrics := metric.NewMetrics()

	metrics.Add(metric.NewValue(c.bufferAllocDescr, float64(bufferstats.Alloc)))
	metrics.Add(metric.NewValue(c.bufferReuseDescr, float64(bufferstats.Reuse)))
	metrics.Add(metric.NewValue(c.bufferRecycleDescr, float64(bufferstats.Recycle)))
	metrics.Add(metric.NewValue(c.bufferDumpDescr, float64(bufferstats.Dump)))
	metrics.Add(metric.NewValue(c.bufferDefaultSizeDescr, float64(bufferstats.DefaultSize)))
	metrics.Add(metric.NewValue(c.bufferMaxSizeDescr, float64(bufferstats.MaxSize)))

	memstats := runtime.MemStats{}
	runtime.ReadMemStats(&memstats)

	metrics.Add(metric.NewValue(c.heapAllocDescr, float64(memstats.HeapAlloc)))
	metrics.Add(metric.NewValue(c.totalAllocDescr, float64(memstats.TotalAlloc)))
	metrics.Add(metric.NewValue(c.heapSysDescr, float64(memstats.HeapSys)))
	metrics.Add(metric.NewValue(c.heapIdleDescr, float64(memstats.HeapIdle)))
	metrics.Add(metric.NewValue(c.heapInuseDescr, float64(memstats.HeapInuse)))
	metrics.Add(metric.NewValue(c.heapObjectsDescr, float64(memstats.HeapObjects)))
	metrics.Add(metric.NewValue(c.mallocsDescr, float64(memstats.Mallocs)))
	metrics.Add(metric.NewValue(c.freesDescr, float64(memstats.Frees)))

	return metrics
}
