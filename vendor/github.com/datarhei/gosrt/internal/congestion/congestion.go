// Package congestions provides congestion control implementations for SRT
package congestion

import (
	"github.com/datarhei/gosrt/internal/circular"
	"github.com/datarhei/gosrt/internal/packet"
)

// SendConfig is the configuration for the liveSend congestion control
type SendConfig struct {
	InitialSequenceNumber circular.Number
	DropInterval          uint64
	MaxBW                 int64
	InputBW               int64
	MinInputBW            int64
	OverheadBW            int64
	OnDeliver             func(p packet.Packet)
}

// Sender is the sending part of the congestion control
type Sender interface {
	Stats() SendStats
	Flush()
	Push(p packet.Packet)
	Tick(now uint64)
	ACK(sequenceNumber circular.Number)
	NAK(sequenceNumbers []circular.Number)
}

// ReceiveConfig is the configuration for the liveResv congestion control
type ReceiveConfig struct {
	InitialSequenceNumber circular.Number
	PeriodicACKInterval   uint64 // microseconds
	PeriodicNAKInterval   uint64 // microseconds
	OnSendACK             func(seq circular.Number, light bool)
	OnSendNAK             func(from, to circular.Number)
	OnDeliver             func(p packet.Packet)
}

// Receiver is the receiving part of the congestion control
type Receiver interface {
	Stats() ReceiveStats
	PacketRate() (pps, bps uint32)
	Flush()
	Push(pkt packet.Packet)
	Tick(now uint64)
	SetNAKInterval(nakInterval uint64)
}

// SendStats are collected statistics from liveSend
type SendStats struct {
	PktSent  uint64
	ByteSent uint64

	PktSentUnique  uint64
	ByteSentUnique uint64

	PktSndLoss  uint64
	ByteSndLoss uint64

	PktRetrans  uint64
	ByteRetrans uint64

	UsSndDuration uint64 // microseconds

	PktSndDrop  uint64
	ByteSndDrop uint64

	// instantaneous
	PktSndBuf  uint64
	ByteSndBuf uint64
	MsSndBuf   uint64

	PktFlightSize uint64

	UsPktSndPeriod float64 // microseconds
	BytePayload    uint64
}

// ReceiveStats are collected statistics from liveRecv
type ReceiveStats struct {
	PktRecv  uint64
	ByteRecv uint64

	PktRecvUnique  uint64
	ByteRecvUnique uint64

	PktRcvLoss  uint64
	ByteRcvLoss uint64

	PktRcvRetrans  uint64
	ByteRcvRetrans uint64

	PktRcvDrop  uint64
	ByteRcvDrop uint64

	// instantaneous
	PktRcvBuf  uint64
	ByteRcvBuf uint64
	MsRcvBuf   uint64

	BytePayload uint64
}
