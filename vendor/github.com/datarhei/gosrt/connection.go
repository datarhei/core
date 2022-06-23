package srt

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/datarhei/gosrt/internal/circular"
	"github.com/datarhei/gosrt/internal/congestion"
	"github.com/datarhei/gosrt/internal/crypto"
	"github.com/datarhei/gosrt/internal/packet"
)

// Conn is a SRT network connection.
type Conn interface {
	// Read reads data from the connection.
	// Read can be made to time out and return an error after a fixed
	// time limit; see SetDeadline and SetReadDeadline.
	Read(p []byte) (int, error)

	// Write writes data to the connection.
	// Write can be made to time out and return an error after a fixed
	// time limit; see SetDeadline and SetWriteDeadline.
	Write(p []byte) (int, error)

	// Close closes the connection.
	// Any blocked Read or Write operations will be unblocked and return errors.
	Close() error

	// LocalAddr returns the local network address. The returned net.Addr is not shared by other invocations of LocalAddr.
	LocalAddr() net.Addr

	// RemoteAddr returns the remote network address. The returned net.Addr is not shared by other invocations of RemoteAddr.
	RemoteAddr() net.Addr

	SetDeadline(t time.Time) error
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error

	// SocketId return the socketid of the connection.
	SocketId() uint32

	// PeerSocketId returns the socketid of the peer of the connection.
	PeerSocketId() uint32

	// StreamId returns the streamid use for the connection.
	StreamId() string

	// Stats returns accumulated and instantaneous statistics of the connection.
	Stats() Statistics
}

type connStats struct {
	headerSize        uint64
	pktSentACK        uint64
	pktRecvACK        uint64
	pktSentACKACK     uint64
	pktRecvACKACK     uint64
	pktSentNAK        uint64
	pktRecvNAK        uint64
	pktSentKM         uint64
	pktRecvKM         uint64
	pktRecvUndecrypt  uint64
	byteRecvUndecrypt uint64
	pktRecvInvalid    uint64
	pktSentKeepalive  uint64
	pktRecvKeepalive  uint64
	pktSentShutdown   uint64
	pktRecvShutdown   uint64
}

// Check if we implement the net.Conn interface
var _ net.Conn = &srtConn{}

type srtConn struct {
	localAddr  net.Addr
	remoteAddr net.Addr

	start time.Time

	shutdown     bool
	shutdownLock sync.RWMutex
	shutdownOnce sync.Once

	socketId     uint32
	peerSocketId uint32

	config Config

	crypto                 crypto.Crypto
	keyBaseEncryption      packet.PacketEncryption
	kmPreAnnounceCountdown uint64
	kmRefreshCountdown     uint64
	kmConfirmed            bool

	peerIdleTimeout *time.Timer

	rtt    float64 // microseconds
	rttVar float64 // microseconds

	nakInterval float64

	ackLock       sync.RWMutex
	ackNumbers    map[uint32]time.Time
	nextACKNumber circular.Number

	initialPacketSequenceNumber circular.Number

	tsbpdTimeBase       uint64 // microseconds
	tsbpdWrapPeriod     bool
	tsbpdTimeBaseOffset uint64 // microseconds
	tsbpdDelay          uint64 // microseconds
	tsbpdDrift          uint64 // microseconds

	// Queue for packets that are coming from the network
	networkQueue     chan packet.Packet
	stopNetworkQueue context.CancelFunc

	// Queue for packets that are written with writePacket() and will be send to the network
	writeQueue     chan packet.Packet
	stopWriteQueue context.CancelFunc
	writeBuffer    bytes.Buffer
	writeData      []byte

	// Queue for packets that will be read locally with ReadPacket()
	readQueue  chan packet.Packet
	readBuffer bytes.Buffer

	stopTicker context.CancelFunc

	onSend     func(p packet.Packet)
	onShutdown func(socketId uint32)

	tick time.Duration

	// Congestion control
	recv congestion.Receiver
	snd  congestion.Sender

	statistics connStats

	logger Logger

	debug struct {
		expectedRcvPacketSequenceNumber  circular.Number
		expectedReadPacketSequenceNumber circular.Number
	}
}

type srtConnConfig struct {
	localAddr                   net.Addr
	remoteAddr                  net.Addr
	config                      Config
	start                       time.Time
	socketId                    uint32
	peerSocketId                uint32
	tsbpdTimeBase               uint64 // microseconds
	tsbpdDelay                  uint64
	initialPacketSequenceNumber circular.Number
	crypto                      crypto.Crypto
	keyBaseEncryption           packet.PacketEncryption
	onSend                      func(p packet.Packet)
	onShutdown                  func(socketId uint32)
	logger                      Logger
}

func newSRTConn(config srtConnConfig) *srtConn {
	c := &srtConn{
		localAddr:                   config.localAddr,
		remoteAddr:                  config.remoteAddr,
		config:                      config.config,
		start:                       config.start,
		socketId:                    config.socketId,
		peerSocketId:                config.peerSocketId,
		tsbpdTimeBase:               config.tsbpdTimeBase,
		tsbpdDelay:                  config.tsbpdDelay,
		initialPacketSequenceNumber: config.initialPacketSequenceNumber,
		crypto:                      config.crypto,
		keyBaseEncryption:           config.keyBaseEncryption,
		onSend:                      config.onSend,
		onShutdown:                  config.onShutdown,
		logger:                      config.logger,
	}

	if c.onSend == nil {
		c.onSend = func(p packet.Packet) {}
	}

	if c.onShutdown == nil {
		c.onShutdown = func(socketId uint32) {}
	}

	c.nextACKNumber = circular.New(1, packet.MAX_TIMESTAMP)
	c.ackNumbers = make(map[uint32]time.Time)

	c.kmPreAnnounceCountdown = c.config.KMRefreshRate - c.config.KMPreAnnounce
	c.kmRefreshCountdown = c.config.KMRefreshRate

	// 4.10.  Round-Trip Time Estimation
	c.rtt = float64((100 * time.Millisecond).Microseconds())
	c.rttVar = float64((50 * time.Millisecond).Microseconds())

	c.nakInterval = float64((20 * time.Millisecond).Microseconds())

	c.networkQueue = make(chan packet.Packet, 1024)

	c.writeQueue = make(chan packet.Packet, 1024)
	c.writeData = make([]byte, int(c.config.PayloadSize))

	c.readQueue = make(chan packet.Packet, 1024)

	c.peerIdleTimeout = time.AfterFunc(c.config.PeerIdleTimeout, func() {
		c.log("connection:close", func() string {
			return fmt.Sprintf("no more data received from peer for %s. shutting down", c.config.PeerIdleTimeout)
		})
		go c.close()
	})

	c.tick = 10 * time.Millisecond

	// 4.8.1.  Packet Acknowledgement (ACKs, ACKACKs) -> periodicACK = 10 milliseconds
	// 4.8.2.  Packet Retransmission (NAKs) -> periodicNAK at least 20 milliseconds
	c.recv = congestion.NewLiveReceive(congestion.ReceiveConfig{
		InitialSequenceNumber: c.initialPacketSequenceNumber,
		PeriodicACKInterval:   10_000,
		PeriodicNAKInterval:   20_000,
		OnSendACK:             c.sendACK,
		OnSendNAK:             c.sendNAK,
		OnDeliver:             c.deliver,
	})

	// 4.6.  Too-Late Packet Drop -> 125% of SRT latency, at least 1 second
	c.snd = congestion.NewLiveSend(congestion.SendConfig{
		InitialSequenceNumber: c.initialPacketSequenceNumber,
		DropInterval:          uint64(c.config.SendDropDelay.Microseconds()),
		MaxBW:                 c.config.MaxBW,
		InputBW:               c.config.InputBW,
		MinInputBW:            c.config.MinInputBW,
		OverheadBW:            c.config.OverheadBW,
		OnDeliver:             c.pop,
	})

	var networkCtx context.Context
	networkCtx, c.stopNetworkQueue = context.WithCancel(context.Background())
	go c.networkQueueReader(networkCtx)

	var writeCtx context.Context
	writeCtx, c.stopWriteQueue = context.WithCancel(context.Background())
	go c.writeQueueReader(writeCtx)

	var tickerCtx context.Context
	tickerCtx, c.stopTicker = context.WithCancel(context.Background())
	go c.ticker(tickerCtx)

	c.debug.expectedRcvPacketSequenceNumber = c.initialPacketSequenceNumber
	c.debug.expectedReadPacketSequenceNumber = c.initialPacketSequenceNumber

	c.statistics.headerSize = 8 + 16 // 8 bytes UDP + 16 bytes SRT
	if strings.Count(c.localAddr.String(), ":") < 2 {
		c.statistics.headerSize += 20 // 20 bytes IPv4 header
	} else {
		c.statistics.headerSize += 40 // 40 bytes IPv6 header
	}

	return c
}

func (c *srtConn) LocalAddr() net.Addr {
	addr, _ := net.ResolveUDPAddr("udp", c.localAddr.String())
	return addr
}

func (c *srtConn) RemoteAddr() net.Addr {
	addr, _ := net.ResolveUDPAddr("udp", c.remoteAddr.String())
	return addr
}

func (c *srtConn) SocketId() uint32 {
	return c.socketId
}

func (c *srtConn) PeerSocketId() uint32 {
	return c.peerSocketId
}

func (c *srtConn) StreamId() string {
	return c.config.StreamId
}

// ticker invokes the congestion control in regular intervals with
// the current connection time.
func (c *srtConn) ticker(ctx context.Context) {
	ticker := time.NewTicker(c.tick)
	defer ticker.Stop()
	defer func() {
		c.log("connection:close", func() string { return "left ticker loop" })
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			tickTime := uint64(t.Sub(c.start).Microseconds())

			c.recv.Tick(c.tsbpdTimeBase + tickTime)
			c.snd.Tick(tickTime)
		}
	}
}

// readPacket reads a packet from the queue of received packets. It blocks
// if the queue is empty. Only data packets are returned.
func (c *srtConn) readPacket() (packet.Packet, error) {
	if c.isShutdown() {
		return nil, io.EOF
	}

	p := <-c.readQueue
	if p == nil {
		return nil, io.EOF
	}

	if p.Header().PacketSequenceNumber.Gt(c.debug.expectedReadPacketSequenceNumber) {
		c.log("connection:error", func() string {
			return fmt.Sprintf("lost packets. got: %d, expected: %d (%d)", p.Header().PacketSequenceNumber.Val(), c.debug.expectedReadPacketSequenceNumber.Val(), c.debug.expectedReadPacketSequenceNumber.Distance(p.Header().PacketSequenceNumber))
		})
	} else if p.Header().PacketSequenceNumber.Lt(c.debug.expectedReadPacketSequenceNumber) {
		c.log("connection:error", func() string {
			return fmt.Sprintf("packet out of order. got: %d, expected: %d (%d)", p.Header().PacketSequenceNumber.Val(), c.debug.expectedReadPacketSequenceNumber.Val(), c.debug.expectedReadPacketSequenceNumber.Distance(p.Header().PacketSequenceNumber))
		})
		return nil, io.EOF
	}

	c.debug.expectedReadPacketSequenceNumber = p.Header().PacketSequenceNumber.Inc()

	return p, nil
}

func (c *srtConn) Read(b []byte) (int, error) {
	if c.readBuffer.Len() != 0 {
		return c.readBuffer.Read(b)
	}

	c.readBuffer.Reset()

	p, err := c.readPacket()
	if err != nil {
		return 0, err
	}

	c.readBuffer.Write(p.Data())

	// The packet is out of congestion control and written to the read buffer
	p.Decommission()

	return c.readBuffer.Read(b)
}

// writePacket writes a packet to the write queue. Packets on the write queue
// will be sent to the peer of the connection. Only data packets will be sent.
func (c *srtConn) writePacket(p packet.Packet) error {
	if c.isShutdown() {
		return io.EOF
	}

	if p.Header().IsControlPacket {
		// Ignore control packets
		return nil
	}

	_, err := c.Write(p.Data())
	if err != nil {
		return err
	}

	return nil
}

func (c *srtConn) Write(b []byte) (int, error) {
	c.writeBuffer.Write(b)

	for {
		n, err := c.writeBuffer.Read(c.writeData)
		if err != nil {
			return 0, err
		}

		p := packet.NewPacket(nil, nil)

		p.SetData(c.writeData[:n])

		p.Header().IsControlPacket = false
		// Give the packet a deliver timestamp
		p.Header().PktTsbpdTime = c.getTimestamp()

		if c.isShutdown() {
			return 0, io.EOF
		}

		// Non-blocking write to the write queue
		select {
		case c.writeQueue <- p:
		default:
			return 0, io.EOF
		}

		if c.writeBuffer.Len() == 0 {
			break
		}
	}

	c.writeBuffer.Reset()

	return len(b), nil
}

// push puts a packet on the network queue. This is where packets go that came in from the network.
func (c *srtConn) push(p packet.Packet) {
	if c.isShutdown() {
		return
	}

	// Non-blocking write to the network queue
	select {
	case c.networkQueue <- p:
	default:
		c.log("connection:error", func() string { return "network queue is full" })
	}
}

// getTimestamp returns the elapsed time since the start of the connection in microseconds.
func (c *srtConn) getTimestamp() uint64 {
	return uint64(time.Since(c.start).Microseconds())
}

// getTimestampForPacket returns the elapsed time since the start of the connection in
// microseconds clamped a 32bit value.
func (c *srtConn) getTimestampForPacket() uint32 {
	return uint32(c.getTimestamp() & uint64(packet.MAX_TIMESTAMP))
}

// pop adds the destination address and socketid to the packet and sends it out to the network.
// The packet will be encrypted if required.
func (c *srtConn) pop(p packet.Packet) {
	p.Header().Addr = c.remoteAddr
	p.Header().DestinationSocketId = c.peerSocketId

	if !p.Header().IsControlPacket {
		if c.crypto != nil {
			p.Header().KeyBaseEncryptionFlag = c.keyBaseEncryption
			c.crypto.EncryptOrDecryptPayload(p.Data(), p.Header().KeyBaseEncryptionFlag, p.Header().PacketSequenceNumber.Val())

			c.kmPreAnnounceCountdown--
			c.kmRefreshCountdown--

			if c.kmPreAnnounceCountdown == 0 && !c.kmConfirmed {
				c.sendKMRequest()

				// Resend the request until we get a response
				c.kmPreAnnounceCountdown = c.config.KMPreAnnounce/10 + 1
			}

			if c.kmRefreshCountdown == 0 {
				c.kmPreAnnounceCountdown = c.config.KMRefreshRate - c.config.KMPreAnnounce
				c.kmRefreshCountdown = c.config.KMRefreshRate

				// Switch the keys
				c.keyBaseEncryption = c.keyBaseEncryption.Opposite()

				c.kmConfirmed = false
			}

			if c.kmRefreshCountdown == c.config.KMRefreshRate-c.config.KMPreAnnounce {
				// Decommission the previous key, resp. create a new SEK that will
				// be used in the next switch.
				c.crypto.GenerateSEK(c.keyBaseEncryption.Opposite())
			}
		}

		c.log("data:send:dump", func() string { return p.Dump() })
	}

	// Send the packet on the wire
	c.onSend(p)
}

// networkQueueReader reads the packets from the network queue in order to process them.
func (c *srtConn) networkQueueReader(ctx context.Context) {
	defer func() {
		c.log("connection:close", func() string { return "left network queue reader loop" })
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case p := <-c.networkQueue:
			c.handlePacket(p)
		}
	}
}

// writeQueueReader reads the packets from the write queue and puts them into congestion
// control for sending.
func (c *srtConn) writeQueueReader(ctx context.Context) {
	defer func() {
		c.log("connection:close", func() string { return "left write queue reader loop" })
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case p := <-c.writeQueue:
			// Put the packet into the send congestion control
			c.snd.Push(p)
		}
	}
}

// deliver writes the packets to the read queue in order to be consumed by the Read function.
func (c *srtConn) deliver(p packet.Packet) {
	if c.isShutdown() {
		return
	}

	// Non-blocking write to the read queue
	select {
	case c.readQueue <- p:
	default:
		c.log("connection:error", func() string { return "readQueue was blocking, dropping packet" })
	}
}

// handlePacket checks the packet header. If it is a control packet it will forwarded to the
// respective handler. If it is a data packet it will be put into congestion control for
// receiving. The packet will be decrypted if required.
func (c *srtConn) handlePacket(p packet.Packet) {
	if p == nil {
		return
	}

	c.peerIdleTimeout.Reset(c.config.PeerIdleTimeout)

	header := p.Header()

	if header.IsControlPacket {
		if header.ControlType == packet.CTRLTYPE_KEEPALIVE {
			c.handleKeepAlive(p)
		} else if header.ControlType == packet.CTRLTYPE_SHUTDOWN {
			c.handleShutdown(p)
		} else if header.ControlType == packet.CTRLTYPE_NAK {
			c.handleNAK(p)
		} else if header.ControlType == packet.CTRLTYPE_ACK {
			c.handleACK(p)
		} else if header.ControlType == packet.CTRLTYPE_ACKACK {
			c.handleACKACK(p)
		} else if header.ControlType == packet.CTRLTYPE_USER {
			// 3.2.2.  Key Material
			if header.SubType == packet.EXTTYPE_KMREQ {
				c.handleKMRequest(p)
			} else if header.SubType == packet.EXTTYPE_KMRSP {
				c.handleKMResponse(p)
			}
		}
	} else {
		if header.PacketSequenceNumber.Gt(c.debug.expectedRcvPacketSequenceNumber) {
			c.log("connection:error", func() string {
				return fmt.Sprintf("recv lost packets. got: %d, expected: %d (%d)\n", header.PacketSequenceNumber.Val(), c.debug.expectedRcvPacketSequenceNumber.Val(), c.debug.expectedRcvPacketSequenceNumber.Distance(header.PacketSequenceNumber))
			})
		}

		c.debug.expectedRcvPacketSequenceNumber = header.PacketSequenceNumber.Inc()

		// Ignore FEC filter control packets
		// https://github.com/Haivision/srt/blob/master/docs/features/packet-filtering-and-fec.md
		// "An FEC control packet is distinguished from a regular data packet by having
		// its message number equal to 0. This value isn't normally used in SRT (message
		// numbers start from 1, increment to a maximum, and then roll back to 1)."
		if header.MessageNumber == 0 {
			c.log("connection:filter", func() string { return "dropped FEC filter control packet" })
			return
		}

		// 4.5.1.1.  TSBPD Time Base Calculation
		if !c.tsbpdWrapPeriod {
			if header.Timestamp > packet.MAX_TIMESTAMP-(30*1000000) {
				c.tsbpdWrapPeriod = true
				c.log("connection:tsbpd", func() string { return "TSBPD wrapping period started" })
			}
		} else {
			if header.Timestamp >= (30*1000000) && header.Timestamp <= (60*1000000) {
				c.tsbpdWrapPeriod = false
				c.tsbpdTimeBaseOffset += uint64(packet.MAX_TIMESTAMP) + 1
				c.log("connection:tsbpd", func() string { return "TSBPD wrapping period finished" })
			}
		}

		tsbpdTimeBaseOffset := c.tsbpdTimeBaseOffset
		if c.tsbpdWrapPeriod {
			if header.Timestamp < (30 * 1000000) {
				tsbpdTimeBaseOffset += uint64(packet.MAX_TIMESTAMP) + 1
			}
		}

		header.PktTsbpdTime = c.tsbpdTimeBase + tsbpdTimeBaseOffset + uint64(header.Timestamp) + c.tsbpdDelay + c.tsbpdDrift

		c.log("data:recv:dump", func() string { return p.Dump() })

		if c.crypto != nil {
			if header.KeyBaseEncryptionFlag != 0 {
				if err := c.crypto.EncryptOrDecryptPayload(p.Data(), header.KeyBaseEncryptionFlag, header.PacketSequenceNumber.Val()); err != nil {
					c.statistics.pktRecvUndecrypt++
					c.statistics.byteRecvUndecrypt += p.Len()
				}
			} else {
				c.statistics.pktRecvUndecrypt++
				c.statistics.byteRecvUndecrypt += p.Len()
			}
		}

		// Put the packet into receive congestion control
		c.recv.Push(p)
	}
}

// handleKeepAlive resets the idle timeout and sends a keepalive to the peer.
func (c *srtConn) handleKeepAlive(p packet.Packet) {
	c.log("control:recv:keepalive:dump", func() string { return p.Dump() })

	c.statistics.pktRecvKeepalive++
	c.statistics.pktSentKeepalive++

	c.peerIdleTimeout.Reset(c.config.PeerIdleTimeout)

	c.log("control:send:keepalive:dump", func() string { return p.Dump() })

	c.pop(p)
}

// handleShutdown closes the connection
func (c *srtConn) handleShutdown(p packet.Packet) {
	c.log("control:recv:shutdown:dump", func() string { return p.Dump() })

	c.statistics.pktRecvShutdown++

	go c.close()
}

// handleACK forwards the acknowledge sequence number to the congestion control and
// returns a ACKACK (on a full ACK). The RTT is also updated in case of a full ACK.
func (c *srtConn) handleACK(p packet.Packet) {
	c.log("control:recv:ACK:dump", func() string { return p.Dump() })

	c.statistics.pktRecvACK++

	cif := &packet.CIFACK{}

	if err := p.UnmarshalCIF(cif); err != nil {
		c.statistics.pktRecvInvalid++
		c.log("control:recv:ACK:error", func() string { return fmt.Sprintf("invalid ACK: %s", err) })
		return
	}

	c.log("control:recv:ACK:cif", func() string { return cif.String() })

	c.snd.ACK(cif.LastACKPacketSequenceNumber)

	if !cif.IsLite && !cif.IsSmall {
		// 4.10.  Round-Trip Time Estimation
		c.recalculateRTT(time.Duration(int64(cif.RTT)) * time.Microsecond)

		c.sendACKACK(p.Header().TypeSpecific)
	}
}

// handleNAK forwards the lost sequence number to the congestion control.
func (c *srtConn) handleNAK(p packet.Packet) {
	c.log("control:recv:NAK:dump", func() string { return p.Dump() })

	c.statistics.pktRecvNAK++

	cif := &packet.CIFNAK{}

	if err := p.UnmarshalCIF(cif); err != nil {
		c.statistics.pktRecvInvalid++
		c.log("control:recv:NAK:error", func() string { return fmt.Sprintf("invalid NAK: %s", err) })
		return
	}

	c.log("control:recv:NAK:cif", func() string { return cif.String() })

	// Inform congestion control about lost packets
	c.snd.NAK(cif.LostPacketSequenceNumber)
}

// handleACKACK updates the RTT and NAK interval for the congestion control.
func (c *srtConn) handleACKACK(p packet.Packet) {
	c.ackLock.RLock()

	c.statistics.pktRecvACKACK++

	c.log("control:recv:ACKACK:dump", func() string { return p.Dump() })

	// p.typeSpecific is the ACKNumber
	if ts, ok := c.ackNumbers[p.Header().TypeSpecific]; ok {
		// 4.10.  Round-Trip Time Estimation
		c.recalculateRTT(time.Since(ts))
		delete(c.ackNumbers, p.Header().TypeSpecific)
	} else {
		c.log("control:recv:ACKACK:error", func() string { return fmt.Sprintf("got unknown ACKACK (%d)", p.Header().TypeSpecific) })
		c.statistics.pktRecvInvalid++
	}

	for i := range c.ackNumbers {
		if i < p.Header().TypeSpecific {
			delete(c.ackNumbers, i)
		}
	}

	nakInterval := uint64(c.nakInterval)

	c.ackLock.RUnlock()

	c.recv.SetNAKInterval(nakInterval)
}

// recalculateRTT recalculates the RTT based on a full ACK exchange
func (c *srtConn) recalculateRTT(rtt time.Duration) {
	// 4.10.  Round-Trip Time Estimation
	lastRTT := float64(rtt.Microseconds())

	c.rtt = c.rtt*0.875 + lastRTT*0.125
	c.rttVar = c.rttVar*0.75 + math.Abs(c.rtt-lastRTT)*0.25

	// 4.8.2.  Packet Retransmission (NAKs)
	nakInterval := (c.rtt + 4*c.rttVar) / 2
	if nakInterval < 20000 {
		c.nakInterval = 20000 // 20ms
	} else {
		c.nakInterval = nakInterval
	}

	c.log("connection:rtt", func() string {
		return fmt.Sprintf("RTT=%.0fus RTTVar=%.0fus NAKInterval=%.0fms", c.rtt, c.rttVar, c.nakInterval/1000)
	})
}

// handleKMRequest checks if the key material is valid and responds with a KM response.
func (c *srtConn) handleKMRequest(p packet.Packet) {
	c.log("control:recv:KM:dump", func() string { return p.Dump() })

	c.statistics.pktRecvKM++

	if c.crypto == nil {
		c.log("control:recv:KM:error", func() string { return "connection is not encrypted" })
		return
	}

	cif := &packet.CIFKM{}

	if err := p.UnmarshalCIF(cif); err != nil {
		c.statistics.pktRecvInvalid++
		c.log("control:recv:KM:error", func() string { return fmt.Sprintf("invalid KM: %s", err) })
		return
	}

	c.log("control:recv:KM:cif", func() string { return cif.String() })

	if cif.KeyBasedEncryption == c.keyBaseEncryption {
		c.statistics.pktRecvInvalid++
		c.log("control:recv:KM:error", func() string {
			return "invalid KM. wants to reset the key that is already in use"
		})
		return
	}

	if err := c.crypto.UnmarshalKM(cif, c.config.Passphrase); err != nil {
		c.statistics.pktRecvInvalid++
		c.log("control:recv:KM:error", func() string { return fmt.Sprintf("invalid KM: %s", err) })
		return
	}

	p.Header().SubType = packet.EXTTYPE_KMRSP

	c.statistics.pktSentKM++

	c.pop(p)
}

// handleKMResponse confirms the change of encryption keys.
func (c *srtConn) handleKMResponse(p packet.Packet) {
	c.log("control:recv:KM:dump", func() string { return p.Dump() })

	c.statistics.pktRecvKM++

	if c.crypto == nil {
		c.log("control:recv:KM:error", func() string { return "connection is not encrypted" })
		return
	}

	if c.kmPreAnnounceCountdown >= c.config.KMPreAnnounce {
		c.log("control:recv:KM:error", func() string { return "not in pre-announce period" })
		// Ignore the response, we're not in the pre-announce period
		return
	}

	c.kmConfirmed = true
}

// sendShutdown sends a shutdown packet to the peer.
func (c *srtConn) sendShutdown() {
	p := packet.NewPacket(c.remoteAddr, nil)

	p.Header().IsControlPacket = true

	p.Header().ControlType = packet.CTRLTYPE_SHUTDOWN
	p.Header().Timestamp = c.getTimestampForPacket()

	cif := packet.CIFShutdown{}

	p.MarshalCIF(&cif)

	c.log("control:send:shutdown:dump", func() string { return p.Dump() })
	c.log("control:send:shutdown:cif", func() string { return cif.String() })

	c.statistics.pktSentShutdown++

	c.pop(p)
}

// sendNAK sends a NAK to the peer with the given range of sequence numbers.
func (c *srtConn) sendNAK(from, to circular.Number) {
	p := packet.NewPacket(c.remoteAddr, nil)

	p.Header().IsControlPacket = true

	p.Header().ControlType = packet.CTRLTYPE_NAK
	p.Header().Timestamp = c.getTimestampForPacket()

	cif := packet.CIFNAK{}

	cif.LostPacketSequenceNumber = append(cif.LostPacketSequenceNumber, from)
	cif.LostPacketSequenceNumber = append(cif.LostPacketSequenceNumber, to)

	p.MarshalCIF(&cif)

	c.log("control:send:NAK:dump", func() string { return p.Dump() })
	c.log("control:send:NAK:cif", func() string { return cif.String() })

	c.statistics.pktSentNAK++

	c.pop(p)
}

// sendACK sends an ACK to the peer with the given sequence number.
func (c *srtConn) sendACK(seq circular.Number, lite bool) {
	p := packet.NewPacket(c.remoteAddr, nil)

	p.Header().IsControlPacket = true

	p.Header().ControlType = packet.CTRLTYPE_ACK
	p.Header().Timestamp = c.getTimestampForPacket()

	cif := packet.CIFACK{
		LastACKPacketSequenceNumber: seq,
	}

	c.ackLock.Lock()
	defer c.ackLock.Unlock()

	if lite {
		cif.IsLite = true

		p.Header().TypeSpecific = 0
	} else {
		pps, _ := c.recv.PacketRate()

		cif.RTT = uint32(c.rtt)
		cif.RTTVar = uint32(c.rttVar)
		cif.AvailableBufferSize = c.config.FC // TODO: available buffer size (packets)
		cif.PacketsReceivingRate = pps        // packets receiving rate (packets/s)
		cif.EstimatedLinkCapacity = 0         // estimated link capacity (packets/s), not relevant for live mode
		cif.ReceivingRate = 0                 // receiving rate (bytes/s), not relevant for live mode

		p.Header().TypeSpecific = c.nextACKNumber.Val()

		c.ackNumbers[p.Header().TypeSpecific] = time.Now()
		c.nextACKNumber = c.nextACKNumber.Inc()
		if c.nextACKNumber.Val() == 0 {
			c.nextACKNumber = c.nextACKNumber.Inc()
		}
	}

	p.MarshalCIF(&cif)

	c.log("control:send:ACK:dump", func() string { return p.Dump() })
	c.log("control:send:ACK:cif", func() string { return cif.String() })

	c.statistics.pktSentACK++

	c.pop(p)
}

// sendACKACK sends an ACKACK to the peer with the given ACK sequence.
func (c *srtConn) sendACKACK(ackSequence uint32) {
	p := packet.NewPacket(c.remoteAddr, nil)

	p.Header().IsControlPacket = true

	p.Header().ControlType = packet.CTRLTYPE_ACKACK
	p.Header().Timestamp = c.getTimestampForPacket()

	p.Header().TypeSpecific = ackSequence

	c.log("control:send:ACKACK:dump", func() string { return p.Dump() })

	c.statistics.pktSentACKACK++

	c.pop(p)
}

// sendKMRequest sends a KM request to the peer.
func (c *srtConn) sendKMRequest() {
	if c.crypto == nil {
		c.log("control:send:KM:error", func() string { return "connection is not encrypted" })
		return
	}

	cif := &packet.CIFKM{}

	c.crypto.MarshalKM(cif, c.config.Passphrase, c.keyBaseEncryption.Opposite())

	p := packet.NewPacket(c.remoteAddr, nil)

	p.Header().IsControlPacket = true

	p.Header().ControlType = packet.CTRLTYPE_USER
	p.Header().SubType = packet.EXTTYPE_KMREQ
	p.Header().Timestamp = c.getTimestampForPacket()

	p.MarshalCIF(cif)

	c.log("control:send:KM:dump", func() string { return p.Dump() })
	c.log("control:send:KM:cif", func() string { return cif.String() })

	c.statistics.pktSentKM++

	c.pop(p)
}

// Close closes the connection.
func (c *srtConn) Close() error {
	c.close()

	return nil
}

func (c *srtConn) isShutdown() bool {
	c.shutdownLock.RLock()
	defer c.shutdownLock.RUnlock()

	return c.shutdown
}

// close closes the connection.
func (c *srtConn) close() {
	c.shutdownLock.Lock()
	c.shutdown = true
	c.shutdownLock.Unlock()

	c.shutdownOnce.Do(func() {
		c.log("connection:close", func() string { return "stopping peer idle timeout" })

		c.peerIdleTimeout.Stop()

		c.log("connection:close", func() string { return "sending shutdown message to peer" })

		c.sendShutdown()

		c.log("connection:close", func() string { return "stopping reader" })

		// send nil to the readQueue in order to abort any pending ReadPacket call
		c.readQueue <- nil

		c.log("connection:close", func() string { return "stopping network reader" })

		c.stopNetworkQueue()

		c.log("connection:close", func() string { return "stopping writer" })

		c.stopWriteQueue()

		c.log("connection:close", func() string { return "stopping ticker" })

		c.stopTicker()

		c.log("connection:close", func() string { return "closing queues" })

		close(c.networkQueue)
		close(c.readQueue)
		close(c.writeQueue)

		c.log("connection:close", func() string { return "flushing congestion" })

		c.snd.Flush()
		c.recv.Flush()

		c.log("connection:close", func() string { return "shutdown" })

		go func() {
			c.onShutdown(c.socketId)
		}()
	})
}

func (c *srtConn) log(topic string, message func() string) {
	c.logger.Print(topic, c.socketId, 2, message)
}

func (c *srtConn) SetDeadline(t time.Time) error      { return nil }
func (c *srtConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *srtConn) SetWriteDeadline(t time.Time) error { return nil }

func (c *srtConn) Stats() Statistics {
	send := c.snd.Stats()
	recv := c.recv.Stats()

	s := Statistics{
		MsTimeStamp: uint64(time.Since(c.start).Milliseconds()),

		// Accumulated
		PktSent:          send.PktSent,
		PktRecv:          recv.PktRecv,
		PktSentUnique:    send.PktSentUnique,
		PktRecvUnique:    recv.PktRecvUnique,
		PktSndLoss:       send.PktSndLoss,
		PktRcvLoss:       recv.PktRcvLoss,
		PktRetrans:       send.PktRetrans,
		PktRcvRetrans:    recv.PktRcvRetrans,
		PktSentACK:       c.statistics.pktSentACK,
		PktRecvACK:       c.statistics.pktRecvACK,
		PktSentNAK:       c.statistics.pktSentNAK,
		PktRecvNAK:       c.statistics.pktRecvNAK,
		PktSentKM:        c.statistics.pktSentKM,
		PktRecvKM:        c.statistics.pktRecvKM,
		UsSndDuration:    send.UsSndDuration,
		PktSndDrop:       send.PktSndDrop,
		PktRcvDrop:       recv.PktRcvDrop,
		PktRcvUndecrypt:  c.statistics.pktRecvUndecrypt,
		ByteSent:         send.ByteSent + (send.PktSent * c.statistics.headerSize),
		ByteRecv:         recv.ByteRecv + (recv.PktRecv * c.statistics.headerSize),
		ByteSentUnique:   send.ByteSentUnique + (send.PktSentUnique * c.statistics.headerSize),
		ByteRecvUnique:   recv.ByteRecvUnique + (recv.PktRecvUnique * c.statistics.headerSize),
		ByteRcvLoss:      recv.ByteRcvLoss + (recv.PktRcvLoss * c.statistics.headerSize),
		ByteRetrans:      send.ByteRetrans + (send.PktRetrans * c.statistics.headerSize),
		ByteSndDrop:      send.ByteSndDrop + (send.PktSndDrop * c.statistics.headerSize),
		ByteRcvDrop:      recv.ByteRcvDrop + (recv.PktRcvDrop * c.statistics.headerSize),
		ByteRcvUndecrypt: c.statistics.byteRecvUndecrypt + (c.statistics.pktRecvUndecrypt * c.statistics.headerSize),

		// Instantaneous
		UsPktSndPeriod:       send.UsPktSndPeriod,
		PktFlowWindow:        uint64(c.config.FC),
		PktFlightSize:        send.PktFlightSize,
		MsRTT:                c.rtt / 1_000,
		MbpsBandwidth:        0,
		ByteAvailSndBuf:      0,
		ByteAvailRcvBuf:      0,
		MbpsMaxBW:            float64(c.config.MaxBW / 1024 / 1024),
		ByteMSS:              uint64(c.config.MSS),
		PktSndBuf:            send.PktSndBuf,
		ByteSndBuf:           send.ByteSndBuf,
		MsSndBuf:             send.MsSndBuf,
		MsSndTsbPdDelay:      uint64(c.config.PeerLatency),
		PktRcvBuf:            recv.PktRcvBuf,
		ByteRcvBuf:           recv.ByteRcvBuf,
		MsRcvBuf:             recv.MsRcvBuf,
		MsRcvTsbPdDelay:      uint64(c.config.ReceiverLatency),
		PktReorderTolerance:  0,
		PktRcvAvgBelatedTime: 0,
	}

	return s
}
