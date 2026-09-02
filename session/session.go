package session

import (
	"context"
	"sync"
	"time"

	"github.com/datarhei/core/v16/log"
	"github.com/datarhei/core/v16/math/average"
)

type session struct {
	id        string
	reference string
	createdAt time.Time
	closedAt  time.Time

	logger log.Logger

	active bool

	location string
	peer     string
	extra    *SessionData
	userdata *SessionData

	stale    *time.Timer
	timeout  time.Duration
	callback func(*session)

	rxBitrate *average.SMA
	rxBytes   uint64

	txBitrate *average.SMA
	txBytes   uint64

	tickerStop context.CancelFunc
	running    bool

	lock         sync.Mutex
	topRxBitrate float64
	topTxBitrate float64
	maxRxBitrate float64
	maxTxBitrate float64
}

func (s *session) Init(id, reference string, closeCallback func(*session), inactive, timeout time.Duration, logger log.Logger) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.id = id
	s.reference = reference
	s.createdAt = time.Now()

	s.logger = logger

	s.active = false

	s.location = ""
	s.peer = ""
	s.extra = &SessionData{}
	s.userdata = &SessionData{}

	s.rxBitrate, _ = average.NewSMA(averageWindow, averageGranularity)
	s.txBitrate, _ = average.NewSMA(averageWindow, averageGranularity)

	s.topRxBitrate = 0.0
	s.topTxBitrate = 0.0

	s.maxRxBitrate = 0.0
	s.maxTxBitrate = 0.0

	s.timeout = timeout
	s.callback = closeCallback
	if s.callback == nil {
		s.callback = func(s *session) {}
	}

	s.running = true

	pendingTimeout := inactive
	if timeout < pendingTimeout {
		pendingTimeout = timeout
	}

	s.stale = time.AfterFunc(pendingTimeout, func() {
		s.close()
	})
}

func (s *session) close() {
	s.lock.Lock()
	defer s.lock.Unlock()

	if !s.running {
		return
	}

	s.running = false

	s.stale.Stop()

	s.closedAt = time.Now()

	if s.tickerStop != nil {
		s.tickerStop()
		s.tickerStop = nil
	}

	s.rxBitrate.Reset()
	s.txBitrate.Reset()
	go s.callback(s)
}

func (s *session) Register(location, peer string) {
	if len(location) == 0 {
		location = "unknown"
	}

	if len(peer) == 0 {
		peer = "unknown"
	}

	s.location = location
	s.peer = peer
}

func (s *session) Activate() bool {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.active {
		return false
	}

	s.active = true
	ctx, cancel := context.WithCancel(context.Background())
	s.tickerStop = cancel

	go s.ticker(ctx)

	return true
}

func (s *session) Ingress(size int64) bool {
	if size == 0 {
		return false
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	s.stale.Stop()
	s.stale.Reset(s.timeout)

	s.rxBitrate.Add(float64(size) * 8)
	s.rxBytes += uint64(size)

	bitrate := s.rxBitrate.Average()
	if bitrate > s.topRxBitrate {
		s.topRxBitrate = bitrate
	}

	if bitrate > s.maxRxBitrate {
		s.maxRxBitrate = bitrate
	}

	return true
}

func (s *session) Egress(size int64) bool {
	if size == 0 {
		return false
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	s.stale.Stop()
	s.stale.Reset(s.timeout)

	s.txBitrate.Add(float64(size) * 8)
	s.txBytes += uint64(size)

	bitrate := s.txBitrate.Average()
	if bitrate > s.topTxBitrate {
		s.topTxBitrate = bitrate
	}

	if bitrate > s.maxTxBitrate {
		s.maxTxBitrate = bitrate
	}

	return true
}

func (s *session) RxBitrate() float64 {
	return s.rxBitrate.Average()
}

func (s *session) TxBitrate() float64 {
	return s.txBitrate.Average()
}

func (s *session) TopRxBitrate() float64 {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.topRxBitrate
}

func (s *session) MaxTxBitrate() float64 {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.maxTxBitrate
}

func (s *session) MaxRxBitrate() float64 {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.maxRxBitrate
}

func (s *session) TopTxBitrate() float64 {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.topTxBitrate
}

func (s *session) SetTopRxBitrate(bitrate float64) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.topRxBitrate = bitrate
}

func (s *session) SetTopTxBitrate(bitrate float64) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.topTxBitrate = bitrate
}

func (s *session) ticker(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.lock.Lock()
			s.topRxBitrate *= 0.95
			s.topTxBitrate *= 0.95
			s.lock.Unlock()
		}
	}
}

func (s *session) Cancel() {
	s.close()
}

func (s *session) Extra() *SessionData {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.extra
}

func (s *session) UserData() *SessionData {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.userdata
}

type SessionData struct {
	data map[string]any

	lock sync.Mutex
}

func (se *SessionData) Set(key string, value any) {
	se.lock.Lock()
	defer se.lock.Unlock()

	if len(se.data) == 0 {
		se.data = map[string]any{}
	}

	se.data[key] = value
}

func (se *SessionData) SetAll(data map[string]any) {
	se.lock.Lock()
	defer se.lock.Unlock()

	if len(se.data) == 0 {
		se.data = map[string]any{}
	}

	for k, v := range data {
		se.data[k] = v
	}
}

func (se *SessionData) Get(key string) (any, bool) {
	se.lock.Lock()
	defer se.lock.Unlock()

	if len(se.data) == 0 {
		return nil, false
	}

	value, ok := se.data[key]

	return value, ok
}

func (se *SessionData) GetAll() map[string]any {
	se.lock.Lock()
	defer se.lock.Unlock()

	data := map[string]any{}

	if len(se.data) == 0 {
		return data
	}

	for k, v := range se.data {
		data[k] = v
	}

	return data
}

func (se *SessionData) Delete(key string) {
	se.lock.Lock()
	defer se.lock.Unlock()

	if len(se.data) == 0 {
		return
	}

	delete(se.data, key)
}
