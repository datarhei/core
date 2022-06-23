// Package packet provides types and implementations for the different SRT packet types
package packet

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"sort"
	"strings"
	"sync"

	"github.com/datarhei/gosrt/internal/circular"
	srtnet "github.com/datarhei/gosrt/internal/net"
)

const MAX_SEQUENCENUMBER uint32 = 0b01111111_11111111_11111111_11111111
const MAX_TIMESTAMP uint32 = 0b11111111_11111111_11111111_11111111

// Table 1: SRT Control Packet Types
const (
	CTRLTYPE_HANDSHAKE uint16 = 0x0000
	CTRLTYPE_KEEPALIVE uint16 = 0x0001
	CTRLTYPE_ACK       uint16 = 0x0002
	CTRLTYPE_NAK       uint16 = 0x0003
	CTRLTYPE_WARN      uint16 = 0x0004 // unimplemented, receiver->sender
	CTRLTYPE_SHUTDOWN  uint16 = 0x0005
	CTRLTYPE_ACKACK    uint16 = 0x0006
	CRTLTYPE_DROPREQ   uint16 = 0x0007 // unimplemented, sender->receiver
	CRTLTYPE_PEERERROR uint16 = 0x0008 // unimplemented, receiver->sender
	CTRLTYPE_USER      uint16 = 0x7FFF
)

type HandshakeType uint32

// Table 4: Handshake Type
const (
	HSTYPE_DONE       HandshakeType = 0xFFFFFFFD
	HSTYPE_AGREEMENT  HandshakeType = 0xFFFFFFFE
	HSTYPE_CONCLUSION HandshakeType = 0xFFFFFFFF
	HSTYPE_WAVEHAND   HandshakeType = 0x00000000
	HSTYPE_INDUCTION  HandshakeType = 0x00000001
)

// Table 7: Handshake Rejection Reason Codes
const (
	REJ_UNKNOWN    HandshakeType = 1000
	REJ_SYSTEM     HandshakeType = 1001
	REJ_PEER       HandshakeType = 1002
	REJ_RESOURCE   HandshakeType = 1003
	REJ_ROGUE      HandshakeType = 1004
	REJ_BACKLOG    HandshakeType = 1005
	REJ_IPE        HandshakeType = 1006
	REJ_CLOSE      HandshakeType = 1007
	REJ_VERSION    HandshakeType = 1008
	REJ_RDVCOOKIE  HandshakeType = 1009
	REJ_BADSECRET  HandshakeType = 1010
	REJ_UNSECURE   HandshakeType = 1011
	REJ_MESSAGEAPI HandshakeType = 1012
	REJ_CONGESTION HandshakeType = 1013
	REJ_FILTER     HandshakeType = 1014
	REJ_GROUP      HandshakeType = 1015
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
	case REJ_UNKNOWN:
		return "REJ_UNKNOWN (unknown reason)"
	case REJ_SYSTEM:
		return "REJ_SYSTEM (system function error)"
	case REJ_PEER:
		return "REJ_PEER (rejected by peer)"
	case REJ_RESOURCE:
		return "REJ_RESOURCE (resource allocation problem)"
	case REJ_ROGUE:
		return "REJ_ROGUE (incorrect data in handshake)"
	case REJ_BACKLOG:
		return "REJ_BACKLOG (listener's backlog exceeded)"
	case REJ_IPE:
		return "REJ_IPE (internal program error)"
	case REJ_CLOSE:
		return "REJ_CLOSE (socket is closing)"
	case REJ_VERSION:
		return "REJ_VERSION (peer is older version than agent's min)"
	case REJ_RDVCOOKIE:
		return "REJ_RDVCOOKIE (rendezvous cookie collision)"
	case REJ_BADSECRET:
		return "REJ_BADSECRET (wrong password)"
	case REJ_UNSECURE:
		return "REJ_UNSECURE (password required or unexpected)"
	case REJ_MESSAGEAPI:
		return "REJ_MESSAGEAPI (stream flag collision)"
	case REJ_CONGESTION:
		return "REJ_CONGESTION (incompatible congestion-controller type)"
	case REJ_FILTER:
		return "REJ_FILTER (incompatible packet filter)"
	case REJ_GROUP:
		return "REJ_GROUP (incompatible group)"
	}

	return "unknown"
}

func (h HandshakeType) IsUnknown() bool {
	return h.String() == "unknown"
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
	if h.IsUnknown() {
		return false
	} else if h.IsHandshake() {
		return false
	}

	return true
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
const (
	EXTTYPE_HSREQ      uint16 = 1
	EXTTYPE_HSRSP      uint16 = 2
	EXTTYPE_KMREQ      uint16 = 3
	EXTTYPE_KMRSP      uint16 = 4
	EXTTYPE_SID        uint16 = 5
	EXTTYPE_CONGESTION uint16 = 6
	EXTTYPE_FILTER     uint16 = 7
	EXTTYPE_GROUP      uint16 = 8
)

type Packet interface {
	String() string
	Clone() Packet
	Header() *PacketHeader
	Data() []byte
	SetData([]byte)
	Len() uint64
	Unmarshal(data []byte) error
	Marshal(w io.Writer)
	Dump() string
	MarshalCIF(c CIF)
	UnmarshalCIF(c CIF) error
	Decommission()
}

//  3. Packet Structure

type PacketHeader struct {
	Addr            net.Addr
	IsControlPacket bool
	PktTsbpdTime    uint64 // microseconds

	// control packet fields

	ControlType  uint16
	SubType      uint16
	TypeSpecific uint32

	// data packet fields

	PacketSequenceNumber    circular.Number
	PacketPositionFlag      PacketPosition
	OrderFlag               bool
	KeyBaseEncryptionFlag   PacketEncryption
	RetransmittedPacketFlag bool
	MessageNumber           uint32

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
			New: func() interface{} {
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

func NewPacket(addr net.Addr, rawdata []byte) Packet {
	p := &pkt{
		header: PacketHeader{
			Addr:                  addr,
			PacketSequenceNumber:  circular.New(0, 0b01111111_11111111_11111111_11111111),
			PacketPositionFlag:    SinglePacket,
			KeyBaseEncryptionFlag: UnencryptedPacket,
			MessageNumber:         1,
		},
		payload: payloadPool.Get(),
	}

	if len(rawdata) != 0 {
		if err := p.Unmarshal(rawdata); err != nil {
			return nil
		}
	}

	return p
}

func (p *pkt) Decommission() {
	payloadPool.Put(p.payload)
	p.payload = nil
}

func (p pkt) String() string {
	var b strings.Builder

	fmt.Fprintf(&b, "timestamp=%#08x, destId=%#08x\n", p.header.Timestamp, p.header.DestinationSocketId)

	if p.header.IsControlPacket {
		fmt.Fprintf(&b, "control packet:\n")
		fmt.Fprintf(&b, "   controlType=%#04x\n", p.header.ControlType)
		fmt.Fprintf(&b, "   subType=%#04x\n", p.header.SubType)
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
		p.header.ControlType = binary.BigEndian.Uint16(data[0:]) & ^uint16(1<<15) // clear the first bit
		p.header.SubType = binary.BigEndian.Uint16(data[2:])
		p.header.TypeSpecific = binary.BigEndian.Uint32(data[4:])
	} else {
		p.header.PacketSequenceNumber = circular.New(binary.BigEndian.Uint32(data[0:]), MAX_SEQUENCENUMBER)
		p.header.PacketPositionFlag = PacketPosition((data[4] & 0b11000000) >> 6)
		p.header.OrderFlag = (data[4] & 0b00100000) != 0
		p.header.KeyBaseEncryptionFlag = PacketEncryption((data[4] & 0b00011000) >> 3)
		p.header.RetransmittedPacketFlag = (data[4] & 0b00000100) != 0
		p.header.MessageNumber = binary.BigEndian.Uint32(data[4:]) & ^uint32(0b11111000<<24)
	}

	p.header.Timestamp = binary.BigEndian.Uint32(data[8:])
	p.header.DestinationSocketId = binary.BigEndian.Uint32(data[12:])

	p.payload.Reset()
	p.payload.Write(data[16:])

	return nil
}

func (p *pkt) Marshal(w io.Writer) {
	var buffer [16]byte

	if p.header.IsControlPacket {
		binary.BigEndian.PutUint16(buffer[0:], p.header.ControlType)  // control type
		binary.BigEndian.PutUint16(buffer[2:], p.header.SubType)      // sub type
		binary.BigEndian.PutUint32(buffer[4:], p.header.TypeSpecific) // type specific

		buffer[0] |= 0x80
	} else {
		binary.BigEndian.PutUint32(buffer[0:], p.header.PacketSequenceNumber.Val()) // sequence number

		p.header.TypeSpecific = 0

		p.header.TypeSpecific |= (uint32(p.header.PacketPositionFlag) << 6)
		if p.header.OrderFlag {
			p.header.TypeSpecific |= (1 << 5)
		}
		p.header.TypeSpecific |= (uint32(p.header.KeyBaseEncryptionFlag) << 3)
		if p.header.RetransmittedPacketFlag {
			p.header.TypeSpecific |= (1 << 2)
		}
		p.header.TypeSpecific = p.header.TypeSpecific << 24
		p.header.TypeSpecific += p.header.MessageNumber

		binary.BigEndian.PutUint32(buffer[4:], p.header.TypeSpecific) // sequence number
	}

	binary.BigEndian.PutUint32(buffer[8:], p.header.Timestamp)            // timestamp
	binary.BigEndian.PutUint32(buffer[12:], p.header.DestinationSocketId) // destination socket ID

	w.Write(buffer[0:])
	w.Write(p.payload.Bytes())
}

func (p *pkt) Dump() string {
	var data bytes.Buffer
	p.Marshal(&data)

	return p.String() + "\n" + hex.Dump(data.Bytes())
}

func (p *pkt) MarshalCIF(c CIF) {
	if !p.header.IsControlPacket {
		return
	}

	p.payload.Reset()
	c.Marshal(p.payload)
}

func (p *pkt) UnmarshalCIF(c CIF) error {
	if !p.header.IsControlPacket {
		return nil
	}

	return c.Unmarshal(p.payload.Bytes())
}

type CIF interface {
	Marshal(w io.Writer)
	Unmarshal(data []byte) error
}

// 3.2.1.  Handshake

type CIFHandshake struct {
	IsRequest bool

	Version                     uint32
	EncryptionField             uint16
	ExtensionField              uint16
	InitialPacketSequenceNumber circular.Number
	MaxTransmissionUnitSize     uint32
	MaxFlowWindowSize           uint32
	HandshakeType               HandshakeType
	SRTSocketId                 uint32
	SynCookie                   uint32
	PeerIP                      srtnet.IP

	HasHS  bool
	HasKM  bool
	HasSID bool

	// 3.2.1.1.  Handshake Extension Message

	SRTVersion uint32
	SRTFlags   struct { // 3.2.1.1.1.  Handshake Extension Message Flags
		TSBPDSND      bool
		TSBPDRCV      bool
		CRYPT         bool
		TLPKTDROP     bool
		PERIODICNAK   bool
		REXMITFLG     bool
		STREAM        bool
		PACKET_FILTER bool
	}
	RecvTSBPDDelay uint16 // milliseconds, see "4.4.  SRT Buffer Latency"
	SendTSBPDDelay uint16 // milliseconds, see "4.4.  SRT Buffer Latency"

	// 3.2.1.2.  Key Material Extension Message
	SRTKM *CIFKM

	// 3.2.1.3.  Stream ID Extension Message
	StreamId string
}

func (c CIFHandshake) String() string {
	var b strings.Builder

	fmt.Fprintf(&b, "--- handshake ---\n")

	fmt.Fprintf(&b, "   version: %#08x\n", c.Version)
	fmt.Fprintf(&b, "   encryptionField: %#04x\n", c.EncryptionField)
	fmt.Fprintf(&b, "   extensionField: %#04x\n", c.ExtensionField)
	fmt.Fprintf(&b, "   initialPacketSequenceNumber: %#08x\n", c.InitialPacketSequenceNumber.Val())
	fmt.Fprintf(&b, "   maxTransmissionUnitSize: %#08x (%d)\n", c.MaxTransmissionUnitSize, c.MaxTransmissionUnitSize)
	fmt.Fprintf(&b, "   maxFlowWindowSize: %#08x (%d)\n", c.MaxFlowWindowSize, c.MaxFlowWindowSize)
	fmt.Fprintf(&b, "   handshakeType: %#08x (%s)\n", c.HandshakeType.Val(), c.HandshakeType.String())
	fmt.Fprintf(&b, "   srtSocketId: %#08x\n", c.SRTSocketId)
	fmt.Fprintf(&b, "   synCookie: %#08x\n", c.SynCookie)
	fmt.Fprintf(&b, "   peerIP: %s\n", c.PeerIP)

	if c.HasHS {
		fmt.Fprintf(&b, "   SRT_CMD_HS(REQ/RSP)\n")
		fmt.Fprintf(&b, "      srtVersion: %#08x\n", c.SRTVersion)
		fmt.Fprintf(&b, "      srtFlags:\n")
		fmt.Fprintf(&b, "         TSBPDSND     : %v\n", c.SRTFlags.TSBPDSND)
		fmt.Fprintf(&b, "         TSBPDRCV     : %v\n", c.SRTFlags.TSBPDRCV)
		fmt.Fprintf(&b, "         CRYPT        : %v\n", c.SRTFlags.CRYPT)
		fmt.Fprintf(&b, "         TLPKTDROP    : %v\n", c.SRTFlags.TLPKTDROP)
		fmt.Fprintf(&b, "         PERIODICNAK  : %v\n", c.SRTFlags.PERIODICNAK)
		fmt.Fprintf(&b, "         REXMITFLG    : %v\n", c.SRTFlags.REXMITFLG)
		fmt.Fprintf(&b, "         STREAM       : %v\n", c.SRTFlags.STREAM)
		fmt.Fprintf(&b, "         PACKET_FILTER: %v\n", c.SRTFlags.PACKET_FILTER)
		fmt.Fprintf(&b, "      recvTSBPDDelay: %#04x (%dms)\n", c.RecvTSBPDDelay, c.RecvTSBPDDelay)
		fmt.Fprintf(&b, "      sendTSBPDDelay: %#04x (%dms)\n", c.SendTSBPDDelay, c.SendTSBPDDelay)
	}

	if c.HasKM {
		fmt.Fprintf(&b, "   SRT_CMD_KM(REQ/RSP)\n")
		fmt.Fprintf(&b, "      s: %d\n", c.SRTKM.S)
		fmt.Fprintf(&b, "      version: %d\n", c.SRTKM.Version)
		fmt.Fprintf(&b, "      packetType: %d\n", c.SRTKM.PacketType)
		fmt.Fprintf(&b, "      sign: %#08x\n", c.SRTKM.Sign)
		fmt.Fprintf(&b, "      resv1: %d\n", c.SRTKM.Resv1)
		fmt.Fprintf(&b, "      keyBasedEncryption: %s\n", c.SRTKM.KeyBasedEncryption.String())
		fmt.Fprintf(&b, "      keyEncryptionKeyIndex: %d\n", c.SRTKM.KeyEncryptionKeyIndex)
		fmt.Fprintf(&b, "      cipher: %d\n", c.SRTKM.Cipher)
		fmt.Fprintf(&b, "      authentication: %d\n", c.SRTKM.Authentication)
		fmt.Fprintf(&b, "      streamEncapsulation: %d\n", c.SRTKM.StreamEncapsulation)
		fmt.Fprintf(&b, "      resv2: %d\n", c.SRTKM.Resv2)
		fmt.Fprintf(&b, "      resv3: %d\n", c.SRTKM.Resv3)
		fmt.Fprintf(&b, "      sLen: %d (%d)\n", c.SRTKM.SLen, c.SRTKM.SLen/4)
		fmt.Fprintf(&b, "      kLen: %d (%d)\n", c.SRTKM.KLen, c.SRTKM.KLen/4)
		fmt.Fprintf(&b, "      salt: %#08x\n", c.SRTKM.Salt)
		fmt.Fprintf(&b, "      wrap: %#08x\n", c.SRTKM.Wrap)
	}

	if c.HasSID {
		fmt.Fprintf(&b, "   SRT_CMD_SID\n")
		fmt.Fprintf(&b, "      streamId : %s\n", c.StreamId)
	}

	fmt.Fprintf(&b, "--- /handshake ---")

	return b.String()
}

func (c *CIFHandshake) Unmarshal(data []byte) error {
	if len(data) < 48 {
		return fmt.Errorf("data too short to unmarshal")
	}

	c.Version = binary.BigEndian.Uint32(data[0:])
	c.EncryptionField = binary.BigEndian.Uint16(data[4:])
	c.ExtensionField = binary.BigEndian.Uint16(data[6:])
	c.InitialPacketSequenceNumber = circular.New(binary.BigEndian.Uint32(data[8:])&MAX_SEQUENCENUMBER, MAX_SEQUENCENUMBER)
	c.MaxTransmissionUnitSize = binary.BigEndian.Uint32(data[12:])
	c.MaxFlowWindowSize = binary.BigEndian.Uint32(data[16:])
	c.HandshakeType = HandshakeType(binary.BigEndian.Uint32(data[20:]))
	c.SRTSocketId = binary.BigEndian.Uint32(data[24:])
	c.SynCookie = binary.BigEndian.Uint32(data[28:])
	c.PeerIP.Unmarshal(data[32:48])

	//if c.handshakeType != HSTYPE_INDUCTION && c.handshakeType != HSTYPE_CONCLUSION {
	//	return fmt.Errorf("unimplemented handshake type")
	//}

	if c.HandshakeType == HSTYPE_INDUCTION {
		// Nothing more to unmarshal
		return nil
	}

	if c.HandshakeType != HSTYPE_CONCLUSION {
		// Everything else is currently not supported
		return nil
	}

	if c.ExtensionField == 0 {
		return nil
	}

	if len(data) <= 48 {
		return fmt.Errorf("data too short to unmarshal")
	}

	switch c.EncryptionField {
	case 0:
	case 2:
	case 3:
	case 4:
	default:
		return fmt.Errorf("invalid encryption field value (%d)", c.EncryptionField)
	}

	pivot := data[48:]

	for {
		extensionType := binary.BigEndian.Uint16(pivot[0:])
		extensionLength := int(binary.BigEndian.Uint16(pivot[2:])) * 4

		pivot = pivot[4:]

		if extensionType == EXTTYPE_HSREQ || extensionType == EXTTYPE_HSRSP {
			// 3.2.1.1.  Handshake Extension Message
			if extensionLength != 12 || len(pivot) < extensionLength {
				return fmt.Errorf("invalid extension length")
			}

			c.HasHS = true

			c.SRTVersion = binary.BigEndian.Uint32(pivot[0:])
			srtFlags := binary.BigEndian.Uint32(pivot[4:])

			c.SRTFlags.TSBPDSND = (srtFlags&SRTFLAG_TSBPDSND != 0)
			c.SRTFlags.TSBPDRCV = (srtFlags&SRTFLAG_TSBPDRCV != 0)
			c.SRTFlags.CRYPT = (srtFlags&SRTFLAG_CRYPT != 0)
			c.SRTFlags.TLPKTDROP = (srtFlags&SRTFLAG_TLPKTDROP != 0)
			c.SRTFlags.PERIODICNAK = (srtFlags&SRTFLAG_PERIODICNAK != 0)
			c.SRTFlags.REXMITFLG = (srtFlags&SRTFLAG_REXMITFLG != 0)
			c.SRTFlags.STREAM = (srtFlags&SRTFLAG_STREAM != 0)
			c.SRTFlags.PACKET_FILTER = (srtFlags&SRTFLAG_PACKET_FILTER != 0)

			c.RecvTSBPDDelay = binary.BigEndian.Uint16(pivot[8:])
			c.SendTSBPDDelay = binary.BigEndian.Uint16(pivot[10:])
		} else if extensionType == EXTTYPE_KMREQ || extensionType == EXTTYPE_KMRSP {
			// 3.2.1.2.  Key Material Extension Message
			if len(pivot) < extensionLength {
				return fmt.Errorf("invalid extension length")
			}

			c.HasKM = true

			c.SRTKM = &CIFKM{}

			if err := c.SRTKM.Unmarshal(pivot); err != nil {
				return err
			}

			if c.EncryptionField == 0 {
				// using default cipher family and key size (AES-128)
				c.EncryptionField = 2
			}

			if c.EncryptionField == 2 && c.SRTKM.KLen != 16 {
				return fmt.Errorf("invalid key length for AES-128 (%d bit)", c.SRTKM.KLen*8)
			} else if c.EncryptionField == 3 && c.SRTKM.KLen != 24 {
				return fmt.Errorf("invalid key length for AES-192 (%d bit)", c.SRTKM.KLen*8)
			} else if c.EncryptionField == 4 && c.SRTKM.KLen != 32 {
				return fmt.Errorf("invalid key length for AES-256 (%d bit)", c.SRTKM.KLen*8)
			}
		} else if extensionType == EXTTYPE_SID {
			// 3.2.1.3.  Stream ID Extension Message
			if extensionLength > 512 || len(pivot) < extensionLength {
				return fmt.Errorf("invalid extension length")
			}

			c.HasSID = true

			var b strings.Builder

			for i := 0; i < extensionLength; i += 4 {
				b.WriteByte(pivot[i+3])
				b.WriteByte(pivot[i+2])
				b.WriteByte(pivot[i+1])
				b.WriteByte(pivot[i+0])
			}

			c.StreamId = strings.TrimRight(b.String(), "\x00")
		} else {
			return fmt.Errorf("unimplemented extension (%d)", extensionType)
		}

		if len(pivot) > extensionLength {
			pivot = pivot[extensionLength:]
		} else {
			break
		}
	}

	return nil
}

func (c *CIFHandshake) Marshal(w io.Writer) {
	var buffer [128]byte

	if len(c.StreamId) == 0 {
		c.HasSID = false
	}

	if c.HandshakeType == HSTYPE_CONCLUSION {
		c.ExtensionField = 0
	}

	if c.HasHS {
		c.ExtensionField = c.ExtensionField | 1
	}

	if c.HasKM {
		c.ExtensionField = c.ExtensionField | 2
	}

	if c.HasSID {
		c.ExtensionField = c.ExtensionField | 4
	}

	binary.BigEndian.PutUint32(buffer[0:], c.Version)                           // version
	binary.BigEndian.PutUint16(buffer[4:], c.EncryptionField)                   // encryption field
	binary.BigEndian.PutUint16(buffer[6:], c.ExtensionField)                    // extension field
	binary.BigEndian.PutUint32(buffer[8:], c.InitialPacketSequenceNumber.Val()) // initialPacketSequenceNumber
	binary.BigEndian.PutUint32(buffer[12:], c.MaxTransmissionUnitSize)          // maxTransmissionUnitSize
	binary.BigEndian.PutUint32(buffer[16:], c.MaxFlowWindowSize)                // maxFlowWindowSize
	binary.BigEndian.PutUint32(buffer[20:], c.HandshakeType.Val())              // handshakeType
	binary.BigEndian.PutUint32(buffer[24:], c.SRTSocketId)                      // Socket ID of the Listener, should be some own generated ID
	binary.BigEndian.PutUint32(buffer[28:], c.SynCookie)                        // SYN cookie
	c.PeerIP.Marshal(buffer[32:])                                               // peerIP

	w.Write(buffer[:48])

	if c.HasHS {
		if c.IsRequest {
			binary.BigEndian.PutUint16(buffer[0:], EXTTYPE_HSREQ)
		} else {
			binary.BigEndian.PutUint16(buffer[0:], EXTTYPE_HSRSP)
		}

		binary.BigEndian.PutUint16(buffer[2:], 3)

		binary.BigEndian.PutUint32(buffer[4:], c.SRTVersion)
		var srtFlags uint32 = 0

		if c.SRTFlags.TSBPDSND {
			srtFlags |= SRTFLAG_TSBPDSND
		}

		if c.SRTFlags.TSBPDRCV {
			srtFlags |= SRTFLAG_TSBPDRCV
		}

		if c.SRTFlags.CRYPT {
			srtFlags |= SRTFLAG_CRYPT
		}

		if c.SRTFlags.TLPKTDROP {
			srtFlags |= SRTFLAG_TLPKTDROP
		}

		if c.SRTFlags.PERIODICNAK {
			srtFlags |= SRTFLAG_PERIODICNAK
		}

		if c.SRTFlags.REXMITFLG {
			srtFlags |= SRTFLAG_REXMITFLG
		}

		if c.SRTFlags.STREAM {
			srtFlags |= SRTFLAG_STREAM
		}

		if c.SRTFlags.PACKET_FILTER {
			srtFlags |= SRTFLAG_PACKET_FILTER
		}

		binary.BigEndian.PutUint32(buffer[8:], srtFlags)
		binary.BigEndian.PutUint16(buffer[12:], c.RecvTSBPDDelay)
		binary.BigEndian.PutUint16(buffer[14:], c.SendTSBPDDelay)

		w.Write(buffer[:16])
	}

	if c.HasKM {
		var data bytes.Buffer

		c.SRTKM.Marshal(&data)

		if c.IsRequest {
			binary.BigEndian.PutUint16(buffer[0:], EXTTYPE_KMREQ)
		} else {
			binary.BigEndian.PutUint16(buffer[0:], EXTTYPE_KMRSP)
		}

		binary.BigEndian.PutUint16(buffer[2:], uint16(data.Len()/4))

		w.Write(buffer[:4])
		w.Write(data.Bytes())
	}

	if c.HasSID {
		streamId := bytes.NewBufferString(c.StreamId)

		missing := (4 - streamId.Len()%4)
		if missing < 4 {
			for i := 0; i < missing; i++ {
				streamId.WriteByte(0)
			}
		}

		binary.BigEndian.PutUint16(buffer[0:], EXTTYPE_SID)
		binary.BigEndian.PutUint16(buffer[2:], uint16(streamId.Len()/4))

		w.Write(buffer[:4])

		b := streamId.Bytes()

		for i := 0; i < len(b); i += 4 {
			buffer[0] = b[i+3]
			buffer[1] = b[i+2]
			buffer[2] = b[i+1]
			buffer[3] = b[i+0]

			w.Write(buffer[:4])
		}
	}
}

// 3.2.2.  Key Material

type CIFKM struct {
	S                     uint8
	Version               uint8
	PacketType            uint8
	Sign                  uint16
	Resv1                 uint8
	KeyBasedEncryption    PacketEncryption
	KeyEncryptionKeyIndex uint32
	Cipher                uint8
	Authentication        uint8
	StreamEncapsulation   uint8
	Resv2                 uint8
	Resv3                 uint16
	SLen                  uint16
	KLen                  uint16
	Salt                  []byte
	Wrap                  []byte
}

func (c CIFKM) String() string {
	var b strings.Builder

	fmt.Fprintf(&b, "--- KM ---\n")

	fmt.Fprintf(&b, "   s: %d\n", c.S)
	fmt.Fprintf(&b, "   version: %d\n", c.Version)
	fmt.Fprintf(&b, "   packetType: %d\n", c.PacketType)
	fmt.Fprintf(&b, "   sign: %#08x\n", c.Sign)
	fmt.Fprintf(&b, "   resv1: %d\n", c.Resv1)
	fmt.Fprintf(&b, "   keyBasedEncryption: %s\n", c.KeyBasedEncryption.String())
	fmt.Fprintf(&b, "   keyEncryptionKeyIndex: %d\n", c.KeyEncryptionKeyIndex)
	fmt.Fprintf(&b, "   cipher: %d\n", c.Cipher)
	fmt.Fprintf(&b, "   authentication: %d\n", c.Authentication)
	fmt.Fprintf(&b, "   streamEncapsulation: %d\n", c.StreamEncapsulation)
	fmt.Fprintf(&b, "   resv2: %d\n", c.Resv2)
	fmt.Fprintf(&b, "   resv3: %d\n", c.Resv3)
	fmt.Fprintf(&b, "   sLen: %d (%d)\n", c.SLen, c.SLen/4)
	fmt.Fprintf(&b, "   kLen: %d (%d)\n", c.KLen, c.KLen/4)
	fmt.Fprintf(&b, "   salt: %#08x\n", c.Salt)
	fmt.Fprintf(&b, "   wrap: %#08x\n", c.Wrap)

	fmt.Fprintf(&b, "--- /KM ---")

	return b.String()
}

func (c *CIFKM) Unmarshal(data []byte) error {
	if len(data) < 16 {
		return fmt.Errorf("data too short to unmarshal")
	}

	c.S = uint8(data[0] & 0b1000_0000 >> 7)
	if c.S != 0 {
		return fmt.Errorf("invalid value for S")
	}

	c.Version = uint8(data[0] & 0b0111_0000 >> 4)
	if c.Version != 1 {
		return fmt.Errorf("invalid version")
	}

	c.PacketType = uint8(data[0] & 0b0000_1111)
	if c.PacketType != 2 {
		return fmt.Errorf("invalid packet type (%d)", c.PacketType)
	}

	c.Sign = binary.BigEndian.Uint16(data[1:])
	if c.Sign != 0x2029 {
		return fmt.Errorf("invalid signature (%#08x)", c.Sign)
	}

	c.Resv1 = uint8(data[3] & 0b1111_1100 >> 2)
	c.KeyBasedEncryption = PacketEncryption(data[3] & 0b0000_0011)
	if !c.KeyBasedEncryption.IsValid() || c.KeyBasedEncryption == UnencryptedPacket {
		return fmt.Errorf("invalid extension format (KK must not be 0)")
	}

	c.KeyEncryptionKeyIndex = binary.BigEndian.Uint32(data[4:])
	if c.KeyEncryptionKeyIndex != 0 {
		return fmt.Errorf("invalid key encryption key index (%d)", c.KeyEncryptionKeyIndex)
	}

	c.Cipher = uint8(data[8])
	c.Authentication = uint8(data[9])
	c.StreamEncapsulation = uint8(data[10])
	if c.StreamEncapsulation != 2 {
		return fmt.Errorf("invalid stream encapsulation (%d)", c.StreamEncapsulation)
	}

	c.Resv2 = uint8(data[11])
	c.Resv3 = binary.BigEndian.Uint16(data[12:])
	c.SLen = uint16(data[14]) * 4
	c.KLen = uint16(data[15]) * 4

	switch c.KLen {
	case 16:
	case 24:
	case 32:
	default:
		return fmt.Errorf("invalid key length")
	}

	offset := 16

	if c.SLen != 0 {
		if c.SLen != 16 {
			return fmt.Errorf("invalid salt length")
		}

		if len(data[offset:]) < 16 {
			return fmt.Errorf("data too short to unmarshal")
		}

		c.Salt = make([]byte, 16)
		copy(c.Salt, data[offset:])

		offset += 16
	}

	n := 1
	if c.KeyBasedEncryption == EvenAndOddKey {
		n = 2
	}

	if len(data[offset:]) < n*int(c.KLen)+8 {
		return fmt.Errorf("data too short to unmarshal")
	}

	c.Wrap = make([]byte, n*int(c.KLen)+8)
	copy(c.Wrap, data[offset:])

	return nil
}

func (c *CIFKM) Marshal(w io.Writer) {
	var buffer [128]byte

	b := byte(0)

	b |= (c.S << 7) & 0b1000_0000
	b |= (c.Version << 4) & 0b0111_0000
	b |= c.PacketType & 0b0000_1111

	buffer[0] = b
	binary.BigEndian.PutUint16(buffer[1:], c.Sign)

	b = 0
	b |= (c.Resv1 << 2) & 0b1111_1100
	b |= uint8(c.KeyBasedEncryption) & 0b0000_0011

	buffer[3] = b
	binary.BigEndian.PutUint32(buffer[4:], c.KeyEncryptionKeyIndex)

	buffer[8] = byte(c.Cipher)
	buffer[9] = byte(c.Authentication)
	buffer[10] = byte(c.StreamEncapsulation)
	buffer[11] = byte(c.Resv2)

	binary.BigEndian.PutUint16(buffer[12:], c.Resv3)

	buffer[14] = byte(c.SLen / 4)
	buffer[15] = byte(c.KLen / 4)

	offset := 16

	if c.SLen != 0 {
		copy(buffer[offset:], c.Salt[0:])
		offset += len(c.Salt)
	}

	copy(buffer[offset:], c.Wrap)
	offset += len(c.Wrap)

	w.Write(buffer[:offset])
}

// 3.2.4.  ACK (Acknowledgment)

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

func (c *CIFACK) Marshal(w io.Writer) {
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
}

// 3.2.5.  NAK (Loss Report)

type CIFNAK struct {
	LostPacketSequenceNumber []circular.Number
}

func (c CIFNAK) String() string {
	var b strings.Builder

	fmt.Fprintf(&b, "--- NAK ---\n")

	if len(c.LostPacketSequenceNumber)%2 != 0 {
		fmt.Fprintf(&b, "   invalid list of sequence numbers\n")
		return b.String()
	}

	for i := 0; i < len(c.LostPacketSequenceNumber); i += 2 {
		if c.LostPacketSequenceNumber[i].Equals(c.LostPacketSequenceNumber[i+1]) {
			fmt.Fprintf(&b, "   single: %#08x\n", c.LostPacketSequenceNumber[i].Val())
		} else {
			fmt.Fprintf(&b, "      row: %#08x to %#08x\n", c.LostPacketSequenceNumber[i].Val(), c.LostPacketSequenceNumber[i+1].Val())
		}
	}

	fmt.Fprintf(&b, "--- /NAK ---")

	return b.String()
}

func (c *CIFNAK) Unmarshal(data []byte) error {
	if len(data)%4 != 0 {
		return fmt.Errorf("data too short to unmarshal")
	}

	// Appendix A

	c.LostPacketSequenceNumber = []circular.Number{}

	var sequenceNumber circular.Number
	isRange := false

	for i := 0; i < len(data); i += 4 {
		sequenceNumber = circular.New(binary.BigEndian.Uint32(data[i:])&MAX_SEQUENCENUMBER, MAX_SEQUENCENUMBER)

		if data[i]&0b10000000 == 0 {
			c.LostPacketSequenceNumber = append(c.LostPacketSequenceNumber, sequenceNumber)

			if !isRange {
				c.LostPacketSequenceNumber = append(c.LostPacketSequenceNumber, sequenceNumber)
			}

			isRange = false
		} else {
			c.LostPacketSequenceNumber = append(c.LostPacketSequenceNumber, sequenceNumber)
			isRange = true
		}
	}

	if len(c.LostPacketSequenceNumber)%2 != 0 {
		return fmt.Errorf("data too short to unmarshal")
	}

	sort.Slice(c.LostPacketSequenceNumber, func(i, j int) bool { return c.LostPacketSequenceNumber[i].Lt(c.LostPacketSequenceNumber[j]) })

	return nil
}

func (c *CIFNAK) Marshal(w io.Writer) {
	if len(c.LostPacketSequenceNumber)%2 != 0 {
		return
	}

	// Appendix A

	var buffer [8]byte

	for i := 0; i < len(c.LostPacketSequenceNumber); i += 2 {
		if c.LostPacketSequenceNumber[i] == c.LostPacketSequenceNumber[i+1] {
			binary.BigEndian.PutUint32(buffer[0:], c.LostPacketSequenceNumber[i].Val())
			w.Write(buffer[0:4])
		} else {
			binary.BigEndian.PutUint32(buffer[0:], c.LostPacketSequenceNumber[i].Val()|0b10000000_00000000_00000000_00000000)
			binary.BigEndian.PutUint32(buffer[4:], c.LostPacketSequenceNumber[i+1].Val())
			w.Write(buffer[0:])
		}
	}
}

//  3.2.7. Shutdown

type CIFShutdown struct{}

func (c CIFShutdown) String() string {
	return "--- Shutdown ---"
}

func (c *CIFShutdown) Unmarshal(data []byte) error {
	if len(data) != 0 && len(data) != 4 {
		return fmt.Errorf("invalid length")
	}

	return nil
}

func (c *CIFShutdown) Marshal(w io.Writer) {
	var buffer [4]byte

	binary.BigEndian.PutUint32(buffer[0:], 0)

	w.Write(buffer[0:])
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
