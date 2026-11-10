package rtmp

import (
	"bufio"
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/datarhei/joy4/av"
	"github.com/datarhei/joy4/av/avutil"
	"github.com/datarhei/joy4/format/flv"
	"github.com/datarhei/joy4/format/flv/flvio"
	"github.com/datarhei/joy4/utils/bits/pio"
)

var Debug bool

// ParseURL parses an rtmp:// or rtmps:// URL. If the URL has no port, the default port is
// added (1935 for rtmp, 443 for rtmps).
func ParseURL(uri string) (u *url.URL, err error) {
	if u, err = url.Parse(uri); err != nil {
		return
	}
	if _, _, serr := net.SplitHostPort(u.Host); serr != nil {
		if u.Scheme == "rtmps" {
			u.Host += ":443"
		} else {
			u.Host += ":1935"
		}
	}
	return
}

// DefaultMaxChunkStreams is the default max. number of chunk streams a peer can use.
const DefaultMaxChunkStreams = 16

// DefaultSetupTimeout is the default time for the handshake and the connect, publish or play
// commands until a connection must be ready for publishing or playing.
const DefaultSetupTimeout = 10 * time.Second

type DialOptions struct {
	MaxProbePacketCount int
	DebugChunks         func(conn net.Conn) bool

	// SetupTimeout is the max. time for the setup of the connection (handshake and commands).
	// 0 is DefaultSetupTimeout, a negative value disables the timeout.
	SetupTimeout time.Duration

	// MaxChunkStreams is the max. number of chunk streams the peer can use. 0 is DefaultMaxChunkStreams.
	MaxChunkStreams int

	// TLSConfig is the TLS configuration for rtmps:// URLs. If nil, the default configuration is used.
	// If ServerName is not set, the host of the URL is used.
	TLSConfig *tls.Config

	// App is the application name that is sent with connect, for an application name with more than one
	// segment (e.g. "app/instance"). The path of the URL must start with it, the rest of the path is the
	// stream name. If empty, the application name is the first segment of the path (see SplitPath).
	App string
}

func Dial(uri string, options DialOptions) (conn *Conn, err error) {
	return DialTimeout(uri, 0, options)
}

func DialTimeout(uri string, timeout time.Duration, options DialOptions) (conn *Conn, err error) {
	var u *url.URL
	if u, err = ParseURL(uri); err != nil {
		return
	}

	if len(options.App) != 0 {
		if _, _, err = splitAppPath(u, options.App); err != nil {
			return
		}
	}

	dailer := net.Dialer{Timeout: timeout}
	var netconn net.Conn
	if u.Scheme == "rtmps" {
		var config *tls.Config
		if options.TLSConfig != nil {
			config = options.TLSConfig.Clone()
		} else {
			config = &tls.Config{}
		}
		if len(config.ServerName) == 0 {
			config.ServerName = u.Hostname()
		}
		if netconn, err = tls.DialWithDialer(&dailer, "tcp", u.Host, config); err != nil {
			return
		}
	} else {
		if netconn, err = dailer.Dial("tcp", u.Host); err != nil {
			return
		}
	}

	conn = NewConn(netconn)
	conn.URL = u
	conn.app = options.App
	conn.prober = flv.NewProber(options.MaxProbePacketCount)
	if options.DebugChunks != nil {
		conn.debugChunks = options.DebugChunks(netconn)
	}
	if options.SetupTimeout != 0 {
		conn.setupTimeout = options.SetupTimeout
	}
	if options.MaxChunkStreams > 0 {
		conn.maxChunkStreams = options.MaxChunkStreams
	}

	return
}

var ErrServerClosed = errors.New("server closed")

type Server struct {
	Addr          string
	TLSConfig     *tls.Config
	HandlePublish func(*Conn)
	HandlePlay    func(*Conn)
	HandleConn    func(*Conn)

	// HandleAuthorize is called when a client wants to publish or play a stream, before the server
	// confirms it. conn.URL is set and IsPublishing or IsPlaying reports true. If it returns an error,
	// the client is rejected with the error as description of the status and the connection is closed.
	HandleAuthorize func(conn *Conn) error

	MaxProbePacketCount   int
	SkipInvalidMessages   bool
	DebugChunks           func(conn net.Conn) bool
	ConnectionIdleTimeout time.Duration

	// SetupTimeout is the max. time for the setup of a connection (handshake and commands).
	// 0 is DefaultSetupTimeout, a negative value disables the timeout.
	SetupTimeout time.Duration

	// MaxChunkStreams is the max. number of chunk streams a peer can use. 0 is DefaultMaxChunkStreams.
	MaxChunkStreams int

	// lock guards the listener, the active connections and whether the server is closed
	lock     sync.Mutex
	listener net.Listener
	conns    map[net.Conn]struct{}
	closed   bool
}

// trackConn adds or removes an active connection. It returns false if the server is closed.
func (s *Server) trackConn(conn net.Conn, add bool) bool {
	s.lock.Lock()
	defer s.lock.Unlock()

	if !add {
		delete(s.conns, conn)
		return true
	}

	if s.closed {
		return false
	}

	if s.conns == nil {
		s.conns = make(map[net.Conn]struct{})
	}
	s.conns[conn] = struct{}{}

	return true
}

func (s *Server) isClosed() bool {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.closed
}

func (s *Server) HandleNetConn(netconn net.Conn) (err error) {
	if !s.trackConn(netconn, true) {
		netconn.Close()
		return ErrServerClosed
	}
	defer s.trackConn(netconn, false)

	conn := NewConn(netconn)
	conn.prober = flv.NewProber(s.MaxProbePacketCount)
	conn.skipInvalidMessages = s.SkipInvalidMessages
	if s.DebugChunks != nil {
		conn.debugChunks = s.DebugChunks(netconn)
	}
	conn.netconn.SetIdleTimeout(s.ConnectionIdleTimeout)
	if s.SetupTimeout != 0 {
		conn.setupTimeout = s.SetupTimeout
	}
	if s.MaxChunkStreams > 0 {
		conn.maxChunkStreams = s.MaxChunkStreams
	}
	conn.isserver = true
	conn.authorize = s.HandleAuthorize

	err = s.handleConn(conn)
	if Debug {
		fmt.Println("rtmp: server: client closed err:", err)
	}

	conn.Close()

	return
}

func (s *Server) handleConn(conn *Conn) (err error) {
	if s.HandleConn != nil {
		s.HandleConn(conn)
	} else {
		if err = conn.prepare(stageCommandDone, 0); err != nil {
			return
		}

		if conn.playing {
			conn.netconn.SetReadIdleTimeout(0)
			if s.HandlePlay != nil {
				s.HandlePlay(conn)
			}
		} else if conn.publishing {
			conn.netconn.SetWriteIdleTimeout(0)
			if s.HandlePublish != nil {
				s.HandlePublish(conn)
			}
		}
	}

	return
}

func (s *Server) ListenAndServe() error {
	addr := s.Addr
	if addr == "" {
		addr = ":1935"
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	return s.Serve(listener)
}

func (s *Server) ListenAndServeTLS(certFile, keyFile string) error {
	addr := s.Addr
	if addr == "" {
		addr = ":1935"
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	return s.ServeTLS(listener, certFile, keyFile)
}

func (s *Server) ServeTLS(listener net.Listener, certFile, keyFile string) error {
	var config *tls.Config

	if s.TLSConfig != nil {
		config = s.TLSConfig.Clone()
	} else {
		config = &tls.Config{}
	}

	configHasCert := len(config.Certificates) > 0 || config.GetCertificate != nil

	if !configHasCert || certFile != "" || keyFile != "" {
		config.Certificates = make([]tls.Certificate, 1)

		var err error
		config.Certificates[0], err = tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return err
		}
	}

	listener = tls.NewListener(listener, config)

	return s.Serve(listener)
}

func (s *Server) Serve(listener net.Listener) error {
	s.lock.Lock()
	if s.closed {
		s.lock.Unlock()
		listener.Close()
		return ErrServerClosed
	}
	s.listener = listener
	s.lock.Unlock()

	defer listener.Close()

	if Debug {
		fmt.Println("rtmp: server: listening on", listener.Addr().String())
	}

	for {
		netconn, err := listener.Accept()
		if err != nil {
			if s.isClosed() {
				return ErrServerClosed
			}

			return err
		}

		if Debug {
			fmt.Println("rtmp: server: accepted")
		}

		// HandleNetConn logs the error
		go s.HandleNetConn(netconn)
	}
}

// Close closes the listener and all active connections. Serve returns ErrServerClosed afterwards.
// It can be called more than once.
func (s *Server) Close() {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.closed {
		return
	}
	s.closed = true

	if s.listener != nil {
		s.listener.Close()
	}

	for conn := range s.conns {
		conn.Close()
	}
}

const (
	stageHandshakeDone = iota + 1
	stageCommandDone
	stageCodecDataDone
)

const (
	prepareReading = iota + 1
	prepareWriting
)

type Conn struct {
	URL *url.URL

	// the application name of a client, if it is not the first segment of the path of the URL
	app string

	authorize func(conn *Conn) error

	prober  *flv.Prober
	streams []av.CodecData

	bufr *bufio.Reader
	bufw *bufio.Writer

	// wlock serializes writing messages, such that the read path (e.g. acks, ping responses) and
	// the write path (e.g. WritePacket) can run in different goroutines. It guards bufw, writebuf
	// and writeMaxChunkSize.
	wlock sync.Mutex

	ackn uint32 // received bytes since the last ack
	acks uint32 // received bytes since the start of the session (sequence number of the ack)

	writebuf []byte
	readbuf  []byte

	netconn   *idleConn
	txrxcount *txrxcount

	writeMaxChunkSize int
	readMaxChunkSize  int
	readAckSize       uint32
	readcsmap         map[uint32]*chunkStream

	writeAckSize   uint32 // the last window ack size sent to the peer
	peerBWLimit    uint8  // the limit type of the last applied SetPeerBandwidth
	peerBWLimitSet bool

	isserver            bool
	publishing, playing bool
	ended               bool // the peer ended the stream
	reading, writing    bool
	stage               int

	// ends the stream of a publishing client once on Close
	unpublishOnce sync.Once

	// the error of the read loop while writing the stream
	readLoopLock sync.Mutex
	readLoopErr  error

	avmsgsid uint32

	gotcommand     bool
	commandname    string
	commandtransid float64
	commandobj     flvio.AMFMap
	commandparams  []interface{}

	// the command object of the connect command
	connectparams flvio.AMFMap

	gotmsg      bool
	timestamp   int64
	msgdata     []byte
	msgtypeid   uint8
	datamsgvals []interface{}
	avtag       flvio.Tag

	metadata flvio.AMFMap

	// the color info to write before the next video packet
	colorInfo        flvio.AMFMap
	colorInfoPending bool

	eventtype uint16

	start time.Time

	// wall clock timestamps for audio and video, if the peer doesn't provide changing timestamps
	genwallclocktime bool
	wallclockbase    int64
	wallclockstart   time.Time

	setupTimeout time.Duration

	maxChunkStreams int

	skipInvalidMessages bool

	debugChunks bool
}

type idleConn struct {
	net.Conn

	// lock guards the timeouts and the deadline, as they can be changed while reading or writing
	lock             sync.Mutex
	ReadIdleTimeout  time.Duration
	WriteIdleTimeout time.Duration
	setupDeadline    time.Time
}

// deadline returns the deadline for the given idle timeout, limited by the setup deadline. The lock must be held.
func (t *idleConn) deadline(idle time.Duration) time.Time {
	deadline := time.Time{}
	if idle > 0 {
		deadline = time.Now().Add(idle)
	}

	if !t.setupDeadline.IsZero() && (deadline.IsZero() || t.setupDeadline.Before(deadline)) {
		deadline = t.setupDeadline
	}

	return deadline
}

func (t *idleConn) Read(p []byte) (int, error) {
	t.lock.Lock()
	if t.ReadIdleTimeout > 0 || !t.setupDeadline.IsZero() {
		t.Conn.SetReadDeadline(t.deadline(t.ReadIdleTimeout))
	}
	t.lock.Unlock()

	n, err := t.Conn.Read(p)
	return n, err
}

func (t *idleConn) Write(p []byte) (int, error) {
	t.lock.Lock()
	if t.WriteIdleTimeout > 0 || !t.setupDeadline.IsZero() {
		t.Conn.SetWriteDeadline(t.deadline(t.WriteIdleTimeout))
	}
	t.lock.Unlock()

	n, err := t.Conn.Write(p)
	return n, err
}

func (t *idleConn) SetReadIdleTimeout(d time.Duration) error {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.ReadIdleTimeout = d

	return t.Conn.SetReadDeadline(t.deadline(d))
}

func (t *idleConn) SetWriteIdleTimeout(d time.Duration) error {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.WriteIdleTimeout = d

	return t.Conn.SetWriteDeadline(t.deadline(d))
}

// SetSetupDeadline sets an absolute deadline for reading and writing in addition to
// the idle timeouts. A zero value removes the deadline.
func (t *idleConn) SetSetupDeadline(deadline time.Time) error {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.setupDeadline = deadline

	if err := t.Conn.SetReadDeadline(t.deadline(t.ReadIdleTimeout)); err != nil {
		return err
	}

	return t.Conn.SetWriteDeadline(t.deadline(t.WriteIdleTimeout))
}

func (t *idleConn) SetIdleTimeout(d time.Duration) error {
	err := t.SetReadIdleTimeout(d)
	if err != nil {
		return err
	}

	return t.SetWriteIdleTimeout(d)
}

// txrxcount counts the transmitted and received bytes. The counters are accessed atomically, as
// reading, writing and reading the counters can happen in different goroutines. They are the first
// fields for the 64-bit alignment of atomic operations on 32-bit platforms.
type txrxcount struct {
	txbytes uint64
	rxbytes uint64
	io.ReadWriteCloser
}

func (t *txrxcount) Read(p []byte) (int, error) {
	n, err := t.ReadWriteCloser.Read(p)
	atomic.AddUint64(&t.rxbytes, uint64(n))
	return n, err
}

func (t *txrxcount) Write(p []byte) (int, error) {
	n, err := t.ReadWriteCloser.Write(p)
	atomic.AddUint64(&t.txbytes, uint64(n))
	return n, err
}

func NewConn(netconn net.Conn) *Conn {
	conn := &Conn{}
	conn.prober = &flv.Prober{}
	conn.netconn = &idleConn{
		Conn: netconn,
	}
	conn.readcsmap = make(map[uint32]*chunkStream)
	conn.readMaxChunkSize = 128
	conn.writeMaxChunkSize = 128
	conn.readAckSize = 1048576
	conn.txrxcount = &txrxcount{ReadWriteCloser: conn.netconn}
	conn.bufr = bufio.NewReaderSize(conn.txrxcount, pio.RecommendBufioSize)
	conn.bufw = bufio.NewWriterSize(conn.txrxcount, pio.RecommendBufioSize)
	conn.writebuf = make([]byte, 4096)
	conn.readbuf = make([]byte, 4096)
	conn.start = time.Now()
	conn.setupTimeout = DefaultSetupTimeout
	conn.maxChunkStreams = DefaultMaxChunkStreams
	return conn
}

type chunkStream struct {
	timenow        uint32
	xtimenow       int64
	prevtimenow    uint32
	hasprevtimenow bool
	sametscount    int
	timedelta      uint32
	hastimeext     bool
	timeext        uint32
	timeextrepeat  int
	msgsid         uint32
	msgtypeid      uint8
	msgdatalen     uint32
	msgdataleft    uint32
	msghdrtype     uint8
	msgdata        []byte
	msgcount       uint64
}

const (
	timeextRepeatUnknown = iota // not yet known whether the peer repeats the extended timestamp in type 3 chunks
	timeextRepeatYes
	timeextRepeatNo
)

func (cs *chunkStream) Start() {
	cs.msgdataleft = cs.msgdatalen
	cs.msgdata = nil
}

// grow extends the message data by n bytes and returns the new part. The buffer
// grows with the data actually received, not with the announced message length,
// such that a peer can't make us allocate memory without sending the data.
func (cs *chunkStream) grow(n int) []byte {
	l := len(cs.msgdata)
	if cap(cs.msgdata)-l < n {
		c := 2 * cap(cs.msgdata)
		if c < l+n {
			c = l + n
		}
		if c > int(cs.msgdatalen) {
			c = int(cs.msgdatalen)
		}
		buf := make([]byte, l, c)
		copy(buf, cs.msgdata)
		cs.msgdata = buf
	}
	cs.msgdata = cs.msgdata[:l+n]
	return cs.msgdata[l:]
}

const (
	msgtypeidUserControl      = 4
	msgtypeidAck              = 3
	msgtypeidWindowAckSize    = 5
	msgtypeidSetPeerBandwidth = 6
	msgtypeidSetChunkSize     = 1
	msgtypeidAbort            = 2
	msgtypeidCommandMsgAMF0   = 20
	msgtypeidCommandMsgAMF3   = 17
	msgtypeidDataMsgAMF0      = 18
	msgtypeidDataMsgAMF3      = 15
	msgtypeidVideoMsg         = 9
	msgtypeidAudioMsg         = 8
)

const (
	peerBWLimitHard    = 0
	peerBWLimitSoft    = 1
	peerBWLimitDynamic = 2
)

const (
	eventtypeStreamBegin      = 0
	eventtypeSetBufferLength  = 3
	eventtypeStreamIsRecorded = 4
	eventtypePingRequest      = 6
	eventtypePingResponse     = 7
)

func (conn *Conn) NetConn() net.Conn {
	return conn.netconn.Conn
}

func (conn *Conn) LocalAddr() net.Addr {
	return conn.netconn.LocalAddr()
}

func (conn *Conn) RemoteAddr() net.Addr {
	return conn.netconn.RemoteAddr()
}

func (conn *Conn) SetReadIdleTimeout(d time.Duration) error {
	return conn.netconn.SetReadIdleTimeout(d)
}

func (conn *Conn) SetWriteIdleTimeout(d time.Duration) error {
	return conn.netconn.SetWriteIdleTimeout(d)
}

func (conn *Conn) SetIdleTimeout(d time.Duration) error {
	return conn.netconn.SetIdleTimeout(d)
}

func (conn *Conn) TxBytes() uint64 {
	return atomic.LoadUint64(&conn.txrxcount.txbytes)
}

func (conn *Conn) RxBytes() uint64 {
	return atomic.LoadUint64(&conn.txrxcount.rxbytes)
}

// IsPublishing returns whether the client publishes a stream.
func (conn *Conn) IsPublishing() bool {
	return conn.publishing
}

// IsPlaying returns whether the client plays a stream.
func (conn *Conn) IsPlaying() bool {
	return conn.playing
}

// ConnectParams returns the command object of the connect command of the client, e.g. with the codecs it
// supports (fourCcList, videoFourCcInfoMap). It is nil before the client has connected.
func (conn *Conn) ConnectParams() flvio.AMFMap {
	return conn.connectparams
}

// Close closes the connection. A publishing client ends the stream with FCUnpublish and deleteStream
// before, as FFmpeg and OBS do, such that the server sees the end of the stream instead of a broken
// connection.
func (conn *Conn) Close() (err error) {
	conn.unpublishOnce.Do(func() {
		if conn.isserver || !conn.publishing || conn.stage < stageCommandDone {
			return
		}

		_, publishpath := conn.splitPath()

		// don't wait long for a peer that doesn't read anymore
		conn.netconn.SetWriteIdleTimeout(time.Second)

		// > FCUnpublish('path'), deleteStream(avmsgsid)
		if err := conn.writeCommandMsg(false, 3, 0, "FCUnpublish", 6, nil, publishpath); err != nil {
			return
		}
		if err := conn.writeCommandMsg(false, 3, 0, "deleteStream", 7, nil, conn.avmsgsid); err != nil {
			return
		}
		conn.flushWrite()
	})

	return conn.netconn.Close()
}

func (conn *Conn) pollCommand() (err error) {
	for {
		if err = conn.pollMsg(); err != nil {
			return
		}
		if conn.gotcommand {
			return
		}
	}
}

func (conn *Conn) pollAVTag() (tag flvio.Tag, err error) {
	if conn.ended {
		err = io.EOF
		return
	}

	for {
		if err = conn.pollMsg(); err != nil {
			return
		}

		if conn.gotcommand && conn.isserver && conn.publishing {
			switch conn.commandname {
			case "FCUnpublish", "deleteStream", "closeStream":
				// the publisher ends the stream
				conn.ended = true
				err = io.EOF
				return
			}
		}

		if conn.gotcommand && !conn.isserver && conn.playing && conn.commandname == "onStatus" {
			info := conn.statusInfo()
			if level, _ := info["level"].(string); level == "error" {
				err = fmt.Errorf("play failed: %s", statusDescription(info))
				return
			}
			switch code, _ := info["code"].(string); code {
			case "NetStream.Play.UnpublishNotify", "NetStream.Play.Stop", "NetStream.Play.Complete":
				// the server ends the stream
				conn.ended = true
				err = io.EOF
				return
			}
		}

		switch conn.msgtypeid {
		case msgtypeidVideoMsg, msgtypeidAudioMsg:
			tag = conn.avtag
			return
		}
	}
}

func (conn *Conn) pollMsg() (err error) {
	conn.gotmsg = false
	conn.gotcommand = false
	conn.datamsgvals = nil
	conn.avtag = flvio.Tag{}
	for {
		if err = conn.readChunk(); err != nil {
			return
		}

		if conn.readAckSize != 0 && conn.ackn > conn.readAckSize {
			if err = conn.writeAck(conn.acks, false); err != nil {
				return fmt.Errorf("writeACK: %w", err)
			}
			conn.flushWrite()
			conn.ackn = 0
		}

		if conn.gotmsg {
			return
		}
	}
}

// SplitPath splits the path of the URL into the application name and the stream name. The application name
// is the first segment of the path, the stream name is the rest including the query, e.g. "app" and
// "instance/stream?token=x" for rtmp://host/app/instance/stream?token=x. FFmpeg would take "app/instance"
// as the application name. Use DialOptions.App for an application name with more than one segment.
func SplitPath(u *url.URL) (app, stream string) {
	pathsegs := strings.SplitN(u.RequestURI(), "/", 3)
	if len(pathsegs) > 1 {
		app = pathsegs[1]
	}
	if len(pathsegs) > 2 {
		stream = pathsegs[2]
	}
	return
}

// splitAppPath splits the path of the URL into the given application name and the stream name. The path must
// start with the application name.
func splitAppPath(u *url.URL, app string) (string, string, error) {
	app = strings.Trim(app, "/")
	uri := u.RequestURI()
	prefix := "/" + app
	if !strings.HasPrefix(uri, prefix) {
		return "", "", fmt.Errorf("rtmp: the path of the URL doesn't start with the app %q", app)
	}
	stream := uri[len(prefix):]
	if len(stream) != 0 && stream[0] != '/' {
		return "", "", fmt.Errorf("rtmp: the path of the URL doesn't start with the app %q", app)
	}
	return app, strings.TrimPrefix(stream, "/"), nil
}

// splitPath returns the application name and the stream name of a client.
func (conn *Conn) splitPath() (app, stream string) {
	if len(conn.app) != 0 {
		// the URL has been checked in DialTimeout
		app, stream, _ = splitAppPath(conn.URL, conn.app)
		return
	}
	return SplitPath(conn.URL)
}

func getTcUrl(u *url.URL, app string) string {
	nu := *u
	nu.Path = "/" + app
	return nu.String()
}

func createURL(tcurl, app, play string) (*url.URL, error) {
	ps := strings.Split(app+"/"+play, "/")
	out := []string{""}
	for _, s := range ps {
		if len(s) > 0 {
			out = append(out, s)
		}
	}
	if len(out) < 2 {
		out = append(out, "")
	}
	path := strings.Join(out, "/")

	u, err := url.ParseRequestURI(path)
	if err != nil {
		return nil, err
	}

	if tcurl != "" {
		tu, _ := url.Parse(tcurl)
		if tu != nil {
			u.Host = tu.Host
			u.Scheme = tu.Scheme
		}
	}

	return u, nil
}

var CodecTypes = flv.CodecTypes

// videoFourCCs are the FourCCs of the supported E-RTMP video codecs.
var videoFourCCs = [][4]byte{flvio.FOURCC_AV1, flvio.FOURCC_VP9, flvio.FOURCC_HEVC, flvio.FOURCC_VVC}

// fourCcCanForward is the flag of the FourCcInfoMask (E-RTMP v2) for a codec that can be forwarded. joy4
// neither decodes nor encodes.
const fourCcCanForward = 0x04

// videoFourCcList returns the supported video codecs for the fourCcList property of the connect command
// and its result (E-RTMP).
func videoFourCcList() flvio.AMFArray {
	list := flvio.AMFArray{}
	for _, fourCC := range videoFourCCs {
		list = append(list, string(fourCC[:]))
	}
	return list
}

// videoFourCcInfoMap returns the supported video codecs for the videoFourCcInfoMap property of the connect
// command and its result (E-RTMP v2).
func videoFourCcInfoMap() flvio.AMFMap {
	infoMap := flvio.AMFMap{}
	for _, fourCC := range videoFourCCs {
		infoMap[string(fourCC[:])] = fourCcCanForward
	}
	return infoMap
}

func (conn *Conn) writeBasicConf() (err error) {
	// > WindowAckSize
	if err = conn.writeWindowAckSize(2500000, false); err != nil {
		return
	}
	if conn.isserver {
		// > SetPeerBandwidth, only sent by the server (as FFmpeg and librtmp do)
		if err = conn.writeSetPeerBandwidth(2500000, 2, true); err != nil {
			return
		}
		// > StreamBegin
		if err = conn.writeStreamBegin(0, true); err != nil {
			return
		}
	}
	// > SetChunkSize
	if err = conn.writeSetChunkSize(1024*64, true); err != nil {
		return
	}
	return
}

func (conn *Conn) readConnect() (err error) {
	var connectpath string

	// < connect("app")
	if err = conn.pollCommand(); err != nil {
		return
	}
	if conn.commandname != "connect" {
		err = fmt.Errorf("first command is not connect")
		return
	}
	if conn.commandobj == nil {
		err = fmt.Errorf("connect command params invalid")
		return
	}

	//fmt.Printf("readConnect: %+v\n", conn.commandobj)
	conn.connectparams = conn.commandobj

	var ok bool
	var _app, _tcurl interface{}
	if _app, ok = conn.commandobj["app"]; !ok {
		err = fmt.Errorf("the `connect` params missing `app`")
		return
	}
	connectpath, _ = _app.(string)

	var tcurl string
	if _tcurl, ok = conn.commandobj["tcUrl"]; !ok {
		_tcurl, ok = conn.commandobj["tcurl"]
	}
	if ok {
		tcurl, _ = _tcurl.(string)
	}

	if err = conn.writeBasicConf(); err != nil {
		return
	}

	properties := flvio.AMFMap{
		"fmsVer":       "FMS/3,0,1,123",
		"capabilities": 31,
	}

	// a client that supports E-RTMP gets the supported codecs (E-RTMP v2 recommends it)
	_, hasFourCcList := conn.commandobj["fourCcList"]
	_, hasVideoFourCcInfoMap := conn.commandobj["videoFourCcInfoMap"]
	if hasFourCcList || hasVideoFourCcInfoMap {
		properties["fourCcList"] = videoFourCcList()
		properties["videoFourCcInfoMap"] = videoFourCcInfoMap()
	}

	// > _result("NetConnection.Connect.Success")
	if err = conn.writeCommandMsg(false, 3, 0, "_result", conn.commandtransid,
		properties,
		flvio.AMFMap{
			"level":          "status",
			"code":           "NetConnection.Connect.Success",
			"description":    "Connection succeeded.",
			"objectEncoding": 0, // only AMF0 is supported
		},
	); err != nil {
		return
	}

	// > onBWDone()
	if err = conn.writeCommandMsg(true, 3, 0, "onBWDone", 0, nil, 8192); err != nil {
		return
	}

	if err = conn.flushWrite(); err != nil {
		return
	}

	for {
		if err = conn.pollMsg(); err != nil {
			return
		}
		if conn.gotcommand {
			switch conn.commandname {

			// < releaseStream("path"), FCPublish("path")
			case "releaseStream", "FCPublish":
				// > _result(), only if the client expects an answer
				if conn.commandtransid != 0 {
					if err = conn.writeCommandMsg(false, 3, 0, "_result", conn.commandtransid, nil); err != nil {
						return
					}
					if err = conn.flushWrite(); err != nil {
						return
					}
				}

			// < createStream
			case "createStream":
				conn.avmsgsid = uint32(1)
				// > _result(streamid)
				if err = conn.writeCommandMsg(false, 3, 0, "_result", conn.commandtransid, nil, conn.avmsgsid); err != nil {
					return
				}
				if err = conn.flushWrite(); err != nil {
					return
				}

			// < publish("path")
			case "publish":
				if Debug {
					fmt.Println("rtmp: < publish")
				}

				if len(conn.commandparams) < 1 {
					err = fmt.Errorf("publish params invalid")
					return
				}
				publishpath, _ := conn.commandparams[0].(string)

				u, uerr := createURL(tcurl, connectpath, publishpath)
				if uerr != nil {
					err = fmt.Errorf("invalid URL: %w", uerr)
					return
				}

				conn.URL = u
				conn.publishing = true

				if conn.authorize != nil {
					if aerr := conn.authorize(conn); aerr != nil {
						// > onStatus()
						if err = conn.writeCommandMsg(false, 5, conn.avmsgsid,
							"onStatus", 0, nil,
							flvio.AMFMap{
								"level":       "error",
								"code":        "NetStream.Publish.BadName",
								"description": aerr.Error(),
							},
						); err != nil {
							return
						}
						if err = conn.flushWrite(); err != nil {
							return
						}

						err = fmt.Errorf("publish not authorized: %w", aerr)
						return
					}
				}

				// > onStatus()
				if err = conn.writeCommandMsg(false, 5, conn.avmsgsid,
					"onStatus", 0, nil,
					flvio.AMFMap{
						"level":       "status",
						"code":        "NetStream.Publish.Start",
						"description": "Start publishing",
					},
				); err != nil {
					return
				}
				if err = conn.flushWrite(); err != nil {
					return
				}

				conn.reading = true
				conn.stage++
				return

			// < play("path")
			case "play":
				if Debug {
					fmt.Println("rtmp: < play")
				}

				if len(conn.commandparams) < 1 {
					err = fmt.Errorf("command play params invalid")
					return
				}
				playpath, _ := conn.commandparams[0].(string)

				u, uerr := createURL(tcurl, connectpath, playpath)
				if uerr != nil {
					err = fmt.Errorf("invalid URL: %w", uerr)
					return
				}

				conn.URL = u
				conn.playing = true

				if conn.authorize != nil {
					if aerr := conn.authorize(conn); aerr != nil {
						// > onStatus()
						if err = conn.writeCommandMsg(false, 5, conn.avmsgsid,
							"onStatus", 0, nil,
							flvio.AMFMap{
								"level":       "error",
								"code":        "NetStream.Play.StreamNotFound",
								"description": aerr.Error(),
							},
						); err != nil {
							return
						}
						if err = conn.flushWrite(); err != nil {
							return
						}

						err = fmt.Errorf("play not authorized: %w", aerr)
						return
					}
				}

				// > streamBegin(streamid)
				if err = conn.writeStreamBegin(conn.avmsgsid, false); err != nil {
					return
				}

				// > onStatus()
				if err = conn.writeCommandMsg(false, 5, conn.avmsgsid,
					"onStatus", 0, nil,
					flvio.AMFMap{
						"level":       "status",
						"code":        "NetStream.Play.Start",
						"description": "Start live",
					},
				); err != nil {
					return
				}

				// > |RtmpSampleAccess()
				//if err = self.writeDataMsg(5, self.avmsgsid,
				//	"|RtmpSampleAccess", true, true,
				//); err != nil {
				//	return
				//}

				if err = conn.flushWrite(); err != nil {
					return
				}

				conn.writing = true
				conn.stage++
				return
			}

		}
	}
}

func (conn *Conn) checkConnectResult() (ok bool, errmsg string) {
	if len(conn.commandparams) < 1 {
		errmsg = "params length < 1"
		return
	}

	obj, _ := conn.commandparams[0].(flvio.AMFMap)
	if obj == nil {
		errmsg = "params[0] not object"
		return
	}

	_code := obj["code"]
	if _code == nil {
		errmsg = "code invalid"
		return
	}

	code, _ := _code.(string)
	if code != "NetConnection.Connect.Success" {
		errmsg = "code != NetConnection.Connect.Success"
		return
	}

	ok = true
	return
}

func (conn *Conn) checkCreateStreamResult() (ok bool, avmsgsid uint32) {
	if len(conn.commandparams) < 1 {
		return
	}

	ok = true
	_avmsgsid, _ := conn.commandparams[0].(float64)
	avmsgsid = uint32(_avmsgsid)
	return
}

func (conn *Conn) probe() (err error) {
	for !conn.prober.Probed() {
		var tag flvio.Tag
		if tag, err = conn.pollAVTag(); err != nil {
			return
		}
		// an invalid tag is skipped, it doesn't fail the connection
		if perr := conn.prober.PushTag(tag, conn.timestamp); perr != nil {
			if Debug {
				fmt.Printf("rtmp: error probing tag: %s\n", perr.Error())
			}
		}
	}

	// without any stream, the reason why the tags were invalid is more useful than an empty stream list
	if len(conn.prober.Streams) == 0 && conn.prober.Err() != nil {
		err = fmt.Errorf("rtmp: no stream probed: %w", conn.prober.Err())
		return
	}

	conn.streams = conn.prober.Streams
	conn.stage++
	return
}

func (conn *Conn) writeConnect(path string) (err error) {
	if err = conn.writeBasicConf(); err != nil {
		return
	}

	// > connect("app")
	if Debug {
		fmt.Printf("rtmp: > connect('%s') host=%s\n", path, conn.URL.Host)
	}
	if err = conn.writeCommandMsg(false, 3, 0, "connect", 1,
		flvio.AMFMap{
			"app":                path,
			"flashVer":           "MAC 22,0,0,192",
			"tcUrl":              getTcUrl(conn.URL, path),
			"fpad":               false,
			"capabilities":       15,
			"audioCodecs":        4071,
			"videoCodecs":        252,
			"videoFunction":      1,
			"fourCcList":         videoFourCcList(),
			"videoFourCcInfoMap": videoFourCcInfoMap(),
		},
	); err != nil {
		return
	}

	if err = conn.flushWrite(); err != nil {
		return
	}

	// < _result("NetConnection.Connect.Success")
	if err = conn.waitResult("connect", 1); err != nil {
		return
	}

	var ok bool
	var errmsg string
	if ok, errmsg = conn.checkConnectResult(); !ok {
		err = fmt.Errorf("command connect failed: %s", errmsg)
		return
	}
	if Debug {
		fmt.Printf("rtmp: < _result() of connect\n")
	}

	return
}

// waitResult waits for the _result or _error of the command with the transaction id. An _error is returned as error.
func (conn *Conn) waitResult(command string, transid float64) (err error) {
	for {
		if err = conn.pollMsg(); err != nil {
			return
		}
		if !conn.gotcommand || conn.commandtransid != transid {
			continue
		}
		switch conn.commandname {
		case "_result":
			return nil
		case "_error":
			return fmt.Errorf("command %s failed: %s", command, statusDescription(conn.statusInfo()))
		}
	}
}

// waitStatus waits for an onStatus with the code after the command with the transaction id. An onStatus
// with the level error, or an _error of the command, is returned as error.
func (conn *Conn) waitStatus(command string, transid float64, code string) (err error) {
	for {
		if err = conn.pollMsg(); err != nil {
			return
		}
		if !conn.gotcommand {
			continue
		}
		switch conn.commandname {
		case "_error":
			if transid != 0 && conn.commandtransid == transid {
				return fmt.Errorf("command %s failed: %s", command, statusDescription(conn.statusInfo()))
			}
		case "onStatus":
			info := conn.statusInfo()
			if level, _ := info["level"].(string); level == "error" {
				return fmt.Errorf("command %s failed: %s", command, statusDescription(info))
			}
			if c, _ := info["code"].(string); c == code {
				return nil
			}
		}
	}
}

// statusInfo returns the info object of an onStatus, _result or _error command.
func (conn *Conn) statusInfo() flvio.AMFMap {
	for _, param := range conn.commandparams {
		if info, ok := param.(flvio.AMFMap); ok {
			return info
		}
	}
	return flvio.AMFMap{}
}

// statusDescription returns the code and the description of an info object.
func statusDescription(info flvio.AMFMap) string {
	code, _ := info["code"].(string)
	description, _ := info["description"].(string)
	if len(description) == 0 {
		return code
	}
	return code + " (" + description + ")"
}

func (conn *Conn) connectPublish() (err error) {
	connectpath, publishpath := conn.splitPath()

	if err = conn.writeConnect(connectpath); err != nil {
		return
	}

	// > releaseStream('path'), FCPublish('path'), as FFmpeg and OBS do. Their answers are not
	// awaited, as not every server answers them (e.g. nginx-rtmp).
	if err = conn.writeCommandMsg(false, 3, 0, "releaseStream", 2, nil, publishpath); err != nil {
		return
	}
	if err = conn.writeCommandMsg(false, 3, 0, "FCPublish", 3, nil, publishpath); err != nil {
		return
	}

	// > createStream()
	if Debug {
		fmt.Printf("rtmp: > createStream()\n")
	}
	if err = conn.writeCommandMsg(false, 3, 0, "createStream", 4, nil); err != nil {
		return
	}

	if err = conn.flushWrite(); err != nil {
		return
	}

	// < _result(avmsgsid) of createStream
	if err = conn.waitResult("createStream", 4); err != nil {
		return
	}

	var ok bool
	if ok, conn.avmsgsid = conn.checkCreateStreamResult(); !ok {
		err = fmt.Errorf("createStream command failed")
		return
	}

	// > publish('app')
	if Debug {
		fmt.Printf("rtmp: > publish('%s')\n", publishpath)
	}
	if err = conn.writeCommandMsg(false, 8, conn.avmsgsid, "publish", 5, nil, publishpath, "live"); err != nil {
		return
	}

	if err = conn.flushWrite(); err != nil {
		return
	}

	// < onStatus("NetStream.Publish.Start")
	if err = conn.waitStatus("publish", 5, "NetStream.Publish.Start"); err != nil {
		return
	}

	conn.writing = true
	conn.publishing = true
	conn.stage++
	return
}

func (conn *Conn) connectPlay() (err error) {
	connectpath, playpath := conn.splitPath()

	if err = conn.writeConnect(connectpath); err != nil {
		return
	}

	// > createStream()
	if Debug {
		fmt.Printf("rtmp: > createStream()\n")
	}
	if err = conn.writeCommandMsg(false, 3, 0, "createStream", 2, nil); err != nil {
		return
	}

	if err = conn.flushWrite(); err != nil {
		return
	}

	// < _result(avmsgsid) of createStream
	if err = conn.waitResult("createStream", 2); err != nil {
		return
	}

	var ok bool
	if ok, conn.avmsgsid = conn.checkCreateStreamResult(); !ok {
		err = fmt.Errorf("createStream command failed")
		return
	}

	// > play('app')
	if Debug {
		fmt.Printf("rtmp: > play('%s')\n", playpath)
	}
	if err = conn.writeCommandMsg(false, 8, conn.avmsgsid, "play", 0, nil, playpath); err != nil {
		return
	}

	// > SetBufferLength(avmsgsid, 100ms)
	if err = conn.writeSetBufferLength(conn.avmsgsid, 100, false); err != nil {
		return
	}

	if err = conn.flushWrite(); err != nil {
		return
	}

	// < onStatus("NetStream.Play.Start")
	if err = conn.waitStatus("play", 0, "NetStream.Play.Start"); err != nil {
		return
	}

	conn.reading = true
	conn.playing = true
	conn.stage++
	return
}

func (conn *Conn) ReadPacket() (pkt av.Packet, err error) {
	if err = conn.prepare(stageCodecDataDone, prepareReading); err != nil {
		return
	}

	if !conn.prober.Empty() {
		pkt = conn.prober.PopPacket()
		return
	}

	for {
		var tag flvio.Tag
		if tag, err = conn.pollAVTag(); err != nil {
			return
		}

		var ok bool
		if pkt, ok = conn.prober.TagToPacket(tag, conn.timestamp); ok {
			return pkt, nil
		}
	}
}

func (conn *Conn) Prepare() (err error) {
	return conn.prepare(stageCommandDone, 0)
}

func (conn *Conn) prepare(stage int, flags int) (err error) {
	for conn.stage < stage {
		switch conn.stage {
		case 0:
			// the setup starts with the handshake and ends after the commands
			if conn.setupTimeout > 0 {
				conn.netconn.SetSetupDeadline(time.Now().Add(conn.setupTimeout))
			}

			if conn.isserver {
				if err = conn.handshakeServer(); err != nil {
					return
				}
			} else {
				if err = conn.handshakeClient(); err != nil {
					return
				}
			}

		case stageHandshakeDone:
			if conn.isserver {
				if err = conn.readConnect(); err != nil {
					return
				}
			} else {
				if flags == prepareReading {
					if err = conn.connectPlay(); err != nil {
						return
					}
				} else {
					if err = conn.connectPublish(); err != nil {
						return
					}
				}
			}

			if err = conn.netconn.SetSetupDeadline(time.Time{}); err != nil {
				return
			}

		case stageCommandDone:
			if flags == prepareReading {
				if err = conn.probe(); err != nil {
					return
				}
			} else {
				err = fmt.Errorf("call WriteHeader() before WritePacket()")
				return
			}
		}
	}
	return
}

func (conn *Conn) Streams() (streams []av.CodecData, err error) {
	if err = conn.prepare(stageCodecDataDone, prepareReading); err != nil {
		return
	}
	streams = conn.streams
	return
}

func (conn *Conn) WritePacket(pkt av.Packet) (err error) {
	if err = conn.prepare(stageCodecDataDone, prepareWriting); err != nil {
		return
	}

	if err = conn.readLoopError(); err != nil {
		return
	}

	stream := conn.streams[pkt.Idx]
	tag, timestamp := flv.PacketToTag(pkt, stream)

	if Debug {
		fmt.Println("rtmp: WritePacket", pkt.Idx, pkt.Time, pkt.CompositionTime)
	}

	if conn.colorInfoPending && stream.Type().IsVideo() {
		conn.colorInfoPending = false
		if colorInfoTag, ok := flv.ColorInfoToTag(conn.colorInfo, stream); ok {
			if err = conn.writeAVTag(colorInfoTag, timestamp); err != nil {
				return
			}
		}
	}

	if err = conn.writeAVTag(tag, timestamp); err != nil {
		return
	}

	return
}

func (conn *Conn) WriteTrailer() (err error) {
	if err = conn.flushWrite(); err != nil {
		return
	}
	return
}

func (conn *Conn) SetMetaData(data flvio.AMFMap) {
	conn.metadata = data
}

func (conn *Conn) GetMetaData() flvio.AMFMap {
	return conn.metadata
}

// SetColorInfo sets the color info (HDR metadata, E-RTMP). It is sent before the next video packet if the
// video codec is HEVC, AV1, VP9 or VVC, otherwise it is dropped. A nil color info resets it.
func (conn *Conn) SetColorInfo(colorInfo flvio.AMFMap) {
	conn.colorInfo = colorInfo
	conn.colorInfoPending = true
}

// GetColorInfo returns the color info (HDR metadata, E-RTMP) that applies to the video packet read last and
// the following ones. It is nil if no color info has been received or it has been reset.
func (conn *Conn) GetColorInfo() flvio.AMFMap {
	return conn.prober.ColorInfo
}

func (conn *Conn) WriteHeader(streams []av.CodecData) (err error) {
	if err = conn.prepare(stageCommandDone, prepareWriting); err != nil {
		return
	}

	var metadata flvio.AMFMap

	if metadata, err = flv.NewMetadataByStreams(streams); err != nil {
		return
	}

	if conn.metadata != nil {
		for key, value := range conn.metadata {
			if _, hasKey := metadata[key]; hasKey {
				continue
			}

			metadata[key] = value
		}
	}

	// > onMetaData()
	if err = conn.writeDataMsg(5, conn.avmsgsid, "onMetaData", metadata); err != nil {
		return
	}

	// > Videodata(decoder config)
	// > Audiodata(decoder config)
	for _, stream := range streams {
		var ok bool
		var tag flvio.Tag
		if tag, ok, err = flv.CodecDataToTag(stream); err != nil {
			return
		}
		if ok {
			if err = conn.writeAVTag(tag, 0); err != nil {
				return
			}
		}
	}

	conn.streams = streams
	conn.stage++

	// the peer is not required to send anything while a client publishes or a server sends to a player
	conn.netconn.SetReadIdleTimeout(0)
	go conn.readLoop()

	return
}

// readLoop reads the messages of the peer while writing the stream, i.e. while a client publishes or
// while a server sends the stream to a player, such that pings are answered and acks are sent. A read
// error, an error status of the server, or the end of the stream by the player (io.EOF) is returned by
// the next WritePacket.
func (conn *Conn) readLoop() {
	for {
		if err := conn.pollMsg(); err != nil {
			conn.setReadLoopError(err)
			return
		}

		if !conn.gotcommand {
			continue
		}

		if conn.isserver {
			switch conn.commandname {
			case "closeStream", "deleteStream":
				// the player ends the stream
				conn.setReadLoopError(io.EOF)
				return
			}
		} else if conn.commandname == "onStatus" {
			info := conn.statusInfo()
			if level, _ := info["level"].(string); level == "error" {
				conn.setReadLoopError(fmt.Errorf("publish failed: %s", statusDescription(info)))
				return
			}
		}
	}
}

func (conn *Conn) setReadLoopError(err error) {
	conn.readLoopLock.Lock()
	defer conn.readLoopLock.Unlock()

	if conn.readLoopErr == nil {
		conn.readLoopErr = err
	}
}

func (conn *Conn) readLoopError() error {
	conn.readLoopLock.Lock()
	defer conn.readLoopLock.Unlock()

	return conn.readLoopErr
}

func (conn *Conn) tmpwbuf(n int) []byte {
	if len(conn.writebuf) < n {
		conn.writebuf = make([]byte, n)
	}
	return conn.writebuf
}

func (conn *Conn) writeSetChunkSize(size int, append bool) (err error) {
	conn.wlock.Lock()
	defer conn.wlock.Unlock()

	conn.writeMaxChunkSize = size
	var b []byte
	var n int

	b = conn.tmpwbuf(chunkHeaderLength + 4)
	n = conn.fillChunkHeader(append, b, 2, 0, msgtypeidSetChunkSize, 0, 4)
	pio.PutU32BE(b[n:], uint32(size))
	n += 4
	_, err = conn.bufw.Write(b[:n])
	return
}

func (conn *Conn) writeAck(seqnum uint32, append bool) (err error) {
	conn.wlock.Lock()
	defer conn.wlock.Unlock()

	var b []byte
	var n int

	b = conn.tmpwbuf(chunkHeaderLength + 4)
	n = conn.fillChunkHeader(append, b, 2, 0, msgtypeidAck, 0, 4)
	pio.PutU32BE(b[n:], seqnum)
	n += 4
	_, err = conn.bufw.Write(b[:n])
	return
}

func (conn *Conn) writeWindowAckSize(size uint32, append bool) (err error) {
	conn.wlock.Lock()
	defer conn.wlock.Unlock()

	var b []byte
	var n int

	b = conn.tmpwbuf(chunkHeaderLength + 4)
	n = conn.fillChunkHeader(append, b, 2, 0, msgtypeidWindowAckSize, 0, 4)
	pio.PutU32BE(b[n:], size)
	n += 4
	if _, err = conn.bufw.Write(b[:n]); err != nil {
		return
	}
	conn.writeAckSize = size
	return
}

func (conn *Conn) writeSetPeerBandwidth(acksize uint32, limittype uint8, append bool) (err error) {
	conn.wlock.Lock()
	defer conn.wlock.Unlock()

	var b []byte
	var n int

	b = conn.tmpwbuf(chunkHeaderLength + 5)
	n = conn.fillChunkHeader(append, b, 2, 0, msgtypeidSetPeerBandwidth, 0, 5)
	pio.PutU32BE(b[n:], acksize)
	n += 4
	b[n] = limittype
	n++
	_, err = conn.bufw.Write(b[:n])
	return
}

func (conn *Conn) writeCommandMsg(append bool, csid, msgsid uint32, args ...interface{}) (err error) {
	return conn.writeAMF0Msg(append, msgtypeidCommandMsgAMF0, csid, msgsid, args...)
}

func (conn *Conn) writeDataMsg(csid, msgsid uint32, args ...interface{}) (err error) {
	return conn.writeAMF0Msg(false, msgtypeidDataMsgAMF0, csid, msgsid, args...)
}

func (conn *Conn) writeAMF0Msg(append bool, msgtypeid uint8, csid, msgsid uint32, args ...interface{}) (err error) {
	conn.wlock.Lock()
	defer conn.wlock.Unlock()

	size := 0
	for _, arg := range args {
		size += flvio.LenAMF0Val(arg)
	}

	if size > maxMessageLength {
		return fmt.Errorf("message too long: %d bytes", size)
	}

	b := conn.tmpwbuf(chunkHeaderLength + size)
	hdrlen := conn.fillChunkHeader(append, b, csid, 0, msgtypeid, msgsid, size)
	n := hdrlen
	for _, arg := range args {
		n += flvio.FillAMF0Val(b[n:], arg)
	}

	return conn.writeChunks(b[:hdrlen], csid, 0, b[hdrlen:n])
}

// writeChunks writes a message as chunks of at most writeMaxChunkSize bytes. The first chunk
// starts with the given chunk header, the following chunks start with a type 3 chunk header.
// The message is the concatenation of the data.
func (conn *Conn) writeChunks(header []byte, csid uint32, timestamp uint32, data ...[]byte) (err error) {
	if _, err = conn.bufw.Write(header); err != nil {
		return
	}

	var header3 [5]byte
	chunksize := conn.writeMaxChunkSize
	left := chunksize

	for _, d := range data {
		for len(d) > 0 {
			if left == 0 {
				n := conn.fillChunkHeader3(header3[:], csid, timestamp)
				if _, err = conn.bufw.Write(header3[:n]); err != nil {
					return
				}
				left = chunksize
			}

			n := len(d)
			if n > left {
				n = left
			}

			if _, err = conn.bufw.Write(d[:n]); err != nil {
				return
			}

			d = d[n:]
			left -= n
		}
	}

	return
}

func (conn *Conn) writeAVTag(tag flvio.Tag, ts int64) (err error) {
	conn.wlock.Lock()
	defer conn.wlock.Unlock()

	var msgtypeid uint8
	var csid uint32
	var data []byte

	switch tag.Type {
	case flvio.TAG_AUDIO:
		msgtypeid = msgtypeidAudioMsg
		csid = 6
		data = tag.Data

	case flvio.TAG_VIDEO:
		msgtypeid = msgtypeidVideoMsg
		csid = 7
		data = tag.Data

	default:
		return fmt.Errorf("rtmp: invalid tag type %d, only audio and video are allowed", tag.Type)
	}

	// RTMP timestamps have 32 bits and wrap around at 2^32
	flvTimestamp := uint32(ts)

	actualChunkHeaderLength := chunkHeaderLength
	if flvTimestamp >= maxFlvTimestamp {
		actualChunkHeaderLength += 4
	}

	b := conn.tmpwbuf(actualChunkHeaderLength + flvio.MaxTagSubHeaderLength)
	hdrlen := tag.FillHeader(b[actualChunkHeaderLength:])

	if hdrlen+len(data) > maxMessageLength {
		return fmt.Errorf("message too long: %d bytes", hdrlen+len(data))
	}

	conn.fillChunkHeader(false, b, csid, flvTimestamp, msgtypeid, conn.avmsgsid, hdrlen+len(data))

	if err = conn.writeChunks(b[:actualChunkHeaderLength], csid, flvTimestamp, b[actualChunkHeaderLength:actualChunkHeaderLength+hdrlen], data); err != nil {
		return
	}

	// every frame is flushed right away (one write per frame) for the lowest latency of a live stream
	err = conn.bufw.Flush()

	return
}

func (conn *Conn) writeStreamBegin(msgsid uint32, append bool) (err error) {
	conn.wlock.Lock()
	defer conn.wlock.Unlock()

	b := conn.tmpwbuf(chunkHeaderLength + 6)
	n := conn.fillChunkHeader(append, b, 2, 0, msgtypeidUserControl, 0, 6)
	pio.PutU16BE(b[n:], eventtypeStreamBegin)
	n += 2
	pio.PutU32BE(b[n:], msgsid)
	n += 4
	_, err = conn.bufw.Write(b[:n])
	return
}

func (conn *Conn) writeSetBufferLength(msgsid uint32, timestamp uint32, append bool) (err error) {
	conn.wlock.Lock()
	defer conn.wlock.Unlock()

	b := conn.tmpwbuf(chunkHeaderLength + 10)
	n := conn.fillChunkHeader(append, b, 2, 0, msgtypeidUserControl, 0, 10)
	pio.PutU16BE(b[n:], eventtypeSetBufferLength)
	n += 2
	pio.PutU32BE(b[n:], msgsid)
	n += 4
	pio.PutU32BE(b[n:], timestamp)
	n += 4
	_, err = conn.bufw.Write(b[:n])
	return
}

func (conn *Conn) writePingResponse(timestamp uint32, append bool) (err error) {
	conn.wlock.Lock()
	defer conn.wlock.Unlock()

	b := conn.tmpwbuf(chunkHeaderLength + 10)
	n := conn.fillChunkHeader(append, b, 2, 0, msgtypeidUserControl, 0, 6)
	pio.PutU16BE(b[n:], eventtypePingResponse)
	n += 2
	pio.PutU32BE(b[n:], timestamp)
	n += 4
	_, err = conn.bufw.Write(b[:n])
	return
}

const chunkHeaderLength = 12
const maxFlvTimestamp = uint32(0xFFFFFF)

// maxMessageLength is the max. length of a message, as the length in the chunk header has 24 bits
const maxMessageLength = 0xFFFFFF

func (conn *Conn) fillChunkHeader(append bool, b []byte, csid uint32, timestamp uint32, msgtypeid uint8, msgsid uint32, msgdatalen int) (n int) {
	if !append {
		//  0                   1                   2                   3
		//  0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
		// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
		// |                   timestamp                   |message length |
		// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
		// |     message length (cont)     |message type id| msg stream id |
		// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
		// |           message stream id (cont)            |
		// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
		//
		//       Figure 9 Chunk Message Header – Type 0

		b[n] = byte(csid) & 0x3f
		n++
		if timestamp < maxFlvTimestamp {
			pio.PutU24BE(b[n:], timestamp)
		} else {
			pio.PutU24BE(b[n:], maxFlvTimestamp)
		}
		n += 3
		pio.PutU24BE(b[n:], uint32(msgdatalen))
		n += 3
		b[n] = msgtypeid
		n++
		pio.PutU32LE(b[n:], msgsid)
		n += 4
		if timestamp >= maxFlvTimestamp {
			pio.PutU32BE(b[n:], timestamp)
			n += 4
		}
	} else {
		//  0                   1                   2                   3
		//  0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
		// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
		// |                timestamp delta                |message length |
		// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
		// |     message length (cont)     |message type id|
		// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
		//
		//       Figure 10 Chunk Message Header – Type 1

		b[n] = 1 << 6
		b[n] += byte(csid) & 0x3f
		n++
		pio.PutU24BE(b[n:], 0) // timestamp delta
		n += 3
		pio.PutU24BE(b[n:], uint32(msgdatalen))
		n += 3
		b[n] = msgtypeid
		n++
	}

	if Debug {
		fmt.Printf("rtmp: write chunk msgdatalen=%d msgsid=%d\n", msgdatalen, msgsid)
		fmt.Print(hex.Dump(b[:n]))
	}

	return
}

func (c *Conn) fillChunkHeader3(b []byte, csid uint32, timestamp uint32) (n int) {
	pio.PutU8(b, (uint8(csid)&0x3f)|3<<6)
	n++
	if timestamp >= maxFlvTimestamp {
		pio.PutU32BE(b[n:], timestamp)
		n += 4
	}

	return
}

func (conn *Conn) flushWrite() (err error) {
	conn.wlock.Lock()
	defer conn.wlock.Unlock()

	if err = conn.bufw.Flush(); err != nil {
		return
	}
	return
}

func (conn *Conn) readChunk() (err error) {
	b := conn.readbuf
	n := 0
	if _, err = io.ReadFull(conn.bufr, b[:1]); err != nil {
		return
	}
	header := b[0]
	n += 1

	var msghdrtype uint8
	var csid uint32

	msghdrtype = header >> 6

	csid = uint32(header) & 0x3f
	switch csid {
	default: // Chunk basic header 1
	case 0: // Chunk basic header 2
		if _, err = io.ReadFull(conn.bufr, b[:1]); err != nil {
			return fmt.Errorf("chunk basic header 2: %w", err)
		}
		n += 1
		csid = uint32(b[0]) + 64
	case 1: // Chunk basic header 3
		if _, err = io.ReadFull(conn.bufr, b[:2]); err != nil {
			return fmt.Errorf("chunk basic header 3: %w", err)
		}
		n += 2
		csid = uint32(pio.U16BE(b)) + 64
	}

	newcs := false
	cs := conn.readcsmap[csid]
	if cs == nil {
		if len(conn.readcsmap) >= conn.maxChunkStreams {
			err = fmt.Errorf("too many chunk streams, max. %d allowed", conn.maxChunkStreams)
			return
		}
		cs = &chunkStream{}
		conn.readcsmap[csid] = cs
		newcs = true
	}

	// critical timestamps: 4:39:00 (2^24 use of extended timestamp)
	// critical timestamps: 596:31:00 (2^31 wraparound from ffmpeg)
	var timestamp uint32

	switch msghdrtype {
	case 0:
		//  0                   1                   2                   3
		//  0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
		// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
		// |                   timestamp                   |message length |
		// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
		// |     message length (cont)     |message type id| msg stream id |
		// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
		// |           message stream id (cont)            |
		// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
		//
		//       Figure 9 Chunk Message Header – Type 0
		if cs.msgdataleft != 0 {
			if !conn.skipInvalidMessages {
				err = fmt.Errorf("chunk msgdataleft=%d invalid", cs.msgdataleft)
				return
			}
		}
		h := b[:11]
		if _, err = io.ReadFull(conn.bufr, h); err != nil {
			return
		}
		n += len(h)
		timestamp = pio.U24BE(h[0:3])
		//fmt.Printf("type 0      : timestamp         = %d", timestamp)
		cs.msghdrtype = msghdrtype
		cs.msgdatalen = pio.U24BE(h[3:6])
		cs.msgtypeid = h[6]
		cs.msgsid = pio.U32LE(h[7:11])
		if timestamp == maxFlvTimestamp {
			if _, err = io.ReadFull(conn.bufr, b[:4]); err != nil {
				return
			}
			n += 4
			timestamp = pio.U32BE(b)
			//fmt.Printf(", timeext = %d", timestamp)
			cs.hastimeext = true
			cs.timeext = timestamp
		} else {
			cs.hastimeext = false
		}
		cs.timenow = timestamp
		// a following type 3 chunk uses the timestamp as delta
		cs.timedelta = timestamp
		//fmt.Printf(", timenow = %d", cs.timenow)
		cs.Start()

	case 1:
		//  0                   1                   2                   3
		//  0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
		// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
		// |                timestamp delta                |message length |
		// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
		// |     message length (cont)     |message type id|
		// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
		//
		//       Figure 10 Chunk Message Header – Type 1
		if newcs {
			err = fmt.Errorf("chunk message type 1 without previous chunk")
			return
		}
		if cs.msgdataleft != 0 {
			if !conn.skipInvalidMessages {
				err = fmt.Errorf("chunk msgdataleft=%d invalid", cs.msgdataleft)
				return
			}
		}
		h := b[:7]
		if _, err = io.ReadFull(conn.bufr, h); err != nil {
			return
		}
		n += len(h)
		timestamp = pio.U24BE(h[0:3])
		//fmt.Printf("type 1      : timestamp (delta) = %d", timestamp)
		cs.msghdrtype = msghdrtype
		cs.msgdatalen = pio.U24BE(h[3:6])
		cs.msgtypeid = h[6]
		if timestamp == maxFlvTimestamp {
			if _, err = io.ReadFull(conn.bufr, b[:4]); err != nil {
				return
			}
			n += 4
			timestamp = pio.U32BE(b)
			//fmt.Printf(", timeext = %d (delta)", timestamp)
			cs.hastimeext = true
			cs.timeext = timestamp
		} else {
			cs.hastimeext = false
		}
		cs.timedelta = timestamp
		cs.timenow += timestamp
		//fmt.Printf(", timenow = %d", cs.timenow)
		cs.Start()

	case 2:
		//  0                   1                   2
		//  0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3
		// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
		// |                timestamp delta                |
		// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
		//
		//       Figure 11 Chunk Message Header – Type 2
		if newcs {
			err = fmt.Errorf("chunk message type 2 without previous chunk")
			return
		}
		if cs.msgdataleft != 0 {
			if !conn.skipInvalidMessages {
				err = fmt.Errorf("chunk msgdataleft=%d invalid", cs.msgdataleft)
				return
			}
		}
		h := b[:3]
		if _, err = io.ReadFull(conn.bufr, h); err != nil {
			return
		}
		n += len(h)
		cs.msghdrtype = msghdrtype
		timestamp = pio.U24BE(h[0:3])
		//fmt.Printf("type 2      : timestamp (delta) = %d", timestamp)
		if timestamp == maxFlvTimestamp {
			if _, err = io.ReadFull(conn.bufr, b[:4]); err != nil {
				return
			}
			n += 4
			timestamp = pio.U32BE(b)
			//fmt.Printf(", timeext = %d (delta)", timestamp)
			cs.hastimeext = true
			cs.timeext = timestamp
		} else {
			cs.hastimeext = false
		}
		cs.timedelta = timestamp
		cs.timenow += timestamp
		//fmt.Printf(", timenow = %d", cs.timenow)
		cs.Start()

	case 3:
		if newcs {
			err = fmt.Errorf("chunk message type 3 without previous chunk")
			return
		}

		if cs.msgdataleft == 0 {
			// A new message with the timestamp delta of the previous chunk. If the previous chunk is
			// of type 0, the delta is its timestamp (RTMP spec 5.3.1.2.4).
			if cs.hastimeext {
				if _, err = io.ReadFull(conn.bufr, b[:4]); err != nil {
					return
				}
				n += 4
				timestamp = pio.U32BE(b)
				cs.timeext = timestamp
			} else {
				timestamp = cs.timedelta
			}
			//fmt.Printf("type 3: timestamp (delta) = %d", timestamp)
			cs.timenow += timestamp
			//fmt.Printf(", timenow = %d", cs.timenow)
			cs.Start()
		} else if cs.hastimeext && cs.timeextrepeat != timeextRepeatNo {
			// A continuation chunk of a message with an extended timestamp. It's not clear from the
			// spec whether the extended timestamp is repeated in these chunks, some peers do (e.g. FFmpeg),
			// others don't. Detect it in the first continuation chunk and stop looking for it if the peer
			// doesn't repeat it, such that the beginning of the chunk data is not mistaken for it.
			var p []byte
			if p, err = conn.bufr.Peek(4); err != nil {
				return
			}
			if pio.U32BE(p) == cs.timeext {
				if _, err = conn.bufr.Discard(4); err != nil {
					return
				}
				n += 4
				cs.timeextrepeat = timeextRepeatYes
			} else {
				cs.timeextrepeat = timeextRepeatNo
			}
		}

	default:
		err = fmt.Errorf("invalid chunk msg header type=%d", msghdrtype)
		return
	}

	size := int(cs.msgdataleft)
	if size > conn.readMaxChunkSize {
		size = conn.readMaxChunkSize
	}
	buf := cs.grow(size)
	if _, err = io.ReadFull(conn.bufr, buf); err != nil {
		return
	}
	n += len(buf)
	cs.msgdataleft -= uint32(size)

	if Debug {
		fmt.Printf("rtmp: chunk msgsid=%d msgtypeid=%d msghdrtype=%d len=%d left=%d max=%d",
			cs.msgsid, cs.msgtypeid, msghdrtype, cs.msgdatalen, cs.msgdataleft, conn.readMaxChunkSize)
	}

	if conn.debugChunks {
		path := "***"
		if conn.URL != nil {
			path = conn.URL.Path
		}
		data := fmt.Sprintf("%s: chunk id=%d msgsid=%d msgtypeid=%d msghdrtype=%d timestamp=%d ext=%v wallclock=%v len=%d left=%d max=%d",
			path, csid, cs.msgsid, cs.msgtypeid, msghdrtype, cs.timenow, cs.hastimeext, conn.genwallclocktime, cs.msgdatalen, cs.msgdataleft, conn.readMaxChunkSize)

		if cs.msgtypeid != msgtypeidVideoMsg && cs.msgtypeid != msgtypeidAudioMsg {
			if len(cs.msgdata) > 1024 {
				data += " data=" + hex.EncodeToString(cs.msgdata[:1024]) + "... "
			} else {
				data += " data=" + hex.EncodeToString(cs.msgdata) + " "
			}
		} else {
			data += " data= "
		}

		data += fmt.Sprintf("(%d bytes)", len(cs.msgdata))

		fmt.Printf("%s\n", data)
	}

	if cs.msgdataleft == 0 {
		if Debug {
			fmt.Println("rtmp: chunk data")
			fmt.Print(hex.Dump(cs.msgdata))
		}

		timestamp = cs.timenow

		if cs.msgtypeid == msgtypeidVideoMsg || cs.msgtypeid == msgtypeidAudioMsg {
			// TODO: this whole wallclocktime hack was probably based on a
			// that lead to identical timestamps caused by a specific sequence
			// of packets, e.g. type 0 followed by type 3. So this might be
			// removed later.
			if !conn.genwallclocktime {
				if cs.hasprevtimenow && cs.prevtimenow == cs.timenow {
					cs.sametscount++
				} else {
					cs.sametscount = 0
				}

				if cs.hasprevtimenow {
					cs.xtimenow += timestampDelta(cs.prevtimenow, cs.timenow)
				} else {
					cs.xtimenow = int64(cs.timenow)
					cs.hasprevtimenow = true
				}

				cs.prevtimenow = cs.timenow
				cs.msgcount++

				// if the timestamp is the same as the previous one for too often in a row, assume defect timestamps
				if cs.sametscount > 10 {
					if cs.msgcount <= 20 { // only consider the first video and audio messages
						conn.startWallclockTime()
					} else { // otherwise bail out
						err = fmt.Errorf("detected sequence of non-changing timestamps: %d (msgtypeid %d)", cs.timenow, cs.msgtypeid)
						return
					}
				}
			}

			if conn.genwallclocktime {
				cs.xtimenow = conn.wallclockbase + time.Since(conn.wallclockstart).Milliseconds()
			}
		}

		//fmt.Printf(", timestamp = %d", timestamp)
		//fmt.Printf(", actual timestamp = %d\n", cs.xtimenow)

		if err = conn.handleMsg(cs.xtimenow, cs.msgsid, cs.msgtypeid, cs.msgdata); err != nil {
			return fmt.Errorf("handleMsg: %w", err)
		}

		cs.msgdata = nil
	} else {
		//fmt.Printf("\n")
	}

	conn.ackn += uint32(n)
	conn.acks += uint32(n)

	return
}

// startWallclockTime switches all audio and video chunk streams to wall clock timestamps. They
// continue from the latest timestamp of the audio and video chunk streams.
func (conn *Conn) startWallclockTime() {
	conn.genwallclocktime = true
	conn.wallclockstart = time.Now()
	conn.wallclockbase = 0

	for _, cs := range conn.readcsmap {
		if cs.hasprevtimenow && cs.xtimenow > conn.wallclockbase {
			conn.wallclockbase = cs.xtimenow
		}
	}
}

// maxTimestampStepBack is the max. step backward between two consecutive timestamps of a chunk stream that
// is not considered a discontinuity. It is much bigger than any interleaving or jitter of the timestamps.
const maxTimestampStepBack = 0x1000000

// timestampDelta returns the difference between two consecutive timestamps of a chunk stream.
//
// Steps forward are taken as they are, also big ones (e.g. from the sequence headers at 0 to the first
// frame). Small steps backward are kept (e.g. interleaved audio and video). A big step backward is a
// wraparound if continuing over 2^31 (e.g. FFmpeg) or 2^32 leads to a small step forward. Otherwise it's
// a discontinuity (e.g. the peer restarted its timestamps) and the timestamps continue without a gap.
func timestampDelta(prev, now uint32) int64 {
	if now >= prev {
		return int64(now - prev)
	}

	if back := prev - now; back < maxTimestampStepBack {
		return -int64(back)
	}

	wrap := int64(0x100000000)
	if prev < 0x80000000 {
		wrap = 0x80000000
	}

	if d := wrap - int64(prev) + int64(now); d < maxTimestampStepBack {
		return d
	}

	return 0
}

func (conn *Conn) handleCommandMsgAMF0(b []byte) (n int, err error) {
	var name, transid, obj interface{}
	var size int

	if name, size, err = flvio.ParseAMF0Val(b[n:]); err != nil {
		return
	}
	n += size
	// the transaction id and the command object are optional, e.g. FFmpeg sends onFCPublish without them
	if n < len(b) {
		if transid, size, err = flvio.ParseAMF0Val(b[n:]); err != nil {
			return
		}
		n += size
	}
	if n < len(b) {
		if obj, size, err = flvio.ParseAMF0Val(b[n:]); err != nil {
			return
		}
		n += size
	}

	var ok bool
	if conn.commandname, ok = name.(string); !ok {
		err = fmt.Errorf("CommandMsgAMF0 command is not string")
		return
	}
	conn.commandtransid, _ = transid.(float64)
	conn.commandobj, _ = obj.(flvio.AMFMap)
	conn.commandparams = []interface{}{}

	for n < len(b) {
		if obj, size, err = flvio.ParseAMF0Val(b[n:]); err != nil {
			return
		}
		n += size
		conn.commandparams = append(conn.commandparams, obj)
	}
	if n < len(b) {
		err = fmt.Errorf("CommandMsgAMF0 left bytes=%d", len(b)-n)
		return
	}

	conn.gotcommand = true
	return
}

func (conn *Conn) handleMsg(timestamp int64, msgsid uint32, msgtypeid uint8, msgdata []byte) (err error) {
	conn.msgdata = msgdata
	conn.msgtypeid = msgtypeid
	conn.timestamp = timestamp

	switch msgtypeid {
	case msgtypeidCommandMsgAMF0:
		if _, err = conn.handleCommandMsgAMF0(msgdata); err != nil {
			err = fmt.Errorf("AMF0: %w", err)
			return
		}

	case msgtypeidCommandMsgAMF3:
		if len(msgdata) < 1 {
			err = fmt.Errorf("short packet of CommandMsgAMF3")
			return
		}
		// skip first byte
		if _, err = conn.handleCommandMsgAMF0(msgdata[1:]); err != nil {
			err = fmt.Errorf("AMF3: %w", err)
			return
		}

	case msgtypeidUserControl:
		if len(msgdata) < 2 {
			err = fmt.Errorf("short packet of UserControl")
			return
		}
		conn.eventtype = pio.U16BE(msgdata)

		if conn.eventtype == eventtypePingRequest {
			if len(msgdata) != 6 {
				err = fmt.Errorf("wrong length for UserControl.PingRequest")
				return
			}
			pingtimestamp := pio.U32BE(msgdata[2:])
			conn.writePingResponse(pingtimestamp, false)
			conn.flushWrite()
		}

	case msgtypeidDataMsgAMF0:
		b := msgdata
		n := 0
		for n < len(b) {
			obj, size, perr := flvio.ParseAMF0Val(b[n:])
			if perr != nil {
				// a data message (e.g. metadata) that can't be parsed is dropped, it doesn't fail the connection
				if Debug {
					fmt.Printf("rtmp: dropping data message: %s\n", perr.Error())
				}
				conn.datamsgvals = nil
				break
			}
			n += size
			conn.datamsgvals = append(conn.datamsgvals, obj)
		}

		// store metadata

		metaindex := -1

		for i, x := range conn.datamsgvals {
			switch x := x.(type) {
			case string:
				if x == "onMetaData" {
					metaindex = i + 1
				}
			}
		}

		if metaindex != -1 && metaindex < len(conn.datamsgvals) {
			metadata, ok := conn.datamsgvals[metaindex].(flvio.AMFMap)
			if ok {
				conn.metadata = metadata
				conn.prober.PushMetadata(metadata)
			}
		}

	case msgtypeidVideoMsg:
		if len(msgdata) == 0 {
			return
		}
		//fmt.Printf("msgdata: %#08x\n", msgdata[:5])
		tag := flvio.Tag{Type: flvio.TAG_VIDEO}
		var n int
		if n, err = (&tag).ParseHeader(msgdata); err != nil {
			return
		}
		//fmt.Printf("tag: %+v\n", tag)
		// the frame type is to be ignored for metadata (e.g. the color info), other frame types (e.g.
		// command frames) don't contain video data
		isMetadata := tag.IsExHeader && tag.PacketType == flvio.PKTTYPE_METADATA
		if !isMetadata && !(tag.FrameType == flvio.FRAME_INTER || tag.FrameType == flvio.FRAME_KEY) {
			return
		}
		tag.Data = msgdata[n:]
		conn.avtag = tag

	case msgtypeidAudioMsg:
		if len(msgdata) == 0 {
			return
		}
		tag := flvio.Tag{Type: flvio.TAG_AUDIO}
		var n int
		if n, err = (&tag).ParseHeader(msgdata); err != nil {
			return
		}
		tag.Data = msgdata[n:]
		conn.avtag = tag

	case msgtypeidSetChunkSize:
		if len(msgdata) < 4 {
			err = fmt.Errorf("short packet of SetChunkSize")
			return
		}
		size := pio.U32BE(msgdata)
		// the first bit must be 0 and the size must be at least 1
		if size == 0 || size&0x80000000 != 0 {
			err = fmt.Errorf("invalid SetChunkSize: %d", size)
			return
		}
		// a chunk can't be bigger than the max. message length
		if size > maxMessageLength {
			size = maxMessageLength
		}
		conn.readMaxChunkSize = int(size)
	case msgtypeidWindowAckSize:
		if len(conn.msgdata) != 4 {
			return fmt.Errorf("invalid packet of WindowAckSize")
		}
		conn.readAckSize = pio.U32BE(conn.msgdata) >> 1
	case msgtypeidAbort:
		if len(msgdata) != 4 {
			return fmt.Errorf("invalid packet of Abort")
		}
		// discard the partially received message of the chunk stream
		if cs := conn.readcsmap[pio.U32BE(msgdata)]; cs != nil {
			cs.msgdataleft = 0
			cs.msgdata = nil
		}
	case msgtypeidSetPeerBandwidth:
		if len(msgdata) != 5 {
			return fmt.Errorf("invalid packet of SetPeerBandwidth")
		}
		if err = conn.handleSetPeerBandwidth(pio.U32BE(msgdata), msgdata[4]); err != nil {
			return fmt.Errorf("SetPeerBandwidth: %w", err)
		}
	default:
		if Debug {
			fmt.Printf("rtmp: unhandled msg: %d\n", msgtypeid)
		}
	}

	conn.gotmsg = true
	return
}

// handleSetPeerBandwidth applies the window size from a SetPeerBandwidth message according to the
// limit type and sends a WindowAckSize message to the peer if the window size changed.
func (conn *Conn) handleSetPeerBandwidth(size uint32, limit uint8) error {
	window := conn.writeAckSize

	switch limit {
	case peerBWLimitHard:
		window = size
	case peerBWLimitSoft:
		if window == 0 || size < window {
			window = size
		}
	case peerBWLimitDynamic:
		// treated as hard if the previous limit was hard, otherwise ignored
		if !conn.peerBWLimitSet || conn.peerBWLimit != peerBWLimitHard {
			return nil
		}
		window = size
		limit = peerBWLimitHard
	default:
		return fmt.Errorf("invalid limit type %d", limit)
	}

	conn.peerBWLimit = limit
	conn.peerBWLimitSet = true

	if window == conn.writeAckSize {
		return nil
	}

	if err := conn.writeWindowAckSize(window, false); err != nil {
		return err
	}

	return conn.flushWrite()
}

var (
	hsClientFullKey = []byte{
		'G', 'e', 'n', 'u', 'i', 'n', 'e', ' ', 'A', 'd', 'o', 'b', 'e', ' ',
		'F', 'l', 'a', 's', 'h', ' ', 'P', 'l', 'a', 'y', 'e', 'r', ' ',
		'0', '0', '1',
		0xF0, 0xEE, 0xC2, 0x4A, 0x80, 0x68, 0xBE, 0xE8, 0x2E, 0x00, 0xD0, 0xD1,
		0x02, 0x9E, 0x7E, 0x57, 0x6E, 0xEC, 0x5D, 0x2D, 0x29, 0x80, 0x6F, 0xAB,
		0x93, 0xB8, 0xE6, 0x36, 0xCF, 0xEB, 0x31, 0xAE,
	}
	hsServerFullKey = []byte{
		'G', 'e', 'n', 'u', 'i', 'n', 'e', ' ', 'A', 'd', 'o', 'b', 'e', ' ',
		'F', 'l', 'a', 's', 'h', ' ', 'M', 'e', 'd', 'i', 'a', ' ',
		'S', 'e', 'r', 'v', 'e', 'r', ' ',
		'0', '0', '1',
		0xF0, 0xEE, 0xC2, 0x4A, 0x80, 0x68, 0xBE, 0xE8, 0x2E, 0x00, 0xD0, 0xD1,
		0x02, 0x9E, 0x7E, 0x57, 0x6E, 0xEC, 0x5D, 0x2D, 0x29, 0x80, 0x6F, 0xAB,
		0x93, 0xB8, 0xE6, 0x36, 0xCF, 0xEB, 0x31, 0xAE,
	}
	hsClientPartialKey = hsClientFullKey[:30]
	hsServerPartialKey = hsServerFullKey[:36]
)

func hsMakeDigest(key []byte, src []byte, gap int) (dst []byte) {
	h := hmac.New(sha256.New, key)
	if gap <= 0 {
		h.Write(src)
	} else {
		h.Write(src[:gap])
		h.Write(src[gap+32:])
	}
	return h.Sum(nil)
}

func hsCalcDigestPos(p []byte, base int) (pos int) {
	for i := 0; i < 4; i++ {
		pos += int(p[base+i])
	}
	pos = (pos % 728) + base + 4
	return
}

func hsFindDigest(p []byte, key []byte, base int) int {
	gap := hsCalcDigestPos(p, base)
	digest := hsMakeDigest(key, p, gap)
	if !bytes.Equal(p[gap:gap+32], digest) {
		return -1
	}
	return gap
}

func hsParse1(p []byte, peerkey []byte, key []byte) (ok bool, digest []byte) {
	var pos int
	if pos = hsFindDigest(p, peerkey, 772); pos == -1 {
		if pos = hsFindDigest(p, peerkey, 8); pos == -1 {
			return
		}
	}
	ok = true
	digest = hsMakeDigest(key, p[pos:pos+32], -1)
	return
}

func hsCreate01(p []byte, time uint32, ver uint32, key []byte) {
	p[0] = 3
	p1 := p[1:]
	rand.Read(p1[8:])
	pio.PutU32BE(p1[0:4], time)
	pio.PutU32BE(p1[4:8], ver)
	gap := hsCalcDigestPos(p1, 8)
	digest := hsMakeDigest(key, p1, gap)
	copy(p1[gap:], digest)
}

func hsCreate2(p []byte, key []byte) {
	rand.Read(p)
	gap := len(p) - 32
	digest := hsMakeDigest(key, p, gap)
	copy(p[gap:], digest)
}

func (conn *Conn) handshakeClient() (err error) {
	var random [(1 + 1536*2) * 2]byte

	C0C1C2 := random[:1536*2+1]
	C0 := C0C1C2[:1]
	C1 := C0C1C2[1 : 1536+1]
	C0C1 := C0C1C2[:1536+1]

	S0S1S2 := random[1536*2+1:]
	S0 := S0S1S2[:1]
	S1 := S0S1S2[1 : 1536+1]

	// C0: version, C1: time, zero, random bytes (simple handshake)
	C0[0] = 3
	pio.PutU32BE(C1[0:4], uint32(time.Since(conn.start).Milliseconds()))
	pio.PutU32BE(C1[4:8], 0)
	if _, err = rand.Read(C1[8:]); err != nil {
		return
	}

	// > C0C1
	if _, err = conn.bufw.Write(C0C1); err != nil {
		return
	}
	if err = conn.bufw.Flush(); err != nil {
		return
	}

	// < S0S1S2
	if _, err = io.ReadFull(conn.bufr, S0S1S2); err != nil {
		return
	}
	conn.acks += uint32(len(S0S1S2))

	if S0[0] != 3 {
		err = fmt.Errorf("handshake version=%d invalid", S0[0])
		return
	}

	if Debug {
		fmt.Println("handshakeClient: server version", S1[4], S1[5], S1[6], S1[7])
	}

	// > C2: echo of S1
	if _, err = conn.bufw.Write(S1); err != nil {
		return
	}

	conn.stage++
	return
}

func (conn *Conn) handshakeServer() (err error) {
	var random [(1 + 1536*2) * 2]byte

	C0C1C2 := random[:1536*2+1]
	C0 := C0C1C2[:1]
	C1 := C0C1C2[1 : 1536+1]
	C0C1 := C0C1C2[:1536+1]
	C2 := C0C1C2[1536+1:]

	S0S1S2 := random[1536*2+1:]
	S0 := S0S1S2[:1]
	S1 := S0S1S2[1 : 1536+1]
	S0S1 := S0S1S2[:1536+1]
	S2 := S0S1S2[1536+1:]

	// < C0C1
	if _, err = io.ReadFull(conn.bufr, C0C1); err != nil {
		return
	}
	conn.acks += uint32(len(C0C1))
	if C0[0] != 3 {
		err = fmt.Errorf("handshake version=%d invalid", C0[0])
		return
	}

	S0[0] = 3

	clitime := pio.U32BE(C1[0:4])
	srvtime := clitime
	srvver := uint32(0x0d0e0a0d)
	cliver := pio.U32BE(C1[4:8])

	if cliver != 0 {
		var ok bool
		var digest []byte
		if ok, digest = hsParse1(C1, hsClientPartialKey, hsServerFullKey); !ok {
			err = fmt.Errorf("handshake server: C1 invalid")
			return
		}
		hsCreate01(S0S1, srvtime, srvver, hsServerPartialKey)
		hsCreate2(S2, digest)
	} else {
		// S1: time, zero, random bytes
		now := uint32(time.Since(conn.start).Milliseconds())
		pio.PutU32BE(S1[0:4], now)
		pio.PutU32BE(S1[4:8], 0)
		if _, err = rand.Read(S1[8:]); err != nil {
			return
		}

		// S2: echo of C1. The spec wants the time C1 has been read in time2, but librtmp
		// based clients (e.g. OBS) expect an exact copy of C1 and log a warning otherwise.
		copy(S2, C1)
	}

	// > S0S1S2
	if _, err = conn.bufw.Write(S0S1S2); err != nil {
		return
	}
	if err = conn.bufw.Flush(); err != nil {
		return
	}

	// < C2
	if _, err = io.ReadFull(conn.bufr, C2); err != nil {
		return
	}
	conn.acks += uint32(len(C2))

	conn.stage++
	return
}

// closeConn is the connection returned by serveOne. Closing it closes the connection and ends
// its handler.
type closeConn struct {
	*Conn
	waitclose chan struct{}
	closeOnce sync.Once
}

func (cc *closeConn) Close() (err error) {
	cc.closeOnce.Do(func() {
		err = cc.Conn.Close()
		close(cc.waitclose)
	})
	return
}

// serveOne serves on the listener until the first client plays (play is true) or publishes, and
// returns that client. The listener is closed after the first client and further clients are
// rejected. An error is returned if serving fails before.
func serveOne(listener net.Listener, play bool) (*closeConn, error) {
	var once sync.Once
	waitconn := make(chan *closeConn, 1)
	waitserve := make(chan error, 1)

	handler := func(conn *Conn) {
		cc := &closeConn{Conn: conn, waitclose: make(chan struct{})}
		accepted := false
		once.Do(func() {
			accepted = true
			waitconn <- cc
			listener.Close()
		})
		if !accepted {
			return
		}
		<-cc.waitclose
	}

	server := &Server{}
	if play {
		server.HandlePlay = handler
	} else {
		server.HandlePublish = handler
	}

	go func() {
		waitserve <- server.Serve(listener)
	}()

	select {
	case cc := <-waitconn:
		return cc, nil
	case err := <-waitserve:
		// reject clients that are still connecting, unless one has been accepted just now
		once.Do(func() {})
		select {
		case cc := <-waitconn:
			return cc, nil
		default:
		}
		server.Close()
		return nil, err
	}
}

func Handler(h *avutil.RegisterHandler) {
	h.UrlDemuxer = func(uri string) (ok bool, demuxer av.DemuxCloser, err error) {
		if !strings.HasPrefix(uri, "rtmp://") && !strings.HasPrefix(uri, "rtmps://") {
			return
		}
		ok = true
		demuxer, err = Dial(uri, DialOptions{})
		return
	}

	h.UrlMuxer = func(uri string) (ok bool, muxer av.MuxCloser, err error) {
		if !strings.HasPrefix(uri, "rtmp://") && !strings.HasPrefix(uri, "rtmps://") {
			return
		}
		ok = true
		muxer, err = Dial(uri, DialOptions{})
		return
	}

	listen := func(uri string) (listener net.Listener, err error) {
		var u *url.URL
		if u, err = ParseURL(uri); err != nil {
			return
		}
		return net.Listen("tcp", u.Host)
	}

	h.ServerMuxer = func(uri string) (ok bool, muxer av.MuxCloser, err error) {
		if !strings.HasPrefix(uri, "rtmp://") {
			return
		}
		ok = true

		var listener net.Listener
		if listener, err = listen(uri); err != nil {
			return
		}

		var conn *closeConn
		if conn, err = serveOne(listener, true); err != nil {
			return
		}
		muxer = conn
		return
	}

	h.ServerDemuxer = func(uri string) (ok bool, demuxer av.DemuxCloser, err error) {
		if !strings.HasPrefix(uri, "rtmp://") {
			return
		}
		ok = true

		var listener net.Listener
		if listener, err = listen(uri); err != nil {
			return
		}

		var conn *closeConn
		if conn, err = serveOne(listener, false); err != nil {
			return
		}
		demuxer = conn
		return
	}

	h.CodecTypes = CodecTypes
}
