package fec

import (
	"encoding/binary"
	"fmt"

	"github.com/datarhei/gosrt/circular"
	"github.com/datarhei/gosrt/packet"
)

// ExtraHeaderSize is the size of the extra FEC header in bytes.
const ExtraHeaderSize = 4

// ControlPacket holds the parsed fields of an FEC control packet.
type ControlPacket struct {
	SNBase         uint32
	TimestampRecov uint32
	GroupIndex     int8
	FlagsRecov     uint8
	LengthRecov    uint16
	PayloadRecov   []byte
}

// IsControlPacket checks if a packet is an FEC control packet.
func IsControlPacket(p packet.Packet) bool {
	return !p.Header().IsControlPacket && p.Header().MessageNumber == 0
}

// ParseControlPacket parses a gosrt packet into an FEC ControlPacket.
func ParseControlPacket(p packet.Packet) (*ControlPacket, error) {
	if !IsControlPacket(p) {
		return nil, fmt.Errorf("not an FEC control packet")
	}

	payload := p.Data()
	if len(payload) < ExtraHeaderSize {
		return nil, fmt.Errorf("FEC control packet payload too small")
	}

	ctrl := &ControlPacket{
		SNBase:         p.Header().PacketSequenceNumber.Val(),
		TimestampRecov: p.Header().Timestamp, // In FEC packets, Timestamp is Timestamp Recovery
		GroupIndex:     int8(payload[0]),
		FlagsRecov:     payload[1],
		LengthRecov:    binary.BigEndian.Uint16(payload[2:4]),
		PayloadRecov:   make([]byte, len(payload)-ExtraHeaderSize),
	}

	copy(ctrl.PayloadRecov, payload[ExtraHeaderSize:])
	return ctrl, nil
}

// WriteControlPacket builds an FEC control packet and returns it.
func WriteControlPacket(destSocketId uint32, ctrl *ControlPacket) packet.Packet {
	p := packet.NewPacket(nil)
	p.Header().IsControlPacket = false
	p.Header().PacketSequenceNumber = circular.New(ctrl.SNBase, packet.MAX_SEQUENCENUMBER)
	p.Header().MessageNumber = 0
	p.Header().Timestamp = ctrl.TimestampRecov
	p.Header().DestinationSocketId = destSocketId

	// Flags for FEC control packet according to spec
	p.Header().PacketPositionFlag = packet.SinglePacket         // 11b
	p.Header().OrderFlag = true                                 // Live mode typical
	p.Header().KeyBaseEncryptionFlag = packet.UnencryptedPacket // 00b
	p.Header().RetransmittedPacketFlag = true                   // Must be 1 to prevent reordering logic

	payload := make([]byte, ExtraHeaderSize+len(ctrl.PayloadRecov))

	// Write Extra FEC header
	payload[0] = byte(ctrl.GroupIndex)
	payload[1] = ctrl.FlagsRecov
	binary.BigEndian.PutUint16(payload[2:4], ctrl.LengthRecov)

	// Write payload recovery
	copy(payload[4:], ctrl.PayloadRecov)

	p.SetData(payload)

	return p
}
