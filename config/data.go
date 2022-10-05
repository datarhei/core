package config

import "time"

// Data is the actual configuration data for the app
type Data struct {
	CreatedAt       time.Time `json:"created_at"`
	LoadedAt        time.Time `json:"-"`
	UpdatedAt       time.Time `json:"-"`
	Version         int64     `json:"version" jsonschema:"minimum=3,maximum=3"`
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
		Email    string `json:"email"`
		CertFile string `json:"cert_file"`
		KeyFile  string `json:"key_file"`
	} `json:"tls"`
	Storage struct {
		Disk struct {
			Dir   string `json:"dir"`
			Size  int64  `json:"max_size_mbytes"`
			Cache struct {
				Enable   bool   `json:"enable"`
				Size     uint64 `json:"max_size_mbytes"`
				TTL      int64  `json:"ttl_seconds"`
				FileSize uint64 `json:"max_file_size_mbytes"`
				Types    struct {
					Allow []string `json:"allow"`
					Block []string `json:"block"`
				} `json:"types"`
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

func NewV3FromV2(d *dataV2) (*Data, error) {
	data := &Data{}

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

	// Actual changes
	data.TLS.Enable = d.TLS.Enable
	data.TLS.Address = d.TLS.Address
	data.TLS.Auto = d.TLS.Auto
	data.TLS.CertFile = d.TLS.CertFile
	data.TLS.KeyFile = d.TLS.KeyFile
	data.TLS.Email = "cert@datarhei.com"

	data.Storage.MimeTypes = d.Storage.MimeTypes

	data.Storage.CORS = d.Storage.CORS
	data.Storage.CORS.Origins = copyStringSlice(d.Storage.CORS.Origins)

	data.Storage.Memory = d.Storage.Memory

	data.Storage.Disk.Dir = d.Storage.Disk.Dir
	data.Storage.Disk.Size = d.Storage.Disk.Size
	data.Storage.Disk.Cache.Enable = d.Storage.Disk.Cache.Enable
	data.Storage.Disk.Cache.Size = d.Storage.Disk.Cache.Size
	data.Storage.Disk.Cache.FileSize = d.Storage.Disk.Cache.FileSize
	data.Storage.Disk.Cache.TTL = d.Storage.Disk.Cache.TTL
	data.Storage.Disk.Cache.Types.Allow = copyStringSlice(d.Storage.Disk.Cache.Types)
	data.Storage.Disk.Cache.Types.Block = []string{}

	data.Version = 3

	return data, nil
}
