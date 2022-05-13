package api

import (
	"net/http"

	"github.com/datarhei/core/config"
	"github.com/datarhei/core/http/api"
	"github.com/datarhei/core/http/handler/util"

	"github.com/labstack/echo/v4"
)

// The ConfigHandler type provides handler functions for reading and manipulating
// the current config.
type ConfigHandler struct {
	store config.Store
}

// NewConfig return a new Config type. You have to provide a valid config store.
func NewConfig(store config.Store) *ConfigHandler {
	return &ConfigHandler{
		store: store,
	}
}

// Get returns the currently active Restreamer configuration
// @Summary Retrieve the currently active Restreamer configuration
// @Description Retrieve the currently active Restreamer configuration
// @ID config-3-get
// @Produce json
// @Success 200 {object} api.Config
// @Security ApiKeyAuth
// @Router /api/v3/config [get]
func (p *ConfigHandler) Get(c echo.Context) error {
	cfg := p.store.GetActive()

	apicfg := api.Config{}
	apicfg.Unmarshal(cfg)

	return c.JSON(http.StatusOK, apicfg)
}

// Set will set the given configuration as new active configuration
// @Summary Update the current Restreamer configuration
// @Description Update the current Restreamer configuration by providing a complete or partial configuration. Fields that are not provided will not be changed.
// @ID config-3-set
// @Accept json
// @Produce json
// @Param config body api.SetConfig true "Restreamer configuration"
// @Success 200 {string} string
// @Failure 400 {object} api.Error
// @Failure 409 {object} api.ConfigError
// @Security ApiKeyAuth
// @Router /api/v3/config [put]
func (p *ConfigHandler) Set(c echo.Context) error {
	cfg := p.store.Get()

	// Set the current config as default config value. This will
	// allow to set a partial config without destroying the other
	// values.
	setConfig := api.NewSetConfig(cfg)

	if err := util.ShouldBindJSON(c, &setConfig); err != nil {
		return api.Err(http.StatusBadRequest, "Invalid JSON", "%s", err)
	}

	// Merge it into the current config
	setConfig.MergeTo(cfg)

	// Now we make a copy from the config and merge it with the environment
	// variables. If this configuration is valid, we will store the un-merged
	// one to disk.

	mergedConfig := config.NewConfigFrom(cfg)
	mergedConfig.Merge()

	// Validate the new merged config
	mergedConfig.Validate(true)
	if mergedConfig.HasErrors() {
		errors := make(map[string][]string)

		mergedConfig.Messages(func(level string, v config.Variable, message string) {
			if level != "error" {
				return
			}

			errors[v.Name] = append(errors[v.Name], message)
		})

		return c.JSON(http.StatusConflict, errors)
	}

	// Save the new config
	if err := p.store.Set(cfg); err != nil {
		return api.Err(http.StatusBadRequest, "Failed to store config", "%s", err)
	}

	// Set the new and merged config as active config
	if err := p.store.SetActive(mergedConfig); err != nil {
		return api.Err(http.StatusBadRequest, "Failed to activate config", "%s", err)
	}

	return c.JSON(http.StatusOK, "OK")
}

// Reload will reload the currently active configuration
// @Summary Reload the currently active configuration
// @Description Reload the currently active configuration. This will trigger a restart of the Restreamer.
// @ID config-3-reload
// @Produce plain
// @Success 200 {string} string "OK"
// @Security ApiKeyAuth
// @Router /api/v3/config/reload [get]
func (p *ConfigHandler) Reload(c echo.Context) error {
	p.store.Reload()

	return c.String(http.StatusOK, "OK")
}
