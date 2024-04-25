package compress

import (
	"io"
	"sync"

	"github.com/klauspost/compress/zstd"
)

type zstdImpl struct {
	pool sync.Pool
}

func NewZstd(level Level) Compression {
	zstdLevel := zstd.SpeedDefault
	if level == BestCompression {
		zstdLevel = zstd.SpeedBestCompression
	} else {
		zstdLevel = zstd.SpeedFastest
	}

	g := &zstdImpl{
		pool: sync.Pool{
			New: func() interface{} {
				w, err := zstd.NewWriter(io.Discard, zstd.WithZeroFrames(true), zstd.WithEncoderLevel(zstdLevel))
				if err != nil {
					return nil
				}
				return w
			},
		},
	}

	return g
}

func (g *zstdImpl) Acquire() Compressor {
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

func (g *zstdImpl) Release(c Compressor) {
	c.Reset(io.Discard)
	g.pool.Put(c)
}
