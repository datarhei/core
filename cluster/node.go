package cluster

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/datarhei/core/v16/cluster/client"
	"github.com/datarhei/core/v16/cluster/proxy"
	"github.com/datarhei/core/v16/config"
	"github.com/datarhei/core/v16/config/copy"
)

type clusterNode struct {
	client client.APIClient

	id                 string
	address            string
	version            string
	lastContact        time.Time
	lastContactErr     error
	lastCoreContact    time.Time
	lastCoreContactErr error
	latency            time.Duration
	pingLock           sync.RWMutex

	runLock    sync.Mutex
	cancelPing context.CancelFunc

	proxyNode proxy.Node
}

func NewClusterNode(id, address string) *clusterNode {
	n := &clusterNode{
		id:      id,
		address: address,
		version: "0.0.0",
		client: client.APIClient{
			Address: address,
			Client: &http.Client{
				Timeout: 5 * time.Second,
			},
		},
	}

	version, err := n.client.Version()
	if err != nil {
		version = "0.0.0"
	}

	n.version = version

	n.start(id)

	return n
}

func (n *clusterNode) start(id string) error {
	n.runLock.Lock()
	defer n.runLock.Unlock()

	if n.cancelPing != nil {
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	n.cancelPing = cancel

	go n.ping(ctx)
	go n.pingCore(ctx)

	n.lastCoreContactErr = fmt.Errorf("not started yet")
	n.lastContactErr = fmt.Errorf("not started yet")

	address, err := n.CoreAPIAddress()
	n.proxyNode = proxy.NewNode(id, address)

	if err != nil {
		n.lastCoreContactErr = err
		go func(ctx context.Context) {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					address, err := n.CoreAPIAddress()
					n.pingLock.Lock()
					if err == nil {
						n.proxyNode.SetAddress(address)
						n.lastCoreContactErr = nil
					} else {
						n.lastCoreContactErr = err
					}
					n.pingLock.Unlock()

					if err == nil {
						return
					}
				case <-ctx.Done():
					return
				}
			}
		}(ctx)
	}

	return nil
}

func (n *clusterNode) Stop() error {
	n.runLock.Lock()
	defer n.runLock.Unlock()

	if n.cancelPing == nil {
		return nil
	}

	n.proxyNode.Disconnect()

	n.cancelPing()
	n.cancelPing = nil

	return nil
}

func (n *clusterNode) Version() string {
	n.pingLock.RLock()
	defer n.pingLock.RUnlock()

	return n.version
}

func (n *clusterNode) Status() (string, error) {
	n.pingLock.RLock()
	defer n.pingLock.RUnlock()

	since := time.Since(n.lastContact)
	if since > 5*time.Second {
		return "offline", fmt.Errorf("the cluster API didn't respond for %s because: %w", since, n.lastContactErr)
	}

	since = time.Since(n.lastCoreContact)
	if since > 5*time.Second {
		return "offline", fmt.Errorf("the core API didn't respond for %s because: %w", since, n.lastCoreContactErr)
	}

	return "online", nil
}

func (n *clusterNode) LastContact() time.Time {
	n.pingLock.RLock()
	defer n.pingLock.RUnlock()

	return n.lastContact
}

func (n *clusterNode) Config() (*config.Config, error) {
	if n.proxyNode == nil {
		return nil, fmt.Errorf("proxy not available")
	}

	apiconfig := n.proxyNode.Config()
	if apiconfig == nil {
		return nil, fmt.Errorf("no config stored")
	}

	config := config.New(nil)

	config.Version = apiconfig.Version
	config.Version = apiconfig.Version
	config.Version = apiconfig.Version
	config.ID = apiconfig.ID
	config.Name = apiconfig.Name
	config.Address = apiconfig.Address
	config.CheckForUpdates = apiconfig.CheckForUpdates

	config.Log = apiconfig.Log
	config.DB = apiconfig.DB
	config.Host = apiconfig.Host
	config.API.ReadOnly = apiconfig.API.ReadOnly
	config.API.Access = apiconfig.API.Access
	config.API.Auth.Enable = apiconfig.API.Auth.Enable
	config.API.Auth.DisableLocalhost = apiconfig.API.Auth.DisableLocalhost
	config.API.Auth.Username = apiconfig.API.Auth.Username
	config.API.Auth.Password = apiconfig.API.Auth.Password
	config.API.Auth.JWT = apiconfig.API.Auth.JWT
	config.TLS = apiconfig.TLS
	config.Storage.MimeTypes = apiconfig.Storage.MimeTypes
	config.Storage.Disk = apiconfig.Storage.Disk
	config.Storage.Memory = apiconfig.Storage.Memory
	config.Storage.CORS = apiconfig.Storage.CORS
	config.RTMP = apiconfig.RTMP
	config.SRT = apiconfig.SRT
	config.FFmpeg = apiconfig.FFmpeg
	config.Playout = apiconfig.Playout
	config.Debug = apiconfig.Debug
	config.Metrics = apiconfig.Metrics
	config.Sessions = apiconfig.Sessions
	config.Service = apiconfig.Service
	config.Router = apiconfig.Router
	config.Resources = apiconfig.Resources
	config.Cluster = apiconfig.Cluster

	config.Log.Topics = copy.Slice(apiconfig.Log.Topics)

	config.Host.Name = copy.Slice(apiconfig.Host.Name)

	config.API.Access.HTTP.Allow = copy.Slice(apiconfig.API.Access.HTTP.Allow)
	config.API.Access.HTTP.Block = copy.Slice(apiconfig.API.Access.HTTP.Block)
	config.API.Access.HTTPS.Allow = copy.Slice(apiconfig.API.Access.HTTPS.Allow)
	config.API.Access.HTTPS.Block = copy.Slice(apiconfig.API.Access.HTTPS.Block)

	//config.API.Auth.Auth0.Tenants = copy.TenantSlice(d.API.Auth.Auth0.Tenants)

	config.Storage.CORS.Origins = copy.Slice(apiconfig.Storage.CORS.Origins)
	config.Storage.Disk.Cache.Types.Allow = copy.Slice(apiconfig.Storage.Disk.Cache.Types.Allow)
	config.Storage.Disk.Cache.Types.Block = copy.Slice(apiconfig.Storage.Disk.Cache.Types.Block)
	//config.Storage.S3 = copy.Slice(d.Storage.S3)

	config.FFmpeg.Access.Input.Allow = copy.Slice(apiconfig.FFmpeg.Access.Input.Allow)
	config.FFmpeg.Access.Input.Block = copy.Slice(apiconfig.FFmpeg.Access.Input.Block)
	config.FFmpeg.Access.Output.Allow = copy.Slice(apiconfig.FFmpeg.Access.Output.Allow)
	config.FFmpeg.Access.Output.Block = copy.Slice(apiconfig.FFmpeg.Access.Output.Block)

	config.Sessions.IPIgnoreList = copy.Slice(apiconfig.Sessions.IPIgnoreList)

	config.SRT.Log.Topics = copy.Slice(apiconfig.SRT.Log.Topics)

	config.Router.BlockedPrefixes = copy.Slice(apiconfig.Router.BlockedPrefixes)
	config.Router.Routes = copy.StringMap(apiconfig.Router.Routes)

	config.Cluster.Peers = copy.Slice(apiconfig.Cluster.Peers)

	return config, nil
}

func (n *clusterNode) CoreAPIAddress() (string, error) {
	return n.client.CoreAPIAddress()
}

func (n *clusterNode) Proxy() proxy.Node {
	return n.proxyNode
}

func (n *clusterNode) ping(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			start := time.Now()
			version, err := n.client.Version()
			if err == nil {
				n.pingLock.Lock()
				n.version = version
				n.lastContact = time.Now()
				n.latency = time.Since(start)
				n.pingLock.Unlock()
			} else {
				n.pingLock.Lock()
				n.lastContactErr = err
				n.pingLock.Unlock()
			}
		case <-ctx.Done():
			return
		}
	}
}

func (n *clusterNode) pingCore(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			_, err := n.proxyNode.IsConnected()

			if err == nil {
				n.pingLock.Lock()
				n.lastCoreContact = time.Now()
				n.lastCoreContactErr = nil
				n.pingLock.Unlock()
			} else {
				n.pingLock.Lock()
				n.lastCoreContactErr = fmt.Errorf("not connected to core api: %w", err)
				n.pingLock.Unlock()
			}
		case <-ctx.Done():
			return
		}
	}
}
