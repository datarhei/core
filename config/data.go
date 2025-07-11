package config

import (
	"time"

	"github.com/datarhei/core/v16/config/copy"
	v2 "github.com/datarhei/core/v16/config/v2"
	"github.com/datarhei/core/v16/config/value"
	"github.com/datarhei/core/v16/io/fs"
	"github.com/datarhei/core/v16/slices"
)

// Data is the actual configuration data for the app
type Data struct {
	CreatedAt       time.Time `json:"created_at"` // When this config has been persisted
	LoadedAt        time.Time `json:"-"`          // When this config has been actually used
	UpdatedAt       time.Time `json:"-"`          // Irrelevant
	Version         int64     `json:"version" jsonschema:"minimum=3,maximum=3" format:"int64"`
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Address         string    `json:"address"`
	CheckForUpdates bool      `json:"update_check"`
	Log             struct {
		Level    string   `json:"level" enums:"debug,info,warn,error,silent" jsonschema:"enum=debug,enum=info,enum=warn,enum=error,enum=silent"`
		Topics   []string `json:"topics"`
		MaxLines int      `json:"max_lines" format:"int"`
	} `json:"log"`
	DB struct {
		Dir string `json:"dir"`
	} `json:"db"`
	Host struct {
		Name []string `json:"name"`
		Auto bool     `json:"auto"`
	} `json:"host"`
	Compress struct {
		Encoding  []string `json:"encoding"`
		MimeTypes []string `json:"mimetypes"`
		MinLength int      `json:"min_length" jsonschema:"minimum=0"`
	} `json:"compress"`
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
				Enable  bool                `json:"enable"`
				Tenants []value.Auth0Tenant `json:"tenants"`
			} `json:"auth0"`
		} `json:"auth"`
	} `json:"api"`
	TLS struct {
		Address  string `json:"address"`
		Enable   bool   `json:"enable"`
		Auto     bool   `json:"auto"`
		Email    string `json:"email"`
		Staging  bool   `json:"staging"`
		Secret   string `json:"secret"`
		CertFile string `json:"cert_file"`
		KeyFile  string `json:"key_file"`
	} `json:"tls"`
	Storage struct {
		Disk struct {
			Dir   string `json:"dir"`
			Size  int64  `json:"max_size_mbytes" format:"int64"`
			Cache struct {
				Enable   bool   `json:"enable"`
				Size     uint64 `json:"max_size_mbytes" format:"uint64"`
				TTL      int64  `json:"ttl_seconds" format:"int64"`
				FileSize uint64 `json:"max_file_size_mbytes" format:"uint64"`
				Types    struct {
					Allow []string `json:"allow"`
					Block []string `json:"block"`
				} `json:"types"`
			} `json:"cache"`
		} `json:"disk"`
		Memory struct {
			Auth struct {
				Enable   bool   `json:"enable"`   // Deprecated, use IAM
				Username string `json:"username"` // Deprecated, use IAM
				Password string `json:"password"` // Deprecated, use IAM
			} `json:"auth"` // Deprecated, use IAM
			Size   int64 `json:"max_size_mbytes" format:"int64"`
			Purge  bool  `json:"purge"`
			Backup struct {
				Dir      string   `json:"dir"`
				Patterns []string `json:"patterns"`
			} `json:"backup"`
		} `json:"memory"`
		S3   []value.S3Storage `json:"s3"`
		CORS struct {
			Origins []string `json:"origins"`
		} `json:"cors"`
		MimeTypesFile string            `json:"mimetypes_file"`
		MimeTypes     map[string]string `json:"mimetypes"`
	} `json:"storage"`
	RTMP struct {
		Enable     bool   `json:"enable"`
		EnableTLS  bool   `json:"enable_tls"`
		Address    string `json:"address"`
		AddressTLS string `json:"address_tls"`
		App        string `json:"app"`
		Token      string `json:"token"` // Deprecated, use IAM
	} `json:"rtmp"`
	SRT struct {
		Enable     bool   `json:"enable"`
		Address    string `json:"address"`
		Passphrase string `json:"passphrase"`
		Token      string `json:"token"` // Deprecated, use IAM
		Log        struct {
			Enable bool     `json:"enable"`
			Topics []string `json:"topics"`
		} `json:"log"`
	} `json:"srt"`
	FFmpeg struct {
		Binary            string `json:"binary"`
		MaxProcesses      int64  `json:"max_processes" format:"int64"`
		DisableThrottling bool   `json:"disable_throttling"`
		Access            struct {
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
			MaxLines          int `json:"max_lines" format:"int"`
			MaxHistory        int `json:"max_history" format:"int"`
			MaxMinimalHistory int `json:"max_minimal_history" format:"int"`
		} `json:"log"`
	} `json:"ffmpeg"`
	Playout struct {
		Enable  bool `json:"enable"`
		MinPort int  `json:"min_port" format:"int"`
		MaxPort int  `json:"max_port" format:"int"`
	} `json:"playout"`
	Debug struct {
		Profiling    bool   `json:"profiling"`
		ForceGC      int    `json:"force_gc" format:"int"` // deprecated, use MemoryLimit instead
		MemoryLimit  int64  `json:"memory_limit_mbytes" format:"int64"`
		AutoMaxProcs bool   `json:"auto_max_procs"`
		AgentAddress string `json:"agent_address"`
	} `json:"debug"`
	Metrics struct {
		Enable           bool  `json:"enable"`
		EnablePrometheus bool  `json:"enable_prometheus"`
		Range            int64 `json:"range_sec" format:"int64"`    // seconds
		Interval         int64 `json:"interval_sec" format:"int64"` // seconds
	} `json:"metrics"`
	Sessions struct {
		Enable                bool     `json:"enable"`
		IPIgnoreList          []string `json:"ip_ignorelist"`
		Persist               bool     `json:"persist"`
		PersistInterval       int      `json:"persist_interval_sec" format:"int"`
		SessionTimeout        int      `json:"session_timeout_sec" format:"int"`
		SessionLogPathPattern string   `json:"session_log_path_pattern"`
		SessionLogBuffer      int      `json:"session_log_buffer_sec" format:"int"`
		MaxBitrate            uint64   `json:"max_bitrate_mbit" format:"uint64"`
		MaxSessions           uint64   `json:"max_sessions" format:"uint64"`
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
	Resources struct {
		MaxCPUUsage       float64 `json:"max_cpu_usage"`        // percent 0-100
		MaxMemoryUsage    float64 `json:"max_memory_usage"`     // percent 0-100
		MaxGPUUsage       float64 `json:"max_gpu_usage"`        // percent 0-100
		MaxGPUMemoryUsage float64 `json:"max_gpu_memory_usage"` // percent 0-100
	} `json:"resources"`
	Cluster struct {
		Enable                 bool     `json:"enable"`
		ID                     string   `json:"id"`
		Address                string   `json:"address"` // ip:port
		Peers                  []string `json:"peers"`
		StartupTimeout         int64    `json:"startup_timeout_sec" format:"int64"`          // seconds
		SyncInterval           int64    `json:"sync_interval_sec" format:"int64"`            // seconds
		NodeRecoverTimeout     int64    `json:"node_recover_timeout_sec" format:"int64"`     // seconds
		EmergencyLeaderTimeout int64    `json:"emergency_leader_timeout_sec" format:"int64"` // seconds
		RecoverTimeout         int64    `json:"recover_timeout_sec" format:"int64"`          // seconds
		Debug                  struct {
			DisableFFmpegCheck bool `json:"disable_ffmpeg_check"`
		} `json:"debug"`
	} `json:"cluster"`
}

func UpgradeV2ToV3(d *v2.Data, fs fs.Filesystem) (*Data, error) {
	cfg := New(fs)

	return MergeV2toV3(&cfg.Data, d)
}

func MergeV2toV3(data *Data, d *v2.Data) (*Data, error) {
	data.CreatedAt = d.CreatedAt
	data.LoadedAt = d.LoadedAt
	data.UpdatedAt = d.UpdatedAt

	data.ID = d.ID
	data.Name = d.Name
	data.Address = d.Address
	data.CheckForUpdates = d.CheckForUpdates

	data.Log = d.Log
	data.DB = d.DB
	data.Host = d.Host
	data.API = d.API
	data.RTMP = d.RTMP
	data.SRT = d.SRT
	data.Playout = d.Playout
	data.Metrics = d.Metrics
	data.Service = d.Service
	data.Router = d.Router

	data.Log.Topics = slices.Copy(d.Log.Topics)

	data.Host.Name = slices.Copy(d.Host.Name)

	data.API.Access.HTTP.Allow = slices.Copy(d.API.Access.HTTP.Allow)
	data.API.Access.HTTP.Block = slices.Copy(d.API.Access.HTTP.Block)
	data.API.Access.HTTPS.Allow = slices.Copy(d.API.Access.HTTPS.Allow)
	data.API.Access.HTTPS.Block = slices.Copy(d.API.Access.HTTPS.Block)

	data.API.Auth.Auth0.Tenants = copy.TenantSlice(d.API.Auth.Auth0.Tenants)

	data.Storage.CORS.Origins = slices.Copy(d.Storage.CORS.Origins)

	data.FFmpeg.Binary = d.FFmpeg.Binary
	data.FFmpeg.MaxProcesses = d.FFmpeg.MaxProcesses
	data.FFmpeg.Access.Input.Allow = slices.Copy(d.FFmpeg.Access.Input.Allow)
	data.FFmpeg.Access.Input.Block = slices.Copy(d.FFmpeg.Access.Input.Block)
	data.FFmpeg.Access.Output.Allow = slices.Copy(d.FFmpeg.Access.Output.Allow)
	data.FFmpeg.Access.Output.Block = slices.Copy(d.FFmpeg.Access.Output.Block)
	data.FFmpeg.Log.MaxLines = d.FFmpeg.Log.MaxLines
	data.FFmpeg.Log.MaxHistory = d.FFmpeg.Log.MaxHistory

	data.Sessions.Enable = d.Sessions.Enable
	data.Sessions.IPIgnoreList = slices.Copy(d.Sessions.IPIgnoreList)
	data.Sessions.SessionTimeout = d.Sessions.SessionTimeout
	data.Sessions.Persist = d.Sessions.Persist
	data.Sessions.PersistInterval = d.Sessions.PersistInterval
	data.Sessions.MaxBitrate = d.Sessions.MaxBitrate
	data.Sessions.MaxSessions = d.Sessions.MaxSessions

	data.SRT.Log.Topics = slices.Copy(d.SRT.Log.Topics)

	data.Router.BlockedPrefixes = slices.Copy(d.Router.BlockedPrefixes)
	data.Router.Routes = copy.StringMap(d.Router.Routes)

	data.Storage.MimeTypesFile = d.Storage.MimeTypes

	data.Storage.CORS = d.Storage.CORS
	data.Storage.CORS.Origins = slices.Copy(d.Storage.CORS.Origins)

	// Actual changes
	data.Debug.Profiling = d.Debug.Profiling
	data.Debug.ForceGC = d.Debug.ForceGC
	data.Debug.MemoryLimit = 0
	data.Debug.AgentAddress = ""

	data.TLS.Enable = d.TLS.Enable
	data.TLS.Address = d.TLS.Address
	data.TLS.Auto = d.TLS.Auto
	data.TLS.CertFile = d.TLS.CertFile
	data.TLS.KeyFile = d.TLS.KeyFile

	data.Storage.Disk.Dir = d.Storage.Disk.Dir
	data.Storage.Disk.Size = d.Storage.Disk.Size
	data.Storage.Disk.Cache.Enable = d.Storage.Disk.Cache.Enable
	data.Storage.Disk.Cache.Size = d.Storage.Disk.Cache.Size
	data.Storage.Disk.Cache.FileSize = d.Storage.Disk.Cache.FileSize
	data.Storage.Disk.Cache.TTL = d.Storage.Disk.Cache.TTL
	data.Storage.Disk.Cache.Types.Allow = slices.Copy(d.Storage.Disk.Cache.Types)

	data.Storage.Memory.Auth.Enable = d.Storage.Memory.Auth.Enable
	data.Storage.Memory.Auth.Username = d.Storage.Memory.Auth.Username
	data.Storage.Memory.Auth.Password = d.Storage.Memory.Auth.Password
	data.Storage.Memory.Size = d.Storage.Memory.Size
	data.Storage.Memory.Purge = d.Storage.Memory.Purge

	data.Storage.S3 = []value.S3Storage{}

	data.FFmpeg.Log.MaxMinimalHistory = 0

	data.Version = 3

	return data, nil
}

func DowngradeV3toV2(d *Data) (*v2.Data, error) {
	data := &v2.Data{}

	data.CreatedAt = d.CreatedAt
	data.LoadedAt = d.LoadedAt
	data.UpdatedAt = d.UpdatedAt

	data.ID = d.ID
	data.Name = d.Name
	data.Address = d.Address
	data.CheckForUpdates = d.CheckForUpdates

	data.Log = d.Log
	data.DB = d.DB
	data.Host = d.Host
	data.API = d.API
	data.RTMP = d.RTMP
	data.SRT = d.SRT
	data.Playout = d.Playout
	data.Metrics = d.Metrics
	data.Service = d.Service
	data.Router = d.Router

	data.Log.Topics = slices.Copy(d.Log.Topics)

	data.Host.Name = slices.Copy(d.Host.Name)

	data.API.Access.HTTP.Allow = slices.Copy(d.API.Access.HTTP.Allow)
	data.API.Access.HTTP.Block = slices.Copy(d.API.Access.HTTP.Block)
	data.API.Access.HTTPS.Allow = slices.Copy(d.API.Access.HTTPS.Allow)
	data.API.Access.HTTPS.Block = slices.Copy(d.API.Access.HTTPS.Block)

	data.API.Auth.Auth0.Tenants = copy.TenantSlice(d.API.Auth.Auth0.Tenants)

	data.Storage.CORS.Origins = slices.Copy(d.Storage.CORS.Origins)

	data.FFmpeg.Binary = d.FFmpeg.Binary
	data.FFmpeg.MaxProcesses = d.FFmpeg.MaxProcesses
	data.FFmpeg.Access.Input.Allow = slices.Copy(d.FFmpeg.Access.Input.Allow)
	data.FFmpeg.Access.Input.Block = slices.Copy(d.FFmpeg.Access.Input.Block)
	data.FFmpeg.Access.Output.Allow = slices.Copy(d.FFmpeg.Access.Output.Allow)
	data.FFmpeg.Access.Output.Block = slices.Copy(d.FFmpeg.Access.Output.Block)
	data.FFmpeg.Log.MaxLines = d.FFmpeg.Log.MaxLines
	data.FFmpeg.Log.MaxHistory = d.FFmpeg.Log.MaxHistory

	data.Sessions.Enable = d.Sessions.Enable
	data.Sessions.IPIgnoreList = slices.Copy(d.Sessions.IPIgnoreList)
	data.Sessions.SessionTimeout = d.Sessions.SessionTimeout
	data.Sessions.Persist = d.Sessions.Persist
	data.Sessions.PersistInterval = d.Sessions.PersistInterval
	data.Sessions.MaxBitrate = d.Sessions.MaxBitrate
	data.Sessions.MaxSessions = d.Sessions.MaxSessions

	data.SRT.Log.Topics = slices.Copy(d.SRT.Log.Topics)

	data.Router.BlockedPrefixes = slices.Copy(d.Router.BlockedPrefixes)
	data.Router.Routes = copy.StringMap(d.Router.Routes)

	// Actual changes
	data.Debug.Profiling = d.Debug.Profiling
	data.Debug.ForceGC = d.Debug.ForceGC

	data.TLS.Enable = d.TLS.Enable
	data.TLS.Address = d.TLS.Address
	data.TLS.Auto = d.TLS.Auto
	data.TLS.CertFile = d.TLS.CertFile
	data.TLS.KeyFile = d.TLS.KeyFile

	data.Storage.MimeTypes = d.Storage.MimeTypesFile

	data.Storage.CORS = d.Storage.CORS
	data.Storage.CORS.Origins = slices.Copy(d.Storage.CORS.Origins)

	data.Storage.Memory.Auth.Enable = d.Storage.Memory.Auth.Enable
	data.Storage.Memory.Auth.Username = d.Storage.Memory.Auth.Username
	data.Storage.Memory.Auth.Password = d.Storage.Memory.Auth.Password
	data.Storage.Memory.Size = d.Storage.Memory.Size
	data.Storage.Memory.Purge = d.Storage.Memory.Purge

	data.Storage.Disk.Dir = d.Storage.Disk.Dir
	data.Storage.Disk.Size = d.Storage.Disk.Size
	data.Storage.Disk.Cache.Enable = d.Storage.Disk.Cache.Enable
	data.Storage.Disk.Cache.Size = d.Storage.Disk.Cache.Size
	data.Storage.Disk.Cache.FileSize = d.Storage.Disk.Cache.FileSize
	data.Storage.Disk.Cache.TTL = d.Storage.Disk.Cache.TTL
	data.Storage.Disk.Cache.Types = slices.Copy(d.Storage.Disk.Cache.Types.Allow)

	data.Version = 2

	return data, nil
}
