// Package config implements types for handling the configuation for the app.
package config

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/datarhei/core/v16/math/rand"

	haikunator "github.com/atrox/haikunatorgo/v2"
	"github.com/google/uuid"
)

type variable struct {
	value       value    // The actual value
	defVal      string   // The default value in string representation
	name        string   // A name for this value
	envName     string   // The environment variable that corresponds to this value
	envAltNames []string // Alternative environment variable names
	description string   // A desriptions for this value
	required    bool     // Whether a non-empty value is required
	disguise    bool     // Whether the value should be disguised if printed
	merged      bool     // Whether this value has been replaced by its corresponding environment variable
}

type Variable struct {
	Value       string
	Name        string
	EnvName     string
	Description string
	Merged      bool
}

type message struct {
	message  string   // The log message
	variable Variable // The config field this message refers to
	level    string   // The loglevel for this message
}

type Auth0Tenant struct {
	Domain   string   `json:"domain"`
	Audience string   `json:"audience"`
	ClientID string   `json:"clientid"`
	Users    []string `json:"users"`
}

// Data is the actual configuration data for the app
type Data struct {
	CreatedAt       time.Time `json:"created_at"`
	LoadedAt        time.Time `json:"-"`
	UpdatedAt       time.Time `json:"-"`
	Version         int64     `json:"version" jsonschema:"minimum=1,maximum=1"`
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Address         string    `json:"address"`
	CheckForUpdates bool      `json:"update_check"`
	Log             struct {
		Level    string   `json:"level" enums:"debug,info,warn,error,silent" jsonschema:"enum=debug,enum=info,enum=warn,enum=error,enum=silent"`
		Topics   []string `json:"topics"`
		MaxLines int      `json:"max_lines"`
	} `json:"log"`
	DB struct {
		Dir string `json:"dir"`
	} `json:"db"`
	Host struct {
		Name []string `json:"name"`
		Auto bool     `json:"auto"`
	} `json:"host"`
	API struct {
		ReadOnly bool `json:"read_only"`
		Access   struct {
			HTTP struct {
				Allow []string `json:"allow"`
				Block []string `json:"block"`
			} `json:"http"`
			HTTPS struct {
				Allow []string `json:"allow"`
				Block []string `json:"block"`
			} `json:"https"`
		} `json:"access"`
		Auth struct {
			Enable           bool   `json:"enable"`
			DisableLocalhost bool   `json:"disable_localhost"`
			Username         string `json:"username"`
			Password         string `json:"password"`
			JWT              struct {
				Secret string `json:"secret"`
			} `json:"jwt"`
			Auth0 struct {
				Enable  bool          `json:"enable"`
				Tenants []Auth0Tenant `json:"tenants"`
			} `json:"auth0"`
		} `json:"auth"`
	} `json:"api"`
	TLS struct {
		Address  string `json:"address"`
		Enable   bool   `json:"enable"`
		Auto     bool   `json:"auto"`
		CertFile string `json:"cert_file"`
		KeyFile  string `json:"key_file"`
	} `json:"tls"`
	Storage struct {
		Disk struct {
			Dir   string `json:"dir"`
			Size  int64  `json:"max_size_mbytes"`
			Cache struct {
				Enable   bool     `json:"enable"`
				Size     uint64   `json:"max_size_mbytes"`
				TTL      int64    `json:"ttl_seconds"`
				FileSize uint64   `json:"max_file_size_mbytes"`
				Types    []string `json:"types"`
			} `json:"cache"`
		} `json:"disk"`
		Memory struct {
			Auth struct {
				Enable   bool   `json:"enable"`
				Username string `json:"username"`
				Password string `json:"password"`
			} `json:"auth"`
			Size  int64 `json:"max_size_mbytes"`
			Purge bool  `json:"purge"`
		} `json:"memory"`
		CORS struct {
			Origins []string `json:"origins"`
		} `json:"cors"`
		MimeTypes string `json:"mimetypes_file"`
	} `json:"storage"`
	RTMP struct {
		Enable     bool   `json:"enable"`
		EnableTLS  bool   `json:"enable_tls"`
		Address    string `json:"address"`
		AddressTLS string `json:"address_tls"`
		App        string `json:"app"`
		Token      string `json:"token"`
	} `json:"rtmp"`
	SRT struct {
		Enable     bool   `json:"enable"`
		Address    string `json:"address"`
		Passphrase string `json:"passphrase"`
		Token      string `json:"token"`
		Log        struct {
			Enable bool     `json:"enable"`
			Topics []string `json:"topics"`
		} `json:"log"`
	} `json:"srt"`
	FFmpeg struct {
		Binary       string `json:"binary"`
		MaxProcesses int64  `json:"max_processes"`
		Access       struct {
			Input struct {
				Allow []string `json:"allow"`
				Block []string `json:"block"`
			} `json:"input"`
			Output struct {
				Allow []string `json:"allow"`
				Block []string `json:"block"`
			} `json:"output"`
		} `json:"access"`
		Log struct {
			MaxLines   int `json:"max_lines"`
			MaxHistory int `json:"max_history"`
		} `json:"log"`
	} `json:"ffmpeg"`
	Playout struct {
		Enable  bool `json:"enable"`
		MinPort int  `json:"min_port"`
		MaxPort int  `json:"max_port"`
	} `json:"playout"`
	Debug struct {
		Profiling bool `json:"profiling"`
		ForceGC   int  `json:"force_gc"`
	} `json:"debug"`
	Metrics struct {
		Enable           bool  `json:"enable"`
		EnablePrometheus bool  `json:"enable_prometheus"`
		Range            int64 `json:"range_sec"`    // seconds
		Interval         int64 `json:"interval_sec"` // seconds
	} `json:"metrics"`
	Sessions struct {
		Enable          bool     `json:"enable"`
		IPIgnoreList    []string `json:"ip_ignorelist"`
		SessionTimeout  int      `json:"session_timeout_sec"`
		Persist         bool     `json:"persist"`
		PersistInterval int      `json:"persist_interval_sec"`
		MaxBitrate      uint64   `json:"max_bitrate_mbit"`
		MaxSessions     uint64   `json:"max_sessions"`
	} `json:"sessions"`
	Service struct {
		Enable bool   `json:"enable"`
		Token  string `json:"token"`
		URL    string `json:"url"`
	} `json:"service"`
	Router struct {
		BlockedPrefixes []string          `json:"blocked_prefixes"`
		Routes          map[string]string `json:"routes"`
		UIPath          string            `json:"ui_path"`
	} `json:"router"`
}

// Config is a wrapper for Data
type Config struct {
	vars []*variable
	logs []message

	Data
}

// New returns a Config which is initialized with its default values
func New() *Config {
	data := &Config{}

	data.init()

	return data
}

// NewConfigFrom returns a clone of a Config
func NewConfigFrom(d *Config) *Config {
	data := New()

	data.CreatedAt = d.CreatedAt
	data.LoadedAt = d.LoadedAt
	data.UpdatedAt = d.UpdatedAt

	data.Version = d.Version
	data.ID = d.ID
	data.Name = d.Name
	data.Address = d.Address
	data.CheckForUpdates = d.CheckForUpdates

	data.Log = d.Log
	data.DB = d.DB
	data.Host = d.Host
	data.API = d.API
	data.TLS = d.TLS
	data.Storage = d.Storage
	data.RTMP = d.RTMP
	data.SRT = d.SRT
	data.FFmpeg = d.FFmpeg
	data.Playout = d.Playout
	data.Debug = d.Debug
	data.Metrics = d.Metrics
	data.Sessions = d.Sessions
	data.Service = d.Service
	data.Router = d.Router

	data.Log.Topics = copyStringSlice(d.Log.Topics)

	data.Host.Name = copyStringSlice(d.Host.Name)

	data.API.Access.HTTP.Allow = copyStringSlice(d.API.Access.HTTP.Allow)
	data.API.Access.HTTP.Block = copyStringSlice(d.API.Access.HTTP.Block)
	data.API.Access.HTTPS.Allow = copyStringSlice(d.API.Access.HTTPS.Allow)
	data.API.Access.HTTPS.Block = copyStringSlice(d.API.Access.HTTPS.Block)

	data.API.Auth.Auth0.Tenants = copyTenantSlice(d.API.Auth.Auth0.Tenants)

	data.Storage.CORS.Origins = copyStringSlice(d.Storage.CORS.Origins)

	data.FFmpeg.Access.Input.Allow = copyStringSlice(d.FFmpeg.Access.Input.Allow)
	data.FFmpeg.Access.Input.Block = copyStringSlice(d.FFmpeg.Access.Input.Block)
	data.FFmpeg.Access.Output.Allow = copyStringSlice(d.FFmpeg.Access.Output.Allow)
	data.FFmpeg.Access.Output.Block = copyStringSlice(d.FFmpeg.Access.Output.Block)

	data.Sessions.IPIgnoreList = copyStringSlice(d.Sessions.IPIgnoreList)

	data.SRT.Log.Topics = copyStringSlice(d.SRT.Log.Topics)

	data.Router.BlockedPrefixes = copyStringSlice(d.Router.BlockedPrefixes)
	data.Router.Routes = copyStringMap(d.Router.Routes)

	for i, v := range d.vars {
		data.vars[i].merged = v.merged
	}

	return data
}

func (d *Config) init() {
	d.val(newInt64Value(&d.Version, 1), "version", "", nil, "Configuration file layout version", true, false)
	d.val(newTimeValue(&d.CreatedAt, time.Now()), "created_at", "", nil, "Configuration file creation time", false, false)
	d.val(newStringValue(&d.ID, uuid.New().String()), "id", "CORE_ID", nil, "ID for this instance", true, false)
	d.val(newStringValue(&d.Name, haikunator.New().Haikunate()), "name", "CORE_NAME", nil, "A human readable name for this instance", false, false)
	d.val(newAddressValue(&d.Address, ":8080"), "address", "CORE_ADDRESS", nil, "HTTP listening address", false, false)
	d.val(newBoolValue(&d.CheckForUpdates, true), "update_check", "CORE_UPDATE_CHECK", nil, "Check for updates and send anonymized data", false, false)

	// Log
	d.val(newStringValue(&d.Log.Level, "info"), "log.level", "CORE_LOG_LEVEL", nil, "Loglevel: silent, error, warn, info, debug", false, false)
	d.val(newStringListValue(&d.Log.Topics, []string{}, ","), "log.topics", "CORE_LOG_TOPICS", nil, "Show only selected log topics", false, false)
	d.val(newIntValue(&d.Log.MaxLines, 1000), "log.max_lines", "CORE_LOG_MAXLINES", nil, "Number of latest log lines to keep in memory", false, false)

	// DB
	d.val(newMustDirValue(&d.DB.Dir, "./config"), "db.dir", "CORE_DB_DIR", nil, "Directory for holding the operational data", false, false)

	// Host
	d.val(newStringListValue(&d.Host.Name, []string{}, ","), "host.name", "CORE_HOST_NAME", nil, "Comma separated list of public host/domain names or IPs", false, false)
	d.val(newBoolValue(&d.Host.Auto, true), "host.auto", "CORE_HOST_AUTO", nil, "Enable detection of public IP addresses", false, false)

	// API
	d.val(newBoolValue(&d.API.ReadOnly, false), "api.read_only", "CORE_API_READ_ONLY", nil, "Allow only ready only access to the API", false, false)
	d.val(newCIDRListValue(&d.API.Access.HTTP.Allow, []string{}, ","), "api.access.http.allow", "CORE_API_ACCESS_HTTP_ALLOW", nil, "List of IPs in CIDR notation (HTTP traffic)", false, false)
	d.val(newCIDRListValue(&d.API.Access.HTTP.Block, []string{}, ","), "api.access.http.block", "CORE_API_ACCESS_HTTP_BLOCK", nil, "List of IPs in CIDR notation (HTTP traffic)", false, false)
	d.val(newCIDRListValue(&d.API.Access.HTTPS.Allow, []string{}, ","), "api.access.https.allow", "CORE_API_ACCESS_HTTPS_ALLOW", nil, "List of IPs in CIDR notation (HTTPS traffic)", false, false)
	d.val(newCIDRListValue(&d.API.Access.HTTPS.Block, []string{}, ","), "api.access.https.block", "CORE_API_ACCESS_HTTPS_BLOCK", nil, "List of IPs in CIDR notation (HTTPS traffic)", false, false)
	d.val(newBoolValue(&d.API.Auth.Enable, false), "api.auth.enable", "CORE_API_AUTH_ENABLE", nil, "Enable authentication for all clients", false, false)
	d.val(newBoolValue(&d.API.Auth.DisableLocalhost, false), "api.auth.disable_localhost", "CORE_API_AUTH_DISABLE_LOCALHOST", nil, "Disable authentication for clients from localhost", false, false)
	d.val(newStringValue(&d.API.Auth.Username, ""), "api.auth.username", "CORE_API_AUTH_USERNAME", []string{"RS_USERNAME"}, "Username", false, false)
	d.val(newStringValue(&d.API.Auth.Password, ""), "api.auth.password", "CORE_API_AUTH_PASSWORD", []string{"RS_PASSWORD"}, "Password", false, true)

	// Auth JWT
	d.val(newStringValue(&d.API.Auth.JWT.Secret, rand.String(32)), "api.auth.jwt.secret", "CORE_API_AUTH_JWT_SECRET", nil, "JWT secret, leave empty for generating a random value", false, true)

	// Auth Auth0
	d.val(newBoolValue(&d.API.Auth.Auth0.Enable, false), "api.auth.auth0.enable", "CORE_API_AUTH_AUTH0_ENABLE", nil, "Enable Auth0", false, false)
	d.val(newTenantListValue(&d.API.Auth.Auth0.Tenants, []Auth0Tenant{}, ","), "api.auth.auth0.tenants", "CORE_API_AUTH_AUTH0_TENANTS", nil, "List of Auth0 tenants", false, false)

	// TLS
	d.val(newAddressValue(&d.TLS.Address, ":8181"), "tls.address", "CORE_TLS_ADDRESS", nil, "HTTPS listening address", false, false)
	d.val(newBoolValue(&d.TLS.Enable, false), "tls.enable", "CORE_TLS_ENABLE", nil, "Enable HTTPS", false, false)
	d.val(newBoolValue(&d.TLS.Auto, false), "tls.auto", "CORE_TLS_AUTO", nil, "Enable Let's Encrypt certificate", false, false)
	d.val(newFileValue(&d.TLS.CertFile, ""), "tls.cert_file", "CORE_TLS_CERTFILE", nil, "Path to certificate file in PEM format", false, false)
	d.val(newFileValue(&d.TLS.KeyFile, ""), "tls.key_file", "CORE_TLS_KEYFILE", nil, "Path to key file in PEM format", false, false)

	// Storage
	d.val(newFileValue(&d.Storage.MimeTypes, "./mime.types"), "storage.mimetypes_file", "CORE_STORAGE_MIMETYPES_FILE", []string{"CORE_MIMETYPES_FILE"}, "Path to file with mime-types", false, false)

	// Storage (Disk)
	d.val(newMustDirValue(&d.Storage.Disk.Dir, "./data"), "storage.disk.dir", "CORE_STORAGE_DISK_DIR", nil, "Directory on disk, exposed on /", false, false)
	d.val(newInt64Value(&d.Storage.Disk.Size, 0), "storage.disk.max_size_mbytes", "CORE_STORAGE_DISK_MAXSIZEMBYTES", nil, "Max. allowed megabytes for storage.disk.dir, 0 for unlimited", false, false)
	d.val(newBoolValue(&d.Storage.Disk.Cache.Enable, true), "storage.disk.cache.enable", "CORE_STORAGE_DISK_CACHE_ENABLE", nil, "Enable cache for /", false, false)
	d.val(newUint64Value(&d.Storage.Disk.Cache.Size, 0), "storage.disk.cache.max_size_mbytes", "CORE_STORAGE_DISK_CACHE_MAXSIZEMBYTES", nil, "Max. allowed cache size, 0 for unlimited", false, false)
	d.val(newInt64Value(&d.Storage.Disk.Cache.TTL, 300), "storage.disk.cache.ttl_seconds", "CORE_STORAGE_DISK_CACHE_TTLSECONDS", nil, "Seconds to keep files in cache", false, false)
	d.val(newUint64Value(&d.Storage.Disk.Cache.FileSize, 1), "storage.disk.cache.max_file_size_mbytes", "CORE_STORAGE_DISK_CACHE_MAXFILESIZEMBYTES", nil, "Max. file size to put in cache", false, false)
	d.val(newStringListValue(&d.Storage.Disk.Cache.Types, []string{}, " "), "storage.disk.cache.types", "CORE_STORAGE_DISK_CACHE_TYPES", nil, "File extensions to cache, empty for all", false, false)

	// Storage (Memory)
	d.val(newBoolValue(&d.Storage.Memory.Auth.Enable, true), "storage.memory.auth.enable", "CORE_STORAGE_MEMORY_AUTH_ENABLE", nil, "Enable basic auth for PUT,POST, and DELETE on /memfs", false, false)
	d.val(newStringValue(&d.Storage.Memory.Auth.Username, "admin"), "storage.memory.auth.username", "CORE_STORAGE_MEMORY_AUTH_USERNAME", nil, "Username for Basic-Auth of /memfs", false, false)
	d.val(newStringValue(&d.Storage.Memory.Auth.Password, rand.StringAlphanumeric(18)), "storage.memory.auth.password", "CORE_STORAGE_MEMORY_AUTH_PASSWORD", nil, "Password for Basic-Auth of /memfs", false, true)
	d.val(newInt64Value(&d.Storage.Memory.Size, 0), "storage.memory.max_size_mbytes", "CORE_STORAGE_MEMORY_MAXSIZEMBYTES", nil, "Max. allowed megabytes for /memfs, 0 for unlimited", false, false)
	d.val(newBoolValue(&d.Storage.Memory.Purge, false), "storage.memory.purge", "CORE_STORAGE_MEMORY_PURGE", nil, "Automatically remove the oldest files if /memfs is full", false, false)

	// Storage (CORS)
	d.val(newCORSOriginsValue(&d.Storage.CORS.Origins, []string{"*"}, ","), "storage.cors.origins", "CORE_STORAGE_CORS_ORIGINS", nil, "Allowed CORS origins for /memfs and /data", false, false)

	// RTMP
	d.val(newBoolValue(&d.RTMP.Enable, false), "rtmp.enable", "CORE_RTMP_ENABLE", nil, "Enable RTMP server", false, false)
	d.val(newBoolValue(&d.RTMP.EnableTLS, false), "rtmp.enable_tls", "CORE_RTMP_ENABLE_TLS", nil, "Enable RTMPS server instead of RTMP", false, false)
	d.val(newAddressValue(&d.RTMP.Address, ":1935"), "rtmp.address", "CORE_RTMP_ADDRESS", nil, "RTMP server listen address", false, false)
	d.val(newAddressValue(&d.RTMP.AddressTLS, ":1936"), "rtmp.address_tls", "CORE_RTMP_ADDRESS_TLS", nil, "RTMPS server listen address", false, false)
	d.val(newStringValue(&d.RTMP.App, "/"), "rtmp.app", "CORE_RTMP_APP", nil, "RTMP app for publishing", false, false)
	d.val(newStringValue(&d.RTMP.Token, ""), "rtmp.token", "CORE_RTMP_TOKEN", nil, "RTMP token for publishing and playing", false, true)

	// SRT
	d.val(newBoolValue(&d.SRT.Enable, false), "srt.enable", "CORE_SRT_ENABLE", nil, "Enable SRT server", false, false)
	d.val(newAddressValue(&d.SRT.Address, ":6000"), "srt.address", "CORE_SRT_ADDRESS", nil, "SRT server listen address", false, false)
	d.val(newStringValue(&d.SRT.Passphrase, ""), "srt.passphrase", "CORE_SRT_PASSPHRASE", nil, "SRT encryption passphrase", false, true)
	d.val(newStringValue(&d.SRT.Token, ""), "srt.token", "CORE_SRT_TOKEN", nil, "SRT token for publishing and playing", false, true)
	d.val(newBoolValue(&d.SRT.Log.Enable, false), "srt.log.enable", "CORE_SRT_LOG_ENABLE", nil, "Enable SRT server logging", false, false)
	d.val(newStringListValue(&d.SRT.Log.Topics, []string{}, ","), "srt.log.topics", "CORE_SRT_LOG_TOPICS", nil, "List of topics to log", false, false)

	// FFmpeg
	d.val(newExecValue(&d.FFmpeg.Binary, "ffmpeg"), "ffmpeg.binary", "CORE_FFMPEG_BINARY", nil, "Path to ffmpeg binary", true, false)
	d.val(newInt64Value(&d.FFmpeg.MaxProcesses, 0), "ffmpeg.max_processes", "CORE_FFMPEG_MAXPROCESSES", nil, "Max. allowed simultaneously running ffmpeg instances, 0 for unlimited", false, false)
	d.val(newStringListValue(&d.FFmpeg.Access.Input.Allow, []string{}, " "), "ffmpeg.access.input.allow", "CORE_FFMPEG_ACCESS_INPUT_ALLOW", nil, "List of allowed expression to match against the input addresses", false, false)
	d.val(newStringListValue(&d.FFmpeg.Access.Input.Block, []string{}, " "), "ffmpeg.access.input.block", "CORE_FFMPEG_ACCESS_INPUT_BLOCK", nil, "List of blocked expression to match against the input addresses", false, false)
	d.val(newStringListValue(&d.FFmpeg.Access.Output.Allow, []string{}, " "), "ffmpeg.access.output.allow", "CORE_FFMPEG_ACCESS_OUTPUT_ALLOW", nil, "List of allowed expression to match against the output addresses", false, false)
	d.val(newStringListValue(&d.FFmpeg.Access.Output.Block, []string{}, " "), "ffmpeg.access.output.block", "CORE_FFMPEG_ACCESS_OUTPUT_BLOCK", nil, "List of blocked expression to match against the output addresses", false, false)
	d.val(newIntValue(&d.FFmpeg.Log.MaxLines, 50), "ffmpeg.log.max_lines", "CORE_FFMPEG_LOG_MAXLINES", nil, "Number of latest log lines to keep for each process", false, false)
	d.val(newIntValue(&d.FFmpeg.Log.MaxHistory, 3), "ffmpeg.log.max_history", "CORE_FFMPEG_LOG_MAXHISTORY", nil, "Number of latest logs to keep for each process", false, false)

	// Playout
	d.val(newBoolValue(&d.Playout.Enable, false), "playout.enable", "CORE_PLAYOUT_ENABLE", nil, "Enable playout proxy where available", false, false)
	d.val(newPortValue(&d.Playout.MinPort, 0), "playout.min_port", "CORE_PLAYOUT_MINPORT", nil, "Min. playout server port", false, false)
	d.val(newPortValue(&d.Playout.MaxPort, 0), "playout.max_port", "CORE_PLAYOUT_MAXPORT", nil, "Max. playout server port", false, false)

	// Debug
	d.val(newBoolValue(&d.Debug.Profiling, false), "debug.profiling", "CORE_DEBUG_PROFILING", nil, "Enable profiling endpoint on /profiling", false, false)
	d.val(newIntValue(&d.Debug.ForceGC, 0), "debug.force_gc", "CORE_DEBUG_FORCEGC", nil, "Number of seconds between forcing GC to return memory to the OS", false, false)

	// Metrics
	d.val(newBoolValue(&d.Metrics.Enable, false), "metrics.enable", "CORE_METRICS_ENABLE", nil, "Enable collecting historic metrics data", false, false)
	d.val(newBoolValue(&d.Metrics.EnablePrometheus, false), "metrics.enable_prometheus", "CORE_METRICS_ENABLE_PROMETHEUS", nil, "Enable prometheus endpoint /metrics", false, false)
	d.val(newInt64Value(&d.Metrics.Range, 300), "metrics.range_seconds", "CORE_METRICS_RANGE_SECONDS", nil, "Seconds to keep history data", false, false)
	d.val(newInt64Value(&d.Metrics.Interval, 2), "metrics.interval_seconds", "CORE_METRICS_INTERVAL_SECONDS", nil, "Interval for collecting metrics", false, false)

	// Sessions
	d.val(newBoolValue(&d.Sessions.Enable, true), "sessions.enable", "CORE_SESSIONS_ENABLE", nil, "Enable collecting HLS session stats for /memfs", false, false)
	d.val(newCIDRListValue(&d.Sessions.IPIgnoreList, []string{"127.0.0.1/32", "::1/128"}, ","), "sessions.ip_ignorelist", "CORE_SESSIONS_IP_IGNORELIST", nil, "List of IP ranges in CIDR notation to ignore", false, false)
	d.val(newIntValue(&d.Sessions.SessionTimeout, 30), "sessions.session_timeout_sec", "CORE_SESSIONS_SESSION_TIMEOUT_SEC", nil, "Timeout for an idle session", false, false)
	d.val(newBoolValue(&d.Sessions.Persist, false), "sessions.persist", "CORE_SESSIONS_PERSIST", nil, "Whether to persist session history. Will be stored as sessions.json in db.dir", false, false)
	d.val(newIntValue(&d.Sessions.PersistInterval, 300), "sessions.persist_interval_sec", "CORE_SESSIONS_PERSIST_INTERVAL_SEC", nil, "Interval in seconds in which to persist the current session history", false, false)
	d.val(newUint64Value(&d.Sessions.MaxBitrate, 0), "sessions.max_bitrate_mbit", "CORE_SESSIONS_MAXBITRATE_MBIT", nil, "Max. allowed outgoing bitrate in mbit/s, 0 for unlimited", false, false)
	d.val(newUint64Value(&d.Sessions.MaxSessions, 0), "sessions.max_sessions", "CORE_SESSIONS_MAXSESSIONS", nil, "Max. allowed number of simultaneous sessions, 0 for unlimited", false, false)

	// Service
	d.val(newBoolValue(&d.Service.Enable, false), "service.enable", "CORE_SERVICE_ENABLE", nil, "Enable connecting to the Restreamer Service", false, false)
	d.val(newStringValue(&d.Service.Token, ""), "service.token", "CORE_SERVICE_TOKEN", nil, "Restreamer Service account token", false, true)
	d.val(newURLValue(&d.Service.URL, "https://service.datarhei.com"), "service.url", "CORE_SERVICE_URL", nil, "URL of the Restreamer Service", false, false)

	// Router
	d.val(newStringListValue(&d.Router.BlockedPrefixes, []string{"/api"}, ","), "router.blocked_prefixes", "CORE_ROUTER_BLOCKED_PREFIXES", nil, "List of path prefixes that can't be routed", false, false)
	d.val(newStringMapStringValue(&d.Router.Routes, nil), "router.routes", "CORE_ROUTER_ROUTES", nil, "List of route mappings", false, false)
	d.val(newDirValue(&d.Router.UIPath, ""), "router.ui_path", "CORE_ROUTER_UI_PATH", nil, "Path to a directory holding UI files mounted as /ui", false, false)
}

func (d *Config) val(val value, name, envName string, envAltNames []string, description string, required, disguise bool) {
	d.vars = append(d.vars, &variable{
		value:       val,
		defVal:      val.String(),
		name:        name,
		envName:     envName,
		envAltNames: envAltNames,
		description: description,
		required:    required,
		disguise:    disguise,
	})
}

func (d *Config) log(level string, v *variable, format string, args ...interface{}) {
	variable := Variable{
		Value:       v.value.String(),
		Name:        v.name,
		EnvName:     v.envName,
		Description: v.description,
		Merged:      v.merged,
	}

	if v.disguise {
		variable.Value = "***"
	}

	l := message{
		message:  fmt.Sprintf(format, args...),
		variable: variable,
		level:    level,
	}

	d.logs = append(d.logs, l)
}

// Merge merges the values of the known environment variables into the configuration
func (d *Config) Merge() {
	for _, v := range d.vars {
		if len(v.envName) == 0 {
			continue
		}

		var envval string
		var ok bool

		envval, ok = os.LookupEnv(v.envName)
		if !ok {
			foundAltName := false

			for _, envName := range v.envAltNames {
				envval, ok = os.LookupEnv(envName)
				if ok {
					foundAltName = true
					d.log("warn", v, "deprecated name, please use %s", v.envName)
					break
				}
			}

			if !foundAltName {
				continue
			}
		}

		err := v.value.Set(envval)
		if err != nil {
			d.log("error", v, "%s", err.Error())
		}

		v.merged = true
	}
}

// Validate validates the current state of the Config for completeness and sanity. Errors are
// written to the log. Use resetLogs to indicate to reset the logs prior validation.
func (d *Config) Validate(resetLogs bool) {
	if resetLogs {
		d.logs = nil
	}

	if d.Version != 1 {
		d.log("error", d.findVariable("version"), "unknown configuration layout version")

		return
	}

	for _, v := range d.vars {
		d.log("info", v, "%s", "")

		err := v.value.Validate()
		if err != nil {
			d.log("error", v, "%s", err.Error())
		}

		if v.required && v.value.IsEmpty() {
			d.log("error", v, "a value is required")
		}
	}

	// Individual sanity checks

	// If HTTP Auth is enabled, check that the username and password are set
	if d.API.Auth.Enable {
		if len(d.API.Auth.Username) == 0 || len(d.API.Auth.Password) == 0 {
			d.log("error", d.findVariable("api.auth.enable"), "api.auth.username and api.auth.password must be set")
		}
	}

	// If Auth0 is enabled, check that domain, audience, and clientid are set
	if d.API.Auth.Auth0.Enable {
		if len(d.API.Auth.Auth0.Tenants) == 0 {
			d.log("error", d.findVariable("api.auth.auth0.enable"), "at least one tenants must be set")
		}

		for i, t := range d.API.Auth.Auth0.Tenants {
			if len(t.Domain) == 0 || len(t.Audience) == 0 || len(t.ClientID) == 0 {
				d.log("error", d.findVariable("api.auth.auth0.tenants"), "domain, audience, and clientid must be set (tenant %d)", i)
			}
		}
	}

	// If TLS is enabled and Let's Encrypt is disabled, require certfile and keyfile
	if d.TLS.Enable && !d.TLS.Auto {
		if len(d.TLS.CertFile) == 0 || len(d.TLS.KeyFile) == 0 {
			d.log("error", d.findVariable("tls.enable"), "tls.certfile and tls.keyfile must be set")
		}
	}

	// If TLS and Let's Encrypt certificate is enabled, we require a public hostname
	if d.TLS.Enable && d.TLS.Auto {
		if len(d.Host.Name) == 0 {
			d.log("error", d.findVariable("host.name"), "a hostname must be set in order to get an automatic TLS certificate")
		} else {
			r := &net.Resolver{
				PreferGo:     true,
				StrictErrors: true,
			}

			for _, host := range d.Host.Name {
				// Don't lookup IP addresses
				if ip := net.ParseIP(host); ip != nil {
					d.log("error", d.findVariable("host.name"), "only host names are allowed if automatic TLS is enabled, but found IP address: %s", host)
				}

				// Lookup host name with a timeout
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

				_, err := r.LookupHost(ctx, host)
				if err != nil {
					d.log("error", d.findVariable("host.name"), "the host '%s' can't be resolved and will not work with automatic TLS", host)
				}

				cancel()
			}
		}
	}

	// If TLS for RTMP is enabled, TLS must be enabled
	if d.RTMP.EnableTLS {
		if !d.TLS.Enable {
			d.log("error", d.findVariable("rtmp.enable_tls"), "RTMPS server can only be enabled if TLS is enabled")
		}
	}

	// If CORE_MEMFS_USERNAME and CORE_MEMFS_PASSWORD are set, automatically active/deactivate Basic-Auth for memfs
	if d.findVariable("storage.memory.auth.username").merged && d.findVariable("storage.memory.auth.password").merged {
		d.Storage.Memory.Auth.Enable = true

		if len(d.Storage.Memory.Auth.Username) == 0 && len(d.Storage.Memory.Auth.Password) == 0 {
			d.Storage.Memory.Auth.Enable = false
		}
	}

	// If Basic-Auth for memfs is enable, check that the username and password are set
	if d.Storage.Memory.Auth.Enable {
		if len(d.Storage.Memory.Auth.Username) == 0 || len(d.Storage.Memory.Auth.Password) == 0 {
			d.log("error", d.findVariable("storage.memory.auth.enable"), "storage.memory.auth.username and storage.memory.auth.password must be set")
		}
	}

	// If playout is enabled, check that the port range is sane
	if d.Playout.Enable {
		if d.Playout.MinPort >= d.Playout.MaxPort {
			d.log("error", d.findVariable("playout.min_port"), "must be bigger than playout.max_port")
		}
	}

	// If cache is enabled, a valid TTL has to be set to a useful value
	if d.Storage.Disk.Cache.Enable && d.Storage.Disk.Cache.TTL < 0 {
		d.log("error", d.findVariable("storage.disk.cache.ttl_seconds"), "must be equal or greater than 0")
	}

	// If the stats are enabled, the session timeout has to be set to a useful value
	if d.Sessions.Enable && d.Sessions.SessionTimeout < 1 {
		d.log("error", d.findVariable("stats.session_timeout_sec"), "must be equal or greater than 1")
	}

	// If the stats and their persistence are enabled, the persist interval has to be set to a useful value
	if d.Sessions.Enable && d.Sessions.PersistInterval < 0 {
		d.log("error", d.findVariable("stats.persist_interval_sec"), "must be at equal or greater than 0")
	}

	// If the service is enabled, the token and enpoint have to be defined
	if d.Service.Enable {
		if len(d.Service.Token) == 0 {
			d.log("error", d.findVariable("service.token"), "must be non-empty")
		}

		if len(d.Service.URL) == 0 {
			d.log("error", d.findVariable("service.url"), "must be non-empty")
		}
	}

	// If historic metrics are enabled, the timerange and interval have to be valid
	if d.Metrics.Enable {
		if d.Metrics.Range <= 0 {
			d.log("error", d.findVariable("metrics.range"), "must be greater 0")
		}

		if d.Metrics.Interval <= 0 {
			d.log("error", d.findVariable("metrics.interval"), "must be greater 0")
		}

		if d.Metrics.Interval > d.Metrics.Range {
			d.log("error", d.findVariable("metrics.interval"), "must be smaller than the range")
		}
	}
}

func (d *Config) findVariable(name string) *variable {
	for _, v := range d.vars {
		if v.name == name {
			return v
		}
	}

	return nil
}

// Messages calls for each log entry the provided callback. The level has the values 'error', 'warn', or 'info'.
// The name is the name of the configuration value, e.g. 'api.auth.enable'. The message is the log message.
func (d *Config) Messages(logger func(level string, v Variable, message string)) {
	for _, l := range d.logs {
		logger(l.level, l.variable, l.message)
	}
}

// HasErrors returns whether there are some error messages in the log.
func (d *Config) HasErrors() bool {
	for _, l := range d.logs {
		if l.level == "error" {
			return true
		}
	}

	return false
}

// Overrides returns a list of configuration value names that have been overriden by an environment variable.
func (d *Config) Overrides() []string {
	overrides := []string{}

	for _, v := range d.vars {
		if v.merged {
			overrides = append(overrides, v.name)
		}
	}

	return overrides
}

func copyStringSlice(src []string) []string {
	dst := make([]string, len(src))
	copy(dst, src)

	return dst
}

func copyStringMap(src map[string]string) map[string]string {
	dst := make(map[string]string)

	for k, v := range src {
		dst[k] = v
	}

	return dst
}

func copyTenantSlice(src []Auth0Tenant) []Auth0Tenant {
	dst := make([]Auth0Tenant, len(src))
	copy(dst, src)

	return dst
}
