package api

import (
	"fmt"
	"strings"
	"time"
)

type ConfigV1 struct {
	Version         int64  `json:"version" jsonschema:"minimum=1,maximum=1"`
	ID              string `json:"id"`
	Name            string `json:"name"`
	Address         string `json:"address"`
	CheckForUpdates bool   `json:"update_check"`
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
		Enable    bool   `json:"enable"`
		EnableTLS bool   `json:"enable_tls"`
		Address   string `json:"address"`
		App       string `json:"app"`
		Token     string `json:"token"`
	} `json:"rtmp"`
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

type ConfigV2 struct {
	Version         int64  `json:"version" jsonschema:"minimum=2,maximum=2"`
	ID              string `json:"id"`
	Name            string `json:"name"`
	Address         string `json:"address"`
	CheckForUpdates bool   `json:"update_check"`
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

type ConfigV3 struct {
	Version         int64  `json:"version" jsonschema:"minimum=3,maximum=3" format:"int64"`
	ID              string `json:"id"`
	Name            string `json:"name"`
	Address         string `json:"address"`
	CheckForUpdates bool   `json:"update_check"`
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
			Size  int64 `json:"max_size_mbytes" format:"int64"`
			Purge bool  `json:"purge"`
		} `json:"memory"`
		S3   []S3Storage `json:"s3"`
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
		Binary       string `json:"binary"`
		MaxProcesses int64  `json:"max_processes" format:"int64"`
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
		MaxCPUUsage    float64 `json:"max_cpu_usage"`    // percent 0-100
		MaxMemoryUsage float64 `json:"max_memory_usage"` // percent 0-100
	} `json:"resources"`
	Cluster struct {
		Enable                 bool     `json:"enable"`
		Debug                  bool     `json:"debug"`
		Address                string   `json:"address"` // ip:port
		Peers                  []string `json:"peers"`
		StartupTimeout         int64    `json:"startup_timeout_sec" format:"int64"`          // seconds
		SyncInterval           int64    `json:"sync_interval_sec" format:"int64"`            // seconds
		NodeRecoverTimeout     int64    `json:"node_recover_timeout_sec" format:"int64"`     // seconds
		EmergencyLeaderTimeout int64    `json:"emergency_leader_timeout_sec" format:"int64"` // seconds
	} `json:"cluster"`
}

type Config struct {
	CreatedAt time.Time `json:"created_at"`
	LoadedAt  time.Time `json:"loaded_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Config interface{} `json:"config"`

	Overrides []string `json:"overrides"`
}

type Auth0Tenant struct {
	Domain   string   `json:"domain"`
	Audience string   `json:"audience"`
	ClientID string   `json:"clientid"`
	Users    []string `json:"users"`
}

type S3Storage struct {
	Name            string        `json:"name"`
	Mountpoint      string        `json:"mountpoint"`
	Auth            S3StorageAuth `json:"auth"`
	Endpoint        string        `json:"endpoint"`
	AccessKeyID     string        `json:"access_key_id"`
	SecretAccessKey string        `json:"secret_access_key"`
	Bucket          string        `json:"bucket"`
	Region          string        `json:"region"`
	UseSSL          bool          `json:"use_ssl"`
}

type S3StorageAuth struct {
	Enable   bool   `json:"enable"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// ConfigError is used to return error messages when uploading a new config
type ConfigError map[string][]string

func (c ConfigError) Error() string {
	s := strings.Builder{}

	for key, messages := range map[string][]string(c) {
		s.WriteString(fmt.Sprintf("%s: %s", key, strings.Join(messages, ",")))
	}

	return s.String()
}
