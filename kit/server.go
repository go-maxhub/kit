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
	"time"

	"github.com/felixge/fgprof"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
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
)

const (
	defaultHTTPAddr   = "0.0.0.0:8080"
	defaultGRPCAddr   = "0.0.0.0:8081"
	grpcHeader        = "application/grpc"
	defaultFgrpofAddr = "0.0.0.0:6060"
	defaultPromAddr   = "0.0.0.0:9090"
	fgprofUrl         = "/debug/fgprof"
)

// Server describes all services configurations, must be executed with Start.
type Server struct {
	// !ATTENTION! Options must be set before Start.
	ServerName    string
	ServerVersion string

	envVars env

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

	// otelMetrics
	fgprofServer http.Handler

	promRegistry   *prometheus.Registry
	PromCollectors []prometheus.Collector

	// End-to-end tests
	tests E2eTests

	// Before and After funcs.
	beforeStart []func() error
	afterStart  []func() error
	afterStop   []func() error
	beforeStop  []func() error

	//Errgroup options
	customGoroutines []func() error
}

// New is base constructor function to create service with options, but won't work without Start.
func New(options ...func(*Server)) *Server {
	srv := &Server{}

	srv.promRegistry = initPrometheusConfiguration()
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
		s.DefaultLogger.Info("!ATTENTION! metrics kit is not initialized")
	}
	if s.GRPCServer == nil {
		s.DefaultLogger.Info("!ATTENTION! grpc kit is not initialized")
	}
	if s.GinServer == nil {
		s.DefaultLogger.Info("!ATTENTION! gin kit is not initialized")
	}
}

// addGracefulShutdown adds graceful shutdown to server.
func (s *Server) addGracefulShutdown(ctx context.Context) {
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

// addChiServer adds metrics engine to server.
func (s *Server) addChiServer() {
	cs := func() error {
		defer s.DefaultLogger.Info("Server stopped.")
		if err := http.ListenAndServe(s.httpAddr, s.ChiServer); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.DefaultLogger.Fatal("start metrics kit", zapl.Error(err))
		}
		return nil
	}
	s.servers = append(s.servers, cs)
}

// addGinServer adds gin engine to server.
func (s *Server) addGinServer() {
	gs := func() error {
		defer s.DefaultLogger.Info("Server stopped.")
		if err := s.GinServer.Run(s.httpAddr); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.DefaultLogger.Fatal("start gin kit", zapl.Error(err))
		}
		return nil
	}
	s.servers = append(s.servers, gs)
}

// addChiAndGRPCMixedServer adds metrics and grpc mixed engines to server.
func (s *Server) addChiAndGRPCMixedServer() {
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
			s.DefaultLogger.Fatal("start metrics and grpc kit", zapl.Error(err))
		}
		return nil
	}
	s.servers = append(s.servers, ms)
}

// addGinAndGRPCMixedServer adds gin and grpc mixed engines to server.
func (s *Server) addGinAndGRPCMixedServer() {
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

// startServer starts all servers.
func (s *Server) startServer() error {
	mode := flag.String("mode", "default", "execute end-to-end tests and shutdown service after")
	flag.Parse()

	s.defaultConfig()

	s.DefaultLogger.Info("Starting service...")

	s.addPyroscopeExporter()

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

	s.addPrometheusServer()

	s.DefaultLogger.Info("Initialized with ports",
		zapl.String("http.httpAddr", s.httpAddr),
		zapl.String("grpc.httpAddr", s.grpcAddr),
		zapl.String("fgprof.httpAddr", defaultFgrpofAddr),
		zapl.String("prometheus.httpAddr", defaultPromAddr),
	)

	if s.envVars.FgprofEnable {
		s.addFgprofServer()
	}

	if s.envVars.PprofEnable {
		s.DefaultLogger.Info("Starting pprof endpoint...")
		s.ChiServer.Mount("/debug", middleware.Profiler())
	}

	s.addGracefulShutdown(ctx)

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

// Start starts all engine.
// Start runs engine with provided options.
func (s *Server) Start() error {
	err := s.startServer()
	if err != nil {
		return err
	}
	return nil
}
