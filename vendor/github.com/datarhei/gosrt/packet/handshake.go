package packet

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strings"

	"github.com/datarhei/gosrt/circular"
	srtnet "github.com/datarhei/gosrt/net"
)

// 3.2.1.  Handshake

// CIFHandshake represents the SRT handshake messages.
type CIFHandshake struct {
	IsRequest bool

	Version                     uint32          // A base protocol version number. Currently used values are 4 and 5. Values greater than 5 are reserved for future use.
	EncryptionField             uint16          // Block cipher family and key size. The values of this field are described in Table 2. The default value is AES-128.
	ExtensionField              uint16          // This field is a message specific extension related to Handshake Type field. The value MUST be set to 0 except for the following cases. (1) If the handshake control packet is the INDUCTION message, this field is sent back by the Listener. (2) In the case of a CONCLUSION message, this field value should contain a combination of Extension Type values. For more details, see Section 4.3.1.
	InitialPacketSequenceNumber circular.Number // The sequence number of the very first data packet to be sent.
	MaxTransmissionUnitSize     uint32          // This value is typically set to 1500, which is the default Maximum Transmission Unit (MTU) size for Ethernet, but can be less.
	MaxFlowWindowSize           uint32          // The value of this field is the maximum number of data packets allowed to be "in flight" (i.e. the number of sent packets for which an ACK control packet has not yet been received).
	HandshakeType               HandshakeType   // This field indicates the handshake packet type. The possible values are described in Table 4. For more details refer to Section 4.3.
	SRTSocketId                 uint32          // This field holds the ID of the source SRT socket from which a handshake packet is issued.
	SynCookie                   uint32          // Randomized value for processing a handshake. The value of this field is specified by the handshake message type. See Section 4.3.
	PeerIP                      srtnet.IP       // IPv4 or IPv6 address of the packet's sender. The value consists of four 32-bit fields. In the case of IPv4 addresses, fields 2, 3 and 4 are filled with zeroes.

	HasHS            bool
	HasKM            bool
	HasSID           bool
	HasCongestionCtl bool
	HasFilter        bool

	// 3.2.1.1.  Handshake Extension Message
	SRTHS *CIFHandshakeExtension

	// 3.2.1.2.  Key Material Extension Message
	SRTKM *CIFKeyMaterialExtension

	// 3.2.1.3.  Stream ID Extension Message
	StreamId string

	// ??? Congestion Control Extension message (handshake.md #### Congestion controller)
	CongestionCtl string

	// Packet Filter Extension
	PacketFilter string
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

	if c.Version == 5 {
		if c.HasHS {
			fmt.Fprintf(&b, "%s\n", c.SRTHS.String())
		}

		if c.HasKM {
			fmt.Fprintf(&b, "%s\n", c.SRTKM.String())
		}

		if c.HasSID {
			fmt.Fprintf(&b, "--- SIDExt ---\n")
			fmt.Fprintf(&b, "   streamId : %s\n", c.StreamId)
			fmt.Fprintf(&b, "--- /SIDExt ---\n")
		}

		if c.HasCongestionCtl {
			fmt.Fprintf(&b, "--- CongestionExt ---\n")
			fmt.Fprintf(&b, "   congestion : %s\n", c.CongestionCtl)
			fmt.Fprintf(&b, "--- /CongestionExt ---\n")
		}

		if c.HasFilter {
			fmt.Fprintf(&b, "--- FilterExt ---\n")
			fmt.Fprintf(&b, "   filter : %s\n", c.PacketFilter)
			fmt.Fprintf(&b, "--- /FilterExt ---\n")
		}
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
		// No extension data
		return nil
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
		if len(pivot) < 4 {
			return fmt.Errorf("invalid extension length")
		}

		extensionType := CtrlSubType(binary.BigEndian.Uint16(pivot[0:]))
		extensionLength := int(binary.BigEndian.Uint16(pivot[2:])) * 4

		pivot = pivot[4:]

		if extensionType == EXTTYPE_HSREQ || extensionType == EXTTYPE_HSRSP {
			// 3.2.1.1.  Handshake Extension Message
			if extensionLength != 12 || len(pivot) < extensionLength {
				return fmt.Errorf("invalid extension length of %d bytes (%s)", extensionLength, extensionType.String())
			}

			c.HasHS = true

			c.SRTHS = &CIFHandshakeExtension{}

			if err := c.SRTHS.Unmarshal(pivot); err != nil {
				return fmt.Errorf("CIFHandshakeExtension: %w", err)
			}
		} else if extensionType == EXTTYPE_KMREQ || extensionType == EXTTYPE_KMRSP {
			// 3.2.1.2.  Key Material Extension Message
			if len(pivot) < extensionLength {
				return fmt.Errorf("invalid extension length of %d bytes (%s)", extensionLength, extensionType.String())
			}

			c.HasKM = true

			c.SRTKM = &CIFKeyMaterialExtension{}

			if err := c.SRTKM.Unmarshal(pivot); err != nil {
				return fmt.Errorf("CIFKeyMaterialExtension: %w", err)
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
				return fmt.Errorf("invalid extension length of %d bytes (%s)", extensionLength, extensionType.String())
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
		} else if extensionType == EXTTYPE_CONGESTION {
			// ??? Congestion Control Extension message (handshake.md #### Congestion controller)
			if extensionLength > 4 || len(pivot) < extensionLength {
				return fmt.Errorf("invalid extension length of %d bytes (%s)", extensionLength, extensionType.String())
			}

			c.HasCongestionCtl = true

			var b strings.Builder

			for i := 0; i < extensionLength; i += 4 {
				b.WriteByte(pivot[i+3])
				b.WriteByte(pivot[i+2])
				b.WriteByte(pivot[i+1])
				b.WriteByte(pivot[i+0])
			}

			c.CongestionCtl = strings.TrimRight(b.String(), "\x00")
		} else if extensionType == EXTTYPE_FILTER {
			if extensionLength > 512 || len(pivot) < extensionLength {
				return fmt.Errorf("invalid extension length of %d bytes (%s)", extensionLength, extensionType.String())
			}

			c.HasFilter = true

			var b strings.Builder

			for i := 0; i < extensionLength; i += 4 {
				b.WriteByte(pivot[i+3])
				b.WriteByte(pivot[i+2])
				b.WriteByte(pivot[i+1])
				b.WriteByte(pivot[i+0])
			}

			c.PacketFilter = strings.TrimRight(b.String(), "\x00")
		} else if extensionType == EXTTYPE_GROUP {
			// Skip unimplemented extensions
			if len(pivot) < extensionLength {
				return fmt.Errorf("invalid extension length of %d bytes (%s)", extensionLength, extensionType.String())
			}
		} else {
			// Skip unknown extensions
			if len(pivot) < extensionLength {
				return fmt.Errorf("invalid extension length of %d bytes (%s)", extensionLength, extensionType.String())
			}
		}

		if len(pivot) > extensionLength {
			pivot = pivot[extensionLength:]
		} else {
			break
		}
	}

	return nil
}

func (c *CIFHandshake) Marshal(w io.Writer) error {
	if w == nil {
		return fmt.Errorf("invalid writer")
	}

	var buffer [48]byte

	if len(c.StreamId) == 0 {
		c.HasSID = false
	}

	if c.Version == 5 {
		if c.HandshakeType == HSTYPE_CONCLUSION {
			c.ExtensionField = 0
		}

		if c.HasHS {
			c.ExtensionField = c.ExtensionField | 1
		}

		if c.HasKM {
			c.EncryptionField = c.SRTKM.KLen / 8
			c.ExtensionField = c.ExtensionField | 2
		}

		if c.HasSID {
			c.ExtensionField = c.ExtensionField | 4
		}

		if c.HasCongestionCtl {
			c.ExtensionField = c.ExtensionField | 4
		}

		if c.HasFilter {
			c.ExtensionField = c.ExtensionField | 4
		}
	} else {
		c.EncryptionField = 0
		c.ExtensionField = 2
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
		var data bytes.Buffer

		c.SRTHS.Marshal(&data)

		if c.IsRequest {
			binary.BigEndian.PutUint16(buffer[0:], EXTTYPE_HSREQ.Value())
		} else {
			binary.BigEndian.PutUint16(buffer[0:], EXTTYPE_HSRSP.Value())
		}

		binary.BigEndian.PutUint16(buffer[2:], 3)

		w.Write(buffer[:4])
		w.Write(data.Bytes())
	}

	if c.HasKM {
		var data bytes.Buffer

		c.SRTKM.Marshal(&data)

		if c.IsRequest {
			binary.BigEndian.PutUint16(buffer[0:], EXTTYPE_KMREQ.Value())
		} else {
			binary.BigEndian.PutUint16(buffer[0:], EXTTYPE_KMRSP.Value())
		}

		binary.BigEndian.PutUint16(buffer[2:], uint16(data.Len()/4))

		w.Write(buffer[:4])
		w.Write(data.Bytes())
	}

	if c.HasSID {
		streamId := bytes.NewBufferString(c.StreamId)

		missing := (4 - streamId.Len()%4)
		if missing < 4 {
			for range missing {
				streamId.WriteByte(0)
			}
		}

		binary.BigEndian.PutUint16(buffer[0:], EXTTYPE_SID.Value())
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

	if c.HasCongestionCtl && c.CongestionCtl != "live" {
		congestion := bytes.NewBufferString(c.CongestionCtl)

		missing := (4 - congestion.Len()%4)
		if missing < 4 {
			for range missing {
				congestion.WriteByte(0)
			}
		}

		binary.BigEndian.PutUint16(buffer[0:], EXTTYPE_CONGESTION.Value())
		binary.BigEndian.PutUint16(buffer[2:], uint16(congestion.Len()/4))

		w.Write(buffer[:4])

		b := congestion.Bytes()

		for i := 0; i < len(b); i += 4 {
			buffer[0] = b[i+3]
			buffer[1] = b[i+2]
			buffer[2] = b[i+1]
			buffer[3] = b[i+0]

			w.Write(buffer[:4])
		}
	}

	if c.HasFilter && len(c.PacketFilter) > 0 {
		filter := bytes.NewBufferString(c.PacketFilter)

		missing := (4 - filter.Len()%4)
		if missing < 4 {
			for range missing {
				filter.WriteByte(0)
			}
		}

		binary.BigEndian.PutUint16(buffer[0:], EXTTYPE_FILTER.Value())
		binary.BigEndian.PutUint16(buffer[2:], uint16(filter.Len()/4))

		w.Write(buffer[:4])

		b := filter.Bytes()

		for i := 0; i < len(b); i += 4 {
			buffer[0] = b[i+3]
			buffer[1] = b[i+2]
			buffer[2] = b[i+1]
			buffer[3] = b[i+0]

			w.Write(buffer[:4])
		}
	}

	return nil
}

// 3.2.1.1.1.  Handshake Extension Message Flags

// CIFHandshakeExtensionFlags represents the Handshake Extension Message Flags
type CIFHandshakeExtensionFlags struct {
	TSBPDSND      bool // Defines if the TSBPD mechanism (Section 4.5) will be used for sending.
	TSBPDRCV      bool // Defines if the TSBPD mechanism (Section 4.5) will be used for receiving.
	CRYPT         bool // MUST be set. It is a legacy flag that indicates the party understands KK field of the SRT Packet (Figure 3).
	TLPKTDROP     bool // Should be set if too-late packet drop mechanism will be used during transmission.  See Section 4.6.
	PERIODICNAK   bool // Indicates the peer will send periodic NAK packets. See Section 4.8.2.
	REXMITFLG     bool // MUST be set. It is a legacy flag that indicates the peer understands the R field of the SRT DATA Packet
	STREAM        bool // Identifies the transmission mode (Section 4.2) to be used in the connection. If the flag is set, the buffer mode (Section 4.2.2) is used. Otherwise, the message mode (Section 4.2.1) is used.
	PACKET_FILTER bool // Indicates if the peer supports packet filter.
}

// 3.2.1.1.  Handshake Extension Message

// CIFHandshakeExtension represents the Handshake Extension Message
type CIFHandshakeExtension struct {
	SRTVersion     uint32
	SRTFlags       CIFHandshakeExtensionFlags
	RecvTSBPDDelay uint16 // milliseconds, see "4.4.  SRT Buffer Latency"
	SendTSBPDDelay uint16 // milliseconds, see "4.4.  SRT Buffer Latency"
}

func (c CIFHandshakeExtension) String() string {
	var b strings.Builder

	fmt.Fprintf(&b, "--- HSExt ---\n")

	fmt.Fprintf(&b, "   srtVersion: %#08x\n", c.SRTVersion)
	fmt.Fprintf(&b, "   srtFlags:\n")
	fmt.Fprintf(&b, "      TSBPDSND     : %v\n", c.SRTFlags.TSBPDSND)
	fmt.Fprintf(&b, "      TSBPDRCV     : %v\n", c.SRTFlags.TSBPDRCV)
	fmt.Fprintf(&b, "      CRYPT        : %v\n", c.SRTFlags.CRYPT)
	fmt.Fprintf(&b, "      TLPKTDROP    : %v\n", c.SRTFlags.TLPKTDROP)
	fmt.Fprintf(&b, "      PERIODICNAK  : %v\n", c.SRTFlags.PERIODICNAK)
	fmt.Fprintf(&b, "      REXMITFLG    : %v\n", c.SRTFlags.REXMITFLG)
	fmt.Fprintf(&b, "      STREAM       : %v\n", c.SRTFlags.STREAM)
	fmt.Fprintf(&b, "      PACKET_FILTER: %v\n", c.SRTFlags.PACKET_FILTER)
	fmt.Fprintf(&b, "   recvTSBPDDelay: %#04x (%dms)\n", c.RecvTSBPDDelay, c.RecvTSBPDDelay)
	fmt.Fprintf(&b, "   sendTSBPDDelay: %#04x (%dms)\n", c.SendTSBPDDelay, c.SendTSBPDDelay)

	fmt.Fprintf(&b, "--- /HSExt ---")

	return b.String()
}

func (c *CIFHandshakeExtension) Unmarshal(data []byte) error {
	if len(data) < 12 {
		return fmt.Errorf("data too short to unmarshal")
	}

	c.SRTVersion = binary.BigEndian.Uint32(data[0:])
	srtFlags := binary.BigEndian.Uint32(data[4:])

	c.SRTFlags.TSBPDSND = (srtFlags&SRTFLAG_TSBPDSND != 0)
	c.SRTFlags.TSBPDRCV = (srtFlags&SRTFLAG_TSBPDRCV != 0)
	c.SRTFlags.CRYPT = (srtFlags&SRTFLAG_CRYPT != 0)
	c.SRTFlags.TLPKTDROP = (srtFlags&SRTFLAG_TLPKTDROP != 0)
	c.SRTFlags.PERIODICNAK = (srtFlags&SRTFLAG_PERIODICNAK != 0)
	c.SRTFlags.REXMITFLG = (srtFlags&SRTFLAG_REXMITFLG != 0)
	c.SRTFlags.STREAM = (srtFlags&SRTFLAG_STREAM != 0)
	c.SRTFlags.PACKET_FILTER = (srtFlags&SRTFLAG_PACKET_FILTER != 0)

	c.RecvTSBPDDelay = binary.BigEndian.Uint16(data[8:])
	c.SendTSBPDDelay = binary.BigEndian.Uint16(data[10:])

	return nil
}

func (c *CIFHandshakeExtension) Marshal(w io.Writer) error {
	if w == nil {
		return fmt.Errorf("invalid writer")
	}

	var buffer [12]byte

	binary.BigEndian.PutUint32(buffer[0:], c.SRTVersion)
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

	binary.BigEndian.PutUint32(buffer[4:], srtFlags)
	binary.BigEndian.PutUint16(buffer[8:], c.RecvTSBPDDelay)
	binary.BigEndian.PutUint16(buffer[10:], c.SendTSBPDDelay)

	_, err := w.Write(buffer[:12])

	return err
}

// 3.2.2.  Key Material

const (
	KM_NOSECRET  uint32 = 3
	KM_BADSECRET uint32 = 4
)

// CIFKeyMaterialExtension represents the Key Material message. It is used as part of
// the v5 handshake or on its own after a v4 handshake.
type CIFKeyMaterialExtension struct {
	Error                 uint32
	S                     uint8            // This is a fixed-width field that is reserved for future usage. value = {0}
	Version               uint8            // This is a fixed-width field that indicates the SRT version. value = {1}
	PacketType            uint8            // This is a fixed-width field that indicates the Packet Type: 0: Reserved, 1: Media Stream Message (MSmsg), 2: Keying Material Message (KMmsg), 7: Reserved to discriminate MPEG-TS packet (0x47=sync byte). value = {2}
	Sign                  uint16           // This is a fixed-width field that contains the signature 'HAI' encoded as a PnP Vendor ID [PNPID] (in big-endian order). value = {0x2029}
	Resv1                 uint8            // This is a fixed-width field reserved for flag extension or other usage. value = {0}
	KeyBasedEncryption    PacketEncryption // This is a fixed-width field that indicates which SEKs (odd and/or even) are provided in the extension: 00b: No SEK is provided (invalid extension format); 01b: Even key is provided; 10b: Odd key is provided; 11b: Both even and odd keys are provided.
	KeyEncryptionKeyIndex uint32           // This is a fixed-width field for specifying the KEK index (big-endian order) was used to wrap (and optionally authenticate) the SEK(s). The value 0 is used to indicate the default key of the current stream. Other values are reserved for the possible use of a key management system in the future to retrieve a cryptographic context. 0: Default stream associated key (stream/system default); 1..255: Reserved for manually indexed keys. value = {0}
	Cipher                uint8            // This is a fixed-width field for specifying encryption cipher and mode: 0: None or KEKI indexed crypto context; 2: AES-CTR [SP800-38A].
	Authentication        uint8            // This is a fixed-width field for specifying a message authentication code algorithm: 0: None or KEKI indexed crypto context.
	StreamEncapsulation   uint8            // This is a fixed-width field for describing the stream encapsulation: 0: Unspecified or KEKI indexed crypto context; 1: MPEG-TS/UDP; 2: MPEG-TS/SRT. value = {2}
	Resv2                 uint8            // This is a fixed-width field reserved for future use. value = {0}
	Resv3                 uint16           // This is a fixed-width field reserved for future use. value = {0}
	SLen                  uint16           // This is a fixed-width field for specifying salt length SLen in bytes divided by 4. Can be zero if no salt/IV present. The only valid length of salt defined is 128 bits.
	KLen                  uint16           // This is a fixed-width field for specifying SEK length in bytes divided by 4. Size of one key even if two keys present. MUST match the key size specified in the Encryption Field of the handshake packet Table 2.
	Salt                  []byte           // This is a variable-width field that complements the keying material by specifying a salt key.
	Wrap                  []byte           // (64 + n * KLen * 8) bits. This is a variable- width field for specifying Wrapped key(s), where n = (KK + 1)/2 and the size of the wrap field is ((n * KLen) + 8) bytes.
}

func (c CIFKeyMaterialExtension) String() string {
	var b strings.Builder

	fmt.Fprintf(&b, "--- KMExt ---\n")

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

	fmt.Fprintf(&b, "--- /KMExt ---")

	return b.String()
}

func (c *CIFKeyMaterialExtension) Unmarshal(data []byte) error {
	if len(data) == 4 {
		// This is an error response
		c.Error = binary.LittleEndian.Uint32(data[0:])
		if c.Error != KM_NOSECRET && c.Error != KM_BADSECRET {
			return fmt.Errorf("invalid error (%d)", c.Error)
		}
		return nil
	} else if len(data) < 16 {
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

func (c *CIFKeyMaterialExtension) Marshal(w io.Writer) error {
	if w == nil {
		return fmt.Errorf("invalid writer")
	}

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

	_, err := w.Write(buffer[:offset])

	return err
}
