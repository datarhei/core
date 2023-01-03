package main

import (
	"fmt"
	"os"
	"regexp"

	cfgstore "github.com/datarhei/core/v16/config/store"
	cfgvars "github.com/datarhei/core/v16/config/vars"
	"github.com/datarhei/core/v16/ffmpeg"
	"github.com/datarhei/core/v16/io/file"
	"github.com/datarhei/core/v16/log"
	"github.com/datarhei/core/v16/restream/store"

	"github.com/Masterminds/semver/v3"
	_ "github.com/joho/godotenv/autoload"
)

func main() {
	logger := log.New("Migration").WithOutput(log.NewConsoleWriter(os.Stderr, log.Linfo, true)).WithFields(log.Fields{
		"from": "ffmpeg4",
		"to":   "ffmpeg5",
	})

	configfile := cfgstore.Location(os.Getenv("CORE_CONFIGFILE"))

	configstore, err := cfgstore.NewJSON(configfile, nil)
	if err != nil {
		logger.Error().WithError(err).Log("Loading configuration failed")
		os.Exit(1)
	}

	if err := doMigration(logger, configstore); err != nil {
		os.Exit(1)
	}
}

func doMigration(logger log.Logger, configstore cfgstore.Store) error {
	if logger == nil {
		logger = log.New("")
	}

	cfg := configstore.Get()

	// Merging the persisted config with the environment variables
	cfg.Merge()

	cfg.Validate(false)
	if cfg.HasErrors() {
		logger.Error().Log("The configuration contains errors")
		messages := []string{}
		cfg.Messages(func(level string, v cfgvars.Variable, message string) {
			if level == "error" {
				logger.Error().WithFields(log.Fields{
					"variable":    v.Name,
					"value":       v.Value,
					"env":         v.EnvName,
					"description": v.Description,
				}).Log(message)

				messages = append(messages, v.Name+": "+message)
			}
		})

		return fmt.Errorf("the configuration contains errors: %v", messages)
	}

	ff, err := ffmpeg.New(ffmpeg.Config{
		Binary: cfg.FFmpeg.Binary,
	})
	if err != nil {
		logger.Error().WithError(err).Log("Loading FFmpeg binary failed")
		return fmt.Errorf("loading FFmpeg binary failed: %w", err)
	}

	version, err := semver.NewVersion(ff.Skills().FFmpeg.Version)
	if err != nil {
		logger.Error().WithError(err).Log("Parsing FFmpeg version failed")
		return fmt.Errorf("parsing FFmpeg version failed: %w", err)
	}

	// The current FFmpeg version is 4. Nothing to do.
	if version.Major() == 4 {
		return nil
	}

	if version.Major() != 5 {
		err := fmt.Errorf("unknown FFmpeg version found: %d", version.Major())
		logger.Error().WithError(err).Log("Unsupported FFmpeg version found")
		return fmt.Errorf("unsupported FFmpeg version found: %w", err)
	}

	// Check if there's a DB file
	dbFilepath := cfg.DB.Dir + "/db.json"

	if _, err = os.Stat(dbFilepath); err != nil {
		// There's no DB to backup
		logger.Info().WithField("db", dbFilepath).Log("Database not found. Migration not required")
		return nil
	}

	// Check if we already have a backup
	backupFilepath := cfg.DB.Dir + "/db_ff4.json"

	if _, err = os.Stat(backupFilepath); err == nil {
		// Yes, we have a backup. The migration already happened
		logger.Info().WithField("backup", backupFilepath).Log("Migration already done")
		return nil
	}

	// Create a backup
	if err := file.Copy(dbFilepath, backupFilepath); err != nil {
		logger.Error().WithError(err).Log("Creating backup file failed")
		return fmt.Errorf("creating backup file failed: %w", err)
	}

	logger.Info().WithField("backup", backupFilepath).Log("Backup created")

	// Load the existing DB
	datastore := store.NewJSONStore(store.JSONConfig{
		Filepath: cfg.DB.Dir + "/db.json",
	})

	data, err := datastore.Load()
	if err != nil {
		logger.Error().WithError(err).Log("Loading database failed")
		return fmt.Errorf("loading database failed: %w", err)
	}

	logger.Info().Log("Migrating processes ...")

	// Migrate the processes to version 5
	// Only this happens:
	// - for RTSP inputs, replace -stimeout with -timeout

	reRTSP := regexp.MustCompile(`^rtsps?://`)
	for id, p := range data.Process {
		logger.Info().WithField("processid", p.ID).Log("")

		for index, input := range p.Config.Input {
			if !reRTSP.MatchString(input.Address) {
				continue
			}

			for i, o := range input.Options {
				if o != "-stimeout" {
					continue
				}

				input.Options[i] = "-timeout"
			}

			p.Config.Input[index] = input
		}
		p.Config.FFVersion = version.String()
		data.Process[id] = p
	}

	logger.Info().Log("Migrating processes done")

	// Store the modified DB
	if err := datastore.Store(data); err != nil {
		logger.Error().WithError(err).Log("Storing database failed")
		return fmt.Errorf("storing database failed: %w", err)
	}

	logger.Info().Log("Completed")

	return nil
}
