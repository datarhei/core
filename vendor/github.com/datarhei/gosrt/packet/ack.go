package packet

import (
	"encoding/binary"
	"fmt"
	"io"
	"strings"

	"github.com/datarhei/gosrt/circular"
)

// 3.2.4.  ACK (Acknowledgment)

// CIFACK represents an ACK message.
type CIFACK struct {
	IsLite                      bool
	IsSmall                     bool
	LastACKPacketSequenceNumber circular.Number
	RTT                         uint32 // microseconds
	RTTVar                      uint32 // microseconds
	AvailableBufferSize         uint32 // bytes
	PacketsReceivingRate        uint32 // packets/s
	EstimatedLinkCapacity       uint32
	ReceivingRate               uint32 // bytes/s
}

func (c CIFACK) String() string {
	var b strings.Builder

	ackType := "full"
	if c.IsLite {
		ackType = "lite"
	} else if c.IsSmall {
		ackType = "small"
	}

	fmt.Fprintf(&b, "--- ACK (type: %s) ---\n", ackType)

	fmt.Fprintf(&b, "   lastACKPacketSequenceNumber: %#08x (%d)\n", c.LastACKPacketSequenceNumber.Val(), c.LastACKPacketSequenceNumber.Val())

	if !c.IsLite {
		fmt.Fprintf(&b, "   rtt: %#08x (%dus)\n", c.RTT, c.RTT)
		fmt.Fprintf(&b, "   rttVar: %#08x (%dus)\n", c.RTTVar, c.RTTVar)
		fmt.Fprintf(&b, "   availableBufferSize: %#08x\n", c.AvailableBufferSize)
		fmt.Fprintf(&b, "   packetsReceivingRate: %#08x\n", c.PacketsReceivingRate)
		fmt.Fprintf(&b, "   estimatedLinkCapacity: %#08x\n", c.EstimatedLinkCapacity)
		fmt.Fprintf(&b, "   receivingRate: %#08x\n", c.ReceivingRate)
	}

	fmt.Fprintf(&b, "--- /ACK ---")

	return b.String()
}

func (c *CIFACK) Unmarshal(data []byte) error {
	c.IsLite = false
	c.IsSmall = false

	if len(data) == 4 {
		c.IsLite = true

		c.LastACKPacketSequenceNumber = circular.New(binary.BigEndian.Uint32(data[0:])&MAX_SEQUENCENUMBER, MAX_SEQUENCENUMBER)

		return nil
	} else if len(data) == 16 {
		c.IsSmall = true

		c.LastACKPacketSequenceNumber = circular.New(binary.BigEndian.Uint32(data[0:])&MAX_SEQUENCENUMBER, MAX_SEQUENCENUMBER)
		c.RTT = binary.BigEndian.Uint32(data[4:])
		c.RTTVar = binary.BigEndian.Uint32(data[8:])
		c.AvailableBufferSize = binary.BigEndian.Uint32(data[12:])

		return nil
	}

	if len(data) < 28 {
		return fmt.Errorf("data too short to unmarshal")
	}

	c.LastACKPacketSequenceNumber = circular.New(binary.BigEndian.Uint32(data[0:])&MAX_SEQUENCENUMBER, MAX_SEQUENCENUMBER)
	c.RTT = binary.BigEndian.Uint32(data[4:])
	c.RTTVar = binary.BigEndian.Uint32(data[8:])
	c.AvailableBufferSize = binary.BigEndian.Uint32(data[12:])
	c.PacketsReceivingRate = binary.BigEndian.Uint32(data[16:])
	c.EstimatedLinkCapacity = binary.BigEndian.Uint32(data[20:])
	c.ReceivingRate = binary.BigEndian.Uint32(data[24:])

	return nil
}

func (c *CIFACK) Marshal(w io.Writer) error {
	if w == nil {
		return fmt.Errorf("invalid writer")
	}

	var buffer [28]byte

	binary.BigEndian.PutUint32(buffer[0:], c.LastACKPacketSequenceNumber.Val())
	binary.BigEndian.PutUint32(buffer[4:], c.RTT)
	binary.BigEndian.PutUint32(buffer[8:], c.RTTVar)
	binary.BigEndian.PutUint32(buffer[12:], c.AvailableBufferSize)
	binary.BigEndian.PutUint32(buffer[16:], c.PacketsReceivingRate)
	binary.BigEndian.PutUint32(buffer[20:], c.EstimatedLinkCapacity)
	binary.BigEndian.PutUint32(buffer[24:], c.ReceivingRate)

	if c.IsLite {
		w.Write(buffer[0:4])
	} else if c.IsSmall {
		w.Write(buffer[0:16])
	} else {
		w.Write(buffer[0:])
	}

	return nil
}
