package main

import (
	"os"
	"os/signal"
	"path"

	"github.com/datarhei/core/v16/app/api"
	"github.com/datarhei/core/v16/log"

	_ "github.com/joho/godotenv/autoload"
)

func main() {
	logger := log.New("Core").WithOutput(log.NewConsoleWriter(os.Stderr, log.Lwarn, true))

	configfile := findConfigfile()

	app, err := api.New(configfile, os.Stderr)
	if err != nil {
		logger.Error().WithError(err).Log("Failed to create new API")
		os.Exit(1)
	}

	go func() {
		defer func() {
			if proc, err := os.FindProcess(os.Getpid()); err == nil {
				proc.Signal(os.Interrupt)
			}
		}()

		for {
			if err := app.Start(); err != api.ErrConfigReload {
				if err != nil {
					logger.Error().WithError(err).Log("Failed to start API")
				}

				break
			} else {
				logger.Warn().WithError(err).Log("Config reload requested")
			}

			app.Stop()

			if err := app.Reload(); err != nil {
				logger.Error().WithError(err).Log("Failed to reload config")
				break
			}
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the app
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit

	// Stop the app
	app.Destroy()
}

// findConfigfie returns the path to the config file. If no path is given
// in the environment variable CORE_CONFIGFILE, different standard location
// will be probed:
// - os.UserConfigDir() + /datarhei-core/config.js
// - os.UserHomeDir() + /.config/datarhei-core/config.js
// - ./config/config.js
// If the config doesn't exist in none of these locations, it will be assumed
// at ./config/config.js
func findConfigfile() string {
	configfile := os.Getenv("CORE_CONFIGFILE")
	if len(configfile) != 0 {
		return configfile
	}

	locations := []string{}

	if dir, err := os.UserConfigDir(); err == nil {
		locations = append(locations, dir+"/datarhei-core/config.js")
	}

	if dir, err := os.UserHomeDir(); err == nil {
		locations = append(locations, dir+"/.config/datarhei-core/config.js")
	}

	locations = append(locations, "./config/config.js")

	for _, path := range locations {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}

		if info.IsDir() {
			continue
		}

		configfile = path
	}

	if len(configfile) == 0 {
		configfile = "./config/config.js"
	}

	os.MkdirAll(path.Dir(configfile), 0740)

	return configfile
}
