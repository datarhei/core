package compress

import (
	"io"
	"sync"

	"github.com/andybalholm/brotli"
)

type brotliImpl struct {
	pool sync.Pool
}

func NewBrotli(level Level) Compression {
	brotliLevel := brotli.DefaultCompression
	if level == BestCompression {
		brotliLevel = brotli.BestCompression
	} else {
		brotliLevel = brotli.BestSpeed
	}

	g := &brotliImpl{
		pool: sync.Pool{
			New: func() interface{} {
				return brotli.NewWriterLevel(io.Discard, brotliLevel)
			},
		},
	}

	return g
}

func (g *brotliImpl) Acquire() Compressor {
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

func (g *brotliImpl) Release(c Compressor) {
	c.Reset(io.Discard)
	g.pool.Put(c)
}
