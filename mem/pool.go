package mem

import (
	"sync"
)

type BufferPool struct {
	pool sync.Pool
}

func NewBufferPool() *BufferPool {
	p := &BufferPool{
		pool: sync.Pool{
			New: func() any {
				return &Buffer{}
			},
		},
	}

	return p
}

func (p *BufferPool) Get() *Buffer {
	buf := p.pool.Get().(*Buffer)
	buf.Reset()

	return buf
}

func (p *BufferPool) Put(buf *Buffer) {
	p.pool.Put(buf)
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
