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
	dropThreshold      uint64

	packetList *list.List
	lossList   *list.List
	lock       sync.RWMutex

	avgPayloadSize float64 // bytes
	pktSndPeriod   float64 // microseconds
	maxBW          float64 // bytes/s
	inputBW        float64 // bytes/s
	overheadBW     float64 // percent

	statistics SendStats

	probeTime uint64

	rate struct {
		period uint64 // microseconds
		last   uint64

		bytes        uint64
		bytesSent    uint64
		bytesRetrans uint64

		estimatedInputBW float64 // bytes/s
		estimatedSentBW  float64 // bytes/s

		pktLossRate float64
	}

	deliver func(p packet.Packet)
}

// NewLiveSend takes a SendConfig and returns a new Sender
func NewLiveSend(config SendConfig) Sender {
	s := &liveSend{
		nextSequenceNumber: config.InitialSequenceNumber,
		dropThreshold:      config.DropThreshold,
		packetList:         list.New(),
		lossList:           list.New(),

		avgPayloadSize: packet.MAX_PAYLOAD_SIZE, //  5.1.2. SRT's Default LiveCC Algorithm
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

	s.rate.period = uint64(time.Second.Microseconds())
	s.rate.last = 0

	return s
}

func (s *liveSend) Stats() SendStats {
	s.lock.RLock()
	defer s.lock.RUnlock()

	s.statistics.UsPktSndPeriod = s.pktSndPeriod
	s.statistics.BytePayload = uint64(s.avgPayloadSize)
	s.statistics.MsBuf = 0

	max := s.lossList.Back()
	min := s.lossList.Front()

	if max != nil && min != nil {
		s.statistics.MsBuf = (max.Value.(packet.Packet).Header().PktTsbpdTime - min.Value.(packet.Packet).Header().PktTsbpdTime) / 1_000
	}

	s.statistics.MbpsEstimatedInputBandwidth = s.rate.estimatedInputBW * 8 / 1024 / 1024
	s.statistics.MbpsEstimatedSentBandwidth = s.rate.estimatedSentBW * 8 / 1024 / 1024

	s.statistics.PktLossRate = s.rate.pktLossRate

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

	// Give to the packet a sequence number
	p.Header().PacketSequenceNumber = s.nextSequenceNumber
	p.Header().PacketPositionFlag = packet.SinglePacket
	p.Header().OrderFlag = false
	p.Header().MessageNumber = 1

	s.nextSequenceNumber = s.nextSequenceNumber.Inc()

	pktLen := p.Len()

	s.statistics.PktBuf++
	s.statistics.ByteBuf += pktLen

	// Input bandwidth calculation
	s.rate.bytes += pktLen

	p.Header().Timestamp = uint32(p.Header().PktTsbpdTime & uint64(packet.MAX_TIMESTAMP))

	// Every 16th and 17th packet should be sent at the same time in order
	// for the receiver to determine the link capacity. Not really well
	// documented in the specs.
	// PktTsbpdTime is used for the timing of sending the packets. Here we
	// can modify it because it has already been used to set the packet's
	// timestamp.
	probe := p.Header().PacketSequenceNumber.Val() & 0xF
	if probe == 0 {
		s.probeTime = p.Header().PktTsbpdTime
	} else if probe == 1 {
		p.Header().PktTsbpdTime = s.probeTime
	}

	s.packetList.PushBack(p)

	s.statistics.PktFlightSize = uint64(s.packetList.Len())
}

func (s *liveSend) Tick(now uint64) {
	// Deliver packets whose PktTsbpdTime is ripe
	s.lock.Lock()
	removeList := make([]*list.Element, 0, s.packetList.Len())
	for e := s.packetList.Front(); e != nil; e = e.Next() {
		p := e.Value.(packet.Packet)
		if p.Header().PktTsbpdTime <= now {
			s.statistics.Pkt++
			s.statistics.PktUnique++

			pktLen := p.Len()

			s.statistics.Byte += pktLen
			s.statistics.ByteUnique += pktLen

			s.statistics.UsSndDuration += uint64(s.pktSndPeriod)

			//  5.1.2. SRT's Default LiveCC Algorithm
			s.avgPayloadSize = 0.875*s.avgPayloadSize + 0.125*float64(pktLen)

			s.rate.bytesSent += pktLen

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

		if p.Header().PktTsbpdTime+s.dropThreshold <= now {
			// Dropped packet because too old
			s.statistics.PktDrop++
			s.statistics.PktLoss++
			s.statistics.ByteDrop += p.Len()
			s.statistics.ByteLoss += p.Len()

			removeList = append(removeList, e)
		}
	}

	// These packets are not needed anymore (too late)
	for _, e := range removeList {
		p := e.Value.(packet.Packet)

		s.statistics.PktBuf--
		s.statistics.ByteBuf -= p.Len()

		s.lossList.Remove(e)

		// This packet has been ACK'd and we don't need it anymore
		p.Decommission()
	}
	s.lock.Unlock()

	s.lock.Lock()
	tdiff := now - s.rate.last

	if tdiff > s.rate.period {
		s.rate.estimatedInputBW = float64(s.rate.bytes) / (float64(tdiff) / 1000 / 1000)
		s.rate.estimatedSentBW = float64(s.rate.bytesSent) / (float64(tdiff) / 1000 / 1000)
		if s.rate.bytesSent != 0 {
			s.rate.pktLossRate = float64(s.rate.bytesRetrans) / float64(s.rate.bytesSent) * 100
		} else {
			s.rate.pktLossRate = 0
		}

		s.rate.bytes = 0
		s.rate.bytesSent = 0
		s.rate.bytesRetrans = 0

		s.rate.last = now
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
			// Remove packet from buffer because it has been successfully transmitted
			removeList = append(removeList, e)
		} else {
			break
		}
	}

	// These packets are not needed anymore (ACK'd)
	for _, e := range removeList {
		p := e.Value.(packet.Packet)

		s.statistics.PktBuf--
		s.statistics.ByteBuf -= p.Len()

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
				s.statistics.Pkt++
				s.statistics.PktLoss++

				s.statistics.ByteRetrans += p.Len()
				s.statistics.Byte += p.Len()
				s.statistics.ByteLoss += p.Len()

				//  5.1.2. SRT's Default LiveCC Algorithm
				s.avgPayloadSize = 0.875*s.avgPayloadSize + 0.125*float64(p.Len())

				s.rate.bytesSent += p.Len()
				s.rate.bytesRetrans += p.Len()

				p.Header().RetransmittedPacketFlag = true
				s.deliver(p)
			}
		}
	}
}

func (s *liveSend) SetDropThreshold(threshold uint64) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.dropThreshold = threshold
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

	avgPayloadSize  float64 // bytes
	avgLinkCapacity float64 // packets per second

	probeTime    time.Time
	probeNextSeq circular.Number

	statistics ReceiveStats

	rate struct {
		last   uint64 // microseconds
		period uint64

		packets      uint64
		bytes        uint64
		bytesRetrans uint64

		packetsPerSecond float64
		bytesPerSecond   float64

		pktLossRate float64
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

	r.rate.last = 0
	r.rate.period = uint64(time.Second.Microseconds())

	return r
}

func (r *liveReceive) Stats() ReceiveStats {
	r.lock.RLock()
	defer r.lock.RUnlock()

	r.statistics.BytePayload = uint64(r.avgPayloadSize)
	r.statistics.MbpsEstimatedRecvBandwidth = r.rate.bytesPerSecond * 8 / 1024 / 1024
	r.statistics.MbpsEstimatedLinkCapacity = r.avgLinkCapacity * packet.MAX_PAYLOAD_SIZE * 8 / 1024 / 1024
	r.statistics.PktLossRate = r.rate.pktLossRate

	return r.statistics
}

func (r *liveReceive) PacketRate() (pps, bps, capacity float64) {
	r.lock.Lock()
	defer r.lock.Unlock()

	pps = r.rate.packetsPerSecond
	bps = r.rate.bytesPerSecond
	capacity = r.avgLinkCapacity

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

	// This is not really well (not at all) described in the specs. See core.cpp and window.h
	// and search for PUMASK_SEQNO_PROBE (0xF). Every 16th and 17th packet are
	// sent in pairs. This is used as a probe for the theoretical capacity of the link.
	if !pkt.Header().RetransmittedPacketFlag {
		probe := pkt.Header().PacketSequenceNumber.Val() & 0xF
		if probe == 0 {
			r.probeTime = time.Now()
			r.probeNextSeq = pkt.Header().PacketSequenceNumber.Inc()
		} else if probe == 1 && pkt.Header().PacketSequenceNumber.Equals(r.probeNextSeq) && !r.probeTime.IsZero() && pkt.Len() != 0 {
			// The time between packets scaled to a fully loaded packet
			diff := float64(time.Since(r.probeTime).Microseconds()) * (packet.MAX_PAYLOAD_SIZE / float64(pkt.Len()))
			if diff != 0 {
				// Here we're doing an average of the measurements.
				r.avgLinkCapacity = 0.875*r.avgLinkCapacity + 0.125*1_000_000/diff
			}
		} else {
			r.probeTime = time.Time{}
		}
	} else {
		r.probeTime = time.Time{}
	}

	r.nPackets++

	pktLen := pkt.Len()

	r.rate.packets++
	r.rate.bytes += pktLen

	r.statistics.Pkt++
	r.statistics.Byte += pktLen

	//pkt.PktTsbpdTime = pkt.Timestamp + r.delay
	if pkt.Header().RetransmittedPacketFlag {
		r.statistics.PktRetrans++
		r.statistics.ByteRetrans += pktLen

		r.rate.bytesRetrans += pktLen
	}

	//  5.1.2. SRT's Default LiveCC Algorithm
	r.avgPayloadSize = 0.875*r.avgPayloadSize + 0.125*float64(pktLen)

	if pkt.Header().PacketSequenceNumber.Lte(r.lastDeliveredSequenceNumber) {
		// Too old, because up until r.lastDeliveredSequenceNumber, we already delivered
		r.statistics.PktBelated++
		r.statistics.ByteBelated += pktLen

		r.statistics.PktDrop++
		r.statistics.ByteDrop += pktLen

		return
	}

	if pkt.Header().PacketSequenceNumber.Lt(r.lastACKSequenceNumber) {
		// Already acknowledged, ignoring
		r.statistics.PktDrop++
		r.statistics.ByteDrop += pktLen

		return
	}

	if pkt.Header().PacketSequenceNumber.Equals(r.maxSeenSequenceNumber.Inc()) {
		// In order, the packet we expected
		r.maxSeenSequenceNumber = pkt.Header().PacketSequenceNumber
	} else if pkt.Header().PacketSequenceNumber.Lte(r.maxSeenSequenceNumber) {
		// Out of order, is it a missing piece? put it in the correct position
		for e := r.packetList.Front(); e != nil; e = e.Next() {
			p := e.Value.(packet.Packet)

			if p.Header().PacketSequenceNumber == pkt.Header().PacketSequenceNumber {
				// Already received (has been sent more than once), ignoring
				r.statistics.PktDrop++
				r.statistics.ByteDrop += pktLen

				break
			} else if p.Header().PacketSequenceNumber.Gt(pkt.Header().PacketSequenceNumber) {
				// Late arrival, this fills a gap
				r.statistics.PktBuf++
				r.statistics.PktUnique++

				r.statistics.ByteBuf += pktLen
				r.statistics.ByteUnique += pktLen

				r.packetList.InsertBefore(pkt, e)

				break
			}
		}

		return
	} else {
		// Too far ahead, there are some missing sequence numbers, immediate NAK report
		// here we can prevent a possibly unnecessary NAK with SRTO_LOXXMAXTTL
		r.sendNAK(r.maxSeenSequenceNumber.Inc(), pkt.Header().PacketSequenceNumber.Dec())

		len := uint64(pkt.Header().PacketSequenceNumber.Distance(r.maxSeenSequenceNumber))
		r.statistics.PktLoss += len
		r.statistics.ByteLoss += len * uint64(r.avgPayloadSize)

		r.maxSeenSequenceNumber = pkt.Header().PacketSequenceNumber
	}

	r.statistics.PktBuf++
	r.statistics.PktUnique++

	r.statistics.ByteBuf += pktLen
	r.statistics.ByteUnique += pktLen

	r.packetList.PushBack(pkt)
}

func (r *liveReceive) periodicACK(now uint64) (ok bool, sequenceNumber circular.Number, lite bool) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	// 4.8.1. Packet Acknowledgement (ACKs, ACKACKs)
	if now-r.lastPeriodicACK < r.periodicACKInterval {
		if r.nPackets >= 64 {
			lite = true // Send light ACK
		} else {
			return
		}
	}

	minPktTsbpdTime, maxPktTsbpdTime := uint64(0), uint64(0)

	ackSequenceNumber := r.lastDeliveredSequenceNumber

	// Find the sequence number up until we have all in a row.
	// Where the first gap is (or at the end of the list) is where we can ACK to.

	e := r.packetList.Front()
	if e != nil {
		p := e.Value.(packet.Packet)

		minPktTsbpdTime = p.Header().PktTsbpdTime
		maxPktTsbpdTime = p.Header().PktTsbpdTime

		// If there are packets that should be delivered by now, move foward.
		if p.Header().PktTsbpdTime <= now {
			for e = e.Next(); e != nil; e = e.Next() {
				p = e.Value.(packet.Packet)

				if p.Header().PktTsbpdTime > now {
					break
				}
			}

			ackSequenceNumber = p.Header().PacketSequenceNumber
			maxPktTsbpdTime = p.Header().PktTsbpdTime

			if e != nil {
				e = e.Next()
				p = e.Value.(packet.Packet)
			}
		}

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

		// Keep track of the last ACK's sequence. with this we can faster ignore
		// packets that come in that have a lower sequence number.
		r.lastACKSequenceNumber = ackSequenceNumber
	}

	r.lastPeriodicACK = now
	r.nPackets = 0

	r.statistics.MsBuf = (maxPktTsbpdTime - minPktTsbpdTime) / 1_000

	return
}

func (r *liveReceive) periodicNAK(now uint64) (ok bool, from, to circular.Number) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	if now-r.lastPeriodicNAK < r.periodicNAKInterval {
		return
	}

	// Send a periodic NAK

	ackSequenceNumber := r.lastDeliveredSequenceNumber

	// Send a NAK only for the first gap.
	// Alternatively send a NAK for max. X gaps because the size of the NAK packet is limited.
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

	// Deliver packets whose PktTsbpdTime is ripe
	r.lock.Lock()
	removeList := make([]*list.Element, 0, r.packetList.Len())
	for e := r.packetList.Front(); e != nil; e = e.Next() {
		p := e.Value.(packet.Packet)

		if p.Header().PacketSequenceNumber.Lte(r.lastACKSequenceNumber) && p.Header().PktTsbpdTime <= now {
			r.statistics.PktBuf--
			r.statistics.ByteBuf -= p.Len()

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
	r.lock.Unlock()

	r.lock.Lock()
	tdiff := now - r.rate.last // microseconds

	if tdiff > r.rate.period {
		r.rate.packetsPerSecond = float64(r.rate.packets) / (float64(tdiff) / 1000 / 1000)
		r.rate.bytesPerSecond = float64(r.rate.bytes) / (float64(tdiff) / 1000 / 1000)
		if r.rate.bytes != 0 {
			r.rate.pktLossRate = float64(r.rate.bytesRetrans) / float64(r.rate.bytes) * 100
		} else {
			r.rate.bytes = 0
		}

		r.rate.packets = 0
		r.rate.bytes = 0
		r.rate.bytesRetrans = 0

		r.rate.last = now
	}
	r.lock.Unlock()
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

type fakeLiveReceive struct {
	maxSeenSequenceNumber       circular.Number
	lastACKSequenceNumber       circular.Number
	lastDeliveredSequenceNumber circular.Number

	nPackets uint

	periodicACKInterval uint64 // config
	periodicNAKInterval uint64 // config

	lastPeriodicACK uint64

	avgPayloadSize float64 // bytes

	rate struct {
		last   time.Time
		period time.Duration

		packets uint64
		bytes   uint64

		pps float64
		bps float64
	}

	sendACK func(seq circular.Number, light bool)
	sendNAK func(from, to circular.Number)
	deliver func(p packet.Packet)

	lock sync.RWMutex
}

func NewFakeLiveReceive(config ReceiveConfig) Receiver {
	r := &fakeLiveReceive{
		maxSeenSequenceNumber:       config.InitialSequenceNumber.Dec(),
		lastACKSequenceNumber:       config.InitialSequenceNumber.Dec(),
		lastDeliveredSequenceNumber: config.InitialSequenceNumber.Dec(),

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

func (r *fakeLiveReceive) Stats() ReceiveStats { return ReceiveStats{} }
func (r *fakeLiveReceive) PacketRate() (pps, bps, capacity float64) {
	r.lock.Lock()
	defer r.lock.Unlock()

	tdiff := time.Since(r.rate.last)

	if tdiff < r.rate.period {
		pps = r.rate.pps
		bps = r.rate.bps

		return
	}

	r.rate.pps = float64(r.rate.packets) / tdiff.Seconds()
	r.rate.bps = float64(r.rate.bytes) / tdiff.Seconds()

	r.rate.packets, r.rate.bytes = 0, 0
	r.rate.last = time.Now()

	pps = r.rate.pps
	bps = r.rate.bps

	return
}

func (r *fakeLiveReceive) Flush() {}

func (r *fakeLiveReceive) Push(pkt packet.Packet) {
	r.lock.Lock()
	defer r.lock.Unlock()

	if pkt == nil {
		return
	}

	r.nPackets++

	pktLen := pkt.Len()

	r.rate.packets++
	r.rate.bytes += pktLen

	//  5.1.2. SRT's Default LiveCC Algorithm
	r.avgPayloadSize = 0.875*r.avgPayloadSize + 0.125*float64(pktLen)

	if pkt.Header().PacketSequenceNumber.Lte(r.lastDeliveredSequenceNumber) {
		// Too old, because up until r.lastDeliveredSequenceNumber, we already delivered
		return
	}

	if pkt.Header().PacketSequenceNumber.Lt(r.lastACKSequenceNumber) {
		// Already acknowledged, ignoring
		return
	}

	if pkt.Header().PacketSequenceNumber.Lte(r.maxSeenSequenceNumber) {
		return
	}

	r.maxSeenSequenceNumber = pkt.Header().PacketSequenceNumber
}

func (r *fakeLiveReceive) periodicACK(now uint64) (ok bool, sequenceNumber circular.Number, lite bool) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	// 4.8.1. Packet Acknowledgement (ACKs, ACKACKs)
	if now-r.lastPeriodicACK < r.periodicACKInterval {
		if r.nPackets >= 64 {
			lite = true // Send light ACK
		} else {
			return
		}
	}

	ok = true
	sequenceNumber = r.maxSeenSequenceNumber.Inc()

	r.lastACKSequenceNumber = r.maxSeenSequenceNumber

	r.lastPeriodicACK = now
	r.nPackets = 0

	return
}

func (r *fakeLiveReceive) Tick(now uint64) {
	if ok, sequenceNumber, lite := r.periodicACK(now); ok {
		r.sendACK(sequenceNumber, lite)
	}

	// Deliver packets whose PktTsbpdTime is ripe
	r.lock.Lock()
	defer r.lock.Unlock()

	r.lastDeliveredSequenceNumber = r.lastACKSequenceNumber
}

func (r *fakeLiveReceive) SetNAKInterval(nakInterval uint64) {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.periodicNAKInterval = nakInterval
}
