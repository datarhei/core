package node

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	client "github.com/datarhei/core-client-go/v16"
)

type Core struct {
	client     client.RestClient
	clientLock sync.RWMutex
	clientErr  error

	address string

	secure      bool
	httpAddress *url.URL
	hasRTMP     bool
	rtmpAddress *url.URL
	hasSRT      bool
	srtAddress  *url.URL
}

var ErrNoPeer = errors.New("not connected to the core API: client not available")

func NewCore(address string) *Core {
	core := &Core{
		address: address,
	}

	return core
}

func (n *Core) Reconnect() {
	n.clientLock.Lock()
	defer n.clientLock.Unlock()

	n.client = nil
}

func (n *Core) IsConnected() (bool, error) {
	n.clientLock.RLock()
	defer n.clientLock.RUnlock()

	if n.client == nil {
		return false, ErrNoPeer
	}

	if n.clientErr != nil {
		return false, n.clientErr
	}

	return true, nil
}

func (n *Core) connect() error {
	n.clientLock.Lock()
	defer n.clientLock.Unlock()

	if n.client != nil && n.state != stateDisconnected {
		return nil
	}

	if len(n.address) == 0 {
		return fmt.Errorf("no address provided")
	}

	if n.config == nil {
		return fmt.Errorf("config not available")
	}

	u, err := url.Parse(n.address)
	if err != nil {
		return fmt.Errorf("invalid address (%s): %w", n.address, err)
	}

	nodehost, _, err := net.SplitHostPort(u.Host)
	if err != nil {
		return fmt.Errorf("invalid address (%s): %w", u.Host, err)
	}

	peer, err := client.New(client.Config{
		Address: u.String(),
		Client: &http.Client{
			Timeout: 5 * time.Second,
		},
	})
	if err != nil {
		return fmt.Errorf("creating client failed (%s): %w", n.address, err)
	}

	n.httpAddress = u

	if n.config.RTMP.Enable {
		n.hasRTMP = true
		n.rtmpAddress = &url.URL{}
		n.rtmpAddress.Scheme = "rtmp"

		isHostIP := net.ParseIP(nodehost) != nil

		address := n.config.RTMP.Address
		if n.secure && n.config.RTMP.EnableTLS && !isHostIP {
			address = n.config.RTMP.AddressTLS
			n.rtmpAddress.Scheme = "rtmps"
		}

		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return fmt.Errorf("invalid rtmp address '%s': %w", address, err)
		}

		if len(host) == 0 {
			n.rtmpAddress.Host = net.JoinHostPort(nodehost, port)
		} else {
			n.rtmpAddress.Host = net.JoinHostPort(host, port)
		}

		n.rtmpAddress = n.rtmpAddress.JoinPath(n.config.RTMP.App)
	}

	if n.config.SRT.Enable {
		n.hasSRT = true
		n.srtAddress = &url.URL{}
		n.srtAddress.Scheme = "srt"

		host, port, err := net.SplitHostPort(n.config.SRT.Address)
		if err != nil {
			return fmt.Errorf("invalid srt address '%s': %w", n.config.SRT.Address, err)
		}

		if len(host) == 0 {
			n.srtAddress.Host = net.JoinHostPort(nodehost, port)
		} else {
			n.srtAddress.Host = net.JoinHostPort(host, port)
		}

		v := url.Values{}

		v.Set("mode", "caller")
		if len(n.config.SRT.Passphrase) != 0 {
			v.Set("passphrase", n.config.SRT.Passphrase)
		}

		n.srtAddress.RawQuery = v.Encode()
	}

	n.peer = peer

	return nil
}
