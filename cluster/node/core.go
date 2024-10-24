package node

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/datarhei/core/v16/config"
	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/http/client"
	"github.com/datarhei/core/v16/log"
	"github.com/datarhei/core/v16/restream/app"
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

	go core.reconnect(ctx, time.Second)

	return core
}

func (n *Core) SetEssentials(address string, config *config.Config) {
	n.lock.Lock()
	defer n.lock.Unlock()

	if n.address != address {
		n.address = address
		n.client = nil // force reconnet
	}

	if config != nil {
		if n.config == nil {
			n.config = config
			n.client = nil // force reconnect
		}

		if n.config != nil && n.config.UpdatedAt != config.UpdatedAt {
			n.config = config
			n.client = nil // force reconnect
		}
	}
}

func (n *Core) reconnect(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
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
			Timeout:   0,
		},
		Timeout: 5 * time.Second,
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

		rtmpAddress = rtmpAddress.JoinPath(n.config.RTMP.App)
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

	n.secure = secure
	n.httpAddress = httpAddress
	n.hasRTMP = hasRTMP
	n.rtmpAddress = rtmpAddress
	n.hasSRT = hasSRT
	n.srtAddress = srtAddress
	n.client = client

	n.lock.Unlock()

	return nil
}

type CoreAbout struct {
	ID          string
	Name        string
	Address     string
	State       string
	Error       error
	CreatedAt   time.Time
	Uptime      time.Duration
	LastContact time.Time
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

func (n *Core) About() (CoreAbout, error) {
	n.lock.RLock()
	client := n.client
	n.lock.RUnlock()

	if client == nil {
		return CoreAbout{}, ErrNoPeer
	}

	about, err := client.About(false)
	if err != nil {
		return CoreAbout{}, err
	}

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

	return cabout, nil
}

func (n *Core) ProcessAdd(config *app.Config, metadata map[string]interface{}) error {
	n.lock.RLock()
	client := n.client
	n.lock.RUnlock()

	if client == nil {
		return ErrNoPeer
	}

	return client.ProcessAdd(config, metadata)
}

func (n *Core) ProcessCommand(id app.ProcessID, command string) error {
	n.lock.RLock()
	client := n.client
	n.lock.RUnlock()

	if client == nil {
		return ErrNoPeer
	}

	return client.ProcessCommand(id, command)
}

func (n *Core) ProcessDelete(id app.ProcessID) error {
	n.lock.RLock()
	client := n.client
	n.lock.RUnlock()

	if client == nil {
		return ErrNoPeer
	}

	return client.ProcessDelete(id)
}

func (n *Core) ProcessUpdate(id app.ProcessID, config *app.Config, metadata map[string]interface{}) error {
	n.lock.RLock()
	client := n.client
	n.lock.RUnlock()

	if client == nil {
		return ErrNoPeer
	}

	return client.ProcessUpdate(id, config, metadata)
}

func (n *Core) ProcessReportSet(id app.ProcessID, report *app.Report) error {
	n.lock.RLock()
	client := n.client
	n.lock.RUnlock()

	if client == nil {
		return ErrNoPeer
	}

	return client.ProcessReportSet(id, report)
}

func (n *Core) ProcessProbe(id app.ProcessID) (api.Probe, error) {
	n.lock.RLock()
	client := n.client
	n.lock.RUnlock()

	if client == nil {
		probe := api.Probe{
			Log: []string{fmt.Sprintf("the node %s where the process %s resides, is not connected", n.id, id.String())},
		}
		return probe, ErrNoPeer
	}

	probe, err := client.ProcessProbe(id)

	probe.Log = append([]string{fmt.Sprintf("probed on node: %s", n.id)}, probe.Log...)

	return probe, err
}

func (n *Core) ProcessProbeConfig(config *app.Config) (api.Probe, error) {
	n.lock.RLock()
	client := n.client
	n.lock.RUnlock()

	if client == nil {
		probe := api.Probe{
			Log: []string{fmt.Sprintf("the node %s where the process config should be probed, is not connected", n.id)},
		}
		return probe, ErrNoPeer
	}

	probe, err := client.ProcessProbeConfig(config)

	probe.Log = append([]string{fmt.Sprintf("probed on node: %s", n.id)}, probe.Log...)

	return probe, err
}

func (n *Core) ProcessList(options client.ProcessListOptions) ([]api.Process, error) {
	n.lock.RLock()
	client := n.client
	n.lock.RUnlock()

	if client == nil {
		return nil, ErrNoPeer
	}

	return client.ProcessList(options)
}

func (n *Core) FilesystemList(storage, pattern string) ([]api.FileInfo, error) {
	n.lock.RLock()
	client := n.client
	n.lock.RUnlock()

	if client == nil {
		return nil, ErrNoPeer
	}

	files, err := client.FilesystemList(storage, pattern, "", "")
	if err != nil {
		return nil, err
	}

	for i := range files {
		files[i].CoreID = n.id
	}

	return files, nil
}

func (n *Core) FilesystemDeleteFile(storage, path string) error {
	n.lock.RLock()
	client := n.client
	n.lock.RUnlock()

	if client == nil {
		return ErrNoPeer
	}

	return client.FilesystemDeleteFile(storage, path)
}

func (n *Core) FilesystemPutFile(storage, path string, data io.Reader) error {
	n.lock.RLock()
	client := n.client
	n.lock.RUnlock()

	if client == nil {
		return ErrNoPeer
	}

	return client.FilesystemAddFile(storage, path, data)
}

func (n *Core) FilesystemGetFileInfo(storage, path string) (int64, time.Time, error) {
	n.lock.RLock()
	client := n.client
	n.lock.RUnlock()

	if client == nil {
		return 0, time.Time{}, ErrNoPeer
	}

	info, err := client.FilesystemList(storage, path, "", "")
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("file not found: %w", err)
	}

	if len(info) != 1 {
		return 0, time.Time{}, fmt.Errorf("ambigous result")
	}

	return info[0].Size, time.Unix(info[0].LastMod, 0), nil
}

func (n *Core) FilesystemGetFile(storage, path string, offset int64) (io.ReadCloser, error) {
	n.lock.RLock()
	client := n.client
	n.lock.RUnlock()

	if client == nil {
		return nil, ErrNoPeer
	}

	return client.FilesystemGetFileOffset(storage, path, offset)
}

type NodeFiles struct {
	ID         string
	Files      []string
	LastUpdate time.Time
}

func (n *Core) MediaList() NodeFiles {
	files := NodeFiles{
		ID:         n.id,
		Files:      []string{},
		LastUpdate: time.Now(),
	}

	errorsChan := make(chan error, 8)
	filesChan := make(chan string, 1024)
	errorList := []error{}

	wgList := sync.WaitGroup{}
	wgList.Add(1)

	go func() {
		defer wgList.Done()

		for file := range filesChan {
			files.Files = append(files.Files, file)
		}

		for err := range errorsChan {
			errorList = append(errorList, err)
		}
	}()

	wg := sync.WaitGroup{}
	wg.Add(2)

	go func(f chan<- string, e chan<- error) {
		defer wg.Done()

		n.lock.RLock()
		client := n.client
		n.lock.RUnlock()

		if client == nil {
			e <- ErrNoPeer
			return
		}

		files, err := client.FilesystemList("mem", "/*", "name", "asc")
		if err != nil {
			e <- err
			return
		}

		for _, file := range files {
			f <- "mem:" + file.Name
		}
	}(filesChan, errorsChan)

	go func(f chan<- string, e chan<- error) {
		defer wg.Done()

		n.lock.RLock()
		client := n.client
		n.lock.RUnlock()

		if client == nil {
			e <- ErrNoPeer
			return
		}

		files, err := client.FilesystemList("disk", "/*", "name", "asc")
		if err != nil {
			e <- err
			return
		}

		for _, file := range files {
			f <- "disk:" + file.Name
		}
	}(filesChan, errorsChan)

	if n.hasRTMP {
		wg.Add(1)

		go func(f chan<- string, e chan<- error) {
			defer wg.Done()

			n.lock.RLock()
			client := n.client
			n.lock.RUnlock()

			if client == nil {
				e <- ErrNoPeer
				return
			}

			files, err := client.RTMPChannels()
			if err != nil {
				e <- err
				return
			}

			for _, file := range files {
				f <- "rtmp:" + file.Name
			}
		}(filesChan, errorsChan)
	}

	if n.hasSRT {
		wg.Add(1)

		go func(f chan<- string, e chan<- error) {
			defer wg.Done()

			n.lock.RLock()
			client := n.client
			n.lock.RUnlock()

			if client == nil {
				e <- ErrNoPeer
				return
			}

			files, err := client.SRTChannels()
			if err != nil {
				e <- err
				return
			}

			for _, file := range files {
				f <- "srt:" + file.Name
			}
		}(filesChan, errorsChan)
	}

	wg.Wait()

	close(filesChan)
	close(errorsChan)

	wgList.Wait()

	return files
}

func cloneURL(src *url.URL) *url.URL {
	dst := &url.URL{
		Scheme:      src.Scheme,
		Opaque:      src.Opaque,
		User:        nil,
		Host:        src.Host,
		Path:        src.Path,
		RawPath:     src.RawPath,
		OmitHost:    src.OmitHost,
		ForceQuery:  src.ForceQuery,
		RawQuery:    src.RawQuery,
		Fragment:    src.Fragment,
		RawFragment: src.RawFragment,
	}

	if src.User != nil {
		username := src.User.Username()
		password, ok := src.User.Password()

		if ok {
			dst.User = url.UserPassword(username, password)
		} else {
			dst.User = url.User(username)
		}
	}

	return dst
}

func (n *Core) MediaGetURL(prefix, path string) (*url.URL, error) {
	var u *url.URL

	if prefix == "mem" {
		u = cloneURL(n.httpAddress)
		u = u.JoinPath("memfs", path)
	} else if prefix == "disk" {
		u = cloneURL(n.httpAddress)
		u = u.JoinPath(path)
	} else if prefix == "rtmp" {
		u = cloneURL(n.rtmpAddress)
		u = u.JoinPath(path)
	} else if prefix == "srt" {
		u = cloneURL(n.srtAddress)
	} else {
		return nil, fmt.Errorf("unknown prefix")
	}

	return u, nil
}

func (n *Core) MediaGetInfo(prefix, path string) (int64, time.Time, error) {
	if prefix == "disk" || prefix == "mem" {
		return n.FilesystemGetFileInfo(prefix, path)
	}

	if prefix != "rtmp" && prefix != "srt" {
		return 0, time.Time{}, fmt.Errorf("unknown prefix: %s", prefix)
	}

	n.lock.RLock()
	client := n.client
	n.lock.RUnlock()

	if client == nil {
		return 0, time.Time{}, ErrNoPeer
	}

	if prefix == "rtmp" {
		files, err := n.client.RTMPChannels()
		if err != nil {
			return 0, time.Time{}, err
		}

		for _, file := range files {
			if path == file.Name {
				return 0, time.Now(), nil
			}
		}

		return 0, time.Time{}, fmt.Errorf("media not found")
	}

	if prefix == "srt" {
		files, err := n.client.SRTChannels()
		if err != nil {
			return 0, time.Time{}, err
		}

		for _, file := range files {
			if path == file.Name {
				return 0, time.Now(), nil
			}
		}

		return 0, time.Time{}, fmt.Errorf("media not found")
	}

	return 0, time.Time{}, fmt.Errorf("unknown prefix: %s", prefix)
}

type Process struct {
	NodeID    string
	Order     string
	State     string
	Resources ProcessResources
	Runtime   time.Duration
	UpdatedAt time.Time
	Config    *app.Config
	Metadata  map[string]interface{}
}

type ProcessResources struct {
	CPU        float64 // Current CPU load of this process, 0-100*ncpu
	Mem        uint64  // Currently consumed memory of this process in bytes
	GPU        ProcessGPUResources
	Throttling bool
}

type ProcessGPUResources struct {
	Index   int     // GPU number
	Usage   float64 // Current GPU load, 0-100
	Encoder float64 // Current GPU encoder load, 0-100
	Decoder float64 // Current GPU decoder load, 0-100
	Mem     uint64  // Currently consumed GPU memory of this process in bytes
}

func (p *ProcessResources) Marshal(a *api.ProcessUsage) {
	p.Throttling = a.CPU.IsThrottling

	if x, err := a.CPU.Current.Float64(); err == nil {
		p.CPU = x
	} else {
		p.CPU = 0
	}

	p.Mem = a.Memory.Current

	if x, err := a.GPU.Usage.Current.Float64(); err == nil {
		p.GPU.Usage = x
	} else {
		p.GPU.Usage = 0
	}

	if x, err := a.GPU.Encoder.Current.Float64(); err == nil {
		p.GPU.Encoder = x
	} else {
		p.GPU.Encoder = 0
	}

	if x, err := a.GPU.Decoder.Current.Float64(); err == nil {
		p.GPU.Decoder = x
	} else {
		p.GPU.Decoder = 0
	}

	p.GPU.Mem = a.GPU.Memory.Current
	p.GPU.Index = a.GPU.Index
}

func (n *Core) ClusterProcessList() ([]Process, error) {
	list, err := n.ProcessList(client.ProcessListOptions{
		Filter: []string{"config", "state", "metadata"},
	})
	if err != nil {
		return nil, err
	}

	nodeid := n.id

	processes := []Process{}

	for _, p := range list {
		if p.State == nil {
			p.State = &api.ProcessState{}
		}

		if p.Config == nil {
			p.Config = &api.ProcessConfig{}
		}

		process := Process{
			NodeID:    nodeid,
			Order:     p.State.Order,
			State:     p.State.State,
			Runtime:   time.Duration(p.State.Runtime) * time.Second,
			UpdatedAt: time.Unix(p.UpdatedAt, 0),
		}

		process.Resources.Marshal(&p.State.Resources)

		config, _ := p.Config.Marshal()

		process.Config = config
		if p.Metadata != nil {
			process.Metadata = p.Metadata.(map[string]interface{})
		}

		processes = append(processes, process)
	}

	return processes, nil
}

func (n *Core) Events(ctx context.Context, filters api.EventFilters) (<-chan api.Event, error) {
	n.lock.RLock()
	client := n.client
	n.lock.RUnlock()

	if client == nil {
		return nil, ErrNoPeer
	}

	return client.Events(ctx, filters)
}
