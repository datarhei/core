package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	httpapi "github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/restream/app"
)

type JoinRequest struct {
	Origin      string `json:"origin"`
	ID          string `json:"id"`
	RaftAddress string `json:"raft_address"`
}

type LeaveRequest struct {
	Origin string `json:"origin"`
	ID     string `json:"id"`
}

type AddProcessRequest struct {
	Origin string     `json:"origin"`
	Config app.Config `json:"config"`
}

type RemoveProcessRequest struct {
	Origin string `json:"origin"`
	ID     string `json:"id"`
}

type UpdateProcessRequest struct {
	Origin string     `json:"origin"`
	ID     string     `json:"id"`
	Config app.Config `json:"config"`
}

type APIClient struct {
	Address string
	Client  *http.Client
}

func (c *APIClient) CoreAPIAddress() (string, error) {
	data, err := c.call(http.MethodGet, "/core", "", nil)
	if err != nil {
		return "", err
	}

	var address string
	err = json.Unmarshal(data, &address)
	if err != nil {
		return "", err
	}

	return address, nil
}

func (c *APIClient) Join(r JoinRequest) error {
	data, err := json.Marshal(&r)
	if err != nil {
		return err
	}

	_, err = c.call(http.MethodPost, "/join", "application/json", bytes.NewReader(data))

	return err
}

func (c *APIClient) Leave(r LeaveRequest) error {
	data, err := json.Marshal(&r)
	if err != nil {
		return err
	}

	_, err = c.call(http.MethodPost, "/leave", "application/json", bytes.NewReader(data))

	return err
}

func (c *APIClient) AddProcess(r AddProcessRequest) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}

	_, err = c.call(http.MethodPost, "/process", "application/json", bytes.NewReader(data))

	return err
}

func (c *APIClient) RemoveProcess(r RemoveProcessRequest) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}

	_, err = c.call(http.MethodPost, "/process/"+r.ID, "application/json", bytes.NewReader(data))

	return err
}

func (c *APIClient) UpdateProcess(r UpdateProcessRequest) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}

	_, err = c.call(http.MethodPut, "/process/"+r.ID, "application/json", bytes.NewReader(data))

	return err
}

func (c *APIClient) Snapshot() (io.ReadCloser, error) {
	return c.stream(http.MethodGet, "/snapshot", "", nil)
}

func (c *APIClient) stream(method, path, contentType string, data io.Reader) (io.ReadCloser, error) {
	if len(c.Address) == 0 {
		return nil, fmt.Errorf("no address defined")
	}

	address := "http://" + c.Address + "/v1" + path

	req, err := http.NewRequest(method, address, data)
	if err != nil {
		return nil, err
	}

	if method == "POST" || method == "PUT" {
		req.Header.Add("Content-Type", contentType)
	}

	status, body, err := c.request(req)
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

func (c *APIClient) call(method, path, contentType string, data io.Reader) ([]byte, error) {
	body, err := c.stream(method, path, contentType, data)
	if err != nil {
		return nil, err
	}

	defer body.Close()

	x, _ := io.ReadAll(body)

	return x, nil
}

func (c *APIClient) request(req *http.Request) (int, io.ReadCloser, error) {
	if c.Client == nil {
		tr := &http.Transport{
			MaxIdleConns:    10,
			IdleConnTimeout: 30 * time.Second,
		}

		c.Client = &http.Client{
			Transport: tr,
			Timeout:   5 * time.Second,
		}
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return -1, nil, err
	}

	return resp.StatusCode, resp.Body, nil
}
