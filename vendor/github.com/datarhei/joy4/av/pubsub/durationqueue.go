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
type DurationQueue struct {
	buf                          *pktque.Buf
	lock                         *sync.RWMutex
	cond                         *sync.Cond
	mintime, maxtime, targettime time.Duration
	streams                      []av.CodecData
	closed                       bool
}

func NewDurationQueue() *DurationQueue {
	q := &DurationQueue{}
	q.buf = pktque.NewBuf()
	q.targettime = 2 * time.Second
	q.lock = &sync.RWMutex{}
	q.cond = sync.NewCond(q.lock.RLocker())
	return q
}

func (q *DurationQueue) SetTargetTime(t time.Duration) {
	q.lock.Lock()
	q.targettime = t
	q.lock.Unlock()
}

func (q *DurationQueue) WriteHeader(streams []av.CodecData) error {
	q.lock.Lock()

	q.streams = streams

	q.cond.Broadcast()

	q.lock.Unlock()

	return nil
}

func (q *DurationQueue) WriteTrailer() error {
	return nil
}

// After Close() called, all QueueCursor's ReadPacket will return io.EOF.
func (q *DurationQueue) Close() (err error) {
	q.lock.Lock()

	q.closed = true
	q.cond.Broadcast()

	q.lock.Unlock()
	return
}

// Put packet into buffer, old packets will be discared.
func (q *DurationQueue) WritePacket(pkt av.Packet) (err error) {
	q.lock.Lock()

	q.buf.Push(pkt)

	if q.buf.Count == 0 {
		q.mintime = pkt.Time
	}

	q.maxtime = pkt.Time

	for q.maxtime-q.mintime > q.targettime && q.buf.Count > 1 {
		pkt := q.buf.Pop()
		q.mintime = pkt.Time

		if q.maxtime-q.mintime <= q.targettime {
			break
		}
	}

	q.cond.Broadcast()

	q.lock.Unlock()
	return
}

type DurationQueueCursor struct {
	que    *DurationQueue
	pos    pktque.BufPos
	gotpos bool
	init   func(buf *pktque.Buf) pktque.BufPos
}

func (q *DurationQueue) newCursor() *DurationQueueCursor {
	return &DurationQueueCursor{
		que: q,
	}
}

// Create cursor position at latest packet.
func (q *DurationQueue) Latest() *DurationQueueCursor {
	cursor := q.newCursor()
	cursor.init = func(buf *pktque.Buf) pktque.BufPos {
		return buf.Tail
	}
	return cursor
}

// Create cursor position at oldest buffered packet.
func (q *DurationQueue) Oldest() *DurationQueueCursor {
	cursor := q.newCursor()
	cursor.init = func(buf *pktque.Buf) pktque.BufPos {
		return buf.Head
	}
	return cursor
}

func (qc *DurationQueueCursor) Streams() (streams []av.CodecData, err error) {
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
func (qc *DurationQueueCursor) ReadPacket() (pkt av.Packet, err error) {
	qc.que.cond.L.Lock()
	buf := qc.que.buf
	if !qc.gotpos {
		qc.pos = qc.init(buf)
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
