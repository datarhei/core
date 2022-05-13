package api

import (
	"time"

	"github.com/datarhei/core/config"
)

// ConfigData embeds config.Data
type ConfigData struct {
	config.Data
}

// Config is the config returned by the API
type Config struct {
	CreatedAt time.Time `json:"created_at"`
	LoadedAt  time.Time `json:"loaded_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Config ConfigData `json:"config"`

	Overrides []string `json:"overrides"`
}

// SetConfig embeds config.Data. It is used to send a new config to the server.
type SetConfig struct {
	config.Data
}

// MergeTo merges a sent config into a config.Config
func (rscfg *SetConfig) MergeTo(cfg *config.Config) {
	cfg.ID = rscfg.ID
	cfg.Name = rscfg.Name
	cfg.Address = rscfg.Address
	cfg.CheckForUpdates = rscfg.CheckForUpdates

	cfg.Log = rscfg.Log
	cfg.DB = rscfg.DB
	cfg.Host = rscfg.Host
	cfg.API = rscfg.API
	cfg.TLS = rscfg.TLS
	cfg.Storage = rscfg.Storage
	cfg.RTMP = rscfg.RTMP
	cfg.FFmpeg = rscfg.FFmpeg
	cfg.Playout = rscfg.Playout
	cfg.Debug = rscfg.Debug
	cfg.Metrics = rscfg.Metrics
	cfg.Sessions = rscfg.Sessions
	cfg.Service = rscfg.Service
	cfg.Router = rscfg.Router
}

// NewSetConfig converts a config.Config into a RestreamerSetConfig in order to prepopulate
// a RestreamerSetConfig with the current values. The uploaded config can have missing fields that
// will be filled with the current values after unmarshalling the JSON.
func NewSetConfig(cfg *config.Config) SetConfig {
	data := SetConfig{
		cfg.Data,
	}

	return data
}

// Unmarshal converts a config.Config to a RestreamerConfig.
func (c *Config) Unmarshal(cfg *config.Config) {
	if cfg == nil {
		return
	}

	configData := ConfigData{
		cfg.Data,
	}

	c.CreatedAt = cfg.CreatedAt
	c.LoadedAt = cfg.LoadedAt
	c.UpdatedAt = cfg.UpdatedAt
	c.Config = configData
	c.Overrides = cfg.Overrides()
}

// ConfigError is used to return error messages when uploading a new config
type ConfigError map[string][]string
