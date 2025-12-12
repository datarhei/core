package rtmp

import (
	"fmt"

	"github.com/datarhei/joy4/av"
)

type connection interface {
	av.MuxCloser
	av.DemuxCloser
	TxBytes() uint64
	RxBytes() uint64
}

// conn implements the connection interface
type conn struct {
	muxer   av.MuxCloser
	demuxer av.DemuxCloser

	txbytes uint64
	rxbytes uint64
}

// Make sure that conn implements the connection interface
var _ connection = &conn{}

func newConnectionFromDemuxCloser(m av.DemuxCloser) connection {
	c := &conn{
		demuxer: m,
	}

	return c
}

func newConnectionFromMuxCloser(m av.MuxCloser) connection {
	c := &conn{
		muxer: m,
	}

	return c
}

func newConnectionFromMuxer(m av.Muxer) connection {
	c := &conn{
		muxer: &fakeMuxCloser{m},
	}

	return c
}

type fakeMuxCloser struct {
	av.Muxer
}

func (f *fakeMuxCloser) Close() error {
	return nil
}

func (c *conn) TxBytes() uint64 {
	return c.txbytes
}

func (c *conn) RxBytes() uint64 {
	return c.rxbytes
}

func (c *conn) ReadPacket() (av.Packet, error) {
	if c.demuxer != nil {
		p, err := c.demuxer.ReadPacket()
		if err == nil {
			c.rxbytes += uint64(len(p.Data))
		}

		return p, err
	}

	return av.Packet{}, fmt.Errorf("no demuxer available")
}

func (c *conn) Streams() ([]av.CodecData, error) {
	if c.demuxer != nil {
		return c.demuxer.Streams()
	}

	return nil, fmt.Errorf("no demuxer available")
}

func (c *conn) WritePacket(p av.Packet) error {
	if c.muxer != nil {
		err := c.muxer.WritePacket(p)
		if err == nil {
			c.txbytes += uint64(len(p.Data))
		}

		return err
	}

	return fmt.Errorf("no muxer available")
}

func (c *conn) WriteHeader(streams []av.CodecData) error {
	if c.muxer != nil {
		return c.muxer.WriteHeader(streams)
	}

	return fmt.Errorf("no muxer available")
}

func (c *conn) WriteTrailer() error {
	if c.muxer != nil {
		return c.muxer.WriteTrailer()
	}

	return fmt.Errorf("no muxer available")
}

func (c *conn) Close() error {
	if c.muxer != nil {
		return c.muxer.Close()
	}

	if c.demuxer != nil {
		return c.demuxer.Close()
	}

	return nil
}
