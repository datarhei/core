package compress

import (
	"io"
	"sync"

	"github.com/klauspost/compress/gzip"
)

type gzipImpl struct {
	pool sync.Pool
}

func NewGzip(level Level) Compression {
	gzipLevel := gzip.DefaultCompression
	if level == BestCompression {
		gzipLevel = gzip.BestCompression
	} else if level == BestSpeed {
		gzipLevel = gzip.BestSpeed
	}

	g := &gzipImpl{
		pool: sync.Pool{
			New: func() interface{} {
				w, err := gzip.NewWriterLevel(io.Discard, gzipLevel)
				if err != nil {
					return nil
				}
				return w
			},
		},
	}

	return g
}

func (g *gzipImpl) Acquire() Compressor {
	c := g.pool.Get()
	if c == nil {
		return nil
	}

	x, ok := c.(Compressor)
	if !ok {
		return nil
	}

	x.Reset(io.Discard)

	return x
}

func (g *gzipImpl) Release(c Compressor) {
	c.Reset(io.Discard)
	g.pool.Put(c)
}
