// Package packet provides types and implementations for the different SRT packet types
package packet

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"

	"github.com/datarhei/gosrt/circular"
)

const MAX_SEQUENCENUMBER uint32 = 0b01111111_11111111_11111111_11111111
const MAX_TIMESTAMP uint32 = 0b11111111_11111111_11111111_11111111
const MAX_PAYLOAD_SIZE = 1456

// Table 1: SRT Control Packet Types
type CtrlType uint16

const (
	CTRLTYPE_HANDSHAKE CtrlType = 0x0000
	CTRLTYPE_KEEPALIVE CtrlType = 0x0001
	CTRLTYPE_ACK       CtrlType = 0x0002
	CTRLTYPE_NAK       CtrlType = 0x0003
	CTRLTYPE_WARN      CtrlType = 0x0004 // unimplemented, receiver->sender
	CTRLTYPE_SHUTDOWN  CtrlType = 0x0005
	CTRLTYPE_ACKACK    CtrlType = 0x0006
	CRTLTYPE_DROPREQ   CtrlType = 0x0007 // unimplemented, sender->receiver
	CRTLTYPE_PEERERROR CtrlType = 0x0008 // unimplemented, receiver->sender (only for file transfers)
	CTRLTYPE_USER      CtrlType = 0x7FFF
)

func (h CtrlType) String() string {
	switch h {
	case CTRLTYPE_HANDSHAKE:
		return "HANDSHAKE"
	case CTRLTYPE_KEEPALIVE:
		return "KEEPALIVE"
	case CTRLTYPE_ACK:
		return "ACK"
	case CTRLTYPE_NAK:
		return "NAK"
	case CTRLTYPE_WARN:
		return "WARN"
	case CTRLTYPE_SHUTDOWN:
		return "SHUTDOWN"
	case CTRLTYPE_ACKACK:
		return "ACKACK"
	case CRTLTYPE_DROPREQ:
		return "DROPREQ"
	case CRTLTYPE_PEERERROR:
		return "PEERERROR"
	case CTRLTYPE_USER:
		return "USER"
	}

	return "unknown"
}

func (h CtrlType) Value() uint16 {
	return uint16(h)
}

type HandshakeType uint32

// Table 4: Handshake Type
const (
	HSTYPE_DONE       HandshakeType = 0xFFFFFFFD
	HSTYPE_AGREEMENT  HandshakeType = 0xFFFFFFFE
	HSTYPE_CONCLUSION HandshakeType = 0xFFFFFFFF
	HSTYPE_WAVEHAND   HandshakeType = 0x00000000
	HSTYPE_INDUCTION  HandshakeType = 0x00000001
)

func (h HandshakeType) String() string {
	switch h {
	case HSTYPE_DONE:
		return "DONE"
	case HSTYPE_AGREEMENT:
		return "AGREEMENT"
	case HSTYPE_CONCLUSION:
		return "CONCLUSION"
	case HSTYPE_WAVEHAND:
		return "WAVEHAND"
	case HSTYPE_INDUCTION:
		return "INDUCTION"
	}

	return "REJECT (" + strconv.FormatUint(uint64(h), 32) + ")"
}

func (h HandshakeType) IsHandshake() bool {
	switch h {
	case HSTYPE_DONE:
	case HSTYPE_AGREEMENT:
	case HSTYPE_CONCLUSION:
	case HSTYPE_WAVEHAND:
	case HSTYPE_INDUCTION:
	default:
		return false
	}

	return true
}

func (h HandshakeType) IsRejection() bool {
	return !h.IsHandshake()
}

func (h HandshakeType) Val() uint32 {
	return uint32(h)
}

// Table 6: Handshake Extension Message Flags
const (
	SRTFLAG_TSBPDSND      uint32 = 1 << 0
	SRTFLAG_TSBPDRCV      uint32 = 1 << 1
	SRTFLAG_CRYPT         uint32 = 1 << 2
	SRTFLAG_TLPKTDROP     uint32 = 1 << 3
	SRTFLAG_PERIODICNAK   uint32 = 1 << 4
	SRTFLAG_REXMITFLG     uint32 = 1 << 5
	SRTFLAG_STREAM        uint32 = 1 << 6
	SRTFLAG_PACKET_FILTER uint32 = 1 << 7
)

// Table 5: Handshake Extension Type values
type CtrlSubType uint16

const (
	CTRLSUBTYPE_NONE   CtrlSubType = 0
	EXTTYPE_HSREQ      CtrlSubType = 1
	EXTTYPE_HSRSP      CtrlSubType = 2
	EXTTYPE_KMREQ      CtrlSubType = 3
	EXTTYPE_KMRSP      CtrlSubType = 4
	EXTTYPE_SID        CtrlSubType = 5
	EXTTYPE_CONGESTION CtrlSubType = 6
	EXTTYPE_FILTER     CtrlSubType = 7 // unimplemented
	EXTTYPE_GROUP      CtrlSubType = 8 // unimplemented
)

func (h CtrlSubType) String() string {
	switch h {
	case CTRLSUBTYPE_NONE:
		return "NONE"
	case EXTTYPE_HSREQ:
		return "EXTTYPE_HSREQ"
	case EXTTYPE_HSRSP:
		return "EXTTYPE_HSRSP"
	case EXTTYPE_KMREQ:
		return "EXTTYPE_KMREQ"
	case EXTTYPE_KMRSP:
		return "EXTTYPE_KMRSP"
	case EXTTYPE_SID:
		return "EXTTYPE_SID"
	case EXTTYPE_CONGESTION:
		return "EXTTYPE_CONGESTION"
	case EXTTYPE_FILTER:
		return "EXTTYPE_FILTER"
	case EXTTYPE_GROUP:
		return "EXTTYPE_GROUP"
	}

	return "unknown"
}

func (h CtrlSubType) Value() uint16 {
	return uint16(h)
}

type Packet interface {
	// String returns a string representation of the packet.
	String() string

	// Clone clones a packet.
	Clone() Packet

	// Header returns a pointer to the packet header.
	Header() *PacketHeader

	// Data returns the payload the packets holds. The packets stays the
	// owner of the data, i.e. modifying the returned data will also
	// modify the payload.
	Data() []byte

	// SetData replaces the payload of the packet with the provided one.
	SetData([]byte)

	// Len return the length of the payload in the packet.
	Len() uint64

	// Marshal writes the bytes representation of the packet to the provided writer.
	Marshal(w io.Writer) error

	// Unmarshal parses the given data into the packet header and its payload. Returns an error on failure.
	Unmarshal(data []byte) error

	// Dump returns the same as String with an additional hex-dump of the marshalled packet.
	Dump() string

	// MarshalCIF writes the byte representation of a control information field as payload
	// of the packet. Only for control packets.
	MarshalCIF(c CIF) error

	// UnmarshalCIF parses the payload into a control information field struct. Returns an error
	// on failure.
	UnmarshalCIF(c CIF) error

	// Decommission frees the payload. The packet shouldn't be uses afterwards.
	Decommission()
}

//  3. Packet Structure

type PacketHeader struct {
	Addr            net.Addr
	LocalAddr       net.Addr
	IsControlPacket bool
	PktTsbpdTime    uint64 // microseconds

	// control packet fields

	ControlType  CtrlType    // Control Packet Type.  The use of these bits is determined by the control packet type definition.
	SubType      CtrlSubType // This field specifies an additional subtype for specific packets.
	TypeSpecific uint32      // The use of this field depends on the particular control packet type. Handshake packets do not use this field.

	// data packet fields

	PacketSequenceNumber    circular.Number  // The sequential number of the data packet.
	PacketPositionFlag      PacketPosition   // This field indicates the position of the data packet in the message. The value "10b" (binary) means the first packet of the message. "00b" indicates a packet in the middle. "01b" designates the last packet. If a single data packet forms the whole message, the value is "11b".
	OrderFlag               bool             // Indicates whether the message should be delivered by the receiver in order (1) or not (0). Certain restrictions apply depending on the data transmission mode used (Section 4.2).
	KeyBaseEncryptionFlag   PacketEncryption // The flag bits indicate whether or not data is encrypted. The value "00b" (binary) means data is not encrypted. "01b" indicates that data is encrypted with an even key, and "10b" is used for odd key encryption. Refer to Section 6.  The value "11b" is only used in control packets.
	RetransmittedPacketFlag bool             // This flag is clear when a packet is transmitted the first time. The flag is set to "1" when a packet is retransmitted.
	MessageNumber           uint32           // The sequential number of consecutive data packets that form a message (see PP field).

	// common fields

	Timestamp           uint32 // microseconds
	DestinationSocketId uint32
}

type pkt struct {
	header PacketHeader

	payload *bytes.Buffer
}

type pool struct {
	pool sync.Pool
}

func newPool() *pool {
	return &pool{
		pool: sync.Pool{
			New: func() any {
				return new(bytes.Buffer)
			},
		},
	}
}

func (p *pool) Get() *bytes.Buffer {
	b := p.pool.Get().(*bytes.Buffer)
	b.Reset()

	return b
}

func (p *pool) Put(b *bytes.Buffer) {
	p.pool.Put(b)
}

var payloadPool *pool = newPool()

func NewPacketFromData(addr net.Addr, rawdata []byte) (Packet, error) {
	p := NewPacket(addr)

	if len(rawdata) != 0 {
		if err := p.Unmarshal(rawdata); err != nil {
			p.Decommission()
			return nil, fmt.Errorf("invalid data: %w", err)
		}
	}

	return p, nil
}

func NewPacket(addr net.Addr) Packet {
	p := &pkt{
		header: PacketHeader{
			Addr:                  addr,
			PacketSequenceNumber:  circular.New(0, MAX_SEQUENCENUMBER),
			PacketPositionFlag:    SinglePacket,
			OrderFlag:             false,
			KeyBaseEncryptionFlag: UnencryptedPacket,
			MessageNumber:         1,
		},
		payload: payloadPool.Get(),
	}

	return p
}

func (p *pkt) Decommission() {
	if p.payload == nil {
		return
	}

	payloadPool.Put(p.payload)
	p.payload = nil
}

func (p pkt) String() string {
	var b strings.Builder

	fmt.Fprintf(&b, "timestamp=%#08x (%d), destId=%#08x\n", p.header.Timestamp, p.header.Timestamp, p.header.DestinationSocketId)

	if p.header.IsControlPacket {
		fmt.Fprintf(&b, "control packet:\n")
		fmt.Fprintf(&b, "   controlType=%#04x (%s)\n", p.header.ControlType.Value(), p.header.ControlType.String())
		fmt.Fprintf(&b, "   subType=%#04x (%s)\n", p.header.SubType.Value(), p.header.SubType.String())
		fmt.Fprintf(&b, "   typeSpecific=%#08x\n", p.header.TypeSpecific)
	} else {
		fmt.Fprintf(&b, "data packet:\n")
		fmt.Fprintf(&b, "   packetSequenceNumber=%#08x (%d)\n", p.header.PacketSequenceNumber.Val(), p.header.PacketSequenceNumber.Val())
		fmt.Fprintf(&b, "   packetPositionFlag=%s\n", p.header.PacketPositionFlag)
		fmt.Fprintf(&b, "   orderFlag=%v\n", p.header.OrderFlag)
		fmt.Fprintf(&b, "   keyBaseEncryptionFlag=%s\n", p.header.KeyBaseEncryptionFlag)
		fmt.Fprintf(&b, "   retransmittedPacketFlag=%v\n", p.header.RetransmittedPacketFlag)
		fmt.Fprintf(&b, "   messageNumber=%#08x (%d)\n", p.header.MessageNumber, p.header.MessageNumber)
	}

	fmt.Fprintf(&b, "data (%d bytes)", p.Len())

	return b.String()
}

func (p *pkt) Clone() Packet {
	clone := *p

	clone.payload = payloadPool.Get()
	clone.payload.Write(p.payload.Bytes())

	return &clone
}

func (p *pkt) Header() *PacketHeader {
	return &p.header
}

func (p *pkt) SetData(data []byte) {
	p.payload.Reset()
	p.payload.Write(data)
}

func (p *pkt) Data() []byte {
	return p.payload.Bytes()
}

func (p *pkt) Len() uint64 {
	return uint64(p.payload.Len())
}

func (p *pkt) Unmarshal(data []byte) error {
	if len(data) < 16 {
		return fmt.Errorf("data too short to unmarshal")
	}

	p.header.IsControlPacket = (data[0] & 0x80) != 0

	if p.header.IsControlPacket {
		p.header.ControlType = CtrlType(binary.BigEndian.Uint16(data[0:]) & ^uint16(1<<15)) // clear the first bit
		p.header.SubType = CtrlSubType(binary.BigEndian.Uint16(data[2:]))
		p.header.TypeSpecific = binary.BigEndian.Uint32(data[4:])
	} else {
		p.header.PacketSequenceNumber = circular.New(binary.BigEndian.Uint32(data[0:]), MAX_SEQUENCENUMBER)
		p.header.PacketPositionFlag = PacketPosition((data[4] & 0b11000000) >> 6)
		p.header.OrderFlag = (data[4] & 0b00100000) != 0
		p.header.KeyBaseEncryptionFlag = PacketEncryption((data[4] & 0b00011000) >> 3)
		p.header.RetransmittedPacketFlag = (data[4] & 0b00000100) != 0
		p.header.MessageNumber = binary.BigEndian.Uint32(data[4:]) & ^uint32(0b11111100<<24)
	}

	p.header.Timestamp = binary.BigEndian.Uint32(data[8:])
	p.header.DestinationSocketId = binary.BigEndian.Uint32(data[12:])

	p.payload.Reset()
	p.payload.Write(data[16:])

	return nil
}

func (p *pkt) Marshal(w io.Writer) error {
	if w == nil {
		return fmt.Errorf("invalid writer")
	}

	var buffer [16]byte

	if p.payload == nil {
		return fmt.Errorf("invalid payload")
	}

	if p.header.IsControlPacket {
		binary.BigEndian.PutUint16(buffer[0:], p.header.ControlType.Value()) // control type
		binary.BigEndian.PutUint16(buffer[2:], p.header.SubType.Value())     // sub type
		binary.BigEndian.PutUint32(buffer[4:], p.header.TypeSpecific)        // type specific

		buffer[0] |= 0x80
	} else {
		binary.BigEndian.PutUint32(buffer[0:], p.header.PacketSequenceNumber.Val()) // sequence number

		var field uint32 = 0

		field |= ((p.header.PacketPositionFlag.Val() & 0b11) << 6) // 0b11000000
		if p.header.OrderFlag {
			field |= (1 << 5) // 0b11100000
		}
		field |= ((p.header.KeyBaseEncryptionFlag.Val() & 0b11) << 3) // 0b11111000
		if p.header.RetransmittedPacketFlag {
			field |= (1 << 2) // 0b11111100
		}
		field = field << 24 // 0b11111100_00000000_00000000_00000000
		field += (p.header.MessageNumber & 0b00000011_11111111_11111111_11111111)

		binary.BigEndian.PutUint32(buffer[4:], field) // sequence number
	}

	binary.BigEndian.PutUint32(buffer[8:], p.header.Timestamp)            // timestamp
	binary.BigEndian.PutUint32(buffer[12:], p.header.DestinationSocketId) // destination socket ID

	w.Write(buffer[0:])
	w.Write(p.payload.Bytes())

	return nil
}

func (p *pkt) Dump() string {
	var data bytes.Buffer
	p.Marshal(&data)

	return p.String() + "\n" + hex.Dump(data.Bytes())
}

func (p *pkt) MarshalCIF(c CIF) error {
	if !p.header.IsControlPacket {
		return fmt.Errorf("packet is not a control packet")
	}

	p.payload.Reset()
	return c.Marshal(p.payload)
}

func (p *pkt) UnmarshalCIF(c CIF) error {
	if !p.header.IsControlPacket {
		return nil
	}

	return c.Unmarshal(p.payload.Bytes())
}

// CIF reepresents a control information field
type CIF interface {
	// Marshal writes a byte representation of the CIF to the provided writer.
	Marshal(w io.Writer) error

	// Unmarshal parses the provided bytes into the CIF. Returns a non nil error of failure.
	Unmarshal(data []byte) error

	// String returns a string representation of the CIF.
	String() string
}

//  3.1. Data Packets

type PacketPosition uint

const (
	FirstPacket  PacketPosition = 2
	MiddlePacket PacketPosition = 0
	LastPacket   PacketPosition = 1
	SinglePacket PacketPosition = 3
)

func (p PacketPosition) String() string {
	switch uint(p) {
	case 0:
		return "middle"
	case 1:
		return "last"
	case 2:
		return "first"
	case 3:
		return "single"
	}

	return `¯\_(ツ)_/¯`
}

func (p PacketPosition) IsValid() bool {
	return p < 4
}

func (p PacketPosition) Val() uint32 {
	return uint32(p)
}

//  3.1. Data Packets

type PacketEncryption uint

const (
	UnencryptedPacket PacketEncryption = 0
	EvenKeyEncrypted  PacketEncryption = 1
	OddKeyEncrypted   PacketEncryption = 2
	EvenAndOddKey     PacketEncryption = 3
)

func (p PacketEncryption) String() string {
	switch uint(p) {
	case 0:
		return "unencrypted"
	case 1:
		return "even key"
	case 2:
		return "odd key"
	case 3:
		return "even and odd key"
	}

	return `¯\_(ツ)_/¯`
}

func (p PacketEncryption) IsValid() bool {
	return p < 4
}

func (p PacketEncryption) Opposite() PacketEncryption {
	if p == EvenKeyEncrypted {
		return OddKeyEncrypted
	}

	if p == OddKeyEncrypted {
		return EvenKeyEncrypted
	}

	return p
}

func (p PacketEncryption) Val() uint32 {
	return uint32(p)
}
