package cluster

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	httpapi "github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/log"
)

// Forwarder forwards any HTTP request from a follower to the leader
type Forwarder interface {
	SetLeader(address string)
	HasLeader() bool
	Join(origin, id, raftAddress, apiAddress, apiUsername, apiPassword string) error
	Leave(origin, id string) error
	Snapshot() ([]byte, error)
	AddProcess() error
	UpdateProcess() error
	RemoveProcess() error
}

type forwarder struct {
	id         string
	leaderAddr string
	lock       sync.RWMutex

	client *http.Client

	logger log.Logger
}

type ForwarderConfig struct {
	ID     string
	Logger log.Logger
}

func NewForwarder(config ForwarderConfig) (Forwarder, error) {
	f := &forwarder{
		id:     config.ID,
		logger: config.Logger,
	}

	if f.logger == nil {
		f.logger = log.New("")
	}

	tr := &http.Transport{
		MaxIdleConns:    10,
		IdleConnTimeout: 30 * time.Second,
	}

	client := &http.Client{
		Transport: tr,
		Timeout:   5 * time.Second,
	}

	f.client = client

	return f, nil
}

func (f *forwarder) SetLeader(address string) {
	f.lock.Lock()
	defer f.lock.Unlock()

	if f.leaderAddr == address {
		return
	}

	f.logger.Debug().Log("setting leader address to %s", address)

	f.leaderAddr = address
}

func (f *forwarder) HasLeader() bool {
	return len(f.leaderAddr) != 0
}

func (f *forwarder) Join(origin, id, raftAddress, apiAddress, apiUsername, apiPassword string) error {
	if origin == "" {
		origin = f.id
	}

	r := JoinRequest{
		Origin:      origin,
		ID:          id,
		RaftAddress: raftAddress,
		APIAddress:  apiAddress,
		APIUsername: apiUsername,
		APIPassword: apiPassword,
	}

	f.logger.Debug().WithField("request", r).Log("forwarding to leader")

	data, err := json.Marshal(&r)
	if err != nil {
		return err
	}

	_, err = f.call(http.MethodPost, "/join", "application/json", bytes.NewReader(data))

	return err
}

func (f *forwarder) Leave(origin, id string) error {
	if origin == "" {
		origin = f.id
	}

	r := LeaveRequest{
		Origin: origin,
		ID:     id,
	}

	f.logger.Debug().WithField("request", r).Log("forwarding to leader")

	data, err := json.Marshal(&r)
	if err != nil {
		return err
	}

	_, err = f.call(http.MethodPost, "/leave", "application/json", bytes.NewReader(data))

	return err
}

func (f *forwarder) Snapshot() ([]byte, error) {
	r, err := f.stream(http.MethodGet, "/snapshot", "", nil)
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (f *forwarder) AddProcess() error {
	return fmt.Errorf("not implemented")
}

func (f *forwarder) UpdateProcess() error {
	return fmt.Errorf("not implemented")
}

func (f *forwarder) RemoveProcess() error {
	return fmt.Errorf("not implemented")
}

func (f *forwarder) stream(method, path, contentType string, data io.Reader) (io.ReadCloser, error) {
	if len(f.leaderAddr) == 0 {
		return nil, fmt.Errorf("no leader address defined")
	}

	f.lock.RLock()
	address := "http://" + f.leaderAddr + "/v1" + path
	f.lock.RUnlock()

	f.logger.Debug().Log("address: %s", address)

	req, err := http.NewRequest(method, address, data)
	if err != nil {
		return nil, err
	}

	if method == "POST" || method == "PUT" {
		req.Header.Add("Content-Type", contentType)
	}

	status, body, err := f.request(req)
	if err != nil {
		return nil, err
	}

	if status < 200 || status >= 300 {
		e := httpapi.Error{}

		defer body.Close()

		x, _ := io.ReadAll(body)

		json.Unmarshal(x, &e)

		return nil, e
	}

	return body, nil
}

func (f *forwarder) call(method, path, contentType string, data io.Reader) ([]byte, error) {
	body, err := f.stream(method, path, contentType, data)
	if err != nil {
		return nil, err
	}

	defer body.Close()

	x, _ := io.ReadAll(body)

	return x, nil
}

func (f *forwarder) request(req *http.Request) (int, io.ReadCloser, error) {
	resp, err := f.client.Do(req)
	if err != nil {
		return -1, nil, err
	}

	return resp.StatusCode, resp.Body, nil
}
