package node

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/datarhei/core/v16/config"
	"github.com/datarhei/core/v16/log"

	client "github.com/datarhei/core-client-go/v16"
)

type Core struct {
	id string

	client    client.RestClient
	clientErr error

	lock sync.RWMutex

	cancel context.CancelFunc

	address string
	config  *config.Config

	secure      bool
	httpAddress *url.URL
	hasRTMP     bool
	rtmpAddress *url.URL
	hasSRT      bool
	srtAddress  *url.URL

	logger log.Logger
}

var ErrNoPeer = errors.New("not connected to the core API: client not available")

func NewCore(id string, logger log.Logger) *Core {
	core := &Core{
		id:     id,
		logger: logger,
	}

	if core.logger == nil {
		core.logger = log.New("")
	}

	ctx, cancel := context.WithCancel(context.Background())
	core.cancel = cancel

	go core.reconnect(ctx)

	return core
}

func (n *Core) SetEssentials(address string, config *config.Config) {
	n.lock.Lock()
	defer n.lock.Unlock()

	if address != n.address {
		n.address = address
		n.client = nil // force reconnet
	}

	if n.config == nil && config != nil {
		n.config = config
		n.client = nil // force reconnect
	}

	if n.config.UpdatedAt != config.UpdatedAt {
		n.config = config
		n.client = nil // force reconnect
	}
}

func (n *Core) reconnect(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := n.connect()

			n.lock.Lock()
			n.clientErr = err
			n.lock.Unlock()
		}
	}
}

func (n *Core) Stop() {
	n.lock.Lock()
	defer n.lock.Unlock()

	if n.cancel == nil {
		return
	}

	n.cancel()
	n.cancel = nil
}

func (n *Core) Reconnect() {
	n.lock.Lock()
	defer n.lock.Unlock()

	n.client = nil
}

func (n *Core) IsConnected() (bool, error) {
	n.lock.RLock()
	defer n.lock.RUnlock()

	if n.client == nil {
		return false, ErrNoPeer
	}

	if n.clientErr != nil {
		return false, n.clientErr
	}

	return true, nil
}

func (n *Core) connect() error {
	n.lock.Lock()

	if n.client != nil {
		n.lock.Unlock()
		return nil
	}

	if len(n.address) == 0 {
		n.lock.Unlock()
		return fmt.Errorf("no address provided")
	}

	if n.config == nil {
		n.lock.Unlock()
		return fmt.Errorf("config not available")
	}

	address := n.address
	config := n.config.Clone()

	n.lock.Unlock()

	u, err := url.Parse(address)
	if err != nil {
		return fmt.Errorf("invalid address (%s): %w", address, err)
	}

	secure := strings.HasPrefix(config.Address, "https://")

	nodehost, _, err := net.SplitHostPort(u.Host)
	if err != nil {
		return fmt.Errorf("invalid address (%s): %w", u.Host, err)
	}

	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.MaxIdleConns = 10
	tr.IdleConnTimeout = 30 * time.Second

	client, err := client.New(client.Config{
		Address: u.String(),
		Client: &http.Client{
			Transport: tr,
			Timeout:   5 * time.Second,
		},
	})
	if err != nil {
		return fmt.Errorf("creating client failed (%s): %w", address, err)
	}

	httpAddress := u
	hasRTMP := false
	rtmpAddress := &url.URL{}

	if config.RTMP.Enable {
		hasRTMP = true
		rtmpAddress.Scheme = "rtmp"

		isHostIP := net.ParseIP(nodehost) != nil

		address := config.RTMP.Address
		if secure && config.RTMP.EnableTLS && !isHostIP {
			address = config.RTMP.AddressTLS
			rtmpAddress.Scheme = "rtmps"
		}

		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return fmt.Errorf("invalid rtmp address '%s': %w", address, err)
		}

		if len(host) == 0 {
			rtmpAddress.Host = net.JoinHostPort(nodehost, port)
		} else {
			rtmpAddress.Host = net.JoinHostPort(host, port)
		}

		rtmpAddress = n.rtmpAddress.JoinPath(n.config.RTMP.App)
	}

	hasSRT := false
	srtAddress := &url.URL{}

	if config.SRT.Enable {
		hasSRT = true
		srtAddress.Scheme = "srt"

		host, port, err := net.SplitHostPort(config.SRT.Address)
		if err != nil {
			return fmt.Errorf("invalid srt address '%s': %w", config.SRT.Address, err)
		}

		if len(host) == 0 {
			srtAddress.Host = net.JoinHostPort(nodehost, port)
		} else {
			srtAddress.Host = net.JoinHostPort(host, port)
		}

		v := url.Values{}

		v.Set("mode", "caller")
		if len(config.SRT.Passphrase) != 0 {
			v.Set("passphrase", config.SRT.Passphrase)
		}

		srtAddress.RawQuery = v.Encode()
	}

	n.lock.Lock()
	defer n.lock.Unlock()

	n.secure = secure
	n.httpAddress = httpAddress
	n.hasRTMP = hasRTMP
	n.rtmpAddress = rtmpAddress
	n.hasSRT = hasSRT
	n.srtAddress = srtAddress
	n.client = client

	return nil
}

func (n *Core) About() (CoreAbout, error) {
	n.lock.RLock()
	defer n.lock.RUnlock()

	if n.client == nil {
		return CoreAbout{}, ErrNoPeer
	}

	about, err := n.client.About(false)

	cabout := CoreAbout{
		ID:      about.ID,
		Name:    about.Name,
		Address: n.address,
		Error:   n.clientErr,
		Version: CoreVersion{
			Number:   about.Version.Number,
			Commit:   about.Version.Commit,
			Branch:   about.Version.Branch,
			Arch:     about.Version.Arch,
			Compiler: about.Version.Compiler,
		},
	}

	createdAt, err := time.Parse(time.RFC3339, about.CreatedAt)
	if err != nil {
		createdAt = time.Now()
	}

	cabout.CreatedAt = createdAt
	cabout.Uptime = time.Since(createdAt)

	build, err := time.Parse(time.RFC3339, about.Version.Build)
	if err != nil {
		build = time.Time{}
	}

	cabout.Version.Build = build

	return cabout, err
}

type CoreAbout struct {
	ID          string
	Name        string
	Address     string
	Status      string
	Error       error
	CreatedAt   time.Time
	Uptime      time.Duration
	LastContact time.Duration
	Latency     time.Duration
	Version     CoreVersion
}

type CoreVersion struct {
	Number   string
	Commit   string
	Branch   string
	Build    time.Time
	Arch     string
	Compiler string
}
