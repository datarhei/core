package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	httpapi "github.com/datarhei/core/v16/http/api"
	iamaccess "github.com/datarhei/core/v16/iam/access"
	iamidentity "github.com/datarhei/core/v16/iam/identity"
	"github.com/datarhei/core/v16/restream/app"
)

type JoinRequest struct {
	ID          string `json:"id"`
	RaftAddress string `json:"raft_address"`
}

type LeaveRequest struct {
	ID string `json:"id"`
}

type AddProcessRequest struct {
	Config app.Config `json:"config"`
}

type UpdateProcessRequest struct {
	ID     app.ProcessID `json:"id"`
	Config app.Config    `json:"config"`
}

type SetProcessMetadataRequest struct {
	ID       app.ProcessID `json:"id"`
	Key      string        `json:"key"`
	Metadata interface{}   `json:"metadata"`
}

type AddIdentityRequest struct {
	Identity iamidentity.User `json:"identity"`
}

type UpdateIdentityRequest struct {
	Name     string           `json:"name"`
	Identity iamidentity.User `json:"identity"`
}

type SetPoliciesRequest struct {
	Name     string             `json:"name"`
	Policies []iamaccess.Policy `json:"policies"`
}

type APIClient struct {
	Address string
	Client  *http.Client
}

func (c *APIClient) CoreAPIAddress() (string, error) {
	data, err := c.call(http.MethodGet, "/core", "", nil, "")
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

func (c *APIClient) Join(origin string, r JoinRequest) error {
	data, err := json.Marshal(&r)
	if err != nil {
		return err
	}

	_, err = c.call(http.MethodPost, "/server", "application/json", bytes.NewReader(data), origin)

	return err
}

func (c *APIClient) Leave(origin string, id string) error {
	_, err := c.call(http.MethodDelete, "/server/"+id, "application/json", nil, origin)

	return err
}

func (c *APIClient) AddProcess(origin string, r AddProcessRequest) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}

	_, err = c.call(http.MethodPost, "/process", "application/json", bytes.NewReader(data), origin)

	return err
}

func (c *APIClient) RemoveProcess(origin string, id app.ProcessID) error {
	_, err := c.call(http.MethodDelete, "/process/"+id.ID+"?domain="+id.Domain, "application/json", nil, origin)

	return err
}

func (c *APIClient) UpdateProcess(origin string, r UpdateProcessRequest) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}

	_, err = c.call(http.MethodPut, "/process/"+r.ID.ID+"?domain="+r.ID.Domain, "application/json", bytes.NewReader(data), origin)

	return err
}

func (c *APIClient) SetProcessMetadata(origin string, r SetProcessMetadataRequest) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}

	_, err = c.call(http.MethodPut, "/process/"+r.ID.ID+"/metadata/"+r.Key+"?domain="+r.ID.Domain, "application/json", bytes.NewReader(data), origin)

	return err
}

func (c *APIClient) AddIdentity(origin string, r AddIdentityRequest) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}

	_, err = c.call(http.MethodPost, "/iam/user", "application/json", bytes.NewReader(data), origin)

	return err
}

func (c *APIClient) UpdateIdentity(origin, name string, r UpdateIdentityRequest) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}

	_, err = c.call(http.MethodPut, "/iam/user/"+name, "application/json", bytes.NewReader(data), origin)

	return err
}

func (c *APIClient) SetPolicies(origin, name string, r SetPoliciesRequest) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}

	_, err = c.call(http.MethodPut, "/iam/user/"+name+"/policies", "application/json", bytes.NewReader(data), origin)

	return err
}

func (c *APIClient) RemoveIdentity(origin string, name string) error {
	_, err := c.call(http.MethodDelete, "/iam/user/"+name, "application/json", nil, origin)

	return err
}

func (c *APIClient) Snapshot() (io.ReadCloser, error) {
	return c.stream(http.MethodGet, "/snapshot", "", nil, "")
}

func (c *APIClient) stream(method, path, contentType string, data io.Reader, origin string) (io.ReadCloser, error) {
	if len(c.Address) == 0 {
		return nil, fmt.Errorf("no address defined")
	}

	address := "http://" + c.Address + "/v1" + path

	req, err := http.NewRequest(method, address, data)
	if err != nil {
		return nil, err
	}

	req.Header.Add("X-Cluster-Origin", origin)

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

func (c *APIClient) call(method, path, contentType string, data io.Reader, origin string) ([]byte, error) {
	body, err := c.stream(method, path, contentType, data, origin)
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
