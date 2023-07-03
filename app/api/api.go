package api

import (
	"context"
	"fmt"
	"io"
	golog "log"
	"math"
	gonet "net"
	gohttp "net/http"
	"net/url"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/caddyserver/certmagic"
	"github.com/datarhei/core/v16/app"
	"github.com/datarhei/core/v16/autocert"
	"github.com/datarhei/core/v16/cluster"
	"github.com/datarhei/core/v16/config"
	configstore "github.com/datarhei/core/v16/config/store"
	configvars "github.com/datarhei/core/v16/config/vars"
	"github.com/datarhei/core/v16/ffmpeg"
	"github.com/datarhei/core/v16/http"
	"github.com/datarhei/core/v16/http/cache"
	httpfs "github.com/datarhei/core/v16/http/fs"
	"github.com/datarhei/core/v16/http/router"
	"github.com/datarhei/core/v16/iam"
	iamaccess "github.com/datarhei/core/v16/iam/access"
	iamidentity "github.com/datarhei/core/v16/iam/identity"
	"github.com/datarhei/core/v16/io/fs"
	"github.com/datarhei/core/v16/log"
	"github.com/datarhei/core/v16/math/rand"
	"github.com/datarhei/core/v16/monitor"
	"github.com/datarhei/core/v16/net"
	"github.com/datarhei/core/v16/prometheus"
	"github.com/datarhei/core/v16/psutil"
	"github.com/datarhei/core/v16/resources"
	"github.com/datarhei/core/v16/restream"
	restreamapp "github.com/datarhei/core/v16/restream/app"
	"github.com/datarhei/core/v16/restream/replace"
	"github.com/datarhei/core/v16/restream/rewrite"
	restreamstore "github.com/datarhei/core/v16/restream/store"
	restreamjsonstore "github.com/datarhei/core/v16/restream/store/json"
	"github.com/datarhei/core/v16/rtmp"
	"github.com/datarhei/core/v16/service"
	"github.com/datarhei/core/v16/session"
	"github.com/datarhei/core/v16/srt"
	srturl "github.com/datarhei/core/v16/srt/url"
	"github.com/datarhei/core/v16/update"
	"github.com/google/gops/agent"

	"github.com/lestrrat-go/strftime"
	"go.uber.org/automaxprocs/maxprocs"
)

// The API interface is the implementation for the restreamer API.
type API interface {
	// Start starts the API. This is blocking until the app has
	// been ended with Stop() or Destroy(). In this case a nil error
	// is returned. An ErrConfigReload error is returned if a
	// configuration reload has been requested.
	Start(ctx context.Context) error

	// Stop stops the API, some states may be kept intact such
	// that they can be reused after starting the API again.
	Stop()

	// Destroy is the same as Stop() but no state will be kept intact.
	Destroy()

	// Reload the configuration for the API. If there's an error the
	// previously loaded configuration is not altered.
	Reload() error
}

type api struct {
	restream        restream.Restreamer
	ffmpeg          ffmpeg.FFmpeg
	diskfs          fs.Filesystem
	memfs           fs.Filesystem
	s3fs            map[string]fs.Filesystem
	rtmpserver      rtmp.Server
	srtserver       srt.Server
	metrics         monitor.HistoryMonitor
	prom            prometheus.Metrics
	service         service.Service
	sessions        session.Registry
	sessionsLimiter net.IPLimiter
	cache           cache.Cacher
	mainserver      *gohttp.Server
	sidecarserver   *gohttp.Server
	update          update.Checker
	replacer        replace.Replacer
	resources       resources.Resources
	cluster         cluster.Cluster
	iam             iam.IAM

	errorChan chan error

	gcTickerStop context.CancelFunc

	log struct {
		writer io.Writer
		buffer log.BufferWriter
		events log.ChannelWriter
		logger struct {
			core    log.Logger
			main    log.Logger
			sidecar log.Logger
			rtmp    log.Logger
			rtmps   log.Logger
			srt     log.Logger
		}
	}

	config struct {
		path   string
		store  configstore.Store
		config *config.Config
	}

	lock   sync.Mutex
	wgStop sync.WaitGroup
	state  string

	undoMaxprocs func()

	process psutil.Process
}

// ErrConfigReload is an error returned to indicate that a reload of
// the configuration has been requested.
var ErrConfigReload = fmt.Errorf("configuration reload")

// New returns a new instance of the API interface
func New(configpath string, logwriter io.Writer) (API, error) {
	a := &api{
		state: "idle",
		s3fs:  map[string]fs.Filesystem{},
	}

	a.config.path = configpath
	a.log.writer = logwriter

	if a.log.writer == nil {
		a.log.writer = io.Discard
	}

	a.errorChan = make(chan error, 1)

	if err := a.Reload(); err != nil {
		return nil, err
	}

	return a, nil
}

func (a *api) Reload() error {
	a.lock.Lock()
	defer a.lock.Unlock()

	if a.state == "running" {
		return fmt.Errorf("can't reload config while running")
	}

	if a.errorChan == nil {
		a.errorChan = make(chan error, 1)
	}

	logger := log.New("Core").WithOutput(log.NewConsoleWriter(a.log.writer, log.Lwarn, true))

	rootfs, _ := fs.NewDiskFilesystem(fs.DiskConfig{})
	store, err := configstore.NewJSON(rootfs, a.config.path, func() {
		a.errorChan <- ErrConfigReload
	})
	if err != nil {
		return err
	}

	cfg := store.Get()

	cfg.Merge()

	if len(cfg.Host.Name) == 0 && cfg.Host.Auto {
		cfg.Host.Name = net.GetPublicIPs(5 * time.Second)
	}

	cfg.Validate(false)

	loglevel := log.Linfo

	switch cfg.Log.Level {
	case "silent":
		loglevel = log.Lsilent
	case "error":
		loglevel = log.Lerror
	case "warn":
		loglevel = log.Lwarn
	case "info":
		loglevel = log.Linfo
	case "debug":
		loglevel = log.Ldebug
	default:
		break
	}

	buffer := log.NewBufferWriter(loglevel, cfg.Log.MaxLines)
	events := log.NewChannelWriter()

	logger = logger.WithOutput(log.NewLevelRewriter(
		log.NewMultiWriter(
			log.NewTopicWriter(
				log.NewConsoleWriter(a.log.writer, loglevel, true),
				cfg.Log.Topics,
			),
			buffer,
			events,
		),
		[]log.LevelRewriteRule{
			// FFmpeg annoyance, move all warnings about unathorized access to memfs from ffmpeg to debug level
			// ts=2022-04-28T07:24:27Z level=WARN component="HTTP" address=":8080" client="::1" latency_ms=0 method="PUT" path="/memfs/00a10a69-416a-4cd5-9d4f-6d88ed3dd7f5_0917.ts" proto="HTTP/1.1" size_bytes=65 status=401 status_text="Unauthorized" user_agent="Lavf/58.76.100"
			{
				Level:     log.Ldebug,
				Component: "HTTP",
				Match: map[string]string{
					"client":      "^(::1|127.0.0.1)$",
					"method":      "^(PUT|POST|DELETE)$",
					"status_text": "^Unauthorized$",
					"user_agent":  "^Lavf/",
				},
			},
		},
	))

	logfields := log.Fields{
		"application": app.Name,
		"version":     app.Version.String(),
		"repository":  "https://github.com/datarhei/core",
		"license":     "Apache License Version 2.0",
		"arch":        app.Arch,
		"compiler":    app.Compiler,
	}

	if len(app.Commit) != 0 && len(app.Branch) != 0 {
		logfields["commit"] = app.Commit
		logfields["branch"] = app.Branch
	}

	if len(app.Build) != 0 {
		logfields["build"] = app.Build
	}

	logger.Info().WithFields(logfields).Log("")

	logger.Info().WithField("path", a.config.path).Log("Read config file")

	configlogger := logger.WithComponent("Config")
	cfg.Messages(func(level string, v configvars.Variable, message string) {
		configlogger = configlogger.WithFields(log.Fields{
			"variable":    v.Name,
			"value":       v.Value,
			"env":         v.EnvName,
			"description": v.Description,
			"override":    v.Merged,
		})
		configlogger.Debug().Log(message)

		switch level {
		case "warn":
			configlogger.Warn().Log(message)
		case "error":
			configlogger.Error().WithField("error", message).Log("")
		default:
			break
		}
	})

	if cfg.HasErrors() {
		logger.Error().WithField("error", "Not all variables are set or are valid. Check the error messages above. Bailing out.").Log("")
		return fmt.Errorf("not all variables are set or valid")
	}

	cfg.LoadedAt = time.Now()

	store.SetActive(cfg)

	a.config.store = store
	a.config.config = cfg
	a.log.logger.core = logger
	a.log.buffer = buffer
	a.log.events = events

	return nil
}

func (a *api) start(ctx context.Context) error {
	a.lock.Lock()
	defer a.lock.Unlock()

	if a.errorChan == nil {
		a.errorChan = make(chan error, 1)
	}

	if a.state == "running" {
		return fmt.Errorf("already running")
	}

	a.state = "starting"

	cfg := a.config.store.GetActive()

	if cfg.Debug.AutoMaxProcs {
		undoMaxprocs, err := maxprocs.Set(maxprocs.Logger(func(format string, args ...interface{}) {
			format = strings.TrimPrefix(format, "maxprocs: ")
			a.log.logger.core.Debug().Log(format, args...)
		}))
		if err != nil {
			a.log.logger.core.Warn().Log("%s", err.Error())
		}

		a.undoMaxprocs = undoMaxprocs
	}

	if len(cfg.Debug.AgentAddress) != 0 {
		if err := agent.Listen(agent.Options{
			Addr:                   cfg.Debug.AgentAddress,
			ReuseSocketAddrAndPort: true,
		}); err != nil {
			a.log.logger.main.Error().WithError(err).Log("")
		}
	}

	resources, err := resources.New(resources.Config{
		MaxCPU:    cfg.Resources.MaxCPUUsage,
		MaxMemory: cfg.Resources.MaxMemoryUsage,
		Logger:    a.log.logger.core.WithComponent("Resources"),
	})
	if err != nil {
		return fmt.Errorf("failed to initialize resource manager: %w", err)
	}

	resources.Start()

	a.resources = resources

	if cfg.Sessions.Enable {
		sessionConfig := session.Config{
			PersistInterval:   time.Duration(cfg.Sessions.PersistInterval) * time.Second,
			LogPattern:        cfg.Sessions.SessionLogPathPattern,
			LogBufferDuration: time.Duration(cfg.Sessions.SessionLogBuffer) * time.Second,
			Logger:            a.log.logger.core.WithComponent("Session"),
		}

		if cfg.Sessions.Persist {
			fs, err := fs.NewRootedDiskFilesystem(fs.RootedDiskConfig{
				Root: filepath.Join(cfg.DB.Dir, "sessions"),
			})
			if err != nil {
				return fmt.Errorf("unable to create filesystem for persisting sessions: %w", err)
			}
			sessionConfig.PersistFS = fs
		}

		sessions, err := session.New(sessionConfig)
		if err != nil {
			return fmt.Errorf("unable to create sessioner: %w", err)
		}

		a.sessions = sessions

		iplimiter, err := net.NewIPLimiter(cfg.Sessions.IPIgnoreList, nil)
		if err != nil {
			return fmt.Errorf("incorret IP ranges for the statistics provided: %w", err)
		}

		a.sessionsLimiter = iplimiter

		config := session.CollectorConfig{
			MaxTxBitrate:    cfg.Sessions.MaxBitrate * 1024 * 1024,
			MaxSessions:     cfg.Sessions.MaxSessions,
			InactiveTimeout: 5 * time.Second,
			SessionTimeout:  time.Duration(cfg.Sessions.SessionTimeout) * time.Second,
			Limiter:         iplimiter,
		}

		hls, err := sessions.Register("hls", config)
		if err != nil {
			return fmt.Errorf("unable to register session collector: %w", err)
		}

		_, err = sessions.Register("hlsingress", session.CollectorConfig{
			SessionTimeout: 30 * time.Second,
		})
		if err != nil {
			return fmt.Errorf("unable to register session collector: %w", err)
		}

		rtmp, err := sessions.Register("rtmp", config)
		if err != nil {
			return fmt.Errorf("unable to register session collector: %w", err)
		}

		srt, err := sessions.Register("srt", config)
		if err != nil {
			return fmt.Errorf("unable to register session collector: %w", err)
		}

		if _, err := sessions.Register("http", config); err != nil {
			return fmt.Errorf("unable to register session collector: %w", err)
		}

		if _, err := sessions.Register("https", config); err != nil {
			return fmt.Errorf("unable to register session collector: %w", err)
		}

		ffmpeg, err := sessions.Register("ffmpeg", config)
		if err != nil {
			return fmt.Errorf("unable to register session collector: %w", err)
		}

		hls.AddCompanion(rtmp)
		hls.AddCompanion(srt)
		hls.AddCompanion(ffmpeg)

		rtmp.AddCompanion(hls)
		rtmp.AddCompanion(ffmpeg)
		rtmp.AddCompanion(srt)

		srt.AddCompanion(hls)
		srt.AddCompanion(ffmpeg)
		srt.AddCompanion(rtmp)

		ffmpeg.AddCompanion(hls)
		ffmpeg.AddCompanion(rtmp)
		ffmpeg.AddCompanion(srt)
	} else {
		sessions, _ := session.New(session.Config{})
		a.sessions = sessions
	}

	if cfg.Cluster.Enable {
		peers := []cluster.Peer{}

		for _, p := range cfg.Cluster.Peers {
			id, address, found := strings.Cut(p, "@")
			if !found {
				continue
			}

			peers = append(peers, cluster.Peer{
				ID:      id,
				Address: address,
			})
		}

		ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.Cluster.StartupTimeout)*time.Second)
		defer cancel()

		cluster, err := cluster.New(ctx, cluster.Config{
			ID:                     cfg.ID,
			Name:                   cfg.Name,
			Path:                   filepath.Join(cfg.DB.Dir, "cluster"),
			Address:                cfg.Cluster.Address,
			Peers:                  peers,
			SyncInterval:           time.Duration(cfg.Cluster.SyncInterval) * time.Second,
			NodeRecoverTimeout:     time.Duration(cfg.Cluster.NodeRecoverTimeout) * time.Second,
			EmergencyLeaderTimeout: time.Duration(cfg.Cluster.EmergencyLeaderTimeout) * time.Second,
			CoreConfig:             cfg.Clone(),
			IPLimiter:              a.sessionsLimiter,
			Logger:                 a.log.logger.core.WithComponent("Cluster"),
		})
		if err != nil {
			return fmt.Errorf("unable to create cluster: %w", err)
		}

		a.cluster = cluster
	}

	var autocertManager autocert.Manager

	if cfg.TLS.Enable {
		if cfg.TLS.Auto {
			if len(cfg.Host.Name) == 0 {
				return fmt.Errorf("at least one host must be provided in host.name or CORE_HOST_NAME")
			}

			if a.cluster == nil {
				var storage certmagic.Storage
				storage = &certmagic.FileStorage{
					Path: filepath.Join(cfg.DB.Dir, "cert"),
				}

				if len(cfg.TLS.Secret) != 0 {
					crypto := autocert.NewCrypto(cfg.TLS.Secret)
					storage = autocert.NewCryptoStorage(storage, crypto)
				}

				manager, err := autocert.New(autocert.Config{
					Storage:         storage,
					DefaultHostname: cfg.Host.Name[0],
					EmailAddress:    cfg.TLS.Email,
					IsProduction:    !cfg.TLS.Staging,
					Logger:          a.log.logger.core.WithComponent("Let's Encrypt"),
				})
				if err != nil {
					return fmt.Errorf("failed to initialize autocert manager: %w", err)
				}

				resctx, rescancel := context.WithCancel(ctx)
				err = manager.HTTPChallengeResolver(resctx, cfg.Address)

				if err == nil {
					ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
					err = manager.AcquireCertificates(ctx, cfg.Host.Name)
					cancel()
				}

				rescancel()

				autocertManager = manager

				if err != nil {
					a.log.logger.core.Error().WithError(err).Log("Failed to acquire certificates")
				}

				if err != nil {
					a.log.logger.core.Warn().Log("Continuing with disabled TLS")
					autocertManager = nil
					cfg.TLS.Enable = false
				} else {
					cfg.TLS.CertFile = ""
					cfg.TLS.KeyFile = ""
				}
			} else {
				autocertManager = a.cluster.CertManager()
			}
		} else {
			a.log.logger.core.Info().Log("Enabling TLS with cert and key files")
		}
	}

	{
		superuser := iamidentity.User{
			Name:      cfg.API.Auth.Username,
			Superuser: true,
			Auth: iamidentity.UserAuth{
				API: iamidentity.UserAuthAPI{
					Auth0: iamidentity.UserAuthAPIAuth0{},
				},
				Services: iamidentity.UserAuthServices{
					Token: []string{
						cfg.RTMP.Token,
						cfg.SRT.Token,
					},
				},
			},
		}

		if cfg.API.Auth.Enable {
			superuser.Auth.API.Password = cfg.API.Auth.Password
		}

		if cfg.API.Auth.Auth0.Enable {
			superuser.Auth.API.Auth0.User = cfg.API.Auth.Auth0.Tenants[0].Users[0]
			superuser.Auth.API.Auth0.Tenant = iamidentity.Auth0Tenant{
				Domain:   cfg.API.Auth.Auth0.Tenants[0].Domain,
				Audience: cfg.API.Auth.Auth0.Tenants[0].Audience,
				ClientID: cfg.API.Auth.Auth0.Tenants[0].ClientID,
			}
		}

		secret := rand.String(32)
		if len(cfg.API.Auth.JWT.Secret) != 0 {
			secret = cfg.API.Auth.Username + cfg.API.Auth.Password + cfg.API.Auth.JWT.Secret
		}

		var manager iam.IAM = nil

		if a.cluster != nil {
			var err error = nil
			manager, err = a.cluster.IAM(superuser, "datarhei-core", secret)
			if err != nil {
				return err
			}
		} else {
			rfs, err := fs.NewRootedDiskFilesystem(fs.RootedDiskConfig{
				Root: filepath.Join(cfg.DB.Dir, "iam"),
			})
			if err != nil {
				return err
			}

			policyAdapter, err := iamaccess.NewJSONAdapter(rfs, "./policy.json", nil)
			if err != nil {
				return err
			}

			identityAdapter, err := iamidentity.NewJSONAdapter(rfs, "./users.json", nil)
			if err != nil {
				return err
			}

			manager, err = iam.New(iam.Config{
				PolicyAdapter:   policyAdapter,
				IdentityAdapter: identityAdapter,
				Superuser:       superuser,
				JWTRealm:        "datarhei-core",
				JWTSecret:       secret,
				Logger:          a.log.logger.core.WithComponent("IAM"),
			})
			if err != nil {
				return fmt.Errorf("iam: %w", err)
			}

			// Check if there are already file created by IAM. If not, create policies
			// and users based on the config in order to mimic the behaviour before IAM.
			if len(rfs.List("/", fs.ListOptions{Pattern: "/*.json"})) == 0 {
				policies := []iamaccess.Policy{
					{
						Name:     "$anon",
						Domain:   "$none",
						Resource: "fs:/**",
						Actions:  []string{"GET", "HEAD", "OPTIONS"},
					},
					{
						Name:     "$anon",
						Domain:   "$none",
						Resource: "api:/api",
						Actions:  []string{"GET", "HEAD", "OPTIONS"},
					},
					{
						Name:     "$anon",
						Domain:   "$none",
						Resource: "api:/api/v3/widget/process/**",
						Actions:  []string{"GET", "HEAD", "OPTIONS"},
					},
				}

				users := map[string]iamidentity.User{}

				if cfg.Storage.Memory.Auth.Enable && cfg.Storage.Memory.Auth.Username != superuser.Name {
					users[cfg.Storage.Memory.Auth.Username] = iamidentity.User{
						Name: cfg.Storage.Memory.Auth.Username,
						Auth: iamidentity.UserAuth{
							Services: iamidentity.UserAuthServices{
								Basic: []string{cfg.Storage.Memory.Auth.Password},
							},
						},
					}

					policies = append(policies, iamaccess.Policy{
						Name:     cfg.Storage.Memory.Auth.Username,
						Domain:   "$none",
						Resource: "fs:/memfs/**",
						Actions:  []string{"ANY"},
					})
				}

				for _, s := range cfg.Storage.S3 {
					if s.Auth.Enable && s.Auth.Username != superuser.Name {
						user, ok := users[s.Auth.Username]
						if !ok {
							users[s.Auth.Username] = iamidentity.User{
								Name: s.Auth.Username,
								Auth: iamidentity.UserAuth{
									Services: iamidentity.UserAuthServices{
										Basic: []string{s.Auth.Password},
									},
								},
							}
						} else {
							user.Auth.Services.Basic = append(user.Auth.Services.Basic, s.Auth.Password)
							users[s.Auth.Username] = user
						}

						policies = append(policies, iamaccess.Policy{
							Name:     s.Auth.Username,
							Domain:   "$none",
							Resource: "fs:" + s.Mountpoint + "/**",
							Actions:  []string{"ANY"},
						})
					}
				}

				if cfg.RTMP.Enable && len(cfg.RTMP.Token) == 0 {
					policies = append(policies, iamaccess.Policy{
						Name:     "$anon",
						Domain:   "$none",
						Resource: "rtmp:/**",
						Actions:  []string{"ANY"},
					})
				}

				if cfg.SRT.Enable && len(cfg.SRT.Token) == 0 {
					policies = append(policies, iamaccess.Policy{
						Name:     "$anon",
						Domain:   "$none",
						Resource: "srt:**",
						Actions:  []string{"ANY"},
					})
				}

				for _, user := range users {
					if _, err := manager.GetIdentity(user.Name); err == nil {
						continue
					}

					err := manager.CreateIdentity(user)
					if err != nil {
						return fmt.Errorf("iam: %w", err)
					}
				}

				for _, policy := range policies {
					manager.AddPolicy(policy.Name, policy.Domain, policy.Resource, policy.Actions)
				}
			}
		}

		a.iam = manager
	}

	diskfs, err := fs.NewRootedDiskFilesystem(fs.RootedDiskConfig{
		Root: cfg.Storage.Disk.Dir,
		Logger: a.log.logger.core.WithComponent("Filesystem").WithFields(log.Fields{
			"type": "disk",
			"name": "disk",
		}),
	})
	if err != nil {
		return fmt.Errorf("disk filesystem: %w", err)
	}

	if diskfsRoot, err := filepath.Abs(cfg.Storage.Disk.Dir); err != nil {
		return err
	} else {
		diskfs.SetMetadata("base", diskfsRoot)
	}

	a.diskfs = diskfs

	baseMemFS := url.URL{
		Scheme: "http",
		Path:   "/memfs",
	}

	host, port, _ := gonet.SplitHostPort(cfg.Address)
	if len(host) == 0 {
		baseMemFS.Host = "localhost:" + port
	} else {
		baseMemFS.Host = cfg.Address
	}

	if cfg.Storage.Memory.Auth.Enable {
		baseMemFS.User = url.UserPassword(cfg.Storage.Memory.Auth.Username, cfg.Storage.Memory.Auth.Password)
	}

	if a.memfs == nil {
		memfs, _ := fs.NewMemFilesystem(fs.MemConfig{
			Logger: a.log.logger.core.WithComponent("Filesystem").WithFields(log.Fields{
				"type": "mem",
				"name": "mem",
			}),
		})

		memfs.SetMetadata("base", baseMemFS.String())

		sizedfs, _ := fs.NewSizedFilesystem(memfs, cfg.Storage.Memory.Size*1024*1024, cfg.Storage.Memory.Purge)

		a.memfs = sizedfs
	} else {
		a.memfs.SetMetadata("base", baseMemFS.String())
		if sizedfs, ok := a.memfs.(fs.SizedFilesystem); ok {
			sizedfs.Resize(cfg.Storage.Memory.Size * 1024 * 1024)
		}
	}

	for _, s3 := range cfg.Storage.S3 {
		if _, ok := a.s3fs[s3.Name]; ok {
			return fmt.Errorf("the name '%s' for a s3 filesystem is already in use", s3.Name)
		}

		baseS3FS := url.URL{
			Scheme: "http",
			Path:   s3.Mountpoint,
		}

		host, port, _ := gonet.SplitHostPort(cfg.Address)
		if len(host) == 0 {
			baseS3FS.Host = "localhost:" + port
		} else {
			baseS3FS.Host = cfg.Address
		}

		if s3.Auth.Enable {
			baseS3FS.User = url.UserPassword(s3.Auth.Username, s3.Auth.Password)
		}

		s3fs, err := fs.NewS3Filesystem(fs.S3Config{
			Name:            s3.Name,
			Endpoint:        s3.Endpoint,
			AccessKeyID:     s3.AccessKeyID,
			SecretAccessKey: s3.SecretAccessKey,
			Region:          s3.Region,
			Bucket:          s3.Bucket,
			UseSSL:          s3.UseSSL,
			Logger: a.log.logger.core.WithComponent("Filesystem").WithFields(log.Fields{
				"type": "s3",
				"name": s3.Name,
			}),
		})
		if err != nil {
			return fmt.Errorf("s3 filesystem (%s): %w", s3.Name, err)
		}

		s3fs.SetMetadata("base", baseS3FS.String())

		a.s3fs[s3.Name] = s3fs
	}

	var portrange net.Portranger

	if cfg.Playout.Enable {
		portrange, err = net.NewPortrange(cfg.Playout.MinPort, cfg.Playout.MaxPort)
		if err != nil {
			return fmt.Errorf("playout port range: %w", err)
		}
	}

	validatorIn, err := ffmpeg.NewValidator(cfg.FFmpeg.Access.Input.Allow, cfg.FFmpeg.Access.Input.Block)
	if err != nil {
		return fmt.Errorf("input address validator: %w", err)
	}

	validatorOut, err := ffmpeg.NewValidator(cfg.FFmpeg.Access.Output.Allow, cfg.FFmpeg.Access.Output.Block)
	if err != nil {
		return fmt.Errorf("output address validator: %w", err)
	}

	ffmpeg, err := ffmpeg.New(ffmpeg.Config{
		Binary:                  cfg.FFmpeg.Binary,
		MaxProc:                 cfg.FFmpeg.MaxProcesses,
		MaxLogLines:             cfg.FFmpeg.Log.MaxLines,
		LogHistoryLength:        cfg.FFmpeg.Log.MaxHistory,
		LogMinimalHistoryLength: cfg.FFmpeg.Log.MaxMinimalHistory,
		ValidatorInput:          validatorIn,
		ValidatorOutput:         validatorOut,
		Portrange:               portrange,
		Collector:               a.sessions.Collector("ffmpeg"),
	})
	if err != nil {
		return fmt.Errorf("unable to create ffmpeg: %w", err)
	}

	a.ffmpeg = ffmpeg

	var rw rewrite.Rewriter

	{
		baseAddress := func(address string) string {
			var base string
			host, port, _ := gonet.SplitHostPort(address)
			if len(host) == 0 {
				base = "localhost:" + port
			} else {
				base = address
			}

			return base
		}

		httpBase := baseAddress(cfg.Address)
		rtmpBase := baseAddress(cfg.RTMP.Address) + cfg.RTMP.App
		srtBase := baseAddress(cfg.SRT.Address)

		rw, err = rewrite.New(rewrite.Config{
			HTTPBase: "http://" + httpBase,
			RTMPBase: "rtmp://" + rtmpBase,
			SRTBase:  "srt://" + srtBase,
		})
		if err != nil {
			return fmt.Errorf("unable to create url rewriter: %w", err)
		}
	}

	a.replacer = replace.New()

	{
		a.replacer.RegisterReplaceFunc("date", func(params map[string]string, config *restreamapp.Config, section string) string {
			t, err := time.Parse(time.RFC3339, params["timestamp"])
			if err != nil {
				return ""
			}

			s, err := strftime.Format(params["format"], t)
			if err != nil {
				return ""
			}

			return s
		}, map[string]string{
			"format":    "%Y-%m-%d_%H-%M-%S",
			"timestamp": "$timestamp",
		})

		a.replacer.RegisterReplaceFunc("diskfs", func(params map[string]string, config *restreamapp.Config, section string) string {
			path := a.diskfs.Metadata("base")
			if len(config.Domain) != 0 {
				path = filepath.Join(path, config.Domain)
			}

			return path
		}, nil)

		a.replacer.RegisterReplaceFunc("fs:disk", func(params map[string]string, config *restreamapp.Config, section string) string {
			path := a.diskfs.Metadata("base")
			if len(config.Domain) != 0 {
				path = filepath.Join(path, config.Domain)
			}

			return path
		}, nil)

		a.replacer.RegisterReplaceFunc("memfs", func(params map[string]string, config *restreamapp.Config, section string) string {
			u, _ := url.Parse(a.memfs.Metadata("base"))

			var identity iamidentity.Verifier = nil

			if len(config.Owner) == 0 {
				identity = a.iam.GetDefaultVerifier()
			} else {
				var err error = nil
				identity, err = a.iam.GetVerifier(config.Owner)
				if err != nil {
					identity = nil
				}
			}

			if identity != nil {
				u.User = url.UserPassword(config.Owner, identity.GetServiceBasicAuth())
			} else {
				u.User = url.User(config.Owner)
			}

			if len(config.Domain) != 0 {
				u = u.JoinPath(config.Domain)
			}

			return u.String()
		}, nil)

		a.replacer.RegisterReplaceFunc("fs:mem", func(params map[string]string, config *restreamapp.Config, section string) string {
			u, _ := url.Parse(a.memfs.Metadata("base"))

			var identity iamidentity.Verifier = nil

			if len(config.Owner) == 0 {
				identity = a.iam.GetDefaultVerifier()
			} else {
				var err error = nil
				identity, err = a.iam.GetVerifier(config.Owner)
				if err != nil {
					identity = nil
				}
			}

			if identity != nil {
				u.User = url.UserPassword(config.Owner, identity.GetServiceBasicAuth())
			} else {
				u.User = url.User(config.Owner)
			}

			if len(config.Domain) != 0 {
				u = u.JoinPath(config.Domain)
			}

			return u.String()
		}, nil)

		for name, s3 := range a.s3fs {
			s3 := s3
			a.replacer.RegisterReplaceFunc("fs:"+name, func(params map[string]string, config *restreamapp.Config, section string) string {
				u, _ := url.Parse(s3.Metadata("base"))

				var identity iamidentity.Verifier = nil

				if len(config.Owner) == 0 {
					identity = a.iam.GetDefaultVerifier()
				} else {
					var err error = nil
					identity, err = a.iam.GetVerifier(config.Owner)
					if err != nil {
						identity = nil
					}
				}

				if identity != nil {
					u.User = url.UserPassword(config.Owner, identity.GetServiceBasicAuth())
				}

				if len(config.Domain) != 0 {
					u = u.JoinPath(config.Domain)
				}

				return u.String()
			}, nil)
		}

		a.replacer.RegisterReplaceFunc("rtmp", func(params map[string]string, config *restreamapp.Config, section string) string {
			host, port, _ := gonet.SplitHostPort(cfg.RTMP.Address)
			if len(host) == 0 {
				host = "localhost"
			}

			u := &url.URL{
				Scheme: "rtmp",
				Host:   host + ":" + port,
				Path:   "/",
			}

			if cfg.RTMP.App != "/" {
				u = u.JoinPath(cfg.RTMP.App)
			}

			if len(config.Domain) != 0 {
				u = u.JoinPath(config.Domain)
			}

			u = u.JoinPath(params["name"])

			var identity iamidentity.Verifier = nil

			if len(config.Owner) == 0 {
				identity = a.iam.GetDefaultVerifier()
			} else {
				var err error = nil
				identity, err = a.iam.GetVerifier(config.Owner)
				if err != nil {
					identity = nil
				}
			}

			if identity != nil {
				u = u.JoinPath(identity.GetServiceToken())
			}

			return u.String()
		}, map[string]string{
			"name": "",
		})

		a.replacer.RegisterReplaceFunc("srt", func(params map[string]string, config *restreamapp.Config, section string) string {
			host, port, _ = gonet.SplitHostPort(cfg.SRT.Address)
			if len(host) == 0 {
				host = "localhost"
			}

			u := srturl.URL{
				Scheme: "srt",
				Host:   host + ":" + port,
			}

			options := url.Values{}

			options.Set("mode", "caller")
			options.Set("transtype", "live")

			if len(cfg.SRT.Passphrase) != 0 {
				options.Set("passphrase", cfg.SRT.Passphrase)
			}

			u.Options = options

			streamid := srturl.StreamInfo{}

			if section == "output" {
				streamid.Mode = "publish"
			}

			if len(config.Domain) != 0 {
				streamid.Resource = config.Domain + "/" + params["name"]
			} else {
				streamid.Resource = params["name"]
			}

			var identity iamidentity.Verifier = nil

			if len(config.Owner) == 0 {
				identity = a.iam.GetDefaultVerifier()
			} else {
				var err error = nil
				identity, err = a.iam.GetVerifier(config.Owner)
				if err != nil {
					identity = nil
				}
			}

			if identity != nil {
				streamid.Token = identity.GetServiceToken()
			}

			u.StreamId = streamid.String()

			return u.String()
		}, map[string]string{
			"name":    "",
			"latency": "20000", // 20 milliseconds, FFmpeg requires microseconds
		})
	}

	filesystems := []fs.Filesystem{
		a.diskfs,
		a.memfs,
	}

	for _, fs := range a.s3fs {
		filesystems = append(filesystems, fs)
	}

	var store restreamstore.Store = nil

	{
		fs, err := fs.NewRootedDiskFilesystem(fs.RootedDiskConfig{
			Root: cfg.DB.Dir,
		})
		if err != nil {
			return err
		}
		store, err = restreamjsonstore.New(restreamjsonstore.Config{
			Filesystem: fs,
			Filepath:   "/db.json",
			Logger:     a.log.logger.core.WithComponent("ProcessStore"),
		})
		if err != nil {
			return err
		}
	}

	restream, err := restream.New(restream.Config{
		ID:           cfg.ID,
		Name:         cfg.Name,
		Store:        store,
		Filesystems:  filesystems,
		Replace:      a.replacer,
		Rewrite:      rw,
		FFmpeg:       a.ffmpeg,
		MaxProcesses: cfg.FFmpeg.MaxProcesses,
		Resources:    a.resources,
		IAM:          a.iam,
		Logger:       a.log.logger.core.WithComponent("Process"),
	})

	if err != nil {
		return fmt.Errorf("unable to create restreamer: %w", err)
	}

	a.restream = restream

	metrics, err := monitor.NewHistory(monitor.HistoryConfig{
		Enable:    cfg.Metrics.Enable,
		Timerange: time.Duration(cfg.Metrics.Range) * time.Second,
		Interval:  time.Duration(cfg.Metrics.Interval) * time.Second,
	})
	if err != nil {
		return fmt.Errorf("unable to create metrics collector with history: %w", err)
	}

	metrics.Register(monitor.NewUptimeCollector())
	metrics.Register(monitor.NewCPUCollector(a.resources))
	metrics.Register(monitor.NewMemCollector(a.resources))
	metrics.Register(monitor.NewNetCollector())
	metrics.Register(monitor.NewDiskCollector(a.diskfs.Metadata("base")))
	metrics.Register(monitor.NewFilesystemCollector("diskfs", a.diskfs))
	metrics.Register(monitor.NewFilesystemCollector("memfs", a.memfs))
	for name, fs := range a.s3fs {
		metrics.Register(monitor.NewFilesystemCollector(name, fs))
	}
	metrics.Register(monitor.NewRestreamCollector(a.restream))
	metrics.Register(monitor.NewFFmpegCollector(a.ffmpeg))
	metrics.Register(monitor.NewSessionCollector(a.sessions, []string{}))

	a.metrics = metrics

	if cfg.Metrics.EnablePrometheus {
		prom := prometheus.New()

		prom.Register(prometheus.NewUptimeCollector(cfg.ID, metrics))
		prom.Register(prometheus.NewCPUCollector(cfg.ID, metrics))
		prom.Register(prometheus.NewMemCollector(cfg.ID, metrics))
		prom.Register(prometheus.NewNetCollector(cfg.ID, metrics))
		prom.Register(prometheus.NewDiskCollector(cfg.ID, metrics))
		prom.Register(prometheus.NewFilesystemCollector(cfg.ID, metrics))
		prom.Register(prometheus.NewRestreamCollector(cfg.ID, metrics))
		prom.Register(prometheus.NewSessionCollector(cfg.ID, metrics))

		a.prom = prom
	}

	if cfg.Service.Enable {
		address := cfg.Address
		domain := "http://"
		if cfg.TLS.Enable {
			address = cfg.TLS.Address
			domain = "https://"
		}

		host, port, _ := gonet.SplitHostPort(address)
		if len(host) == 0 {
			host = "localhost"
		}

		if len(cfg.Host.Name) != 0 {
			host = cfg.Host.Name[0]
		}

		domain += host + ":" + port

		s, err := service.New(service.Config{
			ID:      cfg.ID,
			Version: fmt.Sprintf("%s %s (%s)", app.Name, app.Version, app.Arch),
			Domain:  domain,
			Token:   cfg.Service.Token,
			URL:     cfg.Service.URL,
			Monitor: a.metrics,
			Logger:  a.log.logger.core.WithComponent("Service"),
		})
		if err != nil {
			return fmt.Errorf("unable to connect to service: %w", err)
		}

		a.service = s
	}

	if cfg.CheckForUpdates {
		s, err := update.New(update.Config{
			ID:      cfg.ID,
			Name:    app.Name,
			Version: app.Version.String(),
			Arch:    app.Arch,
			Monitor: a.metrics,
			Logger:  a.log.logger.core.WithComponent("Update"),
		})

		if err != nil {
			return fmt.Errorf("unable to create update checker: %w", err)
		}

		a.update = s
	}

	if cfg.Storage.Disk.Cache.Enable {
		cache, err := cache.NewLRUCache(cache.LRUConfig{
			TTL:             time.Duration(cfg.Storage.Disk.Cache.TTL) * time.Second,
			MaxSize:         cfg.Storage.Disk.Cache.Size * 1024 * 1024,
			MaxFileSize:     cfg.Storage.Disk.Cache.FileSize * 1024 * 1024,
			AllowExtensions: cfg.Storage.Disk.Cache.Types.Allow,
			BlockExtensions: cfg.Storage.Disk.Cache.Types.Block,
			Logger:          a.log.logger.core.WithComponent("HTTPCache"),
		})

		if err != nil {
			return fmt.Errorf("unable to create cache: %w", err)
		}

		a.cache = cache
	}

	if cfg.RTMP.Enable {
		a.log.logger.rtmp = a.log.logger.core.WithComponent("RTMP").WithField("address", cfg.RTMP.Address)

		config := rtmp.Config{
			Addr:      cfg.RTMP.Address,
			TLSAddr:   cfg.RTMP.AddressTLS,
			App:       cfg.RTMP.App,
			Token:     cfg.RTMP.Token,
			Logger:    a.log.logger.rtmp,
			Collector: a.sessions.Collector("rtmp"),
			IAM:       a.iam,
		}

		if a.cluster != nil {
			config.Proxy = a.cluster.ProxyReader()
		}

		if cfg.RTMP.EnableTLS {
			config.Logger = config.Logger.WithComponent("RTMP/S")

			a.log.logger.rtmps = a.log.logger.core.WithComponent("RTMPS").WithField("address", cfg.RTMP.AddressTLS)
			if autocertManager != nil {
				config.TLSConfig = autocertManager.TLSConfig()
			}
		}

		rtmpserver, err := rtmp.New(config)
		if err != nil {
			return fmt.Errorf("unable to create RMTP server: %w", err)
		}

		a.rtmpserver = rtmpserver
	}

	if cfg.SRT.Enable {
		config := srt.Config{
			Addr:       cfg.SRT.Address,
			Passphrase: cfg.SRT.Passphrase,
			Token:      cfg.SRT.Token,
			Logger:     a.log.logger.core.WithComponent("SRT").WithField("address", cfg.SRT.Address),
			Collector:  a.sessions.Collector("srt"),
			IAM:        a.iam,
		}

		if a.cluster != nil {
			config.Proxy = a.cluster.ProxyReader()
		}

		if cfg.SRT.Log.Enable {
			config.SRTLogTopics = cfg.SRT.Log.Topics
		}

		srtserver, err := srt.New(config)
		if err != nil {
			return fmt.Errorf("unable to create SRT server: %w", err)
		}

		a.log.logger.srt = config.Logger
		a.srtserver = srtserver
	}

	logcontext := "HTTP"
	if cfg.TLS.Enable {
		logcontext = "HTTPS"
	}

	var iplimiter net.IPLimitValidator

	if cfg.TLS.Enable {
		limiter, err := net.NewIPLimiter(cfg.API.Access.HTTPS.Block, cfg.API.Access.HTTPS.Allow)
		if err != nil {
			return fmt.Errorf("incorret IP ranges for the API (HTTPS) provided: %w", err)
		}

		iplimiter = limiter
	} else {
		limiter, err := net.NewIPLimiter(cfg.API.Access.HTTP.Block, cfg.API.Access.HTTP.Allow)
		if err != nil {
			return fmt.Errorf("incorret IP ranges for the API (HTTP) provided: %w", err)
		}

		iplimiter = limiter
	}

	router, err := router.New(cfg.Router.BlockedPrefixes, cfg.Router.Routes, cfg.Router.UIPath)
	if err != nil {
		return fmt.Errorf("incorrect routes provided: %w", err)
	}

	a.log.logger.main = a.log.logger.core.WithComponent(logcontext).WithField("address", cfg.Address)

	httpfilesystems := []httpfs.FS{
		{
			Name:               a.diskfs.Name(),
			Mountpoint:         "",
			AllowWrite:         false,
			EnableAuth:         false,
			Username:           "",
			Password:           "",
			DefaultFile:        "index.html",
			DefaultContentType: "text/html",
			Gzip:               true,
			Filesystem:         a.diskfs,
			Cache:              a.cache,
		},
		{
			Name:               a.memfs.Name(),
			Mountpoint:         "/memfs",
			AllowWrite:         true,
			EnableAuth:         cfg.Storage.Memory.Auth.Enable,
			Username:           cfg.Storage.Memory.Auth.Username,
			Password:           cfg.Storage.Memory.Auth.Password,
			DefaultFile:        "",
			DefaultContentType: "application/data",
			Gzip:               true,
			Filesystem:         a.memfs,
			Cache:              nil,
		},
	}

	for _, s3 := range cfg.Storage.S3 {
		httpfilesystems = append(httpfilesystems, httpfs.FS{
			Name:               s3.Name,
			Mountpoint:         s3.Mountpoint,
			AllowWrite:         true,
			EnableAuth:         s3.Auth.Enable,
			Username:           s3.Auth.Username,
			Password:           s3.Auth.Password,
			DefaultFile:        "",
			DefaultContentType: "application/data",
			Gzip:               true,
			Filesystem:         a.s3fs[s3.Name],
			Cache:              a.cache,
		})
	}

	serverConfig := http.Config{
		Logger:        a.log.logger.main,
		LogBuffer:     a.log.buffer,
		LogEvents:     a.log.events,
		Restream:      a.restream,
		Metrics:       a.metrics,
		Prometheus:    a.prom,
		MimeTypesFile: cfg.Storage.MimeTypes,
		Filesystems:   httpfilesystems,
		IPLimiter:     iplimiter,
		Profiling:     cfg.Debug.Profiling,
		Cors: http.CorsConfig{
			Origins: cfg.Storage.CORS.Origins,
		},
		RTMP:     a.rtmpserver,
		SRT:      a.srtserver,
		Config:   a.config.store,
		Sessions: a.sessions,
		Router:   router,
		ReadOnly: cfg.API.ReadOnly,
		Cluster:  a.cluster,
		IAM:      a.iam,
		IAMSkipper: func(ip string) bool {
			if !cfg.API.Auth.Enable {
				return true
			} else {
				isLocalhost := false
				if ip == "127.0.0.1" || ip == "::1" {
					isLocalhost = true
				}

				if isLocalhost && cfg.API.Auth.DisableLocalhost {
					return true
				}
			}

			return false
		},
	}

	mainserverhandler, err := http.NewServer(serverConfig)

	if err != nil {
		return fmt.Errorf("unable to create server: %w", err)
	}

	var wgStart sync.WaitGroup

	sendError := func(err error) {
		select {
		case a.errorChan <- err:
		default:
		}
	}

	a.mainserver = &gohttp.Server{
		Addr:              cfg.Address,
		Handler:           mainserverhandler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
		ErrorLog:          golog.New(a.log.logger.main.Debug(), "", 0),
	}

	if cfg.TLS.Enable {
		a.mainserver.Addr = cfg.TLS.Address
		a.log.logger.main = a.log.logger.main.WithField("address", cfg.TLS.Address)

		iplimiter, err := net.NewIPLimiter(cfg.API.Access.HTTP.Block, cfg.API.Access.HTTP.Allow)
		if err != nil {
			return fmt.Errorf("incorret IP ranges for the API (HTTP) provided: %w", err)
		}

		a.log.logger.sidecar = a.log.logger.core.WithComponent("HTTP").WithField("address", cfg.Address)

		serverConfig.Logger = a.log.logger.sidecar
		serverConfig.IPLimiter = iplimiter

		sidecarserverhandler, err := http.NewServer(serverConfig)

		if err != nil {
			return fmt.Errorf("unable to create sidecar HTTP server: %w", err)
		}

		a.sidecarserver = &gohttp.Server{
			Addr:              cfg.Address,
			Handler:           sidecarserverhandler,
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       120 * time.Second,
			MaxHeaderBytes:    1 << 20,
			ErrorLog:          golog.New(a.log.logger.sidecar.Debug(), "", 0),
		}

		if cfg.TLS.Auto {
			a.mainserver.TLSConfig = autocertManager.TLSConfig()
			a.sidecarserver.Handler = autocertManager.HTTPChallengeHandler(sidecarserverhandler)
		}

		wgStart.Add(1)
		a.wgStop.Add(1)

		go func() {
			logger := a.log.logger.sidecar
			defer func() {
				logger.Info().Log("Sidecar server exited")
				a.wgStop.Done()
			}()

			wgStart.Done()

			logger.Info().Log("Sidecar server started")

			var err error

			err = a.sidecarserver.ListenAndServe()
			if err != nil && err != gohttp.ErrServerClosed {
				err = fmt.Errorf("HTTP sidecar server: %w", err)
			} else {
				err = nil
			}

			sendError(err)
		}()
	}

	if a.rtmpserver != nil {
		wgStart.Add(1)
		a.wgStop.Add(1)

		go func() {
			logger := a.log.logger.rtmp

			defer func() {
				logger.Info().Log("Server exited")
				a.wgStop.Done()
			}()

			wgStart.Done()

			var err error

			logger.Info().Log("Server started")
			err = a.rtmpserver.ListenAndServe()
			if err != nil && err != rtmp.ErrServerClosed {
				err = fmt.Errorf("RTMP server: %w", err)
			} else {
				err = nil
			}

			sendError(err)
		}()

		if cfg.TLS.Enable && cfg.RTMP.EnableTLS {
			wgStart.Add(1)
			a.wgStop.Add(1)

			go func() {
				logger := a.log.logger.rtmps

				defer func() {
					logger.Info().Log("Server exited")
					a.wgStop.Done()
				}()

				wgStart.Done()

				var err error

				logger.Info().Log("Server started")
				err = a.rtmpserver.ListenAndServeTLS(cfg.TLS.CertFile, cfg.TLS.KeyFile)
				if err != nil && err != rtmp.ErrServerClosed {
					err = fmt.Errorf("RTMPS server: %w", err)
				} else {
					err = nil
				}

				sendError(err)
			}()
		}
	}

	if a.srtserver != nil {
		wgStart.Add(1)
		a.wgStop.Add(1)

		go func() {
			logger := a.log.logger.srt

			defer func() {
				logger.Info().Log("Server exited")
				a.wgStop.Done()
			}()

			wgStart.Done()

			var err error

			logger.Info().Log("Server started")
			err = a.srtserver.ListenAndServe()
			if err != nil && err != srt.ErrServerClosed {
				err = fmt.Errorf("SRT server: %w", err)
			} else {
				err = nil
			}

			sendError(err)
		}()
	}

	wgStart.Add(1)
	a.wgStop.Add(1)

	go func() {
		logger := a.log.logger.main

		defer func() {
			logger.Info().Log("Server exited")
			a.wgStop.Done()
		}()

		wgStart.Done()

		var err error

		if cfg.TLS.Enable {
			logger.Info().Log("Server started")
			err = a.mainserver.ListenAndServeTLS(cfg.TLS.CertFile, cfg.TLS.KeyFile)
			if err != nil && err != gohttp.ErrServerClosed {
				err = fmt.Errorf("HTTPS server: %w", err)
			} else {
				err = nil
			}
		} else {
			logger.Info().Log("Server started")
			err = a.mainserver.ListenAndServe()
			if err != nil && err != gohttp.ErrServerClosed {
				err = fmt.Errorf("HTTP server: %w", err)
			} else {
				err = nil
			}
		}

		sendError(err)
	}()

	// Wait for all servers to be started
	wgStart.Wait()

	if cfg.Debug.ForceGC > 0 {
		ctx, cancel := context.WithCancel(ctx)
		a.gcTickerStop = cancel

		go func(ctx context.Context) {
			ticker := time.NewTicker(time.Duration(cfg.Debug.ForceGC) * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					debug.FreeOSMemory()
					/*
						var mem runtime.MemStats
						runtime.ReadMemStats(&mem)
						a.log.logger.main.WithComponent("memory").Debug().WithFields(log.Fields{
							"Sys":          float64(mem.Sys) / (1 << 20),
							"HeapSys":      float64(mem.HeapSys) / (1 << 20),
							"HeapAlloc":    float64(mem.HeapAlloc) / (1 << 20),
							"HeapIdle":     float64(mem.HeapIdle) / (1 << 20),
							"HeapInuse":    float64(mem.HeapInuse) / (1 << 20),
							"HeapReleased": float64(mem.HeapReleased) / (1 << 20),
						}).Log("")
					*/
				}
			}
		}(ctx)
	}

	if cfg.Debug.MemoryLimit > 0 {
		debug.SetMemoryLimit(cfg.Debug.MemoryLimit * 1024 * 1024)
	} else {
		debug.SetMemoryLimit(math.MaxInt64)
	}

	// Start the restream processes
	restream.Start()

	// Start the service
	if a.service != nil {
		a.service.Start()
	}

	// Start the update checker
	if a.update != nil {
		a.update.Start()
	}

	a.state = "running"

	return nil
}

func (a *api) Start(ctx context.Context) error {
	if err := a.start(ctx); err != nil {
		a.stop()
		return err
	}

	// Block until there's an error from the servers
	err := <-a.errorChan

	return err
}

func (a *api) stop() {
	a.lock.Lock()
	defer a.lock.Unlock()

	logger := a.log.logger.core.WithField("action", "shutdown")

	if a.state == "idle" {
		logger.Info().Log("Complete")
		return
	}

	if a.cluster != nil {
		// Hard shutdown without gracefully leaving the cluster
		a.cluster.Shutdown()
	}

	if a.iam != nil {
		a.iam.Close()
	}

	if a.update != nil {
		a.update.Stop()
		a.update = nil
	}

	if a.service != nil {
		a.service.Stop()
		a.service = nil
	}

	// Stop all restream processes
	if a.restream != nil {
		logger.Info().Log("Stopping all processes ...")
		a.restream.Stop()
		a.restream = nil
	}

	if a.process != nil {
		a.process.Stop()
		a.process = nil
	}

	// Stop the session tracker
	if a.sessions != nil {
		a.sessions.UnregisterAll()
		a.sessions.Close()
		a.sessions = nil
	}

	// Unregister all collectors
	if a.metrics != nil {
		a.metrics.UnregisterAll()
		a.metrics = nil
	}

	if a.prom != nil {
		a.prom.UnregisterAll()
		a.prom = nil
	}

	// Free the cached objects
	if a.cache != nil {
		a.cache.Purge()
		a.cache = nil
	}

	// Free the S3 mounts
	a.s3fs = map[string]fs.Filesystem{}

	// Stop the SRT server
	if a.srtserver != nil {
		a.log.logger.srt.Info().Log("Stopping ...")

		a.srtserver.Close()
		a.srtserver = nil
	}

	// Stop the RTMP server
	if a.rtmpserver != nil {
		a.log.logger.rtmp.Info().Log("Stopping ...")

		if a.log.logger.rtmps != nil {
			a.log.logger.rtmps.Info().Log("Stopping ...")
		}

		a.rtmpserver.Close()
		a.rtmpserver = nil
	}

	// Shutdown the HTTP/S mainserver
	if a.mainserver != nil {
		logger := a.log.logger.main
		logger.Info().Log("Stopping ...")

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := a.mainserver.Shutdown(ctx); err != nil {
			logger.Error().WithError(err).Log("")
		}

		a.mainserver = nil
	}

	// Shutdown the HTTP sidecar server
	if a.sidecarserver != nil {
		logger := a.log.logger.sidecar

		logger.Info().Log("Stopping ...")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := a.sidecarserver.Shutdown(ctx); err != nil {
			logger.Error().WithError(err).Log("")
		}

		a.sidecarserver = nil
	}

	// Stop resource observer
	if a.resources != nil {
		a.resources.Stop()
	}

	// Stop the GC ticker
	if a.gcTickerStop != nil {
		a.gcTickerStop()
		a.gcTickerStop = nil
	}

	// Stop gops agent
	agent.Close()

	// Wait for all server goroutines to exit
	logger.Info().Log("Waiting for all servers to stop ...")
	a.wgStop.Wait()

	// Drain error channel
	if a.errorChan != nil {
		close(a.errorChan)
		a.errorChan = nil
	}

	a.state = "idle"

	if a.undoMaxprocs != nil {
		a.undoMaxprocs()
	}

	logger.Info().Log("Complete")
	logger.Close()
}

func (a *api) Stop() {
	a.log.logger.core.Info().Log("Shutdown requested ...")
	a.stop()
}

func (a *api) Destroy() {
	a.log.logger.core.Info().Log("Shutdown requested ...")
	a.stop()

	// Free the MemFS
	if a.memfs != nil {
		a.memfs.RemoveList("/", fs.ListOptions{})
		a.memfs = nil
	}
}
