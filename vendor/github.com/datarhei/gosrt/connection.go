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

	"github.com/datarhei/gosrt/circular"
	"github.com/datarhei/gosrt/congestion"
	"github.com/datarhei/gosrt/congestion/live"
	"github.com/datarhei/gosrt/crypto"
	"github.com/datarhei/gosrt/fec"
	"github.com/datarhei/gosrt/packet"
)

// Conn is a SRT network connection.
type Conn interface {
	// Read reads data from the connection.
	// Read can be made to time out and return an error after a fixed
	// time limit; see SetDeadline and SetReadDeadline.
	Read(p []byte) (int, error)

	// ReadPacket reads a packet from the queue of received packets. It blocks
	// if the queue is empty. Only data packets are returned. Using ReadPacket
	// and Read at the same time may lead to data loss.
	ReadPacket() (packet.Packet, error)

	// Write writes data to the connection.
	// Write can be made to time out and return an error after a fixed
	// time limit; see SetDeadline and SetWriteDeadline.
	Write(p []byte) (int, error)

	// WritePacket writes a packet to the write queue. Packets on the write queue
	// will be sent to the peer of the connection. Only data packets will be sent.
	WritePacket(p packet.Packet) error

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
	Stats(s *Statistics)

	// Version returns the connection version, either 4 or 5. With version 4, the streamid is not available
	Version() uint32
}

type rtt struct {
	rtt    float64 // microseconds
	rttVar float64 // microseconds

	lock sync.RWMutex
}

func (r *rtt) Recalculate(rtt time.Duration) {
	// 4.10.  Round-Trip Time Estimation
	lastRTT := float64(rtt.Microseconds())

	r.lock.Lock()
	defer r.lock.Unlock()

	r.rtt = r.rtt*0.875 + lastRTT*0.125
	r.rttVar = r.rttVar*0.75 + math.Abs(r.rtt-lastRTT)*0.25
}

func (r *rtt) RTT() float64 {
	r.lock.RLock()
	defer r.lock.RUnlock()

	return r.rtt
}

func (r *rtt) RTTVar() float64 {
	r.lock.RLock()
	defer r.lock.RUnlock()

	return r.rttVar
}

func (r *rtt) NAKInterval() float64 {
	r.lock.RLock()
	defer r.lock.RUnlock()

	// 4.8.2.  Packet Retransmission (NAKs)
	nakInterval := (r.rtt + 4*r.rttVar) / 2
	if nakInterval < 20000 {
		nakInterval = 20000 // 20ms
	}

	return nakInterval
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
	mbpsLinkCapacity  float64
}

// Check if we implement the net.Conn interface
var _ net.Conn = &srtConn{}

// ackRecord is a full ACK awaiting its ACKACK: when it was sent, for the RTT
// estimate, and what it acknowledged, so that a repeat of it can be recognised
// once the peer confirms it.
type ackRecord struct {
	timestamp time.Time
	seq       circular.Number
}

type srtConn struct {
	version  uint32
	isCaller bool // Only relevant if version == 4

	localAddr  net.Addr
	remoteAddr net.Addr

	start time.Time

	shutdownOnce sync.Once

	socketId     uint32
	peerSocketId uint32

	config Config

	crypto                 crypto.Crypto
	keyBaseEncryption      packet.PacketEncryption
	kmPreAnnounceCountdown uint64
	kmRefreshCountdown     uint64
	kmConfirmed            bool
	cryptoLock             sync.Mutex

	peerIdleTimeout *time.Timer

	rtt rtt // microseconds

	ackLock       sync.RWMutex
	ackNumbers    map[uint32]ackRecord
	nextACKNumber circular.Number

	// Bookkeeping for suppressing periodic ACKs that carry no news. See
	// sendACK.
	lastSentACK     circular.Number
	lastSentACKTime time.Time
	haveSentACK     bool
	lastACKedACK    circular.Number
	haveACKedACK    bool

	initialPacketSequenceNumber circular.Number

	tsbpdTimeBase       uint64 // microseconds
	tsbpdWrapPeriod     bool
	tsbpdTimeBaseOffset uint64 // microseconds
	tsbpdDelay          uint64 // microseconds
	tsbpdPeerOffset     int64  // microseconds
	havePeerOffset      bool
	tsbpdDrift          int64 // microseconds
	driftTracer         *driftTracer
	peerTsbpdDelay      uint64 // microseconds
	dropThreshold       uint64 // microseconds

	// Queue for packets that are coming from the network
	networkQueue chan packet.Packet

	// Queue for packets that are written with writePacket() and will be send to the network
	writeQueue  chan packet.Packet
	writeBuffer bytes.Buffer
	writeData   []byte

	// Queue for packets that will be read locally with ReadPacket()
	readQueue  chan packet.Packet
	readBuffer bytes.Buffer

	onSend     func(p packet.Packet)
	onShutdown func(*srtConn)

	tick time.Duration

	// Congestion control
	recv congestion.Receiver
	snd  congestion.Sender

	// FEC
	fecGen *fec.Generator
	fecRec *fec.Reconstructor

	// context of all channels and routines
	ctx       context.Context
	cancelCtx context.CancelFunc

	statistics     connStats
	statisticsLock sync.RWMutex

	logger Logger

	debug struct {
		expectedRcvPacketSequenceNumber  circular.Number
		expectedReadPacketSequenceNumber circular.Number
	}

	// HSv4
	stopHSRequests context.CancelFunc
	stopKMRequests context.CancelFunc
}

type srtConnConfig struct {
	version                     uint32
	isCaller                    bool
	localAddr                   net.Addr
	remoteAddr                  net.Addr
	config                      Config
	start                       time.Time
	socketId                    uint32
	peerSocketId                uint32
	tsbpdTimeBase               uint64 // microseconds
	tsbpdDelay                  uint64 // microseconds
	peerTsbpdDelay              uint64 // microseconds
	initialPacketSequenceNumber circular.Number
	crypto                      crypto.Crypto
	keyBaseEncryption           packet.PacketEncryption
	onSend                      func(p packet.Packet)
	onShutdown                  func(*srtConn)
	logger                      Logger
}

func newSRTConn(config srtConnConfig) *srtConn {
	c := &srtConn{
		version:                     config.version,
		isCaller:                    config.isCaller,
		localAddr:                   config.localAddr,
		remoteAddr:                  config.remoteAddr,
		config:                      config.config,
		start:                       config.start,
		socketId:                    config.socketId,
		peerSocketId:                config.peerSocketId,
		tsbpdTimeBase:               config.tsbpdTimeBase,
		tsbpdDelay:                  config.tsbpdDelay,
		peerTsbpdDelay:              config.peerTsbpdDelay,
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
		c.onShutdown = func(*srtConn) {}
	}

	c.nextACKNumber = circular.New(1, packet.MAX_TIMESTAMP)
	c.ackNumbers = make(map[uint32]ackRecord)

	c.kmPreAnnounceCountdown = c.config.KMRefreshRate - c.config.KMPreAnnounce
	c.kmRefreshCountdown = c.config.KMRefreshRate

	// 4.10.  Round-Trip Time Estimation
	c.rtt = rtt{
		rtt:    float64((100 * time.Millisecond).Microseconds()),
		rttVar: float64((50 * time.Millisecond).Microseconds()),
	}

	c.networkQueue = make(chan packet.Packet, 1024)

	c.writeQueue = make(chan packet.Packet, 1024)
	if c.version == 4 {
		// libsrt-1.2.3 receiver doesn't like it when the payload is larger than 7*188 bytes.
		// Here we just take a multiple of a mpegts chunk size.
		c.writeData = make([]byte, int(c.config.PayloadSize/188*188))
	} else {
		// For v5 we use the max. payload size: https://github.com/Haivision/srt/issues/876
		c.writeData = make([]byte, int(c.config.PayloadSize))
	}

	c.readQueue = make(chan packet.Packet, 1024)

	if c.config.DriftTracer {
		c.driftTracer = newDriftTracer()
	}

	c.peerIdleTimeout = time.AfterFunc(c.config.PeerIdleTimeout, func() {
		c.log("connection:close", func() string {
			return fmt.Sprintf("no more data received from peer for %s. shutting down", c.config.PeerIdleTimeout)
		})
		go c.close()
	})

	c.tick = 10 * time.Millisecond

	// 4.8.1.  Packet Acknowledgement (ACKs, ACKACKs) -> periodicACK = 10 milliseconds
	// 4.8.2.  Packet Retransmission (NAKs) -> periodicNAK at least 20 milliseconds
	c.recv = live.NewReceiver(live.ReceiveConfig{
		InitialSequenceNumber: c.initialPacketSequenceNumber,
		PeriodicACKInterval:   10_000,
		PeriodicNAKInterval:   20_000,
		OnSendACK:             c.sendACK,
		OnSendNAK:             c.sendNAK,
		OnDeliver:             c.deliver,
	})

	// 4.6.  Too-Late Packet Drop -> 125% of SRT latency, at least 1 second
	// https://github.com/Haivision/srt/blob/master/docs/API/API-socket-options.md#SRTO_SNDDROPDELAY
	c.dropThreshold = max(uint64(float64(c.peerTsbpdDelay)*1.25)+uint64(c.config.SendDropDelay.Microseconds()), uint64(time.Second.Microseconds()))
	c.dropThreshold += 20_000

	c.snd = live.NewSender(live.SendConfig{
		InitialSequenceNumber: c.initialPacketSequenceNumber,
		DropThreshold:         c.dropThreshold,
		MaxBW:                 c.config.MaxBW,
		InputBW:               c.config.InputBW,
		MinInputBW:            c.config.MinInputBW,
		OverheadBW:            c.config.OverheadBW,
		OnDeliver:             c.pop,
	})

	if len(c.config.PacketFilter) > 0 {
		fecCfg, err := fec.ParseConfig(c.config.PacketFilter)
		if err == nil {
			c.fecGen = fec.NewGenerator(fecCfg)
			c.fecRec = fec.NewReconstructor(fecCfg)
		} else {
			c.log("connection:error", func() string { return fmt.Sprintf("invalid FEC config: %s", err) })
		}
	}

	c.ctx, c.cancelCtx = context.WithCancel(context.Background())

	go c.networkQueueReader(c.ctx)
	go c.writeQueueReader(c.ctx)
	go c.ticker(c.ctx)

	c.debug.expectedRcvPacketSequenceNumber = c.initialPacketSequenceNumber
	c.debug.expectedReadPacketSequenceNumber = c.initialPacketSequenceNumber

	c.statistics.headerSize = 8 + 16 // 8 bytes UDP + 16 bytes SRT
	if strings.Count(c.localAddr.String(), ":") < 2 {
		c.statistics.headerSize += 20 // 20 bytes IPv4 header
	} else {
		c.statistics.headerSize += 40 // 40 bytes IPv6 header
	}

	if c.version == 4 && c.isCaller {
		var hsrequestsCtx context.Context
		hsrequestsCtx, c.stopHSRequests = context.WithCancel(context.Background())
		go c.sendHSRequests(hsrequestsCtx)

		if c.crypto != nil {
			var kmrequestsCtx context.Context
			kmrequestsCtx, c.stopKMRequests = context.WithCancel(context.Background())
			go c.sendKMRequests(kmrequestsCtx)
		}
	}

	return c
}

func (c *srtConn) LocalAddr() net.Addr {
	if c.localAddr == nil {
		return nil
	}

	addr, _ := net.ResolveUDPAddr("udp", c.localAddr.String())
	return addr
}

func (c *srtConn) RemoteAddr() net.Addr {
	if c.remoteAddr == nil {
		return nil
	}

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

func (c *srtConn) Version() uint32 {
	return c.version
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

func (c *srtConn) ReadPacket() (packet.Packet, error) {
	var p packet.Packet
	select {
	case <-c.ctx.Done():
		return nil, io.EOF
	case p = <-c.readQueue:
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

	p, err := c.ReadPacket()
	if err != nil {
		return 0, err
	}

	c.readBuffer.Write(p.Data())

	// The packet is out of congestion control and written to the read buffer
	p.Decommission()

	return c.readBuffer.Read(b)
}

// WritePacket writes a packet to the write queue. Packets on the write queue
// will be sent to the peer of the connection. Only data packets will be sent.
func (c *srtConn) WritePacket(p packet.Packet) error {
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

		p := packet.NewPacket(nil)

		p.SetData(c.writeData[:n])

		p.Header().IsControlPacket = false
		// Give the packet a deliver timestamp
		p.Header().PktTsbpdTime = c.getTimestamp()

		// Non-blocking write to the write queue
		select {
		case <-c.ctx.Done():
			return 0, io.EOF
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
	// Non-blocking write to the network queue
	select {
	case <-c.ctx.Done():
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
	p.Header().LocalAddr = c.localAddr
	p.Header().DestinationSocketId = c.peerSocketId

	if !p.Header().IsControlPacket {
		c.cryptoLock.Lock()
		if c.crypto != nil {
			p.Header().KeyBaseEncryptionFlag = c.keyBaseEncryption
			if !p.Header().RetransmittedPacketFlag {
				c.crypto.EncryptOrDecryptPayload(p.Data(), p.Header().KeyBaseEncryptionFlag, p.Header().PacketSequenceNumber.Val())
			}

			c.kmPreAnnounceCountdown--
			c.kmRefreshCountdown--

			if c.kmPreAnnounceCountdown == 0 && !c.kmConfirmed {
				c.sendKMRequest(c.keyBaseEncryption.Opposite())

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
		c.cryptoLock.Unlock()

		c.log("data:send:dump", func() string { return p.Dump() })
	}

	// Send the packet on the wire
	c.onSend(p)

	if c.fecGen != nil && !p.Header().IsControlPacket {
		fecPkts := c.fecGen.AddPacket(p)
		for _, fecPkt := range fecPkts {
			c.log("fec:send:dump", func() string { return fecPkt.Dump() })
			c.onSend(fecPkt)
		}
	}
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
	// Non-blocking write to the read queue
	select {
	case <-c.ctx.Done():
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
			c.log("connection:recv:ctrl:user", func() string {
				return fmt.Sprintf("got CTRLTYPE_USER packet, subType: %s", header.SubType)
			})

			// HSv4 Extension
			if header.SubType == packet.EXTTYPE_HSREQ {
				c.handleHSRequest(p)
			} else if header.SubType == packet.EXTTYPE_HSRSP {
				c.handleHSResponse(p)
			}

			// 3.2.2.  Key Material
			if header.SubType == packet.EXTTYPE_KMREQ {
				c.handleKMRequest(p)
			} else if header.SubType == packet.EXTTYPE_KMRSP {
				c.handleKMResponse(p)
			}
		}

		return
	}

	if header.PacketSequenceNumber.Gt(c.debug.expectedRcvPacketSequenceNumber) {
		c.log("connection:error", func() string {
			return fmt.Sprintf("recv lost packets. got: %d, expected: %d (%d)\n", header.PacketSequenceNumber.Val(), c.debug.expectedRcvPacketSequenceNumber.Val(), c.debug.expectedRcvPacketSequenceNumber.Distance(header.PacketSequenceNumber))
		})
	}

	c.debug.expectedRcvPacketSequenceNumber = header.PacketSequenceNumber.Inc()

	//fmt.Printf("%s\n", p.String())

	// Ignore FEC filter control packets
	// https://github.com/Haivision/srt/blob/master/docs/features/packet-filtering-and-fec.md
	// "An FEC control packet is distinguished from a regular data packet by having
	// its message number equal to 0. This value isn't normally used in SRT (message
	// numbers start from 1, increment to a maximum, and then roll back to 1)."
	// Process FEC filter
	if c.fecRec != nil {
		recoveredPkts := c.fecRec.AddPacket(p)

		if header.MessageNumber == 0 {
			c.log("connection:filter", func() string { return "processed FEC filter control packet" })
			for _, recPkt := range recoveredPkts {
				c.log("connection:filter", func() string { return fmt.Sprintf("recovered packet %d", recPkt.Header().PacketSequenceNumber.Val()) })
				c.handlePacket(recPkt)
			}
			return
		}

		for _, recPkt := range recoveredPkts {
			c.log("connection:filter", func() string { return fmt.Sprintf("recovered packet %d", recPkt.Header().PacketSequenceNumber.Val()) })
			c.handlePacket(recPkt)
		}
	} else if header.MessageNumber == 0 {
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

	tsbpdTimeBaseOffset := c.getTsbpdTimeBaseOffset(header.Timestamp)

	// The offset between the clock of the peer and the local clock is taken from the first original data packet,
	// such that it is delivered after the TSBPD delay from its arrival. The time of the handshake can't be used for
	// that, because the peer may use a different clock for the timestamps of the handshake and the data packets.
	if !c.havePeerOffset && !header.RetransmittedPacketFlag {
		c.tsbpdPeerOffset = int64(c.getTimestamp()) - int64(tsbpdTimeBaseOffset+uint64(header.Timestamp))
		c.havePeerOffset = true

		c.log("connection:tsbpd", func() string {
			return fmt.Sprintf("peer clock offset=%dus", c.tsbpdPeerOffset)
		})
	}

	header.PktTsbpdTime = uint64(int64(c.tsbpdTimeBase+tsbpdTimeBaseOffset+uint64(header.Timestamp)+c.tsbpdDelay) + c.tsbpdPeerOffset + c.tsbpdDrift)

	c.log("data:recv:dump", func() string { return p.Dump() })

	c.cryptoLock.Lock()
	if c.crypto != nil {
		if header.KeyBaseEncryptionFlag != 0 {
			if err := c.crypto.EncryptOrDecryptPayload(p.Data(), header.KeyBaseEncryptionFlag, header.PacketSequenceNumber.Val()); err != nil {
				c.statisticsLock.Lock()
				c.statistics.pktRecvUndecrypt++
				c.statistics.byteRecvUndecrypt += p.Len()
				c.statisticsLock.Unlock()
			}
		} else {
			c.statisticsLock.Lock()
			c.statistics.pktRecvUndecrypt++
			c.statistics.byteRecvUndecrypt += p.Len()
			c.statisticsLock.Unlock()
		}
	}
	c.cryptoLock.Unlock()

	// Put the packet into receive congestion control
	c.recv.Push(p)
}

// getTsbpdTimeBaseOffset returns the offset to the TSBPD time base for a packet with the given timestamp,
// taking into account the wrapping of the timestamp.
func (c *srtConn) getTsbpdTimeBaseOffset(timestamp uint32) uint64 {
	tsbpdTimeBaseOffset := c.tsbpdTimeBaseOffset
	if c.tsbpdWrapPeriod {
		if timestamp < (30 * 1000000) {
			tsbpdTimeBaseOffset += uint64(packet.MAX_TIMESTAMP) + 1
		}
	}

	return tsbpdTimeBaseOffset
}

// addDriftSample adds a sample to the drift tracer from an ACKACK with the given timestamp and
// the RTT measured with it. If the drift has been updated, the TSBPD time of the following packets
// will be corrected accordingly.
func (c *srtConn) addDriftSample(timestamp uint32, rtt time.Duration) {
	// The drift is relative to the offset of the peer's clock, which is known with the first data packet.
	if !c.havePeerOffset {
		return
	}

	arrival := c.tsbpdTimeBase + c.getTimestamp()
	pktBaseTime := int64(c.tsbpdTimeBase+c.getTsbpdTimeBaseOffset(timestamp)+uint64(timestamp)) + c.tsbpdPeerOffset

	if !c.driftTracer.addSample(int64(arrival), pktBaseTime, rtt.Microseconds()) {
		return
	}

	c.tsbpdDrift = c.driftTracer.correction()

	c.log("connection:tsbpd", func() string {
		return fmt.Sprintf("drift=%dus base shift=%dus correction=%dus", c.driftTracer.drift, c.driftTracer.baseShift, c.tsbpdDrift)
	})
}

// handleKeepAlive resets the idle timeout and sends a keepalive to the peer.
func (c *srtConn) handleKeepAlive(p packet.Packet) {
	c.log("control:recv:keepalive:dump", func() string { return p.Dump() })

	c.statisticsLock.Lock()
	c.statistics.pktRecvKeepalive++
	c.statistics.pktSentKeepalive++
	c.statisticsLock.Unlock()

	c.peerIdleTimeout.Reset(c.config.PeerIdleTimeout)

	c.log("control:send:keepalive:dump", func() string { return p.Dump() })

	c.pop(p)
}

// handleShutdown closes the connection
func (c *srtConn) handleShutdown(p packet.Packet) {
	c.log("control:recv:shutdown:dump", func() string { return p.Dump() })

	c.statisticsLock.Lock()
	c.statistics.pktRecvShutdown++
	c.statisticsLock.Unlock()

	go c.close()
}

// handleACK forwards the acknowledge sequence number to the congestion control and
// returns a ACKACK (on a full ACK). The RTT is also updated in case of a full ACK.
func (c *srtConn) handleACK(p packet.Packet) {
	c.log("control:recv:ACK:dump", func() string { return p.Dump() })

	c.statisticsLock.Lock()
	c.statistics.pktRecvACK++
	c.statisticsLock.Unlock()

	cif := &packet.CIFACK{}

	if err := p.UnmarshalCIF(cif); err != nil {
		c.statisticsLock.Lock()
		c.statistics.pktRecvInvalid++
		c.statisticsLock.Unlock()
		c.log("control:recv:ACK:error", func() string { return fmt.Sprintf("invalid ACK: %s", err) })
		return
	}

	c.log("control:recv:ACK:cif", func() string { return cif.String() })

	c.snd.ACK(cif.LastACKPacketSequenceNumber)

	if !cif.IsLite && !cif.IsSmall {
		// 4.10.  Round-Trip Time Estimation
		c.recalculateRTT(time.Duration(int64(cif.RTT)) * time.Microsecond)

		// Estimated Link Capacity (from packets/s to Mbps)
		c.statisticsLock.Lock()
		c.statistics.mbpsLinkCapacity = float64(cif.EstimatedLinkCapacity) * MAX_PAYLOAD_SIZE * 8 / 1024 / 1024
		c.statisticsLock.Unlock()

		c.sendACKACK(p.Header().TypeSpecific)
	}
}

// handleNAK forwards the lost sequence number to the congestion control.
func (c *srtConn) handleNAK(p packet.Packet) {
	c.log("control:recv:NAK:dump", func() string { return p.Dump() })

	c.statisticsLock.Lock()
	c.statistics.pktRecvNAK++
	c.statisticsLock.Unlock()

	cif := &packet.CIFNAK{}

	if err := p.UnmarshalCIF(cif); err != nil {
		c.statisticsLock.Lock()
		c.statistics.pktRecvInvalid++
		c.statisticsLock.Unlock()
		c.log("control:recv:NAK:error", func() string { return fmt.Sprintf("invalid NAK: %s", err) })
		return
	}

	c.log("control:recv:NAK:cif", func() string { return cif.String() })

	// Inform congestion control about lost packets
	c.snd.NAK(cif.LostPacketSequenceNumber)
}

// handleACKACK updates the RTT and NAK interval for the congestion control.
func (c *srtConn) handleACKACK(p packet.Packet) {
	c.ackLock.Lock()

	c.statisticsLock.Lock()
	c.statistics.pktRecvACKACK++
	c.statisticsLock.Unlock()

	c.log("control:recv:ACKACK:dump", func() string { return p.Dump() })

	// p.typeSpecific is the ACKNumber
	if ack, ok := c.ackNumbers[p.Header().TypeSpecific]; ok {
		// 4.10.  Round-Trip Time Estimation
		rtt := time.Since(ack.timestamp)
		c.recalculateRTT(rtt)

		// The ACKACK is also used to trace the clock drift
		if c.driftTracer != nil {
			c.addDriftSample(p.Header().Timestamp, rtt)
		}

		// Remember what the peer has confirmed, so that sendACK can drop a
		// periodic ACK that would repeat it.
		if !c.haveACKedACK || ack.seq.Gt(c.lastACKedACK) {
			c.lastACKedACK = ack.seq
			c.haveACKedACK = true
		}

		delete(c.ackNumbers, p.Header().TypeSpecific)
	} else {
		c.log("control:recv:ACKACK:error", func() string { return fmt.Sprintf("got unknown ACKACK (%d)", p.Header().TypeSpecific) })
		c.statisticsLock.Lock()
		c.statistics.pktRecvInvalid++
		c.statisticsLock.Unlock()
	}

	for i := range c.ackNumbers {
		if i < p.Header().TypeSpecific {
			delete(c.ackNumbers, i)
		}
	}

	c.ackLock.Unlock()

	c.recv.SetNAKInterval(uint64(c.rtt.NAKInterval()))
}

// recalculateRTT recalculates the RTT based on a full ACK exchange
func (c *srtConn) recalculateRTT(rtt time.Duration) {
	c.rtt.Recalculate(rtt)

	c.log("connection:rtt", func() string {
		return fmt.Sprintf("RTT=%.0fus RTTVar=%.0fus NAKInterval=%.0fms", c.rtt.RTT(), c.rtt.RTTVar(), c.rtt.NAKInterval()/1000)
	})
}

// handleHSRequest handles the HSv4 handshake extension request and sends the response
func (c *srtConn) handleHSRequest(p packet.Packet) {
	c.log("control:recv:HSReq:dump", func() string { return p.Dump() })

	cif := &packet.CIFHandshakeExtension{}

	if err := p.UnmarshalCIF(cif); err != nil {
		c.statisticsLock.Lock()
		c.statistics.pktRecvInvalid++
		c.statisticsLock.Unlock()
		c.log("control:recv:HSReq:error", func() string { return fmt.Sprintf("invalid HSReq: %s", err) })
		return
	}

	c.log("control:recv:HSReq:cif", func() string { return cif.String() })

	if c.version != 4 {
		// Ignore such requests if this is not a v4 connection
		return
	}

	// Check for version
	if cif.SRTVersion < 0x010200 || cif.SRTVersion >= 0x010300 {
		c.log("control:recv:HSReq:error", func() string { return fmt.Sprintf("unsupported version: %#08x", cif.SRTVersion) })
		c.close()
		return
	}

	// Check the required SRT flags
	if !cif.SRTFlags.TSBPDSND {
		c.log("control:recv:HSRes:error", func() string { return "TSBPDSND flag must be set" })
		c.close()

		return
	}

	if !cif.SRTFlags.TLPKTDROP {
		c.log("control:recv:HSRes:error", func() string { return "TLPKTDROP flag must be set" })
		c.close()

		return
	}

	if !cif.SRTFlags.CRYPT {
		c.log("control:recv:HSRes:error", func() string { return "CRYPT flag must be set" })
		c.close()

		return
	}

	if !cif.SRTFlags.REXMITFLG {
		c.log("control:recv:HSRes:error", func() string { return "REXMITFLG flag must be set" })
		c.close()

		return
	}

	// we as receiver don't need this
	cif.SRTFlags.TSBPDSND = false

	// we as receiver are supporting these
	cif.SRTFlags.TSBPDRCV = true
	cif.SRTFlags.PERIODICNAK = true

	// These flag was introduced in HSv5 and should not be set in HSv4
	if cif.SRTFlags.STREAM {
		c.log("control:recv:HSReq:error", func() string { return "STREAM flag is set" })
		c.close()
		return
	}

	if cif.SRTFlags.PACKET_FILTER && len(c.config.PacketFilter) == 0 {
		c.log("control:recv:HSReq:error", func() string { return "Peer set PACKET_FILTER but local does not support it" })
		c.close()
		return
	} else if !cif.SRTFlags.PACKET_FILTER && len(c.config.PacketFilter) > 0 {
		c.log("control:recv:HSReq:error", func() string { return "Local requires PACKET_FILTER but peer did not set it" })
		c.close()
		return
	}

	recvTsbpdDelay := max(cif.SendTSBPDDelay, uint16(c.config.ReceiverLatency.Milliseconds()))

	c.statisticsLock.Lock()
	c.tsbpdDelay = uint64(recvTsbpdDelay) * 1000
	c.statisticsLock.Unlock()

	cif.RecvTSBPDDelay = 0
	cif.SendTSBPDDelay = recvTsbpdDelay

	p.MarshalCIF(cif)

	// Send HS Response
	p.Header().SubType = packet.EXTTYPE_HSRSP

	c.pop(p)
}

// handleHSResponse handles the HSv4 handshake extension response
func (c *srtConn) handleHSResponse(p packet.Packet) {
	c.log("control:recv:HSRes:dump", func() string { return p.Dump() })

	cif := &packet.CIFHandshakeExtension{}

	if err := p.UnmarshalCIF(cif); err != nil {
		c.statisticsLock.Lock()
		c.statistics.pktRecvInvalid++
		c.statisticsLock.Unlock()
		c.log("control:recv:HSRes:error", func() string { return fmt.Sprintf("invalid HSRes: %s", err) })
		return
	}

	c.log("control:recv:HSRes:cif", func() string { return cif.String() })

	if c.version != 4 {
		// Ignore such responses if this is not a v4 connection
		return
	}

	// Check for version
	if cif.SRTVersion < 0x010200 || cif.SRTVersion >= 0x010300 {
		c.log("control:recv:HSRes:error", func() string { return fmt.Sprintf("unsupported version: %#08x", cif.SRTVersion) })
		c.close()
		return
	}

	// TSBPDSND is not relevant from the receiver
	// PERIODICNAK is the sender's decision, we don't care, but will handle them

	// Check the required SRT flags
	if !cif.SRTFlags.TSBPDRCV {
		c.log("control:recv:HSRes:error", func() string { return "TSBPDRCV flag must be set" })
		c.close()

		return
	}

	if !cif.SRTFlags.TLPKTDROP {
		c.log("control:recv:HSRes:error", func() string { return "TLPKTDROP flag must be set" })
		c.close()

		return
	}

	if !cif.SRTFlags.CRYPT {
		c.log("control:recv:HSRes:error", func() string { return "CRYPT flag must be set" })
		c.close()

		return
	}

	if !cif.SRTFlags.REXMITFLG {
		c.log("control:recv:HSRes:error", func() string { return "REXMITFLG flag must be set" })
		c.close()

		return
	}

	// These flag was introduced in HSv5 and should not be set in HSv4
	if cif.SRTFlags.STREAM {
		c.log("control:recv:HSReq:error", func() string { return "STREAM flag is set" })
		c.close()
		return
	}

	if cif.SRTFlags.PACKET_FILTER && len(c.config.PacketFilter) == 0 {
		c.log("control:recv:HSRes:error", func() string { return "Peer set PACKET_FILTER but local does not support it" })
		c.close()
		return
	} else if !cif.SRTFlags.PACKET_FILTER && len(c.config.PacketFilter) > 0 {
		c.log("control:recv:HSRes:error", func() string { return "Local requires PACKET_FILTER but peer did not set it" })
		c.close()
		return
	}

	sendTsbpdDelay := max(cif.SendTSBPDDelay, uint16(c.config.PeerLatency.Milliseconds()))

	c.dropThreshold = max(uint64(float64(sendTsbpdDelay)*1.25)+uint64(c.config.SendDropDelay.Microseconds()), uint64(time.Second.Microseconds()))
	c.dropThreshold += 20_000

	c.snd.SetDropThreshold(c.dropThreshold)

	c.stopHSRequests()
}

// handleKMRequest checks if the key material is valid and responds with a KM response.
func (c *srtConn) handleKMRequest(p packet.Packet) {
	c.log("control:recv:KMReq:dump", func() string { return p.Dump() })

	c.statisticsLock.Lock()
	c.statistics.pktRecvKM++
	c.statisticsLock.Unlock()

	cif := &packet.CIFKeyMaterialExtension{}

	if err := p.UnmarshalCIF(cif); err != nil {
		c.statisticsLock.Lock()
		c.statistics.pktRecvInvalid++
		c.statisticsLock.Unlock()
		c.log("control:recv:KMReq:error", func() string { return fmt.Sprintf("invalid KMReq: %s", err) })
		return
	}

	c.log("control:recv:KMReq:cif", func() string { return cif.String() })

	c.cryptoLock.Lock()

	if c.version == 4 && c.crypto == nil {
		cr, err := crypto.New(int(cif.KLen))
		if err != nil {
			c.log("control:recv:KMReq:error", func() string { return fmt.Sprintf("crypto: %s", err) })
			c.cryptoLock.Unlock()
			c.close()
			return
		}

		c.keyBaseEncryption = cif.KeyBasedEncryption.Opposite()
		c.crypto = cr
	}

	if c.crypto == nil {
		c.log("control:recv:KMReq:error", func() string { return "connection is not encrypted" })
		c.cryptoLock.Unlock()
		return
	}

	if cif.KeyBasedEncryption == c.keyBaseEncryption {
		c.statisticsLock.Lock()
		c.statistics.pktRecvInvalid++
		c.statisticsLock.Unlock()
		c.log("control:recv:KMReq:error", func() string {
			return "invalid KM request. wants to reset the key that is already in use"
		})
		c.cryptoLock.Unlock()
		return
	}

	if err := c.crypto.UnmarshalKM(cif, c.config.Passphrase); err != nil {
		c.statisticsLock.Lock()
		c.statistics.pktRecvInvalid++
		c.statisticsLock.Unlock()
		c.log("control:recv:KMReq:error", func() string { return fmt.Sprintf("invalid KMReq: %s", err) })
		c.cryptoLock.Unlock()
		return
	}

	// Switch the keys
	c.keyBaseEncryption = c.keyBaseEncryption.Opposite()

	c.cryptoLock.Unlock()

	// Send KM Response
	p.Header().SubType = packet.EXTTYPE_KMRSP

	c.statisticsLock.Lock()
	c.statistics.pktSentKM++
	c.statisticsLock.Unlock()

	c.pop(p)
}

// handleKMResponse confirms the change of encryption keys.
func (c *srtConn) handleKMResponse(p packet.Packet) {
	c.log("control:recv:KMRes:dump", func() string { return p.Dump() })

	c.statisticsLock.Lock()
	c.statistics.pktRecvKM++
	c.statisticsLock.Unlock()

	cif := &packet.CIFKeyMaterialExtension{}

	if err := p.UnmarshalCIF(cif); err != nil {
		c.statisticsLock.Lock()
		c.statistics.pktRecvInvalid++
		c.statisticsLock.Unlock()
		c.log("control:recv:KMRes:error", func() string { return fmt.Sprintf("invalid KMRes: %s", err) })
		return
	}

	c.cryptoLock.Lock()
	defer c.cryptoLock.Unlock()

	if c.crypto == nil {
		c.log("control:recv:KMRes:error", func() string { return "connection is not encrypted" })
		return
	}

	if c.version == 4 {
		c.stopKMRequests()

		if cif.Error != 0 {
			if cif.Error == packet.KM_NOSECRET {
				c.log("control:recv:KMRes:error", func() string { return "peer didn't enabled encryption" })
			} else if cif.Error == packet.KM_BADSECRET {
				c.log("control:recv:KMRes:error", func() string { return "peer has a different passphrase" })
			}
			c.close()
			return
		}
	}

	c.log("control:recv:KMRes:cif", func() string { return cif.String() })

	if c.kmPreAnnounceCountdown >= c.config.KMPreAnnounce {
		c.log("control:recv:KMRes:error", func() string { return "not in pre-announce period, ignored" })
		// Ignore the response, we're not in the pre-announce period
		return
	}

	c.kmConfirmed = true
}

// sendShutdown sends a shutdown packet to the peer.
func (c *srtConn) sendShutdown() {
	p := packet.NewPacket(c.remoteAddr)

	p.Header().IsControlPacket = true

	p.Header().ControlType = packet.CTRLTYPE_SHUTDOWN
	p.Header().Timestamp = c.getTimestampForPacket()

	cif := packet.CIFShutdown{}

	p.MarshalCIF(&cif)

	c.log("control:send:shutdown:dump", func() string { return p.Dump() })
	c.log("control:send:shutdown:cif", func() string { return cif.String() })

	c.statisticsLock.Lock()
	c.statistics.pktSentShutdown++
	c.statisticsLock.Unlock()

	c.pop(p)
}

// sendNAK sends a NAK to the peer with the given range of sequence numbers.
func (c *srtConn) sendNAK(list []circular.Number) {
	p := packet.NewPacket(c.remoteAddr)

	p.Header().IsControlPacket = true

	p.Header().ControlType = packet.CTRLTYPE_NAK
	p.Header().Timestamp = c.getTimestampForPacket()

	cif := packet.CIFNAK{}

	cif.LostPacketSequenceNumber = append(cif.LostPacketSequenceNumber, list...)

	p.MarshalCIF(&cif)

	c.log("control:send:NAK:dump", func() string { return p.Dump() })
	c.log("control:send:NAK:cif", func() string { return cif.String() })

	c.statisticsLock.Lock()
	c.statistics.pktSentNAK++
	c.statisticsLock.Unlock()

	c.pop(p)
}

// sendACK sends an ACK to the peer with the given sequence number. A full ACK
// that repeats an unchanged sequence is deferred according to dropRepeatedACK.
func (c *srtConn) sendACK(seq circular.Number, lite bool) {
	c.ackLock.Lock()
	defer c.ackLock.Unlock()

	if !lite && c.dropRepeatedACK(seq) {
		return
	}

	p := packet.NewPacket(c.remoteAddr)

	p.Header().IsControlPacket = true

	p.Header().ControlType = packet.CTRLTYPE_ACK
	p.Header().Timestamp = c.getTimestampForPacket()

	cif := packet.CIFACK{
		LastACKPacketSequenceNumber: seq,
	}

	if lite {
		cif.IsLite = true

		p.Header().TypeSpecific = 0
	} else {
		pps, bps, capacity := c.recv.PacketRate()

		cif.RTT = uint32(c.rtt.RTT())
		cif.RTTVar = uint32(c.rtt.RTTVar())
		cif.AvailableBufferSize = c.config.FC        // TODO: available buffer size (packets)
		cif.PacketsReceivingRate = uint32(pps)       // packets receiving rate (packets/s)
		cif.EstimatedLinkCapacity = uint32(capacity) // estimated link capacity (packets/s), not relevant for live mode
		cif.ReceivingRate = uint32(bps)              // receiving rate (bytes/s), not relevant for live mode

		p.Header().TypeSpecific = c.nextACKNumber.Val()

		c.ackNumbers[p.Header().TypeSpecific] = ackRecord{
			timestamp: time.Now(),
			seq:       seq,
		}
		c.nextACKNumber = c.nextACKNumber.Inc()
		if c.nextACKNumber.Val() == 0 {
			c.nextACKNumber = c.nextACKNumber.Inc()
		}

		c.lastSentACK = seq
		c.lastSentACKTime = time.Now()
		c.haveSentACK = true
	}

	p.MarshalCIF(&cif)

	c.log("control:send:ACK:dump", func() string { return p.Dump() })
	c.log("control:send:ACK:cif", func() string { return cif.String() })

	c.statisticsLock.Lock()
	c.statistics.pktSentACK++
	c.statisticsLock.Unlock()

	c.pop(p)
}

// dropRepeatedACK reports whether a full ACK for seq can be deferred without
// removing the bounded control traffic that keeps an idle peer reachable.
//
// The periodic ACK is paced by a 10 ms timer rather than by traffic, so a
// connection carrying a couple of packets per second still produces ~100 ACKs
// per second. Each is 72 bytes on the wire once IPv4 and UDP are counted, and
// each obliges the sender to answer with a 44 byte ACKACK. On a low bitrate
// live link (a 1 fps camera on a cellular or satellite uplink, for example) that
// exchange costs several times what the media does, in both directions.
//
// libsrt suppresses the repeats: CUDT::sendCtrlAck() returns early when the
// sequence number has not advanced since the last ACKACK, and lets the
// remaining repeats through no more than once per RTT + 4 RTTVar. This is the
// same rule for unconfirmed ACKs. A confirmed repeat is sent at most once per
// second: gosrt does not initiate idle KEEPALIVE packets, so suppressing every
// confirmed repeat lets two idle gosrt peers reach their peer-idle deadlines.
// Anything that acknowledges new data is always sent.
//
// The caller must hold ackLock.
func (c *srtConn) dropRepeatedACK(seq circular.Number) bool {
	if !c.haveSentACK || !seq.Equals(c.lastSentACK) {
		return false // there is something new to acknowledge
	}

	if c.haveACKedACK && seq.Equals(c.lastACKedACK) {
		return time.Since(c.lastSentACKTime) < time.Second
	}

	// Unconfirmed: either the ACK or its ACKACK may have been lost, so keep
	// repeating at the RTT instead of the 10 ms tick.
	repeatAfter := time.Duration(c.rtt.RTT()+4*c.rtt.RTTVar()) * time.Microsecond

	return time.Since(c.lastSentACKTime) < repeatAfter
}

// sendACKACK sends an ACKACK to the peer with the given ACK sequence.
func (c *srtConn) sendACKACK(ackSequence uint32) {
	p := packet.NewPacket(c.remoteAddr)

	p.Header().IsControlPacket = true

	p.Header().ControlType = packet.CTRLTYPE_ACKACK
	p.Header().Timestamp = c.getTimestampForPacket()

	p.Header().TypeSpecific = ackSequence

	// Four bytes of zero padding, to match libsrt.
	//
	// An ACKACK carries no control information of its own — the ACK
	// sequence number travels in the header's Type-specific Information
	// field above — so by the RFC it needs no payload. libsrt, however,
	// rejects a control packet with a zero-length payload and logs an
	// error for every ACKACK it receives:
	//
	//	EPE: incoming UMSG: 6 INVALID SIZE: 0
	//	     (expected > 0 and aligned to 4 bytes)
	//
	// At one ACK every 10 ms that is several error lines a second in the
	// log of every libsrt peer, which buries real transport errors.
	//
	// libsrt pads its own ACKACK, KEEPALIVE and SHUTDOWN with a 4-byte
	// m_extra_pad for the same reason (srtcore/packet.cpp: "control info
	// field should be none but writev does not allow this"), and this
	// package already does exactly this for SHUTDOWN, where
	// CIFShutdown.Marshal writes four zero bytes. This brings ACKACK in
	// line with both.
	p.SetData(make([]byte, 4))

	c.log("control:send:ACKACK:dump", func() string { return p.Dump() })

	c.statisticsLock.Lock()
	c.statistics.pktSentACKACK++
	c.statisticsLock.Unlock()

	c.pop(p)
}

func (c *srtConn) sendHSRequests(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	select {
	case <-ctx.Done():
		return
	case <-ticker.C:
		c.sendHSRequest()
	}
}

func (c *srtConn) sendHSRequest() {
	cif := &packet.CIFHandshakeExtension{
		SRTVersion: 0x00010203,
		SRTFlags: packet.CIFHandshakeExtensionFlags{
			TSBPDSND:      true,  // we send in TSBPD mode
			TSBPDRCV:      false, // not relevant for us as sender
			CRYPT:         true,  // must be always set
			TLPKTDROP:     true,  // must be set in live mode
			PERIODICNAK:   false, // not relevant for us as sender
			REXMITFLG:     true,  // must alwasy be set
			STREAM:        false,
			PACKET_FILTER: len(c.config.PacketFilter) > 0,
		},
		RecvTSBPDDelay: 0,
		SendTSBPDDelay: uint16(c.config.ReceiverLatency.Milliseconds()),
	}

	p := packet.NewPacket(c.remoteAddr)

	p.Header().IsControlPacket = true

	p.Header().ControlType = packet.CTRLTYPE_USER
	p.Header().SubType = packet.EXTTYPE_HSREQ
	p.Header().Timestamp = c.getTimestampForPacket()

	p.MarshalCIF(cif)

	c.log("control:send:HSReq:dump", func() string { return p.Dump() })
	c.log("control:send:HSReq:cif", func() string { return cif.String() })

	c.pop(p)
}

func (c *srtConn) sendKMRequests(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	select {
	case <-ctx.Done():
		return
	case <-ticker.C:
		c.sendKMRequest(c.keyBaseEncryption)
	}
}

// sendKMRequest sends a KM request to the peer.
func (c *srtConn) sendKMRequest(key packet.PacketEncryption) {
	if c.crypto == nil {
		c.log("control:send:KMReq:error", func() string { return "connection is not encrypted" })
		return
	}

	cif := &packet.CIFKeyMaterialExtension{}

	c.crypto.MarshalKM(cif, c.config.Passphrase, key)

	p := packet.NewPacket(c.remoteAddr)

	p.Header().IsControlPacket = true

	p.Header().ControlType = packet.CTRLTYPE_USER
	p.Header().SubType = packet.EXTTYPE_KMREQ
	p.Header().Timestamp = c.getTimestampForPacket()

	p.MarshalCIF(cif)

	c.log("control:send:KMReq:dump", func() string { return p.Dump() })
	c.log("control:send:KMReq:cif", func() string { return cif.String() })

	c.statisticsLock.Lock()
	c.statistics.pktSentKM++
	c.statisticsLock.Unlock()

	c.pop(p)
}

// Close closes the connection.
func (c *srtConn) Close() error {
	c.close()

	return nil
}

// close closes the connection.
func (c *srtConn) close() {

	c.shutdownOnce.Do(func() {
		c.log("connection:close", func() string { return "stopping peer idle timeout" })

		c.peerIdleTimeout.Stop()

		c.log("connection:close", func() string { return "sending shutdown message to peer" })

		c.sendShutdown()

		c.log("connection:close", func() string { return "stopping all routines and channels" })

		c.cancelCtx()

		c.log("connection:close", func() string { return "flushing congestion" })

		c.snd.Flush()
		c.recv.Flush()

		c.log("connection:close", func() string { return "shutdown" })

		go func() {
			c.onShutdown(c)
		}()
	})
}

func (c *srtConn) log(topic string, message func() string) {
	c.logger.Print(topic, c.socketId, 2, message)
}

func (c *srtConn) SetDeadline(t time.Time) error      { return nil }
func (c *srtConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *srtConn) SetWriteDeadline(t time.Time) error { return nil }
