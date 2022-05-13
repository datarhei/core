package config

// Store is a store for the configuration data.
type Store interface {
	// Get the current configuration.
	Get() *Config

	// Set a new configuration for persistence.
	Set(data *Config) error

	// GetActive returns the configuration that has been set as
	// active before, otherwise it return nil.
	GetActive() *Config

	// SetActive will keep the given configuration
	// as active in memory. It can be retrieved later with GetActive()
	SetActive(data *Config) error

	// Reload will reload the stored configuration. It has to make sure
	// that all affected components will receiver their potentially
	// changed configuration.
	Reload() error
}
