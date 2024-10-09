package compress

import (
	"compress/gzip"
	"io"
	"sync"
)

type gogzipImpl struct {
	pool sync.Pool
}

func NewGoGzip(level Level) Compression {
	gzipLevel := gzip.DefaultCompression
	if level == BestCompression {
		gzipLevel = gzip.BestCompression
	} else if level == BestSpeed {
		gzipLevel = gzip.BestSpeed
	}

	g := &gogzipImpl{
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

func (g *gogzipImpl) Acquire() Compressor {
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

func (g *gogzipImpl) Release(c Compressor) {
	c.Reset(io.Discard)
	g.pool.Put(c)
}
