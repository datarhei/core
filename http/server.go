// @title datarhei Core API
// @version 3.0
// @description Expose REST API for the datarhei Core

// @contact.name datarhei Core Support
// @contact.url https://www.datarhei.com
// @contact.email hello@datarhei.com

// @license.name Apache 2.0
// @license.url https://github.com/datarhei/core/v16/blob/main/LICENSE

// @BasePath /

// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name Authorization
// @Param Authorization header string true "Insert your access token" default(Bearer <Add access token here>)

// @securityDefinitions.apikey ApiRefreshKeyAuth
// @in header
// @name Authorization

// @securityDefinitions.apikey Auth0KeyAuth
// @in header
// @name Authorization

// @securityDefinitions.basic BasicAuth

package http

import (
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/datarhei/core/v16/cluster"
	cfgstore "github.com/datarhei/core/v16/config/store"
	"github.com/datarhei/core/v16/http/cache"
	"github.com/datarhei/core/v16/http/errorhandler"
	"github.com/datarhei/core/v16/http/fs"
	"github.com/datarhei/core/v16/http/graph/resolver"
	"github.com/datarhei/core/v16/http/handler"
	api "github.com/datarhei/core/v16/http/handler/api"
	httplog "github.com/datarhei/core/v16/http/log"
	"github.com/datarhei/core/v16/http/router"
	serverhandler "github.com/datarhei/core/v16/http/server"
	"github.com/datarhei/core/v16/http/validator"
	"github.com/datarhei/core/v16/iam"
	"github.com/datarhei/core/v16/log"
	"github.com/datarhei/core/v16/monitor"
	"github.com/datarhei/core/v16/net"
	"github.com/datarhei/core/v16/prometheus"
	"github.com/datarhei/core/v16/restream"
	"github.com/datarhei/core/v16/rtmp"
	"github.com/datarhei/core/v16/session"
	"github.com/datarhei/core/v16/srt"

	mwcache "github.com/datarhei/core/v16/http/middleware/cache"
	mwcompress "github.com/datarhei/core/v16/http/middleware/compress"
	mwcors "github.com/datarhei/core/v16/http/middleware/cors"
	mwhlsrewrite "github.com/datarhei/core/v16/http/middleware/hlsrewrite"
	mwiam "github.com/datarhei/core/v16/http/middleware/iam"
	mwiplimit "github.com/datarhei/core/v16/http/middleware/iplimit"
	mwlog "github.com/datarhei/core/v16/http/middleware/log"
	mwmime "github.com/datarhei/core/v16/http/middleware/mime"
	mwredirect "github.com/datarhei/core/v16/http/middleware/redirect"
	mwsession "github.com/datarhei/core/v16/http/middleware/session"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	echoSwagger "github.com/swaggo/echo-swagger" // echo-swagger middleware

	// Expose the API docs
	_ "github.com/datarhei/core/v16/docs"
)

var ListenAndServe = http.ListenAndServe

type Config struct {
	Logger        log.Logger
	LogBuffer     log.BufferWriter
	LogEvents     log.ChannelWriter
	Restream      restream.Restreamer
	Metrics       monitor.HistoryReader
	Prometheus    prometheus.Reader
	MimeTypesFile string
	Filesystems   []fs.FS
	IPLimiter     net.IPLimitValidator
	Profiling     bool
	Cors          CorsConfig
	RTMP          rtmp.Server
	SRT           srt.Server
	Config        cfgstore.Store
	Cache         cache.Cacher
	Sessions      session.RegistryReader
	Router        router.Router
	ReadOnly      bool
	Cluster       cluster.Cluster
	IAM           iam.IAM
	IAMSkipper    func(ip string) bool
}

type CorsConfig struct {
	Origins []string
}

type server struct {
	logger log.Logger

	handler struct {
		about      *api.AboutHandler
		prometheus *handler.PrometheusHandler
		profiling  *handler.ProfilingHandler
		ping       *handler.PingHandler
		graph      *api.GraphHandler
		jwt        *api.JWTHandler
	}

	v3handler struct {
		log       *api.LogHandler
		events    *api.EventsHandler
		restream  *api.ProcessHandler
		rtmp      *api.RTMPHandler
		srt       *api.SRTHandler
		config    *api.ConfigHandler
		session   *api.SessionHandler
		widget    *api.WidgetHandler
		resources *api.MetricsHandler
		cluster   *api.ClusterHandler
		iam       *api.IAMHandler
	}

	middleware struct {
		iplimit    echo.MiddlewareFunc
		log        echo.MiddlewareFunc
		cors       echo.MiddlewareFunc
		cache      echo.MiddlewareFunc
		hlsrewrite echo.MiddlewareFunc
		iam        echo.MiddlewareFunc
	}

	gzip struct {
		mimetypes []string
	}

	filesystems map[string]*filesystem

	router        *echo.Echo
	mimeTypesFile string
	profiling     bool

	readOnly bool

	metrics struct {
		lock   sync.Mutex
		status map[int]uint64
	}
}

type filesystem struct {
	fs.FS

	handler    *handler.FSHandler
	apihandler *handler.FSHandler
	middleware echo.MiddlewareFunc
}

func NewServer(config Config) (serverhandler.Server, error) {
	s := &server{
		logger:        config.Logger,
		mimeTypesFile: config.MimeTypesFile,
		profiling:     config.Profiling,
		readOnly:      config.ReadOnly,
	}

	s.metrics.status = map[int]uint64{}

	s.filesystems = map[string]*filesystem{}

	corsPrefixes := map[string][]string{
		"/api": {"*"},
	}

	for _, httpfs := range config.Filesystems {
		if _, ok := s.filesystems[httpfs.Name]; ok {
			return nil, fmt.Errorf("the filesystem name '%s' is already in use", httpfs.Name)
		}

		if !strings.HasPrefix(httpfs.Mountpoint, "/") {
			httpfs.Mountpoint = "/" + httpfs.Mountpoint
		}

		if !strings.HasSuffix(httpfs.Mountpoint, "/") {
			httpfs.Mountpoint = strings.TrimSuffix(httpfs.Mountpoint, "/")
		}

		if _, ok := corsPrefixes[httpfs.Mountpoint]; ok {
			return nil, fmt.Errorf("the mount point '%s' is already in use (%s)", httpfs.Mountpoint, httpfs.Name)
		}

		corsPrefixes[httpfs.Mountpoint] = config.Cors.Origins

		apihttpfs := httpfs

		if config.Cluster != nil {
			if httpfs.Filesystem.Type() == "disk" || httpfs.Filesystem.Type() == "mem" {
				httpfs.Filesystem = fs.NewClusterFS(httpfs.Filesystem.Name(), httpfs.Filesystem, config.Cluster.ProxyReader())
			}
		}

		filesystem := &filesystem{
			FS:         httpfs,
			handler:    handler.NewFS(httpfs),
			apihandler: handler.NewFS(apihttpfs),
		}

		if httpfs.Filesystem.Type() == "disk" {
			filesystem.middleware = mwhlsrewrite.NewHLSRewriteWithConfig(mwhlsrewrite.HLSRewriteConfig{
				PathPrefix: httpfs.Filesystem.Metadata("base"),
			})
		}

		s.filesystems[filesystem.Name] = filesystem
	}

	if _, ok := corsPrefixes["/"]; !ok {
		return nil, fmt.Errorf("one filesystem must be mounted at /")
	}

	if config.Logger == nil {
		s.logger = log.New("")
	}

	mounts := []string{}

	for _, fs := range s.filesystems {
		mounts = append(mounts, fs.FS.Mountpoint)
	}

	s.middleware.iam = mwiam.NewWithConfig(mwiam.Config{
		Skipper: func(c echo.Context) bool {
			return config.IAMSkipper(c.RealIP())
		},
		IAM:                  config.IAM,
		Mounts:               mounts,
		WaitAfterFailedLogin: true,
		Logger:               s.logger.WithComponent("IAM"),
	})

	s.handler.about = api.NewAbout(
		config.Restream,
		func() []string { return config.IAM.Validators() },
	)

	s.handler.jwt = api.NewJWT(config.IAM)

	s.v3handler.iam = api.NewIAM(config.IAM)

	s.v3handler.log = api.NewLog(
		config.LogBuffer,
	)

	s.v3handler.events = api.NewEvents(
		config.LogEvents,
	)

	if config.Restream != nil {
		s.v3handler.restream = api.NewProcess(
			config.Restream,
			config.IAM,
		)
	}

	if config.Prometheus != nil {
		s.handler.prometheus = handler.NewPrometheus(
			config.Prometheus.HTTPHandler(),
		)
	}

	if config.Profiling {
		s.handler.profiling = handler.NewProfiling()
	}

	if config.IPLimiter != nil {
		s.middleware.iplimit = mwiplimit.NewWithConfig(mwiplimit.Config{
			Limiter: config.IPLimiter,
		})
	}

	s.handler.ping = handler.NewPing()

	if config.RTMP != nil {
		s.v3handler.rtmp = api.NewRTMP(
			config.RTMP,
		)
	}

	if config.SRT != nil {
		s.v3handler.srt = api.NewSRT(
			config.SRT,
		)
	}

	if config.Config != nil {
		s.v3handler.config = api.NewConfig(
			config.Config,
		)
	}

	if config.Sessions == nil {
		config.Sessions, _ = session.New(session.Config{})

		if config.Sessions.Collector("hlsingress") == nil {
			return nil, fmt.Errorf("hlsingress session collector must be available")
		}

		if config.Sessions.Collector("hls") == nil {
			return nil, fmt.Errorf("hls session collector must be available")
		}

		if config.Sessions.Collector("http") == nil {
			return nil, fmt.Errorf("http session collector must be available")
		}
	}

	s.v3handler.session = api.NewSession(
		config.Sessions,
		config.IAM,
	)

	s.middleware.log = mwlog.NewWithConfig(mwlog.Config{
		Logger: s.logger,
		Status: func(code int) {
			s.metrics.lock.Lock()
			defer s.metrics.lock.Unlock()

			s.metrics.status[code]++
		},
	})

	s.v3handler.widget = api.NewWidget(api.WidgetConfig{
		Restream: config.Restream,
		Registry: config.Sessions,
	})

	s.v3handler.resources = api.NewMetrics(api.MetricsConfig{
		Metrics: config.Metrics,
	})

	if config.Cluster != nil {
		handler, err := api.NewCluster(config.Cluster, config.IAM)
		if err != nil {
			return nil, fmt.Errorf("cluster handler: %w", err)
		}
		s.v3handler.cluster = handler
	}

	if middleware, err := mwcors.NewWithConfig(mwcors.Config{
		Prefixes: corsPrefixes,
	}); err != nil {
		return nil, err
	} else {
		s.middleware.cors = middleware
	}

	s.handler.graph = api.NewGraph(resolver.Resolver{
		Restream:  config.Restream,
		Monitor:   config.Metrics,
		LogBuffer: config.LogBuffer,
		IAM:       config.IAM,
	}, "/api/graph/query")

	s.gzip.mimetypes = []string{
		"text/plain",
		"text/html",
		"text/javascript",
		"application/json",
		"application/x-mpegurl",
		"application/vnd.apple.mpegurl",
		"image/svg+xml",
	}

	s.router = echo.New()
	s.router.JSONSerializer = &GoJSONSerializer{}
	s.router.HTTPErrorHandler = errorhandler.HTTPErrorHandler
	s.router.Validator = validator.New()
	s.router.Use(s.middleware.log)
	s.router.Use(middleware.RecoverWithConfig(middleware.RecoverConfig{
		LogErrorFunc: func(c echo.Context, err error, stack []byte) error {
			rows := strings.Split(string(stack), "\n")
			s.logger.Error().WithField("stack", rows).Log("recovered from a panic")
			return nil
		},
	}))

	s.router.HideBanner = true
	s.router.HidePort = true

	s.router.Logger.SetOutput(httplog.NewWrapper(s.logger))

	if s.middleware.cors != nil {
		s.router.Use(s.middleware.cors)
	}

	s.router.Use(s.middleware.iam)

	s.router.Use(mwsession.NewWithConfig(mwsession.Config{
		HLSIngressCollector: config.Sessions.Collector("hlsingress"),
		HLSEgressCollector:  config.Sessions.Collector("hls"),
		HTTPCollector:       config.Sessions.Collector("http"),
		Skipper: func(c echo.Context) bool {
			// Exclude the API from the sessions
			path := c.Request().URL.Path

			if path == "/api" {
				return true
			}

			return strings.HasPrefix(path, "/api/")
		},
	}))

	// Add static routes
	if path, target := config.Router.StaticRoute(); len(target) != 0 {
		group := s.router.Group(path)
		group.Use(middleware.AddTrailingSlashWithConfig(middleware.TrailingSlashConfig{
			Skipper: func(c echo.Context) bool {
				return path != c.Request().URL.Path
			},
			RedirectCode: 301,
		}))
		group.Use(middleware.StaticWithConfig(middleware.StaticConfig{
			Skipper:    middleware.DefaultSkipper,
			Root:       target,
			Index:      "index.html",
			IgnoreBase: true,
		}))
	}

	s.router.Use(mwredirect.NewWithConfig(mwredirect.Config{
		Redirects: config.Router.FileRoutes(),
	}))

	for prefix, target := range config.Router.DirRoutes() {
		group := s.router.Group(prefix)
		group.Use(middleware.AddTrailingSlashWithConfig(middleware.TrailingSlashConfig{
			Skipper: func(prefix string) func(c echo.Context) bool {
				return func(c echo.Context) bool {
					return prefix != c.Request().URL.Path
				}
			}(prefix),
			RedirectCode: 301,
		}))
		group.Use(middleware.StaticWithConfig(middleware.StaticConfig{
			Skipper:    middleware.DefaultSkipper,
			Root:       target,
			Index:      "index.html",
			IgnoreBase: true,
		}))
	}

	s.setRoutes()

	return s, nil
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *server) HTTPStatus() map[int]uint64 {
	status := map[int]uint64{}

	s.metrics.lock.Lock()
	defer s.metrics.lock.Unlock()

	for code, value := range s.metrics.status {
		status[code] = value
	}

	return status
}

func (s *server) setRoutes() {
	gzipMiddleware := mwcompress.NewWithConfig(mwcompress.Config{
		Level:     mwcompress.BestSpeed,
		MinLength: 1000,
		Skipper:   mwcompress.ContentTypeSkipper(nil),
	})

	// API router grouo
	api := s.router.Group("/api")

	if s.middleware.iplimit != nil {
		api.Use(s.middleware.iplimit)
	}

	// The login endpoint should not be blocked by auth
	s.router.POST("/api/login", s.handler.jwt.Login)
	s.router.GET("/api/login/refresh", s.handler.jwt.Refresh)

	api.GET("", s.handler.about.About)

	// Swagger API documentation router group
	doc := s.router.Group("/api/swagger/*")
	doc.Use(gzipMiddleware)
	doc.GET("", echoSwagger.WrapHandler)

	// Mount filesystems
	for _, filesystem := range s.filesystems {
		// Define a local variable because later in the loop we have a closure
		filesystem := filesystem

		mountpoint := filesystem.Mountpoint + "/*"
		if filesystem.Mountpoint == "/" {
			mountpoint = "/*"
		}

		fs := s.router.Group(mountpoint)
		fs.Use(mwmime.NewWithConfig(mwmime.Config{
			MimeTypesFile:      s.mimeTypesFile,
			DefaultContentType: filesystem.DefaultContentType,
		}))

		if filesystem.Gzip {
			fs.Use(mwcompress.NewWithConfig(mwcompress.Config{
				Skipper:   mwcompress.ContentTypeSkipper(s.gzip.mimetypes),
				Level:     mwcompress.BestSpeed,
				MinLength: 1000,
			}))
		}

		if filesystem.Cache != nil {
			mwcache := mwcache.NewWithConfig(mwcache.Config{
				Cache: filesystem.Cache,
			})
			fs.Use(mwcache)
		}

		if filesystem.middleware != nil {
			fs.Use(filesystem.middleware)
		}

		fs.GET("", filesystem.handler.GetFile)
		fs.HEAD("", filesystem.handler.GetFile)

		if filesystem.AllowWrite {
			fs.POST("", filesystem.handler.PutFile)
			fs.PUT("", filesystem.handler.PutFile)
			fs.DELETE("", filesystem.handler.DeleteFile)
		}

		s.logger.Info().WithFields(log.Fields{
			"name":       filesystem.Name,
			"mountpoint": filesystem.Mountpoint,
		}).Log("Mount filesystem")
	}

	// Prometheus metrics
	if s.handler.prometheus != nil {
		metrics := s.router.Group("/metrics")

		if s.middleware.iplimit != nil {
			metrics.Use(s.middleware.iplimit)
		}

		metrics.GET("", s.handler.prometheus.Metrics)
	}

	// Health check
	s.router.GET("/ping", s.handler.ping.Ping)

	// Profiling routes
	if s.profiling {
		prof := s.router.Group("/profiling")

		if s.middleware.iplimit != nil {
			prof.Use(s.middleware.iplimit)
		}

		s.handler.profiling.Register(prof)
	}

	// GraphQL
	graphql := api.Group("/graph")
	graphql.Use(gzipMiddleware)

	graphql.GET("", s.handler.graph.Playground)
	graphql.POST("/query", s.handler.graph.Query)

	// APIv3 router group
	v3 := api.Group("/v3")

	v3.Use(gzipMiddleware)

	s.setRoutesV3(v3)
}

func (s *server) setRoutesV3(v3 *echo.Group) {
	if s.v3handler.widget != nil {
		// The widget endpoint should not be blocked by auth
		s.router.GET("/api/v3/widget/process/:id", s.v3handler.widget.Get)
	}

	// v3 IAM
	if s.v3handler.iam != nil {
		v3.GET("/iam/user", s.v3handler.iam.ListIdentities)
		v3.GET("/iam/user/:name", s.v3handler.iam.GetIdentity)

		if !s.readOnly {
			v3.POST("/iam/user", s.v3handler.iam.AddIdentity)
			v3.PUT("/iam/user/:name", s.v3handler.iam.UpdateIdentity)
			v3.PUT("/iam/user/:name/policy", s.v3handler.iam.UpdateIdentityPolicies)
			v3.DELETE("/iam/user/:name", s.v3handler.iam.RemoveIdentity)
		}
	}

	// v3 Restreamer
	if s.v3handler.restream != nil {
		v3.GET("/skills", s.v3handler.restream.Skills)
		v3.GET("/skills/reload", s.v3handler.restream.ReloadSkills)

		v3.GET("/process", s.v3handler.restream.GetAll)
		v3.GET("/process/:id", s.v3handler.restream.Get)

		v3.GET("/process/:id/config", s.v3handler.restream.GetConfig)
		v3.GET("/process/:id/state", s.v3handler.restream.GetState)
		v3.GET("/process/:id/report", s.v3handler.restream.GetReport)

		v3.GET("/process/:id/metadata", s.v3handler.restream.GetProcessMetadata)
		v3.GET("/process/:id/metadata/:key", s.v3handler.restream.GetProcessMetadata)

		v3.GET("/metadata", s.v3handler.restream.GetMetadata)
		v3.GET("/metadata/:key", s.v3handler.restream.GetMetadata)

		if !s.readOnly {
			v3.POST("/process/probe", s.v3handler.restream.ProbeConfig)
			v3.GET("/process/:id/probe", s.v3handler.restream.Probe)
			v3.POST("/process", s.v3handler.restream.Add)
			v3.PUT("/process/:id", s.v3handler.restream.Update)
			v3.DELETE("/process/:id", s.v3handler.restream.Delete)
			v3.PUT("/process/:id/command", s.v3handler.restream.Command)
			v3.PUT("/process/:id/metadata/:key", s.v3handler.restream.SetProcessMetadata)
			v3.PUT("/metadata/:key", s.v3handler.restream.SetMetadata)
		}

		// v3 Playout
		v3.GET("/process/:id/playout/:inputid/status", s.v3handler.restream.PlayoutStatus)
		v3.GET("/process/:id/playout/:inputid/reopen", s.v3handler.restream.PlayoutReopenInput)
		v3.GET("/process/:id/playout/:inputid/keyframe/*", s.v3handler.restream.PlayoutKeyframe)
		v3.GET("/process/:id/playout/:inputid/errorframe/encode", s.v3handler.restream.PlayoutEncodeErrorframe)

		if !s.readOnly {
			v3.PUT("/process/:id/playout/:inputid/errorframe/*", s.v3handler.restream.PlayoutSetErrorframe)
			v3.POST("/process/:id/playout/:inputid/errorframe/*", s.v3handler.restream.PlayoutSetErrorframe)

			v3.PUT("/process/:id/playout/:inputid/stream", s.v3handler.restream.PlayoutSetStream)
		}

		// v3 Report
		v3.GET("/report/process", s.v3handler.restream.SearchReportHistory)
	}

	// v3 Filesystems
	fshandlers := map[string]api.FSConfig{}
	for _, fs := range s.filesystems {
		fshandlers[fs.Name] = api.FSConfig{
			Type:       fs.Filesystem.Type(),
			Mountpoint: fs.Mountpoint,
			Handler:    fs.apihandler,
		}
	}

	handler := api.NewFS(fshandlers)

	v3.GET("/fs", handler.List)
	v3.PUT("/fs", handler.FileOperation)

	v3.GET("/fs/:storage", handler.ListFiles)
	v3.GET("/fs/:storage/*", handler.GetFile, mwmime.NewWithConfig(mwmime.Config{
		MimeTypesFile:      s.mimeTypesFile,
		DefaultContentType: "application/data",
	}))
	v3.HEAD("/fs/:storage/*", handler.GetFile, mwmime.NewWithConfig(mwmime.Config{
		MimeTypesFile:      s.mimeTypesFile,
		DefaultContentType: "application/data",
	}))

	if !s.readOnly {
		v3.PUT("/fs/:storage/*", handler.PutFile)
		v3.DELETE("/fs/:storage", handler.DeleteFiles)
		v3.DELETE("/fs/:storage/*", handler.DeleteFile)
	}

	// v3 RTMP
	if s.v3handler.rtmp != nil {
		v3.GET("/rtmp", s.v3handler.rtmp.ListChannels)
	}

	// v3 SRT
	if s.v3handler.srt != nil {
		v3.GET("/srt", s.v3handler.srt.ListChannels)
	}

	// v3 Config
	if s.v3handler.config != nil {
		v3.GET("/config", s.v3handler.config.Get)

		if !s.readOnly {
			v3.PUT("/config", s.v3handler.config.Set)
			v3.GET("/config/reload", s.v3handler.config.Reload)
		}
	}

	// v3 Session
	if s.v3handler.session != nil {
		v3.GET("/session", s.v3handler.session.Summary)
		v3.GET("/session/active", s.v3handler.session.Active)
		v3.PUT("/session/token/:username", s.v3handler.session.CreateToken)
	}

	// v3 Cluster
	if s.v3handler.cluster != nil {
		v3.GET("/cluster", s.v3handler.cluster.About)
		v3.GET("/cluster/healthy", s.v3handler.cluster.Healthy)

		v3.GET("/cluster/snapshot", s.v3handler.cluster.GetSnapshot)

		v3.GET("/cluster/db/process", s.v3handler.cluster.ListStoreProcesses)
		v3.GET("/cluster/db/process/:id", s.v3handler.cluster.GetStoreProcess)
		v3.GET("/cluster/db/user", s.v3handler.cluster.ListStoreIdentities)
		v3.GET("/cluster/db/user/:name", s.v3handler.cluster.ListStoreIdentity)
		v3.GET("/cluster/db/policies", s.v3handler.cluster.ListStorePolicies)
		v3.GET("/cluster/db/locks", s.v3handler.cluster.ListStoreLocks)
		v3.GET("/cluster/db/kv", s.v3handler.cluster.ListStoreKV)
		v3.GET("/cluster/db/map/process", s.v3handler.cluster.GetStoreProcessNodeMap)
		v3.GET("/cluster/db/node", s.v3handler.cluster.ListStoreNodes)

		v3.GET("/cluster/iam/user", s.v3handler.cluster.ListIdentities)
		v3.GET("/cluster/iam/user/:name", s.v3handler.cluster.ListIdentity)
		v3.GET("/cluster/iam/policies", s.v3handler.cluster.ListPolicies)

		v3.GET("/cluster/process", s.v3handler.cluster.GetAllProcesses)
		v3.GET("/cluster/process/:id", s.v3handler.cluster.GetProcess)
		v3.GET("/cluster/process/:id/metadata", s.v3handler.cluster.GetProcessMetadata)
		v3.GET("/cluster/process/:id/metadata/:key", s.v3handler.cluster.GetProcessMetadata)

		v3.GET("/cluster/node", s.v3handler.cluster.GetNodes)
		v3.GET("/cluster/node/:id", s.v3handler.cluster.GetNode)
		v3.GET("/cluster/node/:id/files", s.v3handler.cluster.GetNodeResources)
		v3.GET("/cluster/node/:id/fs/:storage", s.v3handler.cluster.NodeFSListFiles)
		v3.GET("/cluster/node/:id/fs/:storage/*", s.v3handler.cluster.NodeFSGetFile)
		v3.GET("/cluster/node/:id/process", s.v3handler.cluster.ListNodeProcesses)
		v3.GET("/cluster/node/:id/version", s.v3handler.cluster.GetNodeVersion)
		v3.GET("/cluster/node/:id/state", s.v3handler.cluster.GetNodeState)

		v3.GET("/cluster/fs/:storage", s.v3handler.cluster.ListFiles)

		if !s.readOnly {
			v3.PUT("/cluster/transfer/:id", s.v3handler.cluster.TransferLeadership)
			v3.PUT("/cluster/leave", s.v3handler.cluster.Leave)

			v3.POST("/cluster/process", s.v3handler.cluster.AddProcess)
			v3.POST("/cluster/process/probe", s.v3handler.cluster.ProbeProcessConfig)
			v3.PUT("/cluster/process/:id", s.v3handler.cluster.UpdateProcess)
			v3.GET("/cluster/process/:id/probe", s.v3handler.cluster.ProbeProcess)
			v3.DELETE("/cluster/process/:id", s.v3handler.cluster.DeleteProcess)
			v3.PUT("/cluster/process/:id/command", s.v3handler.cluster.SetProcessCommand)
			v3.PUT("/cluster/process/:id/metadata/:key", s.v3handler.cluster.SetProcessMetadata)

			v3.PUT("/cluster/reallocation", s.v3handler.cluster.Reallocation)

			v3.DELETE("/cluster/node/:id/fs/:storage/*", s.v3handler.cluster.NodeFSDeleteFile)
			v3.PUT("/cluster/node/:id/fs/:storage/*", s.v3handler.cluster.NodeFSPutFile)
			v3.PUT("/cluster/node/:id/state", s.v3handler.cluster.SetNodeState)

			v3.PUT("/cluster/iam/reload", s.v3handler.cluster.ReloadIAM)
			v3.POST("/cluster/iam/user", s.v3handler.cluster.AddIdentity)
			v3.PUT("/cluster/iam/user/:name", s.v3handler.cluster.UpdateIdentity)
			v3.PUT("/cluster/iam/user/:name/policy", s.v3handler.cluster.UpdateIdentityPolicies)
			v3.DELETE("/cluster/iam/user/:name", s.v3handler.cluster.RemoveIdentity)
		}
	}

	// v3 Log
	if s.v3handler.log != nil {
		v3.GET("/log", s.v3handler.log.Log)
	}

	// v3 Metrics
	if s.v3handler.resources != nil {
		v3.GET("/metrics", s.v3handler.resources.Describe)
		v3.POST("/metrics", s.v3handler.resources.Metrics)
	}

	// v3 Events
	if s.v3handler.events != nil {
		v3.POST("/events", s.v3handler.events.Events)
	}
}
