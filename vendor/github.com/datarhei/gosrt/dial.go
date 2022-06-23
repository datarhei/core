package srt

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/datarhei/gosrt/internal/circular"
	"github.com/datarhei/gosrt/internal/crypto"
	"github.com/datarhei/gosrt/internal/packet"
)

// ErrClientClosed is returned when the client connection has
// been voluntarily closed.
var ErrClientClosed = errors.New("srt: client closed")

// dialer implements the Conn interface
type dialer struct {
	pc *net.UDPConn

	localAddr  net.Addr
	remoteAddr net.Addr

	config Config

	socketId                    uint32
	initialPacketSequenceNumber circular.Number

	crypto crypto.Crypto

	conn     *srtConn
	connLock sync.RWMutex
	connChan chan connResponse

	start time.Time

	rcvQueue chan packet.Packet // for packets that come from the wire
	sndQueue chan packet.Packet // for packets that go to the wire

	shutdown     bool
	shutdownLock sync.RWMutex
	shutdownOnce sync.Once

	stopReader context.CancelFunc
	stopWriter context.CancelFunc

	doneChan chan error
}

type connResponse struct {
	conn *srtConn
	err  error
}

// Dial connects to the address using the SRT protocol with the given config
// and returns a Conn interface.
//
// The address is of the form "host:port".
//
// Example:
//  Dial("srt", "127.0.0.1:3000", DefaultConfig())
//
// In case of an error the returned Conn is nil and the error is non-nil.
func Dial(network, address string, config Config) (Conn, error) {
	if network != "srt" {
		return nil, fmt.Errorf("the network must be 'srt'")
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	if config.Logger == nil {
		config.Logger = NewLogger(nil)
	}

	dl := &dialer{
		config: config,
	}

	raddr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, fmt.Errorf("unable to resolve address: %w", err)
	}

	pc, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return nil, fmt.Errorf("failed dialing: %w", err)
	}

	file, err := pc.File()
	if err != nil {
		return nil, err
	}

	// Set TOS
	if config.IPTOS > 0 {
		err = syscall.SetsockoptInt(int(file.Fd()), syscall.IPPROTO_IP, syscall.IP_TOS, config.IPTOS)
		if err != nil {
			return nil, fmt.Errorf("failed setting socket option TOS: %w", err)
		}
	}

	// Set TTL
	if config.IPTTL > 0 {
		err = syscall.SetsockoptInt(int(file.Fd()), syscall.IPPROTO_IP, syscall.IP_TTL, config.IPTTL)
		if err != nil {
			return nil, fmt.Errorf("failed setting socket option TTL: %w", err)
		}
	}

	dl.pc = pc

	dl.localAddr = pc.LocalAddr()
	dl.remoteAddr = pc.RemoteAddr()

	dl.conn = nil
	dl.connChan = make(chan connResponse)

	dl.rcvQueue = make(chan packet.Packet, 2048)
	dl.sndQueue = make(chan packet.Packet, 2048)

	dl.doneChan = make(chan error)

	dl.start = time.Now()

	// create a new socket ID
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	dl.socketId = r.Uint32()
	dl.initialPacketSequenceNumber = circular.New(r.Uint32()&packet.MAX_SEQUENCENUMBER, packet.MAX_SEQUENCENUMBER)

	go func() {
		buffer := make([]byte, MAX_MSS_SIZE) // MTU size

		for {
			if dl.isShutdown() {
				dl.doneChan <- ErrClientClosed
				return
			}

			pc.SetReadDeadline(time.Now().Add(3 * time.Second))
			n, _, err := pc.ReadFrom(buffer)
			if err != nil {
				if errors.Is(err, os.ErrDeadlineExceeded) {
					continue
				}

				if dl.isShutdown() {
					dl.doneChan <- ErrClientClosed
					return
				}

				dl.doneChan <- err
				return
			}

			p := packet.NewPacket(dl.remoteAddr, buffer[:n])
			if p == nil {
				continue
			}

			dl.rcvQueue <- p
		}
	}()

	var readerCtx context.Context
	readerCtx, dl.stopReader = context.WithCancel(context.Background())
	go dl.reader(readerCtx)

	var writerCtx context.Context
	writerCtx, dl.stopWriter = context.WithCancel(context.Background())
	go dl.writer(writerCtx)

	// Send the initial handshake request
	dl.sendInduction()

	dl.log("dial", func() string { return "waiting for response" })

	timer := time.AfterFunc(dl.config.ConnectionTimeout, func() {
		dl.connChan <- connResponse{
			conn: nil,
			err:  fmt.Errorf("connection timeout. server didn't respond"),
		}
	})

	// Wait for handshake to conclude
	response := <-dl.connChan
	if response.err != nil {
		dl.Close()
		return nil, response.err
	}

	timer.Stop()

	dl.connLock.Lock()
	dl.conn = response.conn
	dl.connLock.Unlock()

	return dl, nil
}

func (dl *dialer) checkConnection() error {
	select {
	case err := <-dl.doneChan:
		dl.Close()
		return err
	default:
	}

	return nil
}

// reader reads packets from the receive queue and pushes them into the connection
func (dl *dialer) reader(ctx context.Context) {
	defer func() {
		dl.log("dial", func() string { return "left reader loop" })
	}()

	dl.log("dial", func() string { return "reader loop started" })

	for {
		select {
		case <-ctx.Done():
			return
		case p := <-dl.rcvQueue:
			if dl.isShutdown() {
				break
			}

			dl.log("packet:recv:dump", func() string { return p.Dump() })

			if p.Header().DestinationSocketId != dl.socketId {
				break
			}

			if p.Header().IsControlPacket && p.Header().ControlType == packet.CTRLTYPE_HANDSHAKE {
				dl.handleHandshake(p)
				break
			}

			dl.connLock.RLock()
			if dl.conn == nil {
				dl.connLock.RUnlock()
				break
			}

			dl.conn.push(p)
			dl.connLock.RUnlock()
		}
	}
}

// send adds a packet to the send queue
func (dl *dialer) send(p packet.Packet) {
	// non-blocking
	select {
	case dl.sndQueue <- p:
	default:
		dl.log("dial", func() string { return "send queue is full" })
	}
}

// writer reads packets from the send queue and writes them to the wire
func (dl *dialer) writer(ctx context.Context) {
	defer func() {
		dl.log("dial", func() string { return "left writer loop" })
	}()

	dl.log("dial", func() string { return "writer loop started" })

	var data bytes.Buffer

	for {
		select {
		case <-ctx.Done():
			return
		case p := <-dl.sndQueue:
			data.Reset()

			p.Marshal(&data)

			buffer := data.Bytes()

			dl.log("packet:send:dump", func() string { return p.Dump() })

			// Write the packet's contents to the wire.
			dl.pc.Write(buffer)

			if p.Header().IsControlPacket {
				// Control packets can be decommissioned because they will not be sent again
				p.Decommission()
			}
		}
	}
}

func (dl *dialer) handleHandshake(p packet.Packet) {
	cif := &packet.CIFHandshake{}

	err := p.UnmarshalCIF(cif)

	dl.log("handshake:recv:dump", func() string { return p.Dump() })
	dl.log("handshake:recv:cif", func() string { return cif.String() })

	if err != nil {
		dl.log("handshake:recv:error", func() string { return err.Error() })
		return
	}

	// assemble the response (4.3.1.  Caller-Listener Handshake)

	p.Header().ControlType = packet.CTRLTYPE_HANDSHAKE
	p.Header().SubType = 0
	p.Header().TypeSpecific = 0
	p.Header().Timestamp = uint32(time.Since(dl.start).Microseconds())
	p.Header().DestinationSocketId = cif.SRTSocketId

	if cif.HandshakeType == packet.HSTYPE_INDUCTION {
		// Verify version
		if cif.Version != 5 {
			dl.connChan <- connResponse{
				conn: nil,
				err:  fmt.Errorf("peer doesn't support handshake v5"),
			}

			return
		}

		// Verify magic number
		if cif.ExtensionField != 0x4A17 {
			dl.connChan <- connResponse{
				conn: nil,
				err:  fmt.Errorf("peer sent the wrong magic number"),
			}

			return
		}

		// Setup crypto context
		if len(dl.config.Passphrase) != 0 {
			keylen := dl.config.PBKeylen

			// If the server advertises a specific block cipher family and key size,
			// use this one, otherwise, use the configured one
			if cif.EncryptionField != 0 {
				switch cif.EncryptionField {
				case 2:
					keylen = 16
				case 3:
					keylen = 24
				case 4:
					keylen = 32
				}
			}

			cr, err := crypto.New(keylen)
			if err != nil {
				dl.connChan <- connResponse{
					conn: nil,
					err:  fmt.Errorf("failed creating crypto context: %w", err),
				}
			}

			dl.crypto = cr
		}

		cif.IsRequest = true
		cif.HandshakeType = packet.HSTYPE_CONCLUSION
		cif.InitialPacketSequenceNumber = dl.initialPacketSequenceNumber
		cif.MaxTransmissionUnitSize = dl.config.MSS // MTU size
		cif.MaxFlowWindowSize = dl.config.FC
		cif.SRTSocketId = dl.socketId
		cif.PeerIP.FromNetAddr(dl.localAddr)

		cif.HasHS = true
		cif.SRTVersion = SRT_VERSION
		cif.SRTFlags.TSBPDSND = true
		cif.SRTFlags.TSBPDRCV = true
		cif.SRTFlags.CRYPT = true // must always set to true
		cif.SRTFlags.TLPKTDROP = true
		cif.SRTFlags.PERIODICNAK = true
		cif.SRTFlags.REXMITFLG = true
		cif.SRTFlags.STREAM = false
		cif.SRTFlags.PACKET_FILTER = false
		cif.RecvTSBPDDelay = uint16(dl.config.ReceiverLatency.Milliseconds())
		cif.SendTSBPDDelay = uint16(dl.config.PeerLatency.Milliseconds())

		cif.HasSID = true
		cif.StreamId = dl.config.StreamId

		if dl.crypto != nil {
			cif.HasKM = true
			cif.SRTKM = &packet.CIFKM{}

			if err := dl.crypto.MarshalKM(cif.SRTKM, dl.config.Passphrase, packet.EvenKeyEncrypted); err != nil {
				dl.connChan <- connResponse{
					conn: nil,
					err:  err,
				}

				return
			}
		}

		p.MarshalCIF(cif)

		dl.log("handshake:send:dump", func() string { return p.Dump() })
		dl.log("handshake:send:cif", func() string { return cif.String() })

		dl.send(p)
	} else if cif.HandshakeType == packet.HSTYPE_CONCLUSION {
		// We only support HSv5
		if cif.Version != 5 {
			dl.sendShutdown(cif.SRTSocketId)

			dl.connChan <- connResponse{
				conn: nil,
				err:  fmt.Errorf("peer doesn't support handshake v5"),
			}

			return
		}

		// Check if the peer version is sufficient
		if cif.SRTVersion < dl.config.MinVersion {
			dl.sendShutdown(cif.SRTSocketId)

			dl.connChan <- connResponse{
				conn: nil,
				err:  fmt.Errorf("peer SRT version is not sufficient"),
			}

			return
		}

		// Check the required SRT flags
		if !cif.SRTFlags.TSBPDSND || !cif.SRTFlags.TSBPDRCV || !cif.SRTFlags.TLPKTDROP || !cif.SRTFlags.PERIODICNAK || !cif.SRTFlags.REXMITFLG {
			dl.sendShutdown(cif.SRTSocketId)

			dl.connChan <- connResponse{
				conn: nil,
				err:  fmt.Errorf("peer doesn't agree on SRT flags"),
			}

			return
		}

		// We only support live streaming
		if cif.SRTFlags.STREAM {
			dl.sendShutdown(cif.SRTSocketId)

			dl.connChan <- connResponse{
				conn: nil,
				err:  fmt.Errorf("peer doesn't support live streaming"),
			}

			return
		}

		// Use the largest TSBPD delay as advertised by the listener, but
		// at least 120ms
		tsbpdDelay := uint16(120)
		if cif.RecvTSBPDDelay > tsbpdDelay {
			tsbpdDelay = cif.RecvTSBPDDelay
		}

		if cif.SendTSBPDDelay > tsbpdDelay {
			tsbpdDelay = cif.SendTSBPDDelay
		}

		// If the peer has a smaller MTU size, adjust to it
		if cif.MaxTransmissionUnitSize < dl.config.MSS {
			dl.config.MSS = cif.MaxTransmissionUnitSize
			dl.config.PayloadSize = dl.config.MSS - SRT_HEADER_SIZE - UDP_HEADER_SIZE

			if dl.config.PayloadSize < MIN_PAYLOAD_SIZE {
				dl.sendShutdown(cif.SRTSocketId)

				dl.connChan <- connResponse{
					conn: nil,
					err:  fmt.Errorf("effective MSS too small (%d bytes) to fit the minimal payload size (%d bytes)", dl.config.MSS, MIN_PAYLOAD_SIZE),
				}

				return
			}
		}

		// Create a new connection
		conn := newSRTConn(srtConnConfig{
			localAddr:                   dl.localAddr,
			remoteAddr:                  dl.remoteAddr,
			config:                      dl.config,
			start:                       dl.start,
			socketId:                    dl.socketId,
			peerSocketId:                cif.SRTSocketId,
			tsbpdTimeBase:               uint64(time.Since(dl.start).Microseconds()),
			tsbpdDelay:                  uint64(tsbpdDelay) * 1000,
			initialPacketSequenceNumber: cif.InitialPacketSequenceNumber,
			crypto:                      dl.crypto,
			keyBaseEncryption:           packet.EvenKeyEncrypted,
			onSend:                      dl.send,
			onShutdown:                  func(socketId uint32) { dl.Close() },
			logger:                      dl.config.Logger,
		})

		dl.log("connection:new", func() string { return fmt.Sprintf("%#08x (%s)", conn.SocketId(), conn.StreamId()) })

		dl.connChan <- connResponse{
			conn: conn,
			err:  nil,
		}
	} else {
		var err error

		if cif.HandshakeType.IsRejection() {
			err = fmt.Errorf("connection rejected: %s", cif.HandshakeType.String())
		} else {
			err = fmt.Errorf("unsupported handshake: %s", cif.HandshakeType.String())
		}

		dl.connChan <- connResponse{
			conn: nil,
			err:  err,
		}
	}
}

func (dl *dialer) sendInduction() {
	p := packet.NewPacket(dl.remoteAddr, nil)

	p.Header().IsControlPacket = true

	p.Header().ControlType = packet.CTRLTYPE_HANDSHAKE
	p.Header().SubType = 0
	p.Header().TypeSpecific = 0

	p.Header().Timestamp = uint32(time.Since(dl.start).Microseconds())
	p.Header().DestinationSocketId = 0

	cif := &packet.CIFHandshake{
		IsRequest:                   true,
		Version:                     4,
		EncryptionField:             0,
		ExtensionField:              2,
		InitialPacketSequenceNumber: circular.New(0, packet.MAX_SEQUENCENUMBER),
		MaxTransmissionUnitSize:     dl.config.MSS, // MTU size
		MaxFlowWindowSize:           dl.config.FC,
		HandshakeType:               packet.HSTYPE_INDUCTION,
		SRTSocketId:                 dl.socketId,
		SynCookie:                   0,
	}

	cif.PeerIP.FromNetAddr(dl.localAddr)

	p.MarshalCIF(cif)

	dl.log("handshake:send:dump", func() string { return p.Dump() })
	dl.log("handshake:send:cif", func() string { return cif.String() })

	dl.send(p)
}

func (dl *dialer) sendShutdown(peerSocketId uint32) {
	p := packet.NewPacket(dl.remoteAddr, nil)

	data := [4]byte{}
	binary.BigEndian.PutUint32(data[0:], 0)

	p.SetData(data[0:4])

	p.Header().IsControlPacket = true

	p.Header().ControlType = packet.CTRLTYPE_SHUTDOWN
	p.Header().TypeSpecific = 0

	p.Header().Timestamp = uint32(time.Since(dl.start).Microseconds())
	p.Header().DestinationSocketId = peerSocketId

	dl.log("control:send:shutdown:dump", func() string { return p.Dump() })

	dl.send(p)
}

func (dl *dialer) LocalAddr() net.Addr {
	return dl.conn.LocalAddr()
}

func (dl *dialer) RemoteAddr() net.Addr {
	return dl.conn.RemoteAddr()
}

func (dl *dialer) SocketId() uint32 {
	return dl.conn.SocketId()
}

func (dl *dialer) PeerSocketId() uint32 {
	return dl.conn.PeerSocketId()
}

func (dl *dialer) StreamId() string {
	return dl.conn.StreamId()
}

func (dl *dialer) isShutdown() bool {
	dl.shutdownLock.RLock()
	defer dl.shutdownLock.RUnlock()

	return dl.shutdown
}

func (dl *dialer) Close() error {
	dl.shutdownOnce.Do(func() {
		dl.shutdownLock.Lock()
		dl.shutdown = true
		dl.shutdownLock.Unlock()

		dl.connLock.RLock()
		if dl.conn != nil {
			dl.conn.Close()
		}
		dl.connLock.RUnlock()

		dl.stopReader()
		dl.stopWriter()

		dl.log("dial", func() string { return "closing socket" })
		dl.pc.Close()

		select {
		case <-dl.doneChan:
		default:
		}
	})

	return nil
}

func (dl *dialer) Read(p []byte) (n int, err error) {
	if err := dl.checkConnection(); err != nil {
		return 0, err
	}

	dl.connLock.RLock()
	defer dl.connLock.RUnlock()

	return dl.conn.Read(p)
}

func (dl *dialer) Write(p []byte) (n int, err error) {
	if err := dl.checkConnection(); err != nil {
		return 0, err
	}

	dl.connLock.RLock()
	defer dl.connLock.RUnlock()

	return dl.conn.Write(p)
}

func (dl *dialer) SetDeadline(t time.Time) error      { return dl.conn.SetDeadline(t) }
func (dl *dialer) SetReadDeadline(t time.Time) error  { return dl.conn.SetReadDeadline(t) }
func (dl *dialer) SetWriteDeadline(t time.Time) error { return dl.conn.SetWriteDeadline(t) }
func (dl *dialer) Stats() Statistics                  { return dl.conn.Stats() }

func (dl *dialer) log(topic string, message func() string) {
	dl.config.Logger.Print(topic, dl.socketId, 2, message)
}
