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
	"maps"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/datarhei/core/v16/cluster"
	cfgstore "github.com/datarhei/core/v16/config/store"
	"github.com/datarhei/core/v16/event"
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
	"github.com/datarhei/core/v16/resources"
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
	Logger           log.Logger
	LogBuffer        log.BufferWriter
	LogEvents        event.EventSource
	Restream         restream.Restreamer
	Metrics          monitor.HistoryReader
	Prometheus       prometheus.Reader
	MimeTypesFile    string
	MimeTypes        map[string]string
	Filesystems      []fs.FS
	IPLimiter        net.IPLimitValidator
	Profiling        bool
	Cors             CorsConfig
	RTMP             rtmp.Server
	RTMPHTTPFLVMount string
	SRT              srt.Server
	Config           cfgstore.Store
	Cache            cache.Cacher
	Sessions         session.RegistryReader
	Router           router.Router
	ReadOnly         bool
	Cluster          cluster.Cluster
	IAM              iam.IAM
	IAMSkipper       func(ip string) bool
	Resources        resources.Resources
	Compress         CompressConfig
}

type CorsConfig struct {
	Origins []string
}

type CompressConfig struct {
	Encoding  []string
	MimeTypes []string
	MinLength int
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
		process   *api.ProcessHandler
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

	compress struct {
		encoding  []string
		mimetypes []string
		minLength int
	}

	filesystems map[string]*filesystem

	router        *echo.Echo
	mimeTypesFile string
	mimeTypes     map[string]string
	profiling     bool
	httpFLVMount  string

	readOnly bool

	metrics struct {
		lock   sync.Mutex
		status map[string]uint64
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
		mimeTypes:     config.MimeTypes,
		profiling:     config.Profiling,
		readOnly:      config.ReadOnly,
	}

	s.metrics.status = map[string]uint64{}

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
				httpfs.Filesystem = fs.NewClusterFS(httpfs.Filesystem.Name(), httpfs.Filesystem, config.Cluster.Manager())
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
		config.Resources,
		func() []string { return config.IAM.Validators() },
	)

	s.handler.jwt = api.NewJWT(config.IAM)

	s.v3handler.iam = api.NewIAM(config.IAM)

	s.v3handler.log = api.NewLog(
		config.LogBuffer,
	)

	s.v3handler.events = api.NewEvents()

	s.v3handler.events.SetLogSource(config.LogEvents)
	for name, fs := range s.filesystems {
		s.v3handler.events.AddMediaSource(name, fs.Filesystem)
	}
	s.v3handler.events.AddMediaSource("srt", config.SRT)
	s.v3handler.events.AddMediaSource("rtmp", config.RTMP)
	s.v3handler.events.SetProcessSource(config.Restream)

	if config.Restream != nil {
		s.v3handler.process = api.NewProcess(
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

		if len(config.RTMPHTTPFLVMount) != 0 {
			s.httpFLVMount = config.RTMPHTTPFLVMount
		}
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
		Status: func(code int, method, path string, size int64, ttfb time.Duration) {
			key := fmt.Sprintf("%d:%s:%s", code, method, path)

			s.metrics.lock.Lock()
			defer s.metrics.lock.Unlock()

			s.metrics.status[key]++
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

	s.compress.encoding = config.Compress.Encoding
	s.compress.mimetypes = config.Compress.MimeTypes
	s.compress.minLength = config.Compress.MinLength

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

	s.router.Use(mwcompress.NewWithConfig(mwcompress.Config{
		Level:        mwcompress.BestSpeed,
		MinLength:    config.Compress.MinLength,
		Schemes:      config.Compress.Encoding,
		ContentTypes: config.Compress.MimeTypes,
	}))

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

func (s *server) HTTPStatus() map[string]uint64 {
	status := map[string]uint64{}

	s.metrics.lock.Lock()
	defer s.metrics.lock.Unlock()

	maps.Copy(status, s.metrics.status)

	return status
}

func (s *server) setRoutes() {
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
			MimeTypes:          s.mimeTypes,
			DefaultContentType: filesystem.DefaultContentType,
		}))

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

	// RTMP over HTTP
	if s.v3handler.rtmp != nil && len(s.httpFLVMount) != 0 {
		s.router.GET(s.httpFLVMount+"/*", s.v3handler.rtmp.Play)
	}

	// GraphQL
	graphql := api.Group("/graph")
	//graphql.Use(gzipMiddleware)

	graphql.GET("", s.handler.graph.Playground)
	graphql.POST("/query", s.handler.graph.Query)

	// APIv3 router group
	v3 := api.Group("/v3")

	//v3.Use(gzipMiddleware)

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
	if s.v3handler.process != nil {
		v3.GET("/skills", s.v3handler.process.Skills)
		v3.GET("/skills/reload", s.v3handler.process.ReloadSkills)

		v3.GET("/process", s.v3handler.process.GetAll)
		v3.GET("/process/:id", s.v3handler.process.Get)

		v3.GET("/process/:id/config", s.v3handler.process.GetConfig)
		v3.GET("/process/:id/state", s.v3handler.process.GetState)
		v3.GET("/process/:id/report", s.v3handler.process.GetReport)

		v3.GET("/process/:id/metadata", s.v3handler.process.GetProcessMetadata)
		v3.GET("/process/:id/metadata/:key", s.v3handler.process.GetProcessMetadata)

		v3.GET("/metadata", s.v3handler.process.GetMetadata)
		v3.GET("/metadata/:key", s.v3handler.process.GetMetadata)

		if !s.readOnly {
			v3.POST("/process/probe", s.v3handler.process.ProbeConfig)
			v3.POST("/process/validate", s.v3handler.process.ValidateConfig)
			v3.GET("/process/:id/probe", s.v3handler.process.Probe)
			v3.POST("/process", s.v3handler.process.Add)
			v3.PUT("/process/:id", s.v3handler.process.Update)
			v3.DELETE("/process/:id", s.v3handler.process.Delete)
			v3.PUT("/process/:id/command", s.v3handler.process.Command)
			v3.PUT("/process/:id/report", s.v3handler.process.SetReport)
			v3.PUT("/process/:id/metadata/:key", s.v3handler.process.SetProcessMetadata)
			v3.PUT("/metadata/:key", s.v3handler.process.SetMetadata)
		}

		// v3 Playout
		v3.GET("/process/:id/playout/:inputid/status", s.v3handler.process.PlayoutStatus)
		v3.GET("/process/:id/playout/:inputid/reopen", s.v3handler.process.PlayoutReopenInput)
		v3.GET("/process/:id/playout/:inputid/keyframe/*", s.v3handler.process.PlayoutKeyframe)
		v3.GET("/process/:id/playout/:inputid/errorframe/encode", s.v3handler.process.PlayoutEncodeErrorframe)

		if !s.readOnly {
			v3.PUT("/process/:id/playout/:inputid/errorframe/*", s.v3handler.process.PlayoutSetErrorframe)
			v3.POST("/process/:id/playout/:inputid/errorframe/*", s.v3handler.process.PlayoutSetErrorframe)

			v3.PUT("/process/:id/playout/:inputid/stream", s.v3handler.process.PlayoutSetStream)
		}

		// v3 Report
		v3.GET("/report/process", s.v3handler.process.SearchReportHistory)
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
		v3.GET("/cluster/deployments", s.v3handler.cluster.Deployments)

		v3.GET("/cluster/db/process", s.v3handler.cluster.StoreListProcesses)
		v3.GET("/cluster/db/process/:id", s.v3handler.cluster.StoreGetProcess)
		v3.GET("/cluster/db/user", s.v3handler.cluster.StoreListIdentities)
		v3.GET("/cluster/db/user/:name", s.v3handler.cluster.StoreGetIdentity)
		v3.GET("/cluster/db/policies", s.v3handler.cluster.StoreListPolicies)
		v3.GET("/cluster/db/locks", s.v3handler.cluster.StoreListLocks)
		v3.GET("/cluster/db/kv", s.v3handler.cluster.StoreListKV)
		v3.GET("/cluster/db/map/process", s.v3handler.cluster.StoreGetProcessNodeMap)
		v3.GET("/cluster/db/map/reallocate", s.v3handler.cluster.StoreGetProcessRelocateMap)
		v3.GET("/cluster/db/node", s.v3handler.cluster.StoreListNodes)

		v3.GET("/cluster/iam/user", s.v3handler.cluster.IAMIdentityList)
		v3.GET("/cluster/iam/user/:name", s.v3handler.cluster.IAMIdentityGet)
		v3.GET("/cluster/iam/policies", s.v3handler.cluster.IAMPolicyList)

		v3.GET("/cluster/process", s.v3handler.cluster.ProcessList)
		v3.GET("/cluster/process/:id", s.v3handler.cluster.ProcessGet)
		v3.GET("/cluster/process/:id/metadata", s.v3handler.cluster.ProcessGetMetadata)
		v3.GET("/cluster/process/:id/metadata/:key", s.v3handler.cluster.ProcessGetMetadata)

		v3.GET("/cluster/node", s.v3handler.cluster.NodeList)
		v3.GET("/cluster/node/:id", s.v3handler.cluster.NodeGet)
		v3.GET("/cluster/node/:id/files", s.v3handler.cluster.NodeGetMedia)
		v3.GET("/cluster/node/:id/fs/:storage", s.v3handler.cluster.NodeFSListFiles)
		v3.GET("/cluster/node/:id/fs/:storage/*", s.v3handler.cluster.NodeFSGetFile)
		v3.GET("/cluster/node/:id/process", s.v3handler.cluster.NodeListProcesses)
		v3.GET("/cluster/node/:id/version", s.v3handler.cluster.NodeGetVersion)
		v3.GET("/cluster/node/:id/state", s.v3handler.cluster.NodeGetState)

		v3.GET("/cluster/fs/:storage", s.v3handler.cluster.FilesystemListFiles)

		v3.POST("/cluster/events", s.v3handler.cluster.LogEvents)
		v3.POST("/cluster/events/log", s.v3handler.cluster.LogEvents)
		v3.POST("/cluster/events/process", s.v3handler.cluster.ProcessEvents)

		if !s.readOnly {
			v3.PUT("/cluster/transfer/:id", s.v3handler.cluster.TransferLeadership)
			v3.PUT("/cluster/leave", s.v3handler.cluster.Leave)

			v3.POST("/cluster/process", s.v3handler.cluster.ProcessAdd)
			v3.POST("/cluster/process/probe", s.v3handler.cluster.ProcessProbeConfig)
			v3.PUT("/cluster/process/:id", s.v3handler.cluster.ProcessUpdate)
			v3.GET("/cluster/process/:id/probe", s.v3handler.cluster.ProcessProbe)
			v3.DELETE("/cluster/process/:id", s.v3handler.cluster.ProcessDelete)
			v3.PUT("/cluster/process/:id/command", s.v3handler.cluster.ProcessSetCommand)
			v3.PUT("/cluster/process/:id/metadata/:key", s.v3handler.cluster.ProcessSetMetadata)

			v3.PUT("/cluster/reallocation", s.v3handler.cluster.Reallocation)

			v3.DELETE("/cluster/node/:id/fs/:storage/*", s.v3handler.cluster.NodeFSDeleteFile)
			v3.PUT("/cluster/node/:id/fs/:storage/*", s.v3handler.cluster.NodeFSPutFile)
			v3.PUT("/cluster/node/:id/state", s.v3handler.cluster.NodeSetState)

			v3.PUT("/cluster/iam/reload", s.v3handler.cluster.IAMReload)
			v3.POST("/cluster/iam/user", s.v3handler.cluster.IAMIdentityAdd)
			v3.PUT("/cluster/iam/user/:name", s.v3handler.cluster.IAMIdentityUpdate)
			v3.PUT("/cluster/iam/user/:name/policy", s.v3handler.cluster.IAMIdentityUpdatePolicies)
			v3.DELETE("/cluster/iam/user/:name", s.v3handler.cluster.IAMIdentityRemove)
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
		v3.POST("/events", s.v3handler.events.LogEvents)
		v3.POST("/events/log", s.v3handler.events.LogEvents)
		v3.POST("/events/media/:type", s.v3handler.events.MediaEvents)
		v3.POST("/events/process", s.v3handler.events.ProcessEvents)
	}
}
