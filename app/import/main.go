package main

import (
	"os"

	"github.com/datarhei/core/config"
	"github.com/datarhei/core/log"
	"github.com/datarhei/core/restream/store"

	_ "github.com/joho/godotenv/autoload"
)

func main() {
	if ok := doImport(); !ok {
		os.Exit(1)
	}
}

func doImport() bool {
	logger := log.New("Import").WithOutput(log.NewConsoleWriter(os.Stderr, log.Linfo, true)).WithField("version", "v1")

	logger.Info().Log("Database import")

	configstore, err := config.NewJSONStore(os.Getenv("CORE_CONFIGFILE"), nil)
	if err != nil {
		logger.Error().WithError(err).Log("Loading configuration failed")
		return false
	}

	cfg := configstore.Get()

	// Merging the persisted config with the environment variables
	cfg.Merge()

	cfg.Validate(false)
	if cfg.HasErrors() {
		logger.Error().Log("The configuration contains errors")
		cfg.Messages(func(level string, v config.Variable, message string) {
			if level == "error" {
				logger.Error().WithFields(log.Fields{
					"variable":    v.Name,
					"value":       v.Value,
					"env":         v.EnvName,
					"description": v.Description,
				}).Log(message)
			}
		})

		return false
	}

	logger.Info().Log("Checking for database ...")

	// Check if there's a v1.json from the old Restreamer
	v1filename := cfg.DB.Dir + "/v1.json"

	logger = logger.WithField("database", v1filename)

	if _, err := os.Stat(v1filename); err != nil {
		if os.IsNotExist(err) {
			logger.Info().Log("Database doesn't exist and nothing will be imported")
			return true
		}

		logger.Error().WithError(err).Log("Checking for v1 database")

		return false
	}

	logger.Info().Log("Found database")

	// Load an existing DB
	datastore := store.NewJSONStore(store.JSONConfig{
		Dir: cfg.DB.Dir,
	})

	data, err := datastore.Load()
	if err != nil {
		logger.Error().WithError(err).Log("Loading new database failed")
		return false
	}

	// Check if the existing DB has already some data in it.
	// If it's not empty, we will not import any v1 DB.
	if !data.IsEmpty() {
		logger.Info().Log("There's already information stored in the new database and the v1 database will not be imported")
		return true
	}

	logger.Info().Log("Importing database ...")

	// Read the Restreamer config from the environment variables
	importConfig := importConfigFromEnvironment()
	importConfig.binary = cfg.FFmpeg.Binary

	// Rewrite the old database to the new database
	r, err := importV1(v1filename, importConfig)
	if err != nil {
		logger.Error().WithError(err).Log("Importing database failed")
		return false
	}

	// Persist the imported DB
	if err := datastore.Store(r); err != nil {
		logger.Error().WithError(err).Log("Storing imported data to new database failed")
		return false
	}

	// Get the unmerged config for persisting
	cfg = configstore.Get()

	// Add static routes to mimic the old URLs
	cfg.Router.Routes["/hls/live.stream.m3u8"] = "/memfs/" + importConfig.id + ".m3u8"
	cfg.Router.Routes["/images/live.jpg"] = "/memfs/" + importConfig.id + ".jpg"
	cfg.Router.Routes["/player.html"] = "/" + importConfig.id + ".html"

	// Persist the modified config
	if err := configstore.Set(cfg); err != nil {
		logger.Error().WithError(err).Log("Storing adjusted config failed")
		return false
	}

	logger.Info().Log("Successfully imported data")

	return true
}
