package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"time"

	"github.com/datarhei/core/v16/config"
	"github.com/datarhei/core/v16/ffmpeg/skills"
	httpapi "github.com/datarhei/core/v16/http/api"
	iamaccess "github.com/datarhei/core/v16/iam/access"
	iamidentity "github.com/datarhei/core/v16/iam/identity"
	"github.com/datarhei/core/v16/restream/app"
)

type JoinRequest struct {
	ID          string `json:"id"`
	RaftAddress string `json:"raft_address"`
}

type AddProcessRequest struct {
	Config app.Config `json:"config"`
}

type UpdateProcessRequest struct {
	Config app.Config `json:"config"`
}

type SetProcessCommandRequest struct {
	Command string `json:"order"`
}

type SetProcessMetadataRequest struct {
	Metadata interface{} `json:"metadata"`
}

type AddIdentityRequest struct {
	Identity iamidentity.User `json:"identity"`
}

type UpdateIdentityRequest struct {
	Identity iamidentity.User `json:"identity"`
}

type SetPoliciesRequest struct {
	Policies []iamaccess.Policy `json:"policies"`
}

type LockRequest struct {
	Name       string    `json:"name"`
	ValidUntil time.Time `json:"valid_until"`
}

type SetKVRequest struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type GetKVResponse struct {
	Value     string    `json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

type APIClient struct {
	Address string
	Client  *http.Client
}

func (c *APIClient) Version() (string, error) {
	data, err := c.call(http.MethodGet, "/", "", nil, "")
	if err != nil {
		return "", err
	}

	var version string
	err = json.Unmarshal(data, &version)
	if err != nil {
		return "", err
	}

	return version, nil
}

func (c *APIClient) Barrier(name string) (bool, error) {
	data, err := c.call(http.MethodGet, "/v1/barrier/"+url.PathEscape(name), "application/json", nil, "")
	if err != nil {
		return false, err
	}

	var passed bool
	err = json.Unmarshal(data, &passed)
	if err != nil {
		return false, err
	}

	return passed, nil
}

func (c *APIClient) CoreAPIAddress() (string, error) {
	data, err := c.call(http.MethodGet, "/v1/core", "", nil, "")
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

func (c *APIClient) CoreConfig() (*config.Config, error) {
	data, err := c.call(http.MethodGet, "/v1/core/config", "", nil, "")
	if err != nil {
		return nil, err
	}

	cfg := &config.Config{}
	err = json.Unmarshal(data, &cfg)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *APIClient) CoreSkills() (skills.Skills, error) {
	data, err := c.call(http.MethodGet, "/v1/core/skills", "", nil, "")
	if err != nil {
		return skills.Skills{}, err
	}

	s := skills.Skills{}
	err = json.Unmarshal(data, &s)
	if err != nil {
		return skills.Skills{}, err
	}

	return s, nil
}

func (c *APIClient) Join(origin string, r JoinRequest) error {
	data, err := json.Marshal(&r)
	if err != nil {
		return err
	}

	_, err = c.call(http.MethodPost, "/v1/server", "application/json", bytes.NewReader(data), origin)

	return err
}

func (c *APIClient) Leave(origin string, id string) error {
	_, err := c.call(http.MethodDelete, "/v1/server/"+url.PathEscape(id), "application/json", nil, origin)

	return err
}

func (c *APIClient) TransferLeadership(origin, id string) error {
	_, err := c.call(http.MethodPut, "/v1/transfer/"+url.PathEscape(id), "application/json", nil, origin)

	return err
}

func (c *APIClient) AddProcess(origin string, r AddProcessRequest) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}

	_, err = c.call(http.MethodPost, "/v1/process", "application/json", bytes.NewReader(data), origin)

	return err
}

func (c *APIClient) RemoveProcess(origin string, id app.ProcessID) error {
	_, err := c.call(http.MethodDelete, "/v1/process/"+url.PathEscape(id.ID)+"?domain="+url.QueryEscape(id.Domain), "application/json", nil, origin)

	return err
}

func (c *APIClient) UpdateProcess(origin string, id app.ProcessID, r UpdateProcessRequest) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}

	_, err = c.call(http.MethodPut, "/v1/process/"+url.PathEscape(id.ID)+"?domain="+url.QueryEscape(id.Domain), "application/json", bytes.NewReader(data), origin)

	return err
}

func (c *APIClient) SetProcessCommand(origin string, id app.ProcessID, r SetProcessCommandRequest) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}

	_, err = c.call(http.MethodPut, "/v1/process/"+url.PathEscape(id.ID)+"/command?domain="+url.QueryEscape(id.Domain), "application/json", bytes.NewReader(data), origin)

	return err
}

func (c *APIClient) SetProcessMetadata(origin string, id app.ProcessID, key string, r SetProcessMetadataRequest) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}

	_, err = c.call(http.MethodPut, "/v1/process/"+url.PathEscape(id.ID)+"/metadata/"+url.PathEscape(key)+"?domain="+url.QueryEscape(id.Domain), "application/json", bytes.NewReader(data), origin)

	return err
}

func (c *APIClient) AddIdentity(origin string, r AddIdentityRequest) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}

	_, err = c.call(http.MethodPost, "/v1/iam/user", "application/json", bytes.NewReader(data), origin)

	return err
}

func (c *APIClient) UpdateIdentity(origin, name string, r UpdateIdentityRequest) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}

	_, err = c.call(http.MethodPut, "/v1/iam/user/"+url.PathEscape(name), "application/json", bytes.NewReader(data), origin)

	return err
}

func (c *APIClient) SetPolicies(origin, name string, r SetPoliciesRequest) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}

	_, err = c.call(http.MethodPut, "/v1/iam/user/"+url.PathEscape(name)+"/policies", "application/json", bytes.NewReader(data), origin)

	return err
}

func (c *APIClient) RemoveIdentity(origin string, name string) error {
	_, err := c.call(http.MethodDelete, "/v1/iam/user/"+url.PathEscape(name), "application/json", nil, origin)

	return err
}

func (c *APIClient) Lock(origin string, r LockRequest) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}

	_, err = c.call(http.MethodPost, "/v1/lock", "application/json", bytes.NewReader(data), origin)

	return err
}

func (c *APIClient) Unlock(origin string, name string) error {
	_, err := c.call(http.MethodDelete, "/v1/lock/"+url.PathEscape(name), "application/json", nil, origin)

	return err
}

func (c *APIClient) SetKV(origin string, r SetKVRequest) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}

	_, err = c.call(http.MethodPost, "/v1/kv", "application/json", bytes.NewReader(data), origin)

	return err
}

func (c *APIClient) UnsetKV(origin string, key string) error {
	_, err := c.call(http.MethodDelete, "/v1/kv/"+url.PathEscape(key), "application/json", nil, origin)
	if err != nil {
		e, ok := err.(httpapi.Error)
		if ok && e.Code == 404 {
			return fs.ErrNotExist
		}
	}

	return err
}

func (c *APIClient) GetKV(origin string, key string) (string, time.Time, error) {
	data, err := c.call(http.MethodGet, "/v1/kv/"+url.PathEscape(key), "application/json", nil, origin)
	if err != nil {
		e, ok := err.(httpapi.Error)
		if ok && e.Code == 404 {
			return "", time.Time{}, fs.ErrNotExist
		}

		return "", time.Time{}, err
	}

	res := GetKVResponse{}
	err = json.Unmarshal(data, &res)
	if err != nil {
		return "", time.Time{}, err
	}

	return res.Value, res.UpdatedAt, nil
}

func (c *APIClient) Snapshot(origin string) (io.ReadCloser, error) {
	return c.stream(http.MethodGet, "/v1/snapshot", "", nil, origin)
}

func (c *APIClient) stream(method, path, contentType string, data io.Reader, origin string) (io.ReadCloser, error) {
	if len(c.Address) == 0 {
		return nil, fmt.Errorf("no address defined")
	}

	address := "http://" + c.Address + path

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
		tr := http.DefaultTransport.(*http.Transport).Clone()
		tr.MaxIdleConns = 10
		tr.IdleConnTimeout = 30 * time.Second

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
