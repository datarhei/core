package coreclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/datarhei/core-client-go/v16/api"

	"github.com/Masterminds/semver/v3"
	"github.com/gobwas/glob"
	jwtgo "github.com/golang-jwt/jwt/v4"
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

	Ping() (time.Duration, error)

	About(cached bool) (api.About, error) // GET /

	Config() (int64, api.Config, error) // GET /v3/config
	ConfigSet(config interface{}) error // POST /v3/config
	ConfigReload() error                // GET /v3/config/reload

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

	FilesystemList(name, pattern, sort, order string) ([]api.FileInfo, error)       // GET /v3/fs/{name}
	FilesystemHasFile(name, path string) bool                                       // HEAD /v3/fs/{name}/{path}
	FilesystemGetFile(name, path string) (io.ReadCloser, error)                     // GET /v3/fs/{name}/{path}
	FilesystemGetFileOffset(name, path string, offset int64) (io.ReadCloser, error) // GET /v3/fs/{name}/{path}
	FilesystemDeleteFile(name, path string) error                                   // DELETE /v3/fs/{name}/{path}
	FilesystemAddFile(name, path string, data io.Reader) error                      // PUT /v3/fs/{name}/{path}

	Log() ([]api.LogEvent, error) // GET /v3/log

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

	IdentitiesList() ([]api.IAMUser, error)                   // GET /v3/iam/user
	Identity(name string) (api.IAMUser, error)                // GET /v3/iam/user/{name}
	IdentityAdd(u api.IAMUser) error                          // POST /v3/iam/user
	IdentityUpdate(name string, u api.IAMUser) error          // PUT /v3/iam/user/{name}
	IdentitySetPolicies(name string, p []api.IAMPolicy) error // PUT /v3/iam/user/{name}/policy
	IdentityDelete(name string) error                         // DELETE /v3/iam/user/{name}

	Cluster() (api.ClusterAbout, error)      // GET /v3/cluster
	ClusterHealthy() (bool, error)           // GET /v3/cluster/healthy
	ClusterSnapshot() (io.ReadCloser, error) // GET /v3/cluster/snapshot
	ClusterLeave() error                     // PUT /v3/cluster/leave

	ClusterNodeList() ([]api.ClusterNode, error)                                      // GET /v3/cluster/node
	ClusterNode(id string) (api.ClusterNode, error)                                   // GET /v3/cluster/node/{id}
	ClusterNodeFiles(id string) (api.ClusterNodeFiles, error)                         // GET /v3/cluster/node/{id}/files
	ClusterNodeProcessList(id string, opts ProcessListOptions) ([]api.Process, error) // GET /v3/cluster/node/{id}/process
	ClusterNodeVersion(id string) (api.Version, error)                                // GET /v3/cluster/node/{id}/version

	ClusterDBProcessList() ([]api.Process, error)       // GET /v3/cluster/db/process
	ClusterDBProcess(id ProcessID) (api.Process, error) // GET /v3/cluster/db/process/{id}
	ClusterDBUserList() ([]api.IAMUser, error)          // GET /v3/cluster/db/user
	ClusterDBUser(name string) (api.IAMUser, error)     // GET /v3/cluster/db/user/{name}
	ClusterDBPolicies() ([]api.IAMPolicy, error)        // GET /v3/cluster/db/policies
	ClusterDBLocks() ([]api.ClusterLock, error)         // GET /v3/cluster/db/locks
	ClusterDBKeyValues() (api.ClusterKVS, error)        // GET /v3/cluster/db/kv

	ClusterProcessList(opts ProcessListOptions) ([]api.Process, error)               // GET /v3/cluster/process
	ClusterProcess(id ProcessID, filter []string) (api.Process, error)               // POST /v3/cluster/process
	ClusterProcessAdd(p api.ProcessConfig) error                                     // GET /v3/cluster/process/{id}
	ClusterProcessUpdate(id ProcessID, p api.ProcessConfig) error                    // PUT /v3/cluster/process/{id}
	ClusterProcessDelete(id ProcessID) error                                         // DELETE /v3/cluster/process/{id}
	ClusterProcessCommand(id ProcessID, command string) error                        // PUT /v3/cluster/process/{id}/command
	ClusterProcessMetadata(id ProcessID, key string) (api.Metadata, error)           // GET /v3/cluster/process/{id}/metadata/{key}
	ClusterProcessMetadataSet(id ProcessID, key string, metadata api.Metadata) error // PUT /v3/cluster/process/{id}/metadata/{key}
	ClusterProcessProbe(id ProcessID) (api.Probe, error)                             // GET /v3/cluster/process/{id}/probe

	ClusterIdentitiesList() ([]api.IAMUser, error)                   // GET /v3/cluster/iam/user
	ClusterIdentity(name string) (api.IAMUser, error)                // GET /v3/cluster/iam/user/{name}
	ClusterIdentityAdd(u api.IAMUser) error                          // POST /v3/cluster/iam/user
	ClusterIdentityUpdate(name string, u api.IAMUser) error          // PUT /v3/cluster/iam/user/{name}
	ClusterIdentitySetPolicies(name string, p []api.IAMPolicy) error // PUT /v3/cluster/iam/user/{name}/policy
	ClusterIdentityDelete(name string) error                         // DELETE /v3/cluster/iam/user/{name}
	ClusterIAMReload() error                                         // PUT /v3/cluster/iam/reload

	RTMPChannels() ([]api.RTMPChannel, error) // GET /v3/rtmp
	SRTChannels() ([]api.SRTChannel, error)   // GET /v3/srt

	Sessions(collectors []string) (api.SessionsSummary, error)                                  // GET /v3/session
	SessionsActive(collectors []string) (api.SessionsActive, error)                             // GET /v3/session/active
	SessionToken(name string, req []api.SessionTokenRequest) ([]api.SessionTokenRequest, error) // PUT /v3/session/token/{username}

	Skills() (api.Skills, error) // GET /v3/skills
	SkillsReload() error         // GET /v3/skills/reload

	WidgetProcess(id ProcessID) (api.WidgetProcess, error) // GET /v3/widget/process/{id}
}

type Token struct {
	token     string
	expiresAt time.Time
	lock      sync.Mutex
}

func (t *Token) IsSet() bool {
	t.lock.Lock()
	defer t.lock.Unlock()

	return len(t.token) != 0
}

func (t *Token) Set(token string) {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.token = token
	t.expiresAt = time.Time{}

	if len(t.token) == 0 {
		return
	}

	p := &jwtgo.Parser{}
	parsedToken, _, err := p.ParseUnverified(token, jwtgo.MapClaims{})
	if err != nil {
		return
	}

	claims, ok := parsedToken.Claims.(jwtgo.MapClaims)
	if !ok {
		return
	}

	sub, ok := claims["exp"]
	if !ok {
		t.expiresAt = time.Now().Add(time.Hour)
		return
	}

	floatToTime := func(t float64, offset time.Duration) time.Time {
		sec, dec := math.Modf(t)
		return time.Unix(int64(sec), int64(dec*(1e9))).Add(offset)
	}

	switch exp := sub.(type) {
	case float64:
		t.expiresAt = floatToTime(exp, -15*time.Second)
	case json.Number:
		v, _ := exp.Float64()
		t.expiresAt = floatToTime(v, -15*time.Second)
	}
}

func (t *Token) String() string {
	t.lock.Lock()
	defer t.lock.Unlock()

	return t.token
}

func (t *Token) IsExpired() bool {
	t.lock.Lock()
	defer t.lock.Unlock()

	return time.Now().After(t.expiresAt)
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

type apiconstraint struct {
	path       glob.Glob
	constraint *semver.Constraints
}

// restclient implements the RestClient interface.
type restclient struct {
	address      string
	prefix       string
	accessToken  Token
	refreshToken Token
	username     string
	password     string
	auth0Token   string
	client       HTTPClient
	about        api.About
	aboutLock    sync.RWMutex

	version struct {
		connectedCore *semver.Version
		methods       map[string][]apiconstraint
	}
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

	if len(config.AccessToken) != 0 {
		r.accessToken.Set(config.AccessToken)
	}

	if len(config.RefreshToken) != 0 {
		r.refreshToken.Set(config.RefreshToken)
	}

	u, err := url.Parse(r.address)
	if err == nil {
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
	}

	r.address = strings.TrimSuffix(r.address, "/")

	if r.client == nil {
		r.client = &http.Client{
			Timeout: 15 * time.Second,
		}
	}

	mustNewConstraint := func(constraint string) *semver.Constraints {
		v, _ := semver.NewConstraint(constraint)
		return v
	}

	mustNewGlob := func(pattern string) glob.Glob {
		return glob.MustCompile(pattern, '/')
	}

	r.version.methods = map[string][]apiconstraint{
		"GET": {
			{
				path:       mustNewGlob("/api/v3/srt"),
				constraint: mustNewConstraint("^16.9.0"),
			},
			{
				path:       mustNewGlob("/api/v3/metrics"),
				constraint: mustNewConstraint("^16.10.0"),
			},
			{
				path:       mustNewGlob("/api/v3/iam/user"),
				constraint: mustNewConstraint("^16.14.0"),
			},
			{
				path:       mustNewGlob("/v3/cluster"),
				constraint: mustNewConstraint("^16.14.0"),
			},
			{
				path:       mustNewGlob("/v3/cluster/healthy"),
				constraint: mustNewConstraint("^16.14.0"),
			},
			{
				path:       mustNewGlob("/v3/cluster/snapshot"),
				constraint: mustNewConstraint("^16.14.0"),
			},
			{
				path:       mustNewGlob("/v3/cluster/db/process"),
				constraint: mustNewConstraint("^16.14.0"),
			},
			{
				path:       mustNewGlob("/v3/cluster/db/process/*"),
				constraint: mustNewConstraint("^16.14.0"),
			},
			{
				path:       mustNewGlob("/v3/cluster/db/user"),
				constraint: mustNewConstraint("^16.14.0"),
			},
			{
				path:       mustNewGlob("/v3/cluster/db/user/*"),
				constraint: mustNewConstraint("^16.14.0"),
			},
			{
				path:       mustNewGlob("/v3/cluster/db/policies"),
				constraint: mustNewConstraint("^16.14.0"),
			},
			{
				path:       mustNewGlob("/v3/cluster/db/locks"),
				constraint: mustNewConstraint("^16.14.0"),
			},
			{
				path:       mustNewGlob("/v3/cluster/db/kv"),
				constraint: mustNewConstraint("^16.14.0"),
			},
			{
				path:       mustNewGlob("/v3/cluster/process"),
				constraint: mustNewConstraint("^16.14.0"),
			},
			{
				path:       mustNewGlob("/v3/cluster/process/*"),
				constraint: mustNewConstraint("^16.14.0"),
			},
			{
				path:       mustNewGlob("/v3/cluster/process/*/metadata/**"),
				constraint: mustNewConstraint("^16.14.0"),
			},
			{
				path:       mustNewGlob("/v3/cluster/iam/user"),
				constraint: mustNewConstraint("^16.14.0"),
			},
			{
				path:       mustNewGlob("/v3/cluster/iam/user/*"),
				constraint: mustNewConstraint("^16.14.0"),
			},
			{
				path:       mustNewGlob("/v3/cluster/process/*/probe"),
				constraint: mustNewConstraint("^16.14.0"),
			},
			{
				path:       mustNewGlob("/v3/cluster/node"),
				constraint: mustNewConstraint("^16.14.0"),
			},
			{
				path:       mustNewGlob("/v3/cluster/node/*"),
				constraint: mustNewConstraint("^16.14.0"),
			},
			{
				path:       mustNewGlob("/v3/cluster/node/*/files"),
				constraint: mustNewConstraint("^16.14.0"),
			},
			{
				path:       mustNewGlob("/v3/cluster/node/*/process"),
				constraint: mustNewConstraint("^16.14.0"),
			},
			{
				path:       mustNewGlob("/v3/cluster/node/*/version"),
				constraint: mustNewConstraint("^16.14.0"),
			},
		},
		"POST": {
			{
				path:       mustNewGlob("/api/v3/iam/user"),
				constraint: mustNewConstraint("^16.14.0"),
			},
			{
				path:       mustNewGlob("/v3/cluster/process"),
				constraint: mustNewConstraint("^16.14.0"),
			},
			{
				path:       mustNewGlob("/v3/cluster/iam/user"),
				constraint: mustNewConstraint("^16.14.0"),
			},
		},
		"PUT": {
			{
				path:       mustNewGlob("/api/v3/iam/user/*"),
				constraint: mustNewConstraint("^16.14.0"),
			},
			{
				path:       mustNewGlob("/api/v3/iam/user/*/policy"),
				constraint: mustNewConstraint("^16.14.0"),
			},
			{
				path:       mustNewGlob("/v3/cluster/leave"),
				constraint: mustNewConstraint("^16.14.0"),
			},
			{
				path:       mustNewGlob("/v3/cluster/process/*"),
				constraint: mustNewConstraint("^16.14.0"),
			},
			{
				path:       mustNewGlob("/v3/cluster/process/*/command"),
				constraint: mustNewConstraint("^16.14.0"),
			},
			{
				path:       mustNewGlob("/v3/cluster/process/*/metadata/**"),
				constraint: mustNewConstraint("^16.14.0"),
			},
			{
				path:       mustNewGlob("/v3/cluster/iam/user/*"),
				constraint: mustNewConstraint("^16.14.0"),
			},
			{
				path:       mustNewGlob("/v3/cluster/iam/user/*/policy"),
				constraint: mustNewConstraint("^16.14.0"),
			},
			{
				path:       mustNewGlob("/v3/cluster/iam/reload"),
				constraint: mustNewConstraint("^16.14.0"),
			},
			{
				path:       mustNewGlob("/v3/session/token/*"),
				constraint: mustNewConstraint("^16.14.0"),
			},
		},
		"DELETE": {
			{
				path:       mustNewGlob("/api/v3/iam/user/*"),
				constraint: mustNewConstraint("^16.14.0"),
			},
			{
				path:       mustNewGlob("/v3/cluster/process/{id}"),
				constraint: mustNewConstraint("^16.14.0"),
			},
			{
				path:       mustNewGlob("/v3/cluster/iam/user/{name}"),
				constraint: mustNewConstraint("^16.14.0"),
			},
		},
	}

	about, err := r.info()
	if err != nil {
		return nil, err
	}

	if about.App != coreapp {
		return nil, fmt.Errorf("didn't receive the expected API response (got: %s, want: %s)", about.Name, coreapp)
	}

	r.aboutLock.Lock()
	r.about = about
	r.aboutLock.Unlock()

	if len(about.ID) != 0 {
		c, _ := semver.NewConstraint(coreversion)
		v, err := semver.NewVersion(about.Version.Number)
		if err != nil {
			return nil, err
		}

		if !c.Check(v) {
			return nil, fmt.Errorf("the core version (%s) is not supported, because a version %s is required", about.Version.Number, coreversion)
		}

		r.aboutLock.Lock()
		r.version.connectedCore = v
		r.aboutLock.Unlock()
	} else {
		v, err := semver.NewVersion(about.Version.Number)
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

func (r *restclient) String() string {
	r.aboutLock.RLock()
	defer r.aboutLock.RUnlock()

	return fmt.Sprintf("%s %s (%s) %s @ %s", r.about.Name, r.about.Version.Number, r.about.Version.Arch, r.about.ID, r.address)
}

func (r *restclient) ID() string {
	r.aboutLock.RLock()
	defer r.aboutLock.RUnlock()

	return r.about.ID
}

func (r *restclient) Tokens() (string, string) {
	return r.accessToken.String(), r.refreshToken.String()
}

func (r *restclient) Address() string {
	return r.address
}

func (r *restclient) About(cached bool) (api.About, error) {
	if cached {
		return r.about, nil
	}

	about, err := r.info()
	if err != nil {
		return api.About{}, err
	}

	r.about = about

	return about, nil
}

func (r *restclient) Ping() (time.Duration, error) {
	req, err := http.NewRequest(http.MethodGet, r.address+"/ping", nil)
	if err != nil {
		return time.Duration(0), err
	}

	start := time.Now()

	status, body, err := r.request(req)
	if err != nil {
		return time.Duration(0), err
	}

	defer body.Close()

	if status != 200 {
		return time.Duration(0), err
	}

	io.ReadAll(body)

	return time.Since(start), nil
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

	r.accessToken.Set(jwt.AccessToken)
	r.refreshToken.Set(jwt.RefreshToken)

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

	r.aboutLock.Lock()
	r.version.connectedCore = v
	r.about = about
	r.aboutLock.Unlock()

	return nil
}

func (r *restclient) checkVersion(method, path string) error {
	constraints := r.version.methods[method]
	if constraints == nil {
		return nil
	}

	var c *semver.Constraints = nil

	for _, constraint := range constraints {
		if !constraint.path.Match(path) {
			continue
		}

		c = constraint.constraint
		break
	}

	if c == nil {
		return nil
	}

	r.aboutLock.RLock()
	defer r.aboutLock.RUnlock()

	if !c.Check(r.version.connectedCore) {
		return fmt.Errorf("this method is only available as of version %s of the core", c.String())
	}

	return nil
}

func (r *restclient) refresh() error {
	if r.refreshToken.IsExpired() {
		return fmt.Errorf("no valid refresh token available")
	}

	req, err := http.NewRequest("GET", r.address+r.prefix+"/login/refresh", nil)
	if err != nil {
		return err
	}

	req.Header.Add("Authorization", "Bearer "+r.refreshToken.String())

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

	r.accessToken.Set(jwt.AccessToken)

	return nil
}

func (r *restclient) info() (api.About, error) {
	req, err := http.NewRequest("GET", r.address+r.prefix, nil)
	if err != nil {
		return api.About{}, err
	}

	if r.accessToken.IsSet() && !r.accessToken.IsExpired() {
		req.Header.Add("Authorization", "Bearer "+r.accessToken.String())
	}

	status, body, _ := r.request(req)
	if status == http.StatusUnauthorized {
		req.Header.Del("Authorization")
		status, body, _ = r.request(req)
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

func (r *restclient) stream(method, path string, query *url.Values, header http.Header, contentType string, data io.Reader) (io.ReadCloser, error) {
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

	if header != nil {
		req.Header = header.Clone()
	}

	if method == "POST" || method == "PUT" {
		req.Header.Add("Content-Type", contentType)
	}

	if r.accessToken.IsSet() {
		if r.accessToken.IsExpired() {
			if err := r.refresh(); err != nil {
				if err := r.login(); err != nil {
					return nil, err
				}
			}
		}

		req.Header.Add("Authorization", "Bearer "+r.accessToken.String())
	}

	status, body, err := r.request(req)
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

func (r *restclient) call(method, path string, query *url.Values, header http.Header, contentType string, data io.Reader) ([]byte, error) {
	body, err := r.stream(method, path, query, header, contentType, data)
	if err != nil {
		return nil, err
	}

	defer body.Close()

	x, err := io.ReadAll(body)

	return x, err
}
