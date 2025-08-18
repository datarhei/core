// Packege pubsub implements publisher-subscribers model used in multi-channel streaming.
package pubsub

import (
	"io"
	"sync"
	"time"

	"github.com/datarhei/joy4/av"
	"github.com/datarhei/joy4/av/pktque"
)

//        time
// ----------------->
//
// V-A-V-V-A-V-V-A-V-V
// |                 |
// 0        5        10
// head             tail
// oldest          latest
//

// One publisher and multiple subscribers thread-safe packet buffer queue.
type Queue struct {
	buf                      *pktque.Buf
	lock                     *sync.RWMutex
	cond                     *sync.Cond
	curgopcount, maxgopcount int
	streams                  []av.CodecData
	videoidx                 int
	closed                   bool
}

func NewQueue() *Queue {
	q := &Queue{}
	q.buf = pktque.NewBuf()
	q.maxgopcount = 2
	q.lock = &sync.RWMutex{}
	q.cond = sync.NewCond(q.lock.RLocker())
	q.videoidx = -1
	return q
}

func (q *Queue) SetMaxGopCount(n int) {
	q.lock.Lock()
	q.maxgopcount = n
	q.lock.Unlock()
}

func (q *Queue) WriteHeader(streams []av.CodecData) error {
	q.lock.Lock()

	q.streams = streams
	for i, stream := range streams {
		if stream.Type().IsVideo() {
			q.videoidx = i
		}
	}
	q.cond.Broadcast()

	q.lock.Unlock()

	return nil
}

func (q *Queue) WriteTrailer() error {
	return nil
}

// After Close() called, all QueueCursor's ReadPacket will return io.EOF.
func (q *Queue) Close() (err error) {
	q.lock.Lock()

	q.closed = true
	q.cond.Broadcast()

	q.lock.Unlock()
	return
}

// Put packet into buffer, old packets will be discared.
func (q *Queue) WritePacket(pkt av.Packet) (err error) {
	q.lock.Lock()

	q.buf.Push(pkt)

	if q.videoidx == -1 { // audio only stream
		if pkt.IsKeyFrame {
			q.curgopcount++
		}

		for q.curgopcount >= q.maxgopcount && q.buf.Count > 1 {
			pkt := q.buf.Pop()
			if pkt.IsKeyFrame {
				q.curgopcount--
			}
			if q.curgopcount < q.maxgopcount {
				break
			}
		}
	} else { // video only or video+audio stream
		if pkt.Idx == int8(q.videoidx) && pkt.IsKeyFrame {
			q.curgopcount++
		}

		for q.curgopcount >= q.maxgopcount && q.buf.Count > 1 {
			pkt := q.buf.Pop()
			if pkt.Idx == int8(q.videoidx) && pkt.IsKeyFrame {
				q.curgopcount--
			}
			if q.curgopcount < q.maxgopcount {
				break
			}
		}
	}

	q.cond.Broadcast()

	q.lock.Unlock()
	return
}

type QueueCursor struct {
	que    *Queue
	pos    pktque.BufPos
	gotpos bool
	init   func(buf *pktque.Buf, videoidx int) pktque.BufPos
}

func (q *Queue) newCursor() *QueueCursor {
	return &QueueCursor{
		que: q,
	}
}

// Create cursor position at latest packet.
func (q *Queue) Latest() *QueueCursor {
	cursor := q.newCursor()
	cursor.init = func(buf *pktque.Buf, videoidx int) pktque.BufPos {
		return buf.Tail
	}
	return cursor
}

// Create cursor position at oldest buffered packet.
func (q *Queue) Oldest() *QueueCursor {
	cursor := q.newCursor()
	cursor.init = func(buf *pktque.Buf, videoidx int) pktque.BufPos {
		return buf.Head
	}
	return cursor
}

// Create cursor position at specific time in buffered packets.
func (q *Queue) DelayedTime(dur time.Duration) *QueueCursor {
	cursor := q.newCursor()
	cursor.init = func(buf *pktque.Buf, videoidx int) pktque.BufPos {
		i := buf.Tail - 1
		if buf.IsValidPos(i) {
			end := buf.Get(i)
			for buf.IsValidPos(i) {
				if end.Time-buf.Get(i).Time > dur {
					break
				}
				i--
			}
		}
		return i
	}
	return cursor
}

// Create cursor position at specific delayed GOP count in buffered packets.
func (q *Queue) DelayedGopCount(n int) *QueueCursor {
	cursor := q.newCursor()
	cursor.init = func(buf *pktque.Buf, videoidx int) pktque.BufPos {
		i := buf.Tail - 1
		if videoidx != -1 {
			for gop := 0; buf.IsValidPos(i) && gop < n; i-- {
				pkt := buf.Get(i)
				if pkt.Idx == int8(q.videoidx) && pkt.IsKeyFrame {
					gop++
				}
			}
		}
		return i
	}
	return cursor
}

func (qc *QueueCursor) Streams() (streams []av.CodecData, err error) {
	qc.que.cond.L.Lock()
	for qc.que.streams == nil && !qc.que.closed {
		qc.que.cond.Wait()
	}
	if qc.que.streams != nil {
		streams = qc.que.streams
	} else {
		err = io.EOF
	}
	qc.que.cond.L.Unlock()
	return
}

// ReadPacket will not consume packets in Queue, it's just a cursor.
func (qc *QueueCursor) ReadPacket() (pkt av.Packet, err error) {
	qc.que.cond.L.Lock()
	buf := qc.que.buf
	if !qc.gotpos {
		qc.pos = qc.init(buf, qc.que.videoidx)
		qc.gotpos = true
	}
	for {
		if qc.pos.LT(buf.Head) {
			qc.pos = buf.Head
		} else if qc.pos.GT(buf.Tail) {
			qc.pos = buf.Tail
		}
		if buf.IsValidPos(qc.pos) {
			pkt = buf.Get(qc.pos)
			qc.pos++
			break
		}
		if qc.que.closed {
			err = io.EOF
			break
		}
		qc.que.cond.Wait()
	}
	qc.que.cond.L.Unlock()
	return
}
