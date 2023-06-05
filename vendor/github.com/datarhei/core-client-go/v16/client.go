package coreclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/datarhei/core-client-go/v16/api"

	"github.com/Masterminds/semver/v3"
)

const (
	coreapp     = "datarhei-core"
	coremajor   = 16
	coreversion = "^16.7.2" // first public release
)

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type RestClient interface {
	// String returns a string representation of the connection
	String() string

	// ID returns the ID of the connected datarhei Core
	ID() string

	// Tokens returns the access and refresh token of the current session
	Tokens() (string, string)

	// Address returns the address of the connected datarhei Core
	Address() string

	Ping() (bool, time.Duration)

	About() api.About // GET /

	Config() (int64, api.Config, error) // GET /config
	ConfigSet(config interface{}) error // POST /config
	ConfigReload() error                // GET /config/reload

	Graph(query api.GraphQuery) (api.GraphResponse, error) // POST /graph

	DiskFSList(sort, order string) ([]api.FileInfo, error) // GET /v3/fs/disk
	DiskFSHasFile(path string) bool                        // HEAD /v3/fs/disk/{path}
	DiskFSGetFile(path string) (io.ReadCloser, error)      // GET /v3/fs/disk/{path}
	DiskFSDeleteFile(path string) error                    // DELETE /v3/fs/disk/{path}
	DiskFSAddFile(path string, data io.Reader) error       // PUT /v3/fs/disk/{path}

	MemFSList(sort, order string) ([]api.FileInfo, error) // GET /v3/fs/mem
	MemFSHasFile(path string) bool                        // HEAD /v3/fs/mem/{path}
	MemFSGetFile(path string) (io.ReadCloser, error)      // GET /v3/fs/mem/{path}
	MemFSDeleteFile(path string) error                    // DELETE /v3/fs/mem/{path}
	MemFSAddFile(path string, data io.Reader) error       // PUT /v3/fs/mem/{path}

	FilesystemList(name, pattern, sort, order string) ([]api.FileInfo, error) // GET /v3/fs/{name}
	FilesystemHasFile(name, path string) bool                                 // HEAD /v3/fs/{name}/{path}
	FilesystemGetFile(name, path string) (io.ReadCloser, error)               // GET /v3/fs/{name}/{path}
	FilesystemDeleteFile(name, path string) error                             // DELETE /v3/fs/{name}/{path}
	FilesystemAddFile(name, path string, data io.Reader) error                // PUT /v3/fs/{name}/{path}

	Log() ([]api.LogEvent, error) // GET /log

	Metadata(key string) (api.Metadata, error)           // GET /v3/metadata/{key}
	MetadataSet(key string, metadata api.Metadata) error // PUT /v3/metadata/{key}

	MetricsList() ([]api.MetricsDescription, error)              // GET /v3/metrics
	Metrics(query api.MetricsQuery) (api.MetricsResponse, error) // POST /v3/metrics

	ProcessList(opts ProcessListOptions) ([]api.Process, error)               // GET /v3/process
	ProcessAdd(p api.ProcessConfig) error                                     // POST /v3/process
	Process(id ProcessID, filter []string) (api.Process, error)               // GET /v3/process/{id}
	ProcessUpdate(id ProcessID, p api.ProcessConfig) error                    // PUT /v3/process/{id}
	ProcessDelete(id ProcessID) error                                         // DELETE /v3/process/{id}
	ProcessCommand(id ProcessID, command string) error                        // PUT /v3/process/{id}/command
	ProcessProbe(id ProcessID) (api.Probe, error)                             // GET /v3/process/{id}/probe
	ProcessConfig(id ProcessID) (api.ProcessConfig, error)                    // GET /v3/process/{id}/config
	ProcessReport(id ProcessID) (api.ProcessReport, error)                    // GET /v3/process/{id}/report
	ProcessState(id ProcessID) (api.ProcessState, error)                      // GET /v3/process/{id}/state
	ProcessMetadata(id ProcessID, key string) (api.Metadata, error)           // GET /v3/process/{id}/metadata/{key}
	ProcessMetadataSet(id ProcessID, key string, metadata api.Metadata) error // PUT /v3/process/{id}/metadata/{key}

	RTMPChannels() ([]api.RTMPChannel, error) // GET /v3/rtmp
	SRTChannels() ([]api.SRTChannel, error)   // GET /v3/srt

	Sessions(collectors []string) (api.SessionsSummary, error)      // GET /v3/session
	SessionsActive(collectors []string) (api.SessionsActive, error) // GET /v3/session/active

	Skills() (api.Skills, error) // GET /v3/skills
	SkillsReload() error         // GET /v3/skills/reload

	WidgetProcess(id ProcessID) (api.WidgetProcess, error) // GET /v3/widget/process/{id}
}

// Config is the configuration for a new REST API client.
type Config struct {
	// Address is the address of the datarhei Core to connect to.
	Address string

	// Username and password are credentials to authorize access to the API.
	Username string
	Password string

	// Access and refresh token from an existing session to authorize access to the API.
	AccessToken  string
	RefreshToken string

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

	version struct {
		connectedCore *semver.Version
		methods       map[string]*semver.Constraints
	}
}

// New returns a new REST API client for the given config. The error is non-nil
// in case of an error.
func New(config Config) (RestClient, error) {
	r := &restclient{
		address:      config.Address,
		prefix:       "/api",
		username:     config.Username,
		password:     config.Password,
		accessToken:  config.AccessToken,
		refreshToken: config.RefreshToken,
		auth0Token:   config.Auth0Token,
		client:       config.Client,
	}

	u, err := url.Parse(r.address)
	if err != nil {
		return nil, err
	}

	username := u.User.Username()
	if len(username) != 0 {
		r.username = username
	}

	if password, ok := u.User.Password(); ok {
		r.password = password
	}

	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""

	r.address = u.String()

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

	mustNewConstraint := func(constraint string) *semver.Constraints {
		v, _ := semver.NewConstraint(constraint)
		return v
	}

	r.version.methods = map[string]*semver.Constraints{
		"GET/api/v3/srt":     mustNewConstraint("^16.9.0"),
		"GET/api/v3/metrics": mustNewConstraint("^16.10.0"),
	}

	if len(r.about.ID) != 0 {
		c, _ := semver.NewConstraint(coreversion)
		v, err := semver.NewVersion(r.about.Version.Number)
		if err != nil {
			return nil, err
		}

		if !c.Check(v) {
			return nil, fmt.Errorf("the core version (%s) is not supported, because a version %s is required", r.about.Version.Number, coreversion)
		}

		r.version.connectedCore = v
	} else {
		v, err := semver.NewVersion(r.about.Version.Number)
		if err != nil {
			return nil, err
		}

		if coremajor != v.Major() {
			return nil, fmt.Errorf("the core major version (%d) is not supported, because %d is required", v.Major(), coremajor)
		}

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

func (r *restclient) Tokens() (string, string) {
	return r.accessToken, r.refreshToken
}

func (r *restclient) Address() string {
	return r.address
}

func (r *restclient) About() api.About {
	return r.about
}

func (r *restclient) Ping() (bool, time.Duration) {
	req, err := http.NewRequest(http.MethodGet, r.address+"/ping", nil)
	if err != nil {
		return false, time.Duration(0)
	}

	start := time.Now()

	status, body, err := r.request(req)
	if err != nil {
		return false, time.Since(start)
	}

	defer body.Close()

	if status != 200 {
		return false, time.Since(start)
	}

	return true, time.Since(start)
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

	data, _ := io.ReadAll(body)

	jwt := api.JWT{}

	json.Unmarshal(data, &jwt)

	r.accessToken = jwt.AccessToken
	r.refreshToken = jwt.RefreshToken

	about, err := r.info()
	if err != nil {
		return err
	}

	if len(about.ID) == 0 {
		return fmt.Errorf("login to the API failed")
	}

	c, _ := semver.NewConstraint(coreversion)
	v, err := semver.NewVersion(about.Version.Number)
	if err != nil {
		return err
	}

	if !c.Check(v) {
		return fmt.Errorf("the core version (%s) is not supported, because a version %s is required", about.Version.Number, coreversion)
	}

	r.version.connectedCore = v
	r.about = about

	return nil
}

func (r *restclient) checkVersion(method, path string) error {
	c := r.version.methods[method+path]
	if c == nil {
		return nil
	}

	if !c.Check(r.version.connectedCore) {
		return fmt.Errorf("this method is only available in version %s of the core", c.String())
	}

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

	data, _ := io.ReadAll(body)

	jwt := api.JWTRefresh{}

	json.Unmarshal(data, &jwt)

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

	data, _ := io.ReadAll(body)

	about := api.About{}

	json.Unmarshal(data, &about)

	return about, nil
}

func (r *restclient) request(req *http.Request) (int, io.ReadCloser, error) {
	resp, err := r.client.Do(req)
	if err != nil {
		return -1, nil, err
	}

	return resp.StatusCode, resp.Body, nil
}

func (r *restclient) stream(method, path string, query *url.Values, contentType string, data io.Reader) (io.ReadCloser, error) {
	if err := r.checkVersion(method, r.prefix+path); err != nil {
		return nil, err
	}

	u := r.address + r.prefix + path
	if query != nil {
		u += "?" + query.Encode()
	}

	req, err := http.NewRequest(method, u, data)
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
		e := api.Error{
			Code: status,
		}

		defer body.Close()

		data, err := io.ReadAll(body)
		if err != nil {
			return nil, e
		}

		e.Body = data

		err = json.Unmarshal(data, &e)
		if err != nil {
			return nil, e
		}

		// In case it's not an api.Error, reconstruct the return code. With this
		// and the body, the caller can reconstruct the correct error.
		if e.Code == 0 {
			e.Code = status
		}

		return nil, e
	}

	return body, nil
}

func (r *restclient) call(method, path string, query *url.Values, contentType string, data io.Reader) ([]byte, error) {
	body, err := r.stream(method, path, query, contentType, data)
	if err != nil {
		return nil, err
	}

	defer body.Close()

	x, err := io.ReadAll(body)

	return x, err
}
