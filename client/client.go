package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/datarhei/core/v16/app"
	"github.com/datarhei/core/v16/http/api"

	"github.com/Masterminds/semver/v3"
)

var coreapp = app.Name
var coreversion = "~" + app.Version.MinorString()

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type RestClient interface {
	// String returns a string representation of the connection
	String() string

	// ID returns the ID of the connected datarhei Core
	ID() string

	// Address returns the address of the connected datarhei Core
	Address() string

	About() api.About // GET /

	Config() (api.Config, error)           // GET /config
	ConfigSet(config api.ConfigData) error // POST /config
	ConfigReload() error                   // GET /config/reload

	DiskFSList(sort, order string) ([]api.FileInfo, error) // GET /fs/disk
	DiskFSHasFile(path string) bool                        // GET /fs/disk/{path}
	DiskFSDeleteFile(path string) error                    // DELETE /fs/disk/{path}
	DiskFSAddFile(path string, data io.Reader) error       // PUT /fs/disk/{path}

	MemFSList(sort, order string) ([]api.FileInfo, error) // GET /fs/mem
	MemFSHasFile(path string) bool                        // GET /fs/mem/{path}
	MemFSDeleteFile(path string) error                    // DELETE /fs/mem/{path}
	MemFSAddFile(path string, data io.Reader) error       // PUT /fs/mem/{path}

	Log() ([]api.LogEvent, error) // GET /log

	Metadata(id, key string) (api.Metadata, error)           // GET /metadata/{key}
	MetadataSet(id, key string, metadata api.Metadata) error // PUT /metadata/{key}

	Metrics(api.MetricsQuery) (api.MetricsResponse, error) // POST /metrics

	ProcessList(id, filter []string) ([]api.Process, error)         // GET /process
	Process(id string, filter []string) (api.Process, error)        // GET /process/{id}
	ProcessAdd(p api.ProcessConfig) error                           // POST /process
	ProcessDelete(id string) error                                  // DELETE /process/{id}
	ProcessCommand(id, command string) error                        // PUT /process/{id}/command
	ProcessProbe(id string) (api.Probe, error)                      // GET /process/{id}/probe
	ProcessConfig(id string) (api.ProcessConfig, error)             // GET /process/{id}/config
	ProcessReport(id string) (api.ProcessReport, error)             // GET /process/{id}/report
	ProcessState(id string) (api.ProcessState, error)               // GET /process/{id}/state
	ProcessMetadata(id, key string) (api.Metadata, error)           // GET /process/{id}/metadata/{key}
	ProcessMetadataSet(id, key string, metadata api.Metadata) error // PUT /process/{id}/metadata/{key}

	RTMPChannels() (api.RTMPChannel, error) // GET /rtmp

	Sessions(collectors []string) (api.SessionsSummary, error)      // GET /session
	SessionsActive(collectors []string) (api.SessionsActive, error) // GET /session/active

	Skills() (api.Skills, error) // GET /skills
	SkillsReload() error         // GET /skills/reload
}

// Config is the configuration for a new REST API client.
type Config struct {
	// Address is the address of the datarhei Core to connect to.
	Address string

	// Username and Password are credentials to authorize access to the API.
	Username string
	Password string

	// Auth0Token is a valid Auth0 token to authorize access to the API.
	Auth0Token string

	// Client is a HTTPClient that will be used for the API calls. Optional.
	Client HTTPClient
}

// restclient implements the RestClient interface.
type restclient struct {
	address      string
	prefix       string
	accessToken  string
	refreshToken string
	username     string
	password     string
	auth0Token   string
	client       HTTPClient
	about        api.About
}

// New returns a new REST API client for the given config. The error is non-nil
// in case of an error.
func New(config Config) (RestClient, error) {
	r := &restclient{
		address:    config.Address,
		prefix:     "/api",
		username:   config.Username,
		password:   config.Password,
		auth0Token: config.Auth0Token,
		client:     config.Client,
	}

	if r.client == nil {
		r.client = &http.Client{
			Timeout: 15 * time.Second,
		}
	}

	about, err := r.info()
	if err != nil {
		return nil, err
	}

	r.about = about

	if r.about.App != coreapp {
		return nil, fmt.Errorf("didn't receive the expected API response (got: %s, want: %s)", r.about.Name, coreapp)
	}

	c, _ := semver.NewConstraint(coreversion)
	v, err := semver.NewVersion(r.about.Version.Number)
	if err != nil {
		return nil, err
	}

	if !c.Check(v) {
		return nil, fmt.Errorf("the core version (%s) is not supported (%s)", r.about.Version.Number, coreversion)
	}

	if len(r.about.ID) == 0 {
		if err := r.login(); err != nil {
			return nil, err
		}
	}

	return r, nil
}

func (r restclient) String() string {
	return fmt.Sprintf("%s %s (%s) %s @ %s", r.about.Name, r.about.Version.Number, r.about.Version.Arch, r.about.ID, r.address)
}

func (r *restclient) ID() string {
	return r.about.ID
}

func (r *restclient) Address() string {
	return r.address
}

func (r *restclient) About() api.About {
	return r.about
}

func (r *restclient) login() error {
	login := api.Login{}

	hasLocalJWT := false
	useLocalJWT := false
	hasAuth0 := false
	useAuth0 := false

	for _, auths := range r.about.Auths {
		if auths == "localjwt" {
			hasLocalJWT = true
			break
		} else if strings.HasPrefix(auths, "auth0 ") {
			hasAuth0 = true
			break
		}
	}

	if !hasLocalJWT && !hasAuth0 {
		return fmt.Errorf("the API doesn't provide any supported auth method")
	}

	if len(r.auth0Token) != 0 && hasAuth0 {
		useAuth0 = true
	}

	if !useAuth0 {
		if (len(r.username) != 0 || len(r.password) != 0) && hasLocalJWT {
			useLocalJWT = true
		}
	}

	if !useAuth0 && !useLocalJWT {
		return fmt.Errorf("none of the provided auth credentials can be used")
	}

	if useLocalJWT {
		login.Username = r.username
		login.Password = r.password
	}

	var buf bytes.Buffer

	e := json.NewEncoder(&buf)
	e.Encode(login)

	req, err := http.NewRequest("POST", r.address+r.prefix+"/login", &buf)
	if err != nil {
		return err
	}

	req.Header.Add("Content-Type", "application/json")
	if useAuth0 {
		req.Header.Add("Authorization", "Bearer "+r.auth0Token)
	}

	status, body, err := r.request(req)
	if err != nil {
		return err
	}

	if status != 200 {
		return fmt.Errorf("wrong username and/or password")
	}

	jwt := api.JWT{}

	json.Unmarshal(body, &jwt)

	r.accessToken = jwt.AccessToken
	r.refreshToken = jwt.RefreshToken

	about, err := r.info()
	if err != nil {
		return err
	}

	if len(about.ID) == 0 {
		return fmt.Errorf("login to the API failed")
	}

	r.about = about

	return nil
}

func (r *restclient) refresh() error {
	req, err := http.NewRequest("GET", r.address+r.prefix+"/login/refresh", nil)
	if err != nil {
		return err
	}

	req.Header.Add("Authorization", "Bearer "+r.refreshToken)

	status, body, err := r.request(req)
	if err != nil {
		return err
	}

	if err != nil {
		return err
	}

	if status != 200 {
		return fmt.Errorf("invalid refresh token")
	}

	jwt := api.JWTRefresh{}

	json.Unmarshal(body, &jwt)

	r.accessToken = jwt.AccessToken

	return nil
}

func (r *restclient) info() (api.About, error) {
	req, err := http.NewRequest("GET", r.address+r.prefix, nil)
	if err != nil {
		return api.About{}, err
	}

	if len(r.accessToken) != 0 {
		req.Header.Add("Authorization", "Bearer "+r.accessToken)
	}

	status, body, err := r.request(req)
	if err != nil {
		return api.About{}, err
	}

	if status != 200 {
		return api.About{}, fmt.Errorf("access to API failed (%d)", status)
	}

	about := api.About{}

	json.Unmarshal(body, &about)

	return about, nil
}

func (r *restclient) call(method, path, contentType string, data io.Reader) ([]byte, error) {
	req, err := http.NewRequest(method, r.address+r.prefix+"/v3"+path, data)
	if err != nil {
		return nil, err
	}

	if method == "POST" || method == "PUT" {
		req.Header.Add("Content-Type", contentType)
	}

	if len(r.accessToken) != 0 {
		req.Header.Add("Authorization", "Bearer "+r.accessToken)
	}

	status, body, err := r.request(req)
	if status == http.StatusUnauthorized {
		if err := r.refresh(); err != nil {
			if err := r.login(); err != nil {
				return nil, err
			}
		}

		req.Header.Set("Authorization", "Bearer "+r.accessToken)
		status, body, err = r.request(req)
	}

	if err != nil {
		return nil, err
	}

	if status < 200 || status >= 300 {
		e := api.Error{}

		json.Unmarshal(body, &e)

		return nil, fmt.Errorf("%w", e)
	}

	return body, nil
}

func (r *restclient) request(req *http.Request) (int, []byte, error) {
	resp, err := r.client.Do(req)
	if err != nil {
		return -1, nil, err
	}

	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	return resp.StatusCode, body, nil
}
