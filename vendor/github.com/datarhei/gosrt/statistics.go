// https://github.com/Haivision/srt/blob/master/docs/API/statistics.md

package srt

import (
	"time"
)

// Statistics represents the statistics for a connection
type Statistics struct {
	MsTimeStamp uint64 // The time elapsed, in milliseconds, since the SRT socket has been created

	// Accumulated
	Accumulated StatisticsAccumulated

	// Interval
	Interval StatisticsInterval

	// Instantaneous
	Instantaneous StatisticsInstantaneous
}

type StatisticsAccumulated struct {
	PktSent          uint64 // The total number of sent DATA packets, including retransmitted packets
	PktRecv          uint64 // The total number of received DATA packets, including retransmitted packets
	PktSentUnique    uint64 // The total number of unique DATA packets sent by the SRT sender
	PktRecvUnique    uint64 // The total number of unique original, retransmitted or recovered by the packet filter DATA packets received in time, decrypted without errors and, as a result, scheduled for delivery to the upstream application by the SRT receiver.
	PktSendLoss      uint64 // The total number of data packets considered or reported as lost at the sender side. Does not correspond to the packets detected as lost at the receiver side.
	PktRecvLoss      uint64 // The total number of SRT DATA packets detected as presently missing (either reordered or lost) at the receiver side
	PktRecvLost      uint64 // The total number of actually lost (not delivered) SRT DATA packets
	PktRetrans       uint64 // The total number of retransmitted packets sent by the SRT sender
	PktRecvRetrans   uint64 // The total number of retransmitted packets registered at the receiver side
	PktSentACK       uint64 // The total number of sent ACK (Acknowledgement) control packets
	PktRecvACK       uint64 // The total number of received ACK (Acknowledgement) control packets
	PktSentNAK       uint64 // The total number of sent NAK (Negative Acknowledgement) control packets
	PktRecvNAK       uint64 // The total number of received NAK (Negative Acknowledgement) control packets
	PktSentKM        uint64 // The total number of sent KM (Key Material) control packets
	PktRecvKM        uint64 // The total number of received KM (Key Material) control packets
	PktRecvShutdown  uint64 // The total number of received SHUTDOWN control packets: the peer closed the connection on purpose
	UsSndDuration    uint64 // The total accumulated time in microseconds, during which the SRT sender has some data to transmit, including packets that have been sent, but not yet acknowledged
	PktRecvBelated   uint64 // The total number of packets that arrived too late
	PktRecvLate      uint64 // The total number of packets that arrived late (out of order) but in time
	PktSendDrop      uint64 // The total number of dropped by the SRT sender DATA packets that have no chance to be delivered in time
	PktRecvDrop      uint64 // The total number of dropped by the SRT receiver and, as a result, not delivered to the upstream application DATA packets
	PktRecvUndecrypt uint64 // The total number of packets that failed to be decrypted at the receiver side

	ByteSent          uint64 // Same as pktSent, but expressed in bytes, including payload and all the headers (IP, TCP, SRT)
	ByteRecv          uint64 // Same as pktRecv, but expressed in bytes, including payload and all the headers (IP, TCP, SRT)
	ByteSentUnique    uint64 // Same as pktSentUnique, but expressed in bytes, including payload and all the headers (IP, TCP, SRT)
	ByteRecvUnique    uint64 // Same as pktRecvUnique, but expressed in bytes, including payload and all the headers (IP, TCP, SRT)
	ByteRecvLoss      uint64 // Same as pktRecvLoss, but expressed in bytes, including payload and all the headers (IP, TCP, SRT), bytes for the presently missing (either reordered or lost) packets' payloads are estimated based on the average packet size
	ByteRetrans       uint64 // Same as pktRetrans, but expressed in bytes, including payload and all the headers (IP, TCP, SRT)
	ByteRecvRetrans   uint64 // Same as pktRecvRetrans, but expressed in bytes, including payload and all the headers (IP, TCP, SRT)
	ByteRecvBelated   uint64 // Same as pktRecvBelated, but expressed in bytes, including payload and all the headers (IP, TCP, SRT)
	ByteRecvLate      uint64 // Same as pktRecvLate, but expressed in bytes, including payload and all the headers (IP, TCP, SRT)
	ByteSendDrop      uint64 // Same as pktSendDrop, but expressed in bytes, including payload and all the headers (IP, TCP, SRT)
	ByteRecvDrop      uint64 // Same as pktRecvDrop, but expressed in bytes, including payload and all the headers (IP, TCP, SRT)
	ByteRecvUndecrypt uint64 // Same as pktRecvUndecrypt, but expressed in bytes, including payload and all the headers (IP, TCP, SRT)
}

type StatisticsInterval struct {
	MsInterval uint64 // Length of the interval, in milliseconds

	PktSent        uint64 // Number of sent DATA packets, including retransmitted packets
	PktRecv        uint64 // Number of received DATA packets, including retransmitted packets
	PktSentUnique  uint64 // Number of unique DATA packets sent by the SRT sender
	PktRecvUnique  uint64 // Number of unique original, retransmitted or recovered by the packet filter DATA packets received in time, decrypted without errors and, as a result, scheduled for delivery to the upstream application by the SRT receiver.
	PktSendLoss    uint64 // Number of data packets considered or reported as lost at the sender side. Does not correspond to the packets detected as lost at the receiver side.
	PktRecvLoss    uint64 // Number of SRT DATA packets detected as presently missing (either reordered or lost) at the receiver side
	PktRecvLost    uint64 // Number of actually lost (not delivered) SRT DATA packets
	PktRetrans     uint64 // Number of retransmitted packets sent by the SRT sender
	PktRecvRetrans uint64 // Number of retransmitted packets registered at the receiver side
	PktSentACK     uint64 // Number of sent ACK (Acknowledgement) control packets
	PktRecvACK     uint64 // Number of received ACK (Acknowledgement) control packets
	PktSentNAK     uint64 // Number of sent NAK (Negative Acknowledgement) control packets
	PktRecvNAK     uint64 // Number of received NAK (Negative Acknowledgement) control packets

	MbpsSendRate float64 // Sending rate, in Mbps
	MbpsRecvRate float64 // Receiving rate, in Mbps

	UsSndDuration uint64 // Accumulated time in microseconds, during which the SRT sender has some data to transmit, including packets that have been sent, but not yet acknowledged

	PktReorderDistance uint64
	PktRecvBelated     uint64 // Number of packets that arrived too late
	PktRecvLate        uint64 // Number of packets that arrived late (out of order) but in time
	PktSndDrop         uint64 // Number of dropped by the SRT sender DATA packets that have no chance to be delivered in time
	PktRecvDrop        uint64 // Number of dropped by the SRT receiver and, as a result, not delivered to the upstream application DATA packets
	PktRecvUndecrypt   uint64 // Number of packets that failed to be decrypted at the receiver side

	ByteSent          uint64 // Same as pktSent, but expressed in bytes, including payload and all the headers (IP, TCP, SRT)
	ByteRecv          uint64 // Same as pktRecv, but expressed in bytes, including payload and all the headers (IP, TCP, SRT)
	ByteSentUnique    uint64 // Same as pktSentUnique, but expressed in bytes, including payload and all the headers (IP, TCP, SRT)
	ByteRecvUnique    uint64 // Same as pktRecvUnique, but expressed in bytes, including payload and all the headers (IP, TCP, SRT)
	ByteRecvLoss      uint64 // Same as pktRecvLoss, but expressed in bytes, including payload and all the headers (IP, TCP, SRT), bytes for the presently missing (either reordered or lost) packets' payloads are estimated based on the average packet size
	ByteRetrans       uint64 // Same as pktRetrans, but expressed in bytes, including payload and all the headers (IP, TCP, SRT)
	ByteRecvRetrans   uint64 // Same as pktRecvRetrans, but expressed in bytes, including payload and all the headers (IP, TCP, SRT)
	ByteRecvBelated   uint64 // Same as pktRecvBelated, but expressed in bytes, including payload and all the headers (IP, TCP, SRT)
	ByteRecvLate      uint64 // Same as pktRecvLate, but expressed in bytes, including payload and all the headers (IP, TCP, SRT)
	ByteSendDrop      uint64 // Same as pktSendDrop, but expressed in bytes, including payload and all the headers (IP, TCP, SRT)
	ByteRecvDrop      uint64 // Same as pktRecvDrop, but expressed in bytes, including payload and all the headers (IP, TCP, SRT)
	ByteRecvUndecrypt uint64 // Same as pktRecvUndecrypt, but expressed in bytes, including payload and all the headers (IP, TCP, SRT)
}

type StatisticsInstantaneous struct {
	UsPktSendPeriod       float64 // Current minimum time interval between which consecutive packets are sent, in microseconds
	PktFlowWindow         uint64  // The maximum number of packets that can be "in flight"
	PktFlightSize         uint64  // The number of packets in flight
	MsRTT                 float64 // Smoothed round-trip time (SRTT), an exponentially-weighted moving average (EWMA) of an endpoint's RTT samples, in milliseconds
	MbpsSentRate          float64 // Current transmission bandwidth, in Mbps
	MbpsRecvRate          float64 // Current receiving bandwidth, in Mbps
	MbpsLinkCapacity      float64 // Estimated capacity of the network link, in Mbps
	ByteAvailSendBuf      uint64  // The available space in the sender's buffer, in bytes
	ByteAvailRecvBuf      uint64  // The available space in the receiver's buffer, in bytes
	MbpsMaxBW             float64 // Transmission bandwidth limit, in Mbps
	ByteMSS               uint64  // Maximum Segment Size (MSS), in bytes
	PktSendBuf            uint64  // The number of packets in the sender's buffer that are already scheduled for sending or even possibly sent, but not yet acknowledged
	ByteSendBuf           uint64  // Instantaneous (current) value of pktSndBuf, but expressed in bytes, including payload and all headers (IP, TCP, SRT)
	MsSendBuf             uint64  // The timespan (msec) of packets in the sender's buffer (unacknowledged packets)
	MsSendTsbPdDelay      uint64  // Timestamp-based Packet Delivery Delay value of the peer
	PktRecvBuf            uint64  // The number of acknowledged packets in receiver's buffer
	ByteRecvBuf           uint64  // Instantaneous (current) value of pktRcvBuf, expressed in bytes, including payload and all headers (IP, TCP, SRT)
	MsRecvBuf             uint64  // The timespan (msec) of acknowledged packets in the receiver's buffer
	MsRecvTsbPdDelay      uint64  // Timestamp-based Packet Delivery Delay value set on the socket via SRTO_RCVLATENCY or SRTO_LATENCY
	PktReorderTolerance   uint64  // Instant value of the packet reorder tolerance
	PktRecvAvgBelatedTime uint64  // Accumulated difference between the current time and the time-to-play of a packet that is received late
	PktSendLossRate       float64 // Percentage of resent data vs. sent data
	PktRecvLossRate       float64 // Percentage of retransmitted data vs. received data
}

func (c *srtConn) Stats(s *Statistics) {
	if s == nil {
		return
	}

	now := uint64(time.Since(c.start).Milliseconds())

	send := c.snd.Stats()
	recv := c.recv.Stats()

	previous := s.Accumulated
	interval := now - s.MsTimeStamp

	c.statisticsLock.RLock()
	defer c.statisticsLock.RUnlock()

	// Accumulated
	s.Accumulated = StatisticsAccumulated{
		PktSent:           send.Pkt,
		PktRecv:           recv.Pkt,
		PktSentUnique:     send.PktUnique,
		PktRecvUnique:     recv.PktUnique,
		PktSendLoss:       send.PktLoss,
		PktRecvLoss:       recv.PktLoss,
		PktRecvLost:       recv.PktLost,
		PktRecvBelated:    recv.PktBelated,
		PktRecvLate:       recv.PktLate,
		PktRetrans:        send.PktRetrans,
		PktRecvRetrans:    recv.PktRetrans,
		PktSentACK:        c.statistics.pktSentACK,
		PktRecvACK:        c.statistics.pktRecvACK,
		PktSentNAK:        c.statistics.pktSentNAK,
		PktRecvNAK:        c.statistics.pktRecvNAK,
		PktSentKM:         c.statistics.pktSentKM,
		PktRecvKM:         c.statistics.pktRecvKM,
		PktRecvShutdown:   c.statistics.pktRecvShutdown,
		UsSndDuration:     send.UsSndDuration,
		PktSendDrop:       send.PktDrop,
		PktRecvDrop:       recv.PktDrop,
		PktRecvUndecrypt:  c.statistics.pktRecvUndecrypt,
		ByteSent:          send.Byte + (send.Pkt * c.statistics.headerSize),
		ByteRecv:          recv.Byte + (recv.Pkt * c.statistics.headerSize),
		ByteSentUnique:    send.ByteUnique + (send.PktUnique * c.statistics.headerSize),
		ByteRecvUnique:    recv.ByteUnique + (recv.PktUnique * c.statistics.headerSize),
		ByteRecvLoss:      recv.ByteLoss + (recv.PktLoss * c.statistics.headerSize),
		ByteRecvBelated:   recv.ByteBelated + (recv.PktBelated * c.statistics.headerSize),
		ByteRecvLate:      recv.ByteLate + (recv.PktBelated * c.statistics.headerSize),
		ByteRetrans:       send.ByteRetrans + (send.PktRetrans * c.statistics.headerSize),
		ByteRecvRetrans:   recv.ByteRetrans + (recv.PktRetrans * c.statistics.headerSize),
		ByteSendDrop:      send.ByteDrop + (send.PktDrop * c.statistics.headerSize),
		ByteRecvDrop:      recv.ByteDrop + (recv.PktDrop * c.statistics.headerSize),
		ByteRecvUndecrypt: c.statistics.byteRecvUndecrypt + (c.statistics.pktRecvUndecrypt * c.statistics.headerSize),
	}

	// Interval
	s.Interval = StatisticsInterval{
		MsInterval:         interval,
		PktSent:            s.Accumulated.PktSent - previous.PktSent,
		PktRecv:            s.Accumulated.PktRecv - previous.PktRecv,
		PktSentUnique:      s.Accumulated.PktSentUnique - previous.PktSentUnique,
		PktRecvUnique:      s.Accumulated.PktRecvUnique - previous.PktRecvUnique,
		PktSendLoss:        s.Accumulated.PktSendLoss - previous.PktSendLoss,
		PktRecvLoss:        s.Accumulated.PktRecvLoss - previous.PktRecvLoss,
		PktRecvLost:        s.Accumulated.PktRecvLost - previous.PktRecvLost,
		PktRetrans:         s.Accumulated.PktRetrans - previous.PktRetrans,
		PktRecvRetrans:     s.Accumulated.PktRecvRetrans - previous.PktRecvRetrans,
		PktSentACK:         s.Accumulated.PktSentACK - previous.PktSentACK,
		PktRecvACK:         s.Accumulated.PktRecvACK - previous.PktRecvACK,
		PktSentNAK:         s.Accumulated.PktSentNAK - previous.PktSentNAK,
		PktRecvNAK:         s.Accumulated.PktRecvNAK - previous.PktRecvNAK,
		MbpsSendRate:       float64(s.Accumulated.ByteSent-previous.ByteSent) * 8 / 1024 / 1024 / (float64(interval) / 1000),
		MbpsRecvRate:       float64(s.Accumulated.ByteRecv-previous.ByteRecv) * 8 / 1024 / 1024 / (float64(interval) / 1000),
		UsSndDuration:      s.Accumulated.UsSndDuration - previous.UsSndDuration,
		PktReorderDistance: 0,
		PktRecvBelated:     s.Accumulated.PktRecvBelated - previous.PktRecvBelated,
		PktRecvLate:        s.Accumulated.PktRecvLate - previous.PktRecvLate,
		PktSndDrop:         s.Accumulated.PktSendDrop - previous.PktSendDrop,
		PktRecvDrop:        s.Accumulated.PktRecvDrop - previous.PktRecvDrop,
		PktRecvUndecrypt:   s.Accumulated.PktRecvUndecrypt - previous.PktRecvUndecrypt,
		ByteSent:           s.Accumulated.ByteSent - previous.ByteSent,
		ByteRecv:           s.Accumulated.ByteRecv - previous.ByteRecv,
		ByteSentUnique:     s.Accumulated.ByteSentUnique - previous.ByteSentUnique,
		ByteRecvUnique:     s.Accumulated.ByteRecvUnique - previous.ByteRecvUnique,
		ByteRecvLoss:       s.Accumulated.ByteRecvLoss - previous.ByteRecvLoss,
		ByteRetrans:        s.Accumulated.ByteRetrans - previous.ByteRetrans,
		ByteRecvRetrans:    s.Accumulated.ByteRecvRetrans - previous.ByteRecvRetrans,
		ByteRecvBelated:    s.Accumulated.ByteRecvBelated - previous.ByteRecvBelated,
		ByteRecvLate:       s.Accumulated.ByteRecvLate - previous.ByteRecvLate,
		ByteSendDrop:       s.Accumulated.ByteSendDrop - previous.ByteSendDrop,
		ByteRecvDrop:       s.Accumulated.ByteRecvDrop - previous.ByteRecvDrop,
		ByteRecvUndecrypt:  s.Accumulated.ByteRecvUndecrypt - previous.ByteRecvUndecrypt,
	}

	// Instantaneous
	s.Instantaneous = StatisticsInstantaneous{
		UsPktSendPeriod:       send.UsPktSndPeriod,
		PktFlowWindow:         uint64(c.config.FC),
		PktFlightSize:         send.PktFlightSize,
		MsRTT:                 c.rtt.RTT() / 1000,
		MbpsSentRate:          send.MbpsEstimatedSentBandwidth,
		MbpsRecvRate:          recv.MbpsEstimatedRecvBandwidth,
		MbpsLinkCapacity:      recv.MbpsEstimatedLinkCapacity,
		ByteAvailSendBuf:      0, // unlimited
		ByteAvailRecvBuf:      0, // unlimited
		MbpsMaxBW:             float64(c.config.MaxBW) / 1024 / 1024,
		ByteMSS:               uint64(c.config.MSS),
		PktSendBuf:            send.PktBuf,
		ByteSendBuf:           send.ByteBuf,
		MsSendBuf:             send.MsBuf,
		MsSendTsbPdDelay:      c.peerTsbpdDelay / 1000,
		PktRecvBuf:            recv.PktBuf,
		ByteRecvBuf:           recv.ByteBuf,
		MsRecvBuf:             recv.MsBuf,
		MsRecvTsbPdDelay:      c.tsbpdDelay / 1000,
		PktReorderTolerance:   uint64(c.config.LossMaxTTL),
		PktRecvAvgBelatedTime: 0,
		PktSendLossRate:       send.PktLossRate,
		PktRecvLossRate:       recv.PktLossRate,
	}

	// If we're only sending, the receiver congestion control value for the link capacity is zero,
	// use the value that we got from the receiver via the ACK packets.
	if s.Instantaneous.MbpsLinkCapacity == 0 {
		s.Instantaneous.MbpsLinkCapacity = c.statistics.mbpsLinkCapacity
	}

	if c.config.MaxBW < 0 {
		s.Instantaneous.MbpsMaxBW = -1
	}

	s.MsTimeStamp = now
}
