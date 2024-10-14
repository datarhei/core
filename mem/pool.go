package mem

// Based on github.com/valyala/bytebufferpool

import (
	"sort"
	"sync"
	"sync/atomic"
)

const (
	minBitSize = 6 // 2**6=64 is a CPU cache line size
	steps      = 20

	minSize = 1 << minBitSize
	maxSize = 1 << (minBitSize + steps - 1)

	calibrateCallsThreshold = 42000
	maxPercentile           = 0.95
)

type BufferPool struct {
	calls       [steps]uint64
	calibrating uint64

	defaultSize uint64
	maxSize     uint64

	pool sync.Pool
}

func NewBufferPool() *BufferPool {
	p := &BufferPool{
		pool: sync.Pool{},
	}

	return p
}

func (p *BufferPool) Get() *Buffer {
	v := p.pool.Get()
	if v != nil {
		return v.(*Buffer)
	}

	return &Buffer{
		data: make([]byte, 0, atomic.LoadUint64(&p.defaultSize)),
	}
}

func (p *BufferPool) Put(buf *Buffer) {
	idx := index(len(buf.data))

	if atomic.AddUint64(&p.calls[idx], 1) > calibrateCallsThreshold {
		p.calibrate()
	}

	maxSize := int(atomic.LoadUint64(&p.maxSize))
	if maxSize == 0 || cap(buf.data) <= maxSize {
		buf.Reset()
		p.pool.Put(buf)
	}
}

func (p *BufferPool) calibrate() {
	if !atomic.CompareAndSwapUint64(&p.calibrating, 0, 1) {
		return
	}

	a := make(callSizes, 0, steps)
	var callsSum uint64
	for i := uint64(0); i < steps; i++ {
		calls := atomic.SwapUint64(&p.calls[i], 0)
		callsSum += calls
		a = append(a, callSize{
			calls: calls,
			size:  minSize << i,
		})
	}
	sort.Sort(a)

	defaultSize := a[0].size
	maxSize := defaultSize

	maxSum := uint64(float64(callsSum) * maxPercentile)
	callsSum = 0
	for i := 0; i < steps; i++ {
		if callsSum > maxSum {
			break
		}
		callsSum += a[i].calls
		size := a[i].size
		if size > maxSize {
			maxSize = size
		}
	}

	atomic.StoreUint64(&p.defaultSize, defaultSize)
	atomic.StoreUint64(&p.maxSize, maxSize)

	atomic.StoreUint64(&p.calibrating, 0)
}

type callSize struct {
	calls uint64
	size  uint64
}

type callSizes []callSize

func (ci callSizes) Len() int {
	return len(ci)
}

func (ci callSizes) Less(i, j int) bool {
	return ci[i].calls > ci[j].calls
}

func (ci callSizes) Swap(i, j int) {
	ci[i], ci[j] = ci[j], ci[i]
}

func index(n int) int {
	n--
	n >>= minBitSize
	idx := 0
	for n > 0 {
		n >>= 1
		idx++
	}
	if idx >= steps {
		idx = steps - 1
	}
	return idx
}

var DefaultBufferPool *BufferPool

func init() {
	DefaultBufferPool = NewBufferPool()
}

func Get() *Buffer {
	return DefaultBufferPool.Get()
}

func Put(buf *Buffer) {
	DefaultBufferPool.Put(buf)
}
