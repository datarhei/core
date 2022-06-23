package congestion

import (
	"container/list"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/datarhei/gosrt/internal/circular"
	"github.com/datarhei/gosrt/internal/packet"
)

// liveSend implements the Sender interface
type liveSend struct {
	nextSequenceNumber circular.Number

	packetList *list.List
	lossList   *list.List
	lock       sync.RWMutex

	dropInterval uint64 // microseconds

	avgPayloadSize float64 // bytes
	pktSndPeriod   float64 // microseconds
	maxBW          float64 // bytes/s
	inputBW        float64 // bytes/s
	overheadBW     float64 // percent

	statistics SendStats

	rate struct {
		period time.Duration
		last   time.Time

		bytes     uint64
		prevBytes uint64

		estimatedInputBW float64 // bytes/s
	}

	deliver func(p packet.Packet)
}

// NewLiveSend takes a SendConfig and returns a new Sender
func NewLiveSend(config SendConfig) Sender {
	s := &liveSend{
		nextSequenceNumber: config.InitialSequenceNumber,
		packetList:         list.New(),
		lossList:           list.New(),

		dropInterval: config.DropInterval, // microseconds

		avgPayloadSize: 1456, //  5.1.2. SRT's Default LiveCC Algorithm
		maxBW:          float64(config.MaxBW),
		inputBW:        float64(config.InputBW),
		overheadBW:     float64(config.OverheadBW),

		deliver: config.OnDeliver,
	}

	if s.deliver == nil {
		s.deliver = func(p packet.Packet) {}
	}

	s.maxBW = 128 * 1024 * 1024 // 1 Gbit/s
	s.pktSndPeriod = (s.avgPayloadSize + 16) * 1_000_000 / s.maxBW

	s.rate.period = time.Second
	s.rate.last = time.Now()

	return s
}

func (s *liveSend) Stats() SendStats {
	s.lock.RLock()
	defer s.lock.RUnlock()

	s.statistics.UsPktSndPeriod = s.pktSndPeriod
	s.statistics.BytePayload = uint64(s.avgPayloadSize)
	s.statistics.MsSndBuf = 0

	max := s.lossList.Back()
	min := s.lossList.Front()

	if max != nil && min != nil {
		s.statistics.MsSndBuf = (max.Value.(packet.Packet).Header().PktTsbpdTime - min.Value.(packet.Packet).Header().PktTsbpdTime) / 1_000
	}

	return s.statistics
}

func (s *liveSend) Flush() {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.packetList = s.packetList.Init()
	s.lossList = s.lossList.Init()
}

func (s *liveSend) Push(p packet.Packet) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if p == nil {
		return
	}

	// give to the packet a sequence number
	p.Header().PacketSequenceNumber = s.nextSequenceNumber
	s.nextSequenceNumber = s.nextSequenceNumber.Inc()

	pktLen := p.Len()

	s.statistics.PktSndBuf++
	s.statistics.ByteSndBuf += pktLen

	// bandwidth calculation
	s.rate.bytes += pktLen

	now := time.Now()
	tdiff := now.Sub(s.rate.last)

	if tdiff > s.rate.period {
		s.rate.estimatedInputBW = float64(s.rate.bytes-s.rate.prevBytes) / tdiff.Seconds()

		s.rate.prevBytes = s.rate.bytes
		s.rate.last = now
	}

	p.Header().Timestamp = uint32(p.Header().PktTsbpdTime & uint64(packet.MAX_TIMESTAMP))

	s.packetList.PushBack(p)

	s.statistics.PktFlightSize = uint64(s.packetList.Len())
}

func (s *liveSend) Tick(now uint64) {
	// deliver packets whose PktTsbpdTime is ripe
	s.lock.Lock()
	removeList := make([]*list.Element, 0, s.packetList.Len())
	for e := s.packetList.Front(); e != nil; e = e.Next() {
		p := e.Value.(packet.Packet)
		if p.Header().PktTsbpdTime <= now {
			s.statistics.PktSent++
			s.statistics.PktSentUnique++

			s.statistics.ByteSent += p.Len()
			s.statistics.ByteSentUnique += p.Len()

			s.statistics.UsSndDuration += uint64(s.pktSndPeriod)

			//  5.1.2. SRT's Default LiveCC Algorithm
			s.avgPayloadSize = 0.875*s.avgPayloadSize + 0.125*float64(p.Len())

			s.deliver(p)
			removeList = append(removeList, e)
		} else {
			break
		}
	}

	for _, e := range removeList {
		s.lossList.PushBack(e.Value)
		s.packetList.Remove(e)
	}
	s.lock.Unlock()

	s.lock.Lock()
	removeList = make([]*list.Element, 0, s.lossList.Len())
	for e := s.lossList.Front(); e != nil; e = e.Next() {
		p := e.Value.(packet.Packet)

		if p.Header().PktTsbpdTime+s.dropInterval <= now {
			// dropped packet because too old
			s.statistics.PktSndDrop++
			s.statistics.PktSndLoss++
			s.statistics.ByteSndDrop += p.Len()
			s.statistics.ByteSndLoss += p.Len()

			removeList = append(removeList, e)
		}
	}

	// These packets are not needed anymore (too late)
	for _, e := range removeList {
		p := e.Value.(packet.Packet)

		s.statistics.PktSndBuf--
		s.statistics.ByteSndBuf -= p.Len()

		s.lossList.Remove(e)

		// This packet has been ACK'd and we don't need it anymore
		p.Decommission()
	}
	s.lock.Unlock()
}

func (s *liveSend) ACK(sequenceNumber circular.Number) {
	s.lock.Lock()
	defer s.lock.Unlock()

	removeList := make([]*list.Element, 0, s.lossList.Len())
	for e := s.lossList.Front(); e != nil; e = e.Next() {
		p := e.Value.(packet.Packet)
		if p.Header().PacketSequenceNumber.Lt(sequenceNumber) {
			// remove packet from buffer because it has been successfully transmitted
			removeList = append(removeList, e)
		} else {
			break
		}
	}

	// These packets are not needed anymore (ACK'd)
	for _, e := range removeList {
		p := e.Value.(packet.Packet)

		s.statistics.PktSndBuf--
		s.statistics.ByteSndBuf -= p.Len()

		s.lossList.Remove(e)

		// This packet has been ACK'd and we don't need it anymore
		p.Decommission()
	}

	s.pktSndPeriod = (s.avgPayloadSize + 16) * 1000000 / s.maxBW
}

func (s *liveSend) NAK(sequenceNumbers []circular.Number) {
	if len(sequenceNumbers) == 0 {
		return
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	for e := s.lossList.Back(); e != nil; e = e.Prev() {
		p := e.Value.(packet.Packet)

		for i := 0; i < len(sequenceNumbers); i += 2 {
			if p.Header().PacketSequenceNumber.Gte(sequenceNumbers[i]) && p.Header().PacketSequenceNumber.Lte(sequenceNumbers[i+1]) {
				s.statistics.PktRetrans++
				s.statistics.PktSent++
				s.statistics.PktSndLoss++

				s.statistics.ByteRetrans += p.Len()
				s.statistics.ByteSent += p.Len()
				s.statistics.ByteSndLoss += p.Len()

				//  5.1.2. SRT's Default LiveCC Algorithm
				s.avgPayloadSize = 0.875*s.avgPayloadSize + 0.125*float64(p.Len())

				p.Header().RetransmittedPacketFlag = true
				s.deliver(p)
			}
		}
	}
}

// liveReceive implements the Receiver interface
type liveReceive struct {
	maxSeenSequenceNumber       circular.Number
	lastACKSequenceNumber       circular.Number
	lastDeliveredSequenceNumber circular.Number
	packetList                  *list.List
	lock                        sync.RWMutex

	nPackets uint

	periodicACKInterval uint64 // config
	periodicNAKInterval uint64 // config

	lastPeriodicACK uint64
	lastPeriodicNAK uint64

	avgPayloadSize float64 // bytes

	statistics ReceiveStats

	rate struct {
		last   time.Time
		period time.Duration

		packets     uint64
		prevPackets uint64
		bytes       uint64
		prevBytes   uint64

		pps uint32
		bps uint32
	}

	sendACK func(seq circular.Number, light bool)
	sendNAK func(from, to circular.Number)
	deliver func(p packet.Packet)
}

// NewLiveReceive takes a ReceiveConfig and returns a new Receiver
func NewLiveReceive(config ReceiveConfig) Receiver {
	r := &liveReceive{
		maxSeenSequenceNumber:       config.InitialSequenceNumber.Dec(),
		lastACKSequenceNumber:       config.InitialSequenceNumber.Dec(),
		lastDeliveredSequenceNumber: config.InitialSequenceNumber.Dec(),
		packetList:                  list.New(),

		periodicACKInterval: config.PeriodicACKInterval,
		periodicNAKInterval: config.PeriodicNAKInterval,

		avgPayloadSize: 1456, //  5.1.2. SRT's Default LiveCC Algorithm

		sendACK: config.OnSendACK,
		sendNAK: config.OnSendNAK,
		deliver: config.OnDeliver,
	}

	if r.sendACK == nil {
		r.sendACK = func(seq circular.Number, light bool) {}
	}

	if r.sendNAK == nil {
		r.sendNAK = func(from, to circular.Number) {}
	}

	if r.deliver == nil {
		r.deliver = func(p packet.Packet) {}
	}

	r.rate.last = time.Now()
	r.rate.period = time.Second

	return r
}

func (r *liveReceive) Stats() ReceiveStats {
	r.lock.RLock()
	defer r.lock.RUnlock()

	r.statistics.BytePayload = uint64(r.avgPayloadSize)

	return r.statistics
}

func (r *liveReceive) PacketRate() (pps, bps uint32) {
	r.lock.Lock()
	defer r.lock.Unlock()

	tdiff := time.Since(r.rate.last)

	if tdiff < r.rate.period {
		pps = r.rate.pps
		bps = r.rate.bps

		return
	}

	pdiff := r.rate.packets - r.rate.prevPackets
	bdiff := r.rate.bytes - r.rate.prevBytes

	r.rate.pps = uint32(float64(pdiff) / tdiff.Seconds())
	r.rate.bps = uint32(float64(bdiff) / tdiff.Seconds())

	r.rate.prevPackets, r.rate.prevBytes = r.rate.packets, r.rate.bytes
	r.rate.last = time.Now()

	pps = r.rate.pps
	bps = r.rate.bps

	return
}

func (r *liveReceive) Flush() {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.packetList = r.packetList.Init()
}

func (r *liveReceive) Push(pkt packet.Packet) {
	r.lock.Lock()
	defer r.lock.Unlock()

	if pkt == nil {
		return
	}

	r.nPackets++

	pktLen := pkt.Len()

	r.rate.packets++
	r.rate.bytes += pktLen

	r.statistics.PktRecv++
	r.statistics.ByteRecv += pktLen

	//pkt.PktTsbpdTime = pkt.Timestamp + r.delay
	if pkt.Header().RetransmittedPacketFlag {
		r.statistics.PktRcvRetrans++
		r.statistics.ByteRcvRetrans += pktLen
	}

	//  5.1.2. SRT's Default LiveCC Algorithm
	r.avgPayloadSize = 0.875*r.avgPayloadSize + 0.125*float64(pktLen)

	if pkt.Header().PacketSequenceNumber.Lte(r.lastDeliveredSequenceNumber) {
		// too old, because up until r.lastDeliveredSequenceNumber, we already delivered
		r.statistics.PktRcvDrop++
		r.statistics.ByteRcvDrop += pktLen

		return
	}

	if pkt.Header().PacketSequenceNumber.Lt(r.lastACKSequenceNumber) {
		// already acknowledged, ignoring
		r.statistics.PktRcvDrop++
		r.statistics.ByteRcvDrop += pktLen

		return
	}

	if pkt.Header().PacketSequenceNumber.Equals(r.maxSeenSequenceNumber.Inc()) {
		// in order, the packet we expected
		r.maxSeenSequenceNumber = pkt.Header().PacketSequenceNumber
	} else if pkt.Header().PacketSequenceNumber.Lte(r.maxSeenSequenceNumber) {
		// out of order, is it a missing piece? put it in the correct position
		for e := r.packetList.Front(); e != nil; e = e.Next() {
			p := e.Value.(packet.Packet)

			if p.Header().PacketSequenceNumber == pkt.Header().PacketSequenceNumber {
				// already received (has been sent more than once), ignoring
				r.statistics.PktRcvDrop++
				r.statistics.ByteRcvDrop += pktLen

				break
			} else if p.Header().PacketSequenceNumber.Gt(pkt.Header().PacketSequenceNumber) {
				// late arrival, this fills a gap
				r.statistics.PktRcvBuf++
				r.statistics.PktRecvUnique++

				r.statistics.ByteRcvBuf += pktLen
				r.statistics.ByteRecvUnique += pktLen

				r.packetList.InsertBefore(pkt, e)

				break
			}
		}

		return
	} else {
		// too far ahead, there are some missing sequence numbers, immediate NAK report
		// here we can prevent a possibly unnecessary NAK with SRTO_LOXXMAXTTL
		r.sendNAK(r.maxSeenSequenceNumber.Inc(), pkt.Header().PacketSequenceNumber.Dec())

		len := uint64(pkt.Header().PacketSequenceNumber.Distance(r.maxSeenSequenceNumber))
		r.statistics.PktRcvLoss += len
		r.statistics.ByteRcvLoss += len * uint64(r.avgPayloadSize)

		r.maxSeenSequenceNumber = pkt.Header().PacketSequenceNumber
	}

	r.statistics.PktRcvBuf++
	r.statistics.PktRecvUnique++

	r.statistics.ByteRcvBuf += pktLen
	r.statistics.ByteRecvUnique += pktLen

	r.packetList.PushBack(pkt)
}

func (r *liveReceive) periodicACK(now uint64) (ok bool, sequenceNumber circular.Number, lite bool) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	// 4.8.1. Packet Acknowledgement (ACKs, ACKACKs)
	if now-r.lastPeriodicACK < r.periodicACKInterval {
		if r.nPackets >= 64 {
			lite = true // send light ACK
		} else {
			return
		}
	}

	minPktTsbpdTime, maxPktTsbpdTime := uint64(0), uint64(0)

	ackSequenceNumber := r.lastDeliveredSequenceNumber

	// find the sequence number up until we have all in a row.
	// where the first gap is (or at the end of the list) is where we can ACK to.
	e := r.packetList.Front()
	if e != nil {
		p := e.Value.(packet.Packet)

		minPktTsbpdTime = p.Header().PktTsbpdTime
		maxPktTsbpdTime = p.Header().PktTsbpdTime

		if p.Header().PacketSequenceNumber.Equals(ackSequenceNumber.Inc()) {
			ackSequenceNumber = p.Header().PacketSequenceNumber

			for e = e.Next(); e != nil; e = e.Next() {
				p = e.Value.(packet.Packet)
				if !p.Header().PacketSequenceNumber.Equals(ackSequenceNumber.Inc()) {
					break
				}

				ackSequenceNumber = p.Header().PacketSequenceNumber
				maxPktTsbpdTime = p.Header().PktTsbpdTime
			}
		}

		ok = true
		sequenceNumber = ackSequenceNumber.Inc()

		// keep track of the last ACK's sequence. with this we can faster ignore
		// packets that come in that have a lower sequence number.
		r.lastACKSequenceNumber = ackSequenceNumber
	}

	r.lastPeriodicACK = now
	r.nPackets = 0

	r.statistics.MsRcvBuf = (maxPktTsbpdTime - minPktTsbpdTime) / 1_000

	return
}

func (r *liveReceive) periodicNAK(now uint64) (ok bool, from, to circular.Number) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	if now-r.lastPeriodicNAK < r.periodicNAKInterval {
		return
	}

	// send a periodic NAK

	ackSequenceNumber := r.lastDeliveredSequenceNumber

	// send a NAK only for the first gap.
	// alternatively send a NAK for max. X gaps because the size of the NAK packet is limited
	for e := r.packetList.Front(); e != nil; e = e.Next() {
		p := e.Value.(packet.Packet)

		if !p.Header().PacketSequenceNumber.Equals(ackSequenceNumber.Inc()) {
			nackSequenceNumber := ackSequenceNumber.Inc()

			ok = true
			from = nackSequenceNumber
			to = p.Header().PacketSequenceNumber.Dec()

			break
		}

		ackSequenceNumber = p.Header().PacketSequenceNumber
	}

	r.lastPeriodicNAK = now

	return
}

func (r *liveReceive) Tick(now uint64) {
	if ok, sequenceNumber, lite := r.periodicACK(now); ok {
		r.sendACK(sequenceNumber, lite)
	}

	if ok, from, to := r.periodicNAK(now); ok {
		r.sendNAK(from, to)
	}

	// deliver packets whose PktTsbpdTime is ripe
	r.lock.Lock()
	defer r.lock.Unlock()

	removeList := make([]*list.Element, 0, r.packetList.Len())
	for e := r.packetList.Front(); e != nil; e = e.Next() {
		p := e.Value.(packet.Packet)

		if p.Header().PacketSequenceNumber.Lte(r.lastACKSequenceNumber) && p.Header().PktTsbpdTime <= now {
			r.statistics.PktRcvBuf--
			r.statistics.ByteRcvBuf -= p.Len()

			r.lastDeliveredSequenceNumber = p.Header().PacketSequenceNumber

			r.deliver(p)
			removeList = append(removeList, e)
		} else {
			break
		}
	}

	for _, e := range removeList {
		r.packetList.Remove(e)
	}
}

func (r *liveReceive) SetNAKInterval(nakInterval uint64) {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.periodicNAKInterval = nakInterval
}

func (r *liveReceive) String(t uint64) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("maxSeen=%d lastACK=%d lastDelivered=%d\n", r.maxSeenSequenceNumber.Val(), r.lastACKSequenceNumber.Val(), r.lastDeliveredSequenceNumber.Val()))

	r.lock.RLock()
	for e := r.packetList.Front(); e != nil; e = e.Next() {
		p := e.Value.(packet.Packet)

		b.WriteString(fmt.Sprintf("   %d @ %d (in %d)\n", p.Header().PacketSequenceNumber.Val(), p.Header().PktTsbpdTime, int64(p.Header().PktTsbpdTime)-int64(t)))
	}
	r.lock.RUnlock()

	return b.String()
}
