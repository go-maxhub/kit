package kit

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/felixge/fgprof"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"

	gincore "github.com/gin-gonic/gin"
	chicore "github.com/go-chi/chi/v5"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	zapl "go.uber.org/zap"
	grpccore "google.golang.org/grpc"

	"github.com/go-maxhub/kit/kit/metric"
	"github.com/go-maxhub/kit/kit/servers/chi"
	"github.com/go-maxhub/kit/kit/servers/gin"
	"github.com/go-maxhub/kit/kit/servers/grpc"
)

const (
	defaultHTTPAddr   = "0.0.0.0:8080"
	defaultGRPCAddr   = "0.0.0.0:8081"
	grpcHeader        = "application/grpc"
	defaultFgrpofAddr = "0.0.0.0:6060"
	defaultPromAddr   = "0.0.0.0:9090"
	fgprofUrl         = "/debug/fgprof"
)

// mixHTTPAndGRPC configures mixed http and grpc handler.
func mixHTTPAndGRPC(httpHandler http.Handler, grpcHandler *grpccore.Server) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor == 2 && strings.HasPrefix(r.Header.Get("content-type"), grpcHeader) {
			grpcHandler.ServeHTTP(w, r)
			return
		}
		httpHandler.ServeHTTP(w, r)
	})
}

// Server describes all services configurations, must be executed with Start.
type Server struct {
	// !ATTENTION! Options must be set before Start.
	ServerName    string
	ServerVersion string

	envVars Env

	RootCtx context.Context

	servers []func() error

	httpAddr  string
	GinServer *gincore.Engine
	ChiServer *chicore.Mux

	tp *sdktrace.TracerProvider

	grpcAddr   string
	GRPCServer *grpccore.Server

	// Starts 2 services on different ports, 8080 and 8081 if any others not provided in Server options.
	parallelMode bool

	// Loggers.
	DefaultLogger *zapl.Logger

	// Metrics
	fgprofServer http.Handler

	promRegistry   *prometheus.Registry
	PromCollectors []prometheus.Collector

	// End-to-end tests
	tests e2eTests

	// Before and After funcs.
	beforeStart []func() error
	afterStart  []func() error
	afterStop   []func() error
	beforeStop  []func() error

	//Errgroup options
	customGoroutines []func() error
}

// New is base constructor function to create service with options, but won't start it without Start.
func New(options ...func(*Server)) *Server {
	srv := &Server{}
	srv.promRegistry = metric.InitPrometheusConfiguration()
	srv.RootCtx = context.Background()
	srv.DefaultLogger = initDefaultZapLogger()

	envVars, err := initEnvVars(srv.DefaultLogger)
	if err != nil {
		panic(err)
	}
	srv.envVars = envVars

	for _, o := range options {
		o(srv)
	}

	if srv.ServerName != "" {
		srv.DefaultLogger = srv.DefaultLogger.WithOptions(
			zapl.Fields(
				zapl.String("service.name", srv.ServerName)))
	}

	return srv
}

// defaultConfig sets Server struct default values for not provided options.
func (s *Server) defaultConfig() {
	if s.httpAddr == "" {
		s.httpAddr = defaultHTTPAddr
	}
	if s.grpcAddr == "" {
		s.grpcAddr = defaultGRPCAddr
	}

	s.ChiServer.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write([]byte("OK"))
		if err != nil {
			return
		}
	})
	s.ServerVersion = s.envVars.ServerVersion
	s.fgprofServer = fgprof.Handler()
}

// validateServers validate servers to prevent usage of nil client.
func (s *Server) validateServers() {
	if s.ChiServer == nil {
		s.DefaultLogger.Info("!ATTENTION! chi kit is not initialized")
	}
	if s.GRPCServer == nil {
		s.DefaultLogger.Info("!ATTENTION! grpc kit is not initialized")
	}
	if s.GinServer == nil {
		s.DefaultLogger.Info("!ATTENTION! gin kit is not initialized")
	}
}

func (s *Server) AddGracefulShutdown(ctx context.Context) {
	gs := func() error {
		// Guaranteed way to kill application.
		if len(s.beforeStop) > 0 {
			for _, fn := range s.beforeStop {
				if err := fn(); err != nil {
					s.DefaultLogger.Error("before stop func", zapl.Error(err))
				}
			}
		}
		<-ctx.Done()
		if len(s.afterStop) > 0 {
			for _, fn := range s.afterStop {
				if err := fn(); err != nil {
					s.DefaultLogger.Error("after stop func", zapl.Error(err))
				}
			}
		}
		// Context is canceled, giving application time to shut down gracefully.
		s.DefaultLogger.Info("Graceful shutdown initiated, waiting to terminate...")
		s.DefaultLogger.Info("Graceful shutdown config", zapl.String("gs.duration", s.envVars.GracefulShutdownTimeout.String()))
		time.Sleep(s.envVars.GracefulShutdownTimeout)

		// Probably deadlock, forcing shutdown.
		s.DefaultLogger.Fatal("Service terminated gracefully!")

		return nil
	}
	s.servers = append(s.servers, gs)
}

func (s *Server) AddFgprofServer() {
	s.DefaultLogger.Info("Starting fgprof endpoint...")
	http.DefaultServeMux.Handle(fgprofUrl, s.fgprofServer)
	fs := func() error {
		if err := http.ListenAndServe(defaultFgrpofAddr, nil); err != nil {
			s.DefaultLogger.Error("init fgprof kit", zapl.Error(err))
		}
		return nil
	}
	s.servers = append(s.servers, fs)
}

func (s *Server) AddPrometheusServer() {
	s.DefaultLogger.Info("Starting prometheus metric endpoint...")
	s.promRegistry.MustRegister(s.PromCollectors...)
	http.Handle("/metrics", promhttp.Handler())
	ps := func() error {
		if err := http.ListenAndServe(defaultPromAddr, nil); err != nil {
			s.DefaultLogger.Error("init prometheus kit", zapl.Error(err))
		}
		return nil
	}
	s.servers = append(s.servers, ps)
}

func (s *Server) AddChiServer() {
	cs := func() error {
		defer s.DefaultLogger.Info("Server stopped.")
		if err := http.ListenAndServe(s.httpAddr, s.ChiServer); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.DefaultLogger.Fatal("start chi kit", zapl.Error(err))
		}
		return nil
	}
	s.servers = append(s.servers, cs)
}

func (s *Server) AddGRPCServer() {
	lis, err := net.Listen("tcp", s.grpcAddr)
	if err != nil {
		panic(err)
	}
	grpcs := func() error {
		defer s.DefaultLogger.Info("Server stopped.")
		if err = s.GRPCServer.Serve(lis); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.DefaultLogger.Fatal("start grpc kit", zapl.Error(err))
		}
		return nil
	}
	s.servers = append(s.servers, grpcs)
}

func (s *Server) AddGinServer() {
	gs := func() error {
		defer s.DefaultLogger.Info("Server stopped.")
		if err := s.GinServer.Run(s.httpAddr); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.DefaultLogger.Fatal("start gin kit", zapl.Error(err))
		}
		return nil
	}
	s.servers = append(s.servers, gs)
}

func (s *Server) AddEndToEndTests(configPath string) {
	eet, err := loadTestConfig(configPath)
	if err != nil {
		s.DefaultLogger.Error("load end-to-end tests config", zapl.Error(err))
	}
	s.tests = *eet
}

func (s *Server) AddChiAndGRPCMixedServer() {
	mh := mixHTTPAndGRPC(s.ChiServer, s.GRPCServer)
	http2Server := &http2.Server{}
	http1Server := &http.Server{Handler: h2c.NewHandler(mh, http2Server)}
	lis, err := net.Listen("tcp", s.httpAddr)
	if err != nil {
		panic(err)
	}
	ms := func() error {
		defer s.DefaultLogger.Info("Server stopped.")

		if err = http1Server.Serve(lis); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.DefaultLogger.Fatal("start chi and grpc kit", zapl.Error(err))
		}
		return nil
	}
	s.servers = append(s.servers, ms)
}

func (s *Server) AddGinAndGRPCMixedServer() {
	grpc_health_v1.RegisterHealthServer(s.GRPCServer, health.NewServer())
	mh := mixHTTPAndGRPC(s.GinServer, s.GRPCServer)
	http2Server := &http2.Server{}
	http1Server := &http.Server{Handler: h2c.NewHandler(mh, http2Server)}
	lis, err := net.Listen("tcp", s.httpAddr)
	if err != nil {
		panic(err)
	}
	ms := func() error {
		defer s.DefaultLogger.Info("Server stopped.")
		if err := http1Server.Serve(lis); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.DefaultLogger.Fatal("start gin and grpc kit", zapl.Error(err))
		}
		return nil
	}
	s.servers = append(s.servers, ms)
}

func (s *Server) startServer() error {
	mode := flag.String("mode", "default", "execute end-to-end tests and shutdown service after")
	flag.Parse()

	s.defaultConfig()

	s.DefaultLogger.Info("Starting service...")

	if len(s.beforeStart) > 0 {
		for _, fn := range s.beforeStart {
			if err := fn(); err != nil {
				s.DefaultLogger.Error("before start func", zapl.Error(err))
			}
		}
	}

	ctx, cancel := signal.NotifyContext(s.RootCtx, os.Interrupt)
	defer cancel()

	s.DefaultLogger.Info(
		"OTEL options",
		zapl.String("KIT_TRACING_JAEGER_HOST", os.Getenv("KIT_TRACING_JAEGER_HOST")),
	)

	g, ctx := errgroup.WithContext(ctx)

	defer func() {
		if err := s.tp.Shutdown(ctx); err != nil {
			log.Printf("shutdown tracer provider: %v", err)
		}
	}()

	s.AddPrometheusServer()

	s.DefaultLogger.Info("Initialized with ports",
		zapl.String("http.httpAddr", s.httpAddr),
		zapl.String("grpc.httpAddr", s.grpcAddr),
		zapl.String("fgprof.httpAddr", defaultFgrpofAddr),
		zapl.String("prometheus.httpAddr", defaultPromAddr),
	)

	if s.envVars.FgprofEnable {
		s.AddFgprofServer()
	}

	if s.envVars.PprofEnable {
		s.DefaultLogger.Info("Starting pprof endpoint...")
		s.ChiServer.Mount("/debug", middleware.Profiler())
	}

	s.AddGracefulShutdown(ctx)

	if len(s.customGoroutines) > 0 {
		for _, fn := range s.customGoroutines {
			g.Go(fn)
		}
	}

	if len(s.afterStart) > 0 {
		for _, fn := range s.afterStart {
			if err := fn(); err != nil {
				s.DefaultLogger.Error("after start func", zapl.Error(err))
			}
		}
	}

	for _, server := range s.servers {
		srv := server
		g.Go(func() error {
			return srv()
		})
	}

	s.validateServers()

	if *mode == "e2e" {
		// hack preventing server no initialized
		time.Sleep(5 * time.Second)

		s.DefaultLogger.Info("Running end-to-end tests...")
		s.DefaultLogger.Info(fmt.Sprintf("Found %d end-to-end tests to execute", len(s.tests)))

		for _, eet := range s.tests {
			s.runTest(eet)
		}

		s.DefaultLogger.Info("All end-to-end passed successfully!")
		os.Exit(0)
	}
	return g.Wait()
}

// Start runs servers with provided options.
func (s *Server) Start() error {
	err := s.startServer()
	if err != nil {
		return err
	}
	return nil
}

// WithServerName sets name of service.
func WithServerName(name string) func(*Server) {
	return func(s *Server) {
		switch {
		case name == "":
			panic("kit name evaluated, but not defined")
		default:
			s.ServerName = name
		}
	}
}

// WithEndToEndTests sets name of service.
func WithEndToEndTests(configPath string) func(*Server) {
	return func(s *Server) {
		switch {
		case configPath == "":
			panic("path to config name evaluated, but not defined")
		default:
			s.AddEndToEndTests(configPath)
		}
	}
}

// WithHTTPServerPort sets provided port to http kit.
func WithHTTPServerPort(port string) func(*Server) {
	return func(s *Server) {
		switch {
		case port == "":
			panic("http port evaluated, but not defined")
		default:
			s.httpAddr = "0.0.0.0:" + port
		}
	}
}

// WithGRPCServerPort sets provided port to grpc kit.
func WithGRPCServerPort(port string) func(*Server) {
	return func(s *Server) {
		switch {
		case port == "":
			panic("grpc port evaluated, but not defined")
		default:
			s.grpcAddr = "0.0.0.0:" + port
		}
	}
}

// WithParallelMode sets http and grpc servers to bind on one port and segregate requests by headers.
func WithParallelMode() func(*Server) {
	return func(s *Server) {
		s.parallelMode = true
	}
}

// WithGinServer provides gin http kit and runs it after Start.
func WithGinServer(cfg gin.Config) func(*Server) {
	return func(s *Server) {
		switch {
		case cfg.Blank:
			s.GinServer = cfg.NewBlankGin()
		case cfg.Default:
			s.GinServer = cfg.NewDefaultGin()
		default:
			s.GinServer = cfg.NewDefaultGin()
		}
		s.AddGinServer()
	}
}

// WithChiServer provides chi http kit and runs it after Start.
func WithChiServer(cfg chi.Config) func(*Server) {
	return func(s *Server) {
		srv, tp := cfg.NewDefaultChi(s.RootCtx, s.DefaultLogger, s.ServerName, s.ServerVersion, s.envVars.OTELJaegerHost, s.envVars.DebugHeaders)
		s.ChiServer = srv
		s.tp = tp
		s.AddChiServer()
	}
}

// WithChiAndGRPCServer provides chi http and grpc servers and runs them after Start.
func WithChiAndGRPCServer(cfg chi.Config, gcfg grpc.Config) func(*Server) {
	return func(s *Server) {
		srv, tp := cfg.NewDefaultChi(s.RootCtx, s.DefaultLogger, s.ServerName, s.ServerVersion, s.envVars.OTELJaegerHost, s.envVars.DebugHeaders)
		s.ChiServer = srv
		s.tp = tp

		switch {
		case cfg.Default:
			s.GRPCServer = gcfg.NewDefaultGRPCServer()
		default:
			s.GRPCServer = gcfg.NewDefaultGRPCServer()
		}

		if s.parallelMode {
			s.AddChiAndGRPCMixedServer()
		} else {
			s.AddGRPCServer()
			s.AddChiServer()
		}
	}
}

// WithGinAndGRPCServer provides gin http and grpc servers and runs them after Start.
func WithGinAndGRPCServer(cfg gin.Config, gcfg grpc.Config) func(*Server) {
	return func(s *Server) {
		switch {
		case cfg.Blank:
			s.GinServer = cfg.NewBlankGin()
		case cfg.Default:
			s.GinServer = cfg.NewDefaultGin()
		default:
			s.GinServer = cfg.NewDefaultGin()
		}

		switch {
		case cfg.Default:
			s.GRPCServer = gcfg.NewDefaultGRPCServer()
		default:
			s.GRPCServer = gcfg.NewDefaultGRPCServer()
		}

		if s.parallelMode {
			s.AddGinAndGRPCMixedServer()
		} else {
			s.AddGRPCServer()
			s.AddGinServer()
		}
	}
}

// WithGRPCServer provides grpc kit and runs it after Start.
func WithGRPCServer(cfg grpc.Config) func(*Server) {
	return func(s *Server) {
		switch {
		case cfg.Default:
			s.GRPCServer = cfg.NewDefaultGRPCServer()
		default:
			s.GRPCServer = cfg.NewDefaultGRPCServer()
		}
		s.AddGRPCServer()
	}
}

// WithBeforeStart executes functions before servers start.
func WithBeforeStart(funcs []func() error) func(*Server) {
	return func(s *Server) {
		s.beforeStart = funcs
	}
}

// WithAfterStart executes functions after servers start.
func WithAfterStart(funcs []func() error) func(*Server) {
	return func(s *Server) {
		s.afterStart = funcs
	}
}

// WithAfterStop executes functions after servers stop.
func WithAfterStop(funcs []func() error) func(*Server) {
	return func(s *Server) {
		s.afterStop = funcs
	}
}

// WithBeforeStop executes functions before servers stop.
func WithBeforeStop(funcs []func() error) func(*Server) {
	return func(s *Server) {
		s.beforeStop = funcs
	}
}

// WithCustomGoroutines adds goroutines to main errgroup instance of kit.
func WithCustomGoroutines(funcs []func() error) func(*Server) {
	return func(s *Server) {
		s.customGoroutines = funcs
	}
}
