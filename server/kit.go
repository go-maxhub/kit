package kit

import (
	"context"
	"errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"kit/server/metrics"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"

	gincore "github.com/gin-gonic/gin"
	chicore "github.com/go-chi/chi/v5"
	zapl "go.uber.org/zap"
	grpccore "google.golang.org/grpc"
	pb "google.golang.org/grpc/examples/features/proto/echo"

	"kit/server/servers/chi"
	"kit/server/servers/fgprof"
	"kit/server/servers/gin"
	"kit/server/servers/grpc"
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
	serverName string

	httpAddr  string
	GinServer *gincore.Engine
	ChiServer *chicore.Mux

	grpcAddr   string
	GRPCServer *grpccore.Server

	// Starts 2 services on different ports, 8080 and 8081 if any others not provided in Server options.
	parallelMode bool

	// Loggers.
	DefaultLogger *zapl.Logger

	// Metrics
	fgprofServer   *http.Handler
	fgprofAddr     string
	promRegistry   *prometheus.Registry
	PromCollectors []prometheus.Collector

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
	for _, o := range options {
		o(srv)
	}
	srv.DefaultLogger = initDefaultZapLogger(srv.serverName)
	srv.promRegistry = metrics.InitPrometheusConfiguration()
	return srv
}

type echoServer struct {
	pb.UnimplementedEchoServer
}

// defaultConfig sets Server struct default values for not provided options.
func (s *Server) defaultConfig() {
	if s.httpAddr == "" {
		s.httpAddr = defaultHTTPAddr
	}
	if s.grpcAddr == "" {
		s.grpcAddr = defaultGRPCAddr
	}
	if s.fgprofAddr == "" {
		s.fgprofAddr = defaultFgrpofAddr
	}
}

// validateServers validate servers to prevent usage of nil client.
func (s *Server) validateServers() {
	if s.ChiServer == nil {
		s.DefaultLogger.Info("!ATTENTION! chi server is not initialized")
	}
	if s.GRPCServer == nil {
		s.DefaultLogger.Info("!ATTENTION! grpc server is not initialized")
	}
	if s.GinServer == nil {
		s.DefaultLogger.Info("!ATTENTION! gin server is not initialized")
	}
}

// Start runs servers with provided options.
func (s *Server) Start() error {
	s.defaultConfig()

	s.DefaultLogger.Info("Starting service...")

	rootCtx := context.Background()

	if len(s.beforeStart) > 0 {
		for _, fn := range s.beforeStart {
			if err := fn(); err != nil {
				s.DefaultLogger.Error("before start func", zapl.Error(err))
			}
		}
	}

	ctx, cancel := signal.NotifyContext(rootCtx, os.Interrupt)
	defer cancel()

	s.DefaultLogger.Info("Starting errgroup...")
	g, ctx := errgroup.WithContext(ctx)

	s.DefaultLogger.Info("Starting prometheus metrics server...")
	s.promRegistry.MustRegister(s.PromCollectors...)
	http.Handle("/metrics", promhttp.Handler())
	g.Go(func() error {
		if err := http.ListenAndServe(defaultPromAddr, nil); err != nil {
			s.DefaultLogger.Error("init prometheus server", zapl.Error(err))
		}
		return nil
	})

	s.DefaultLogger.Info("Initialized with ports",
		zapl.String("http.httpAddr", s.httpAddr),
		zapl.String("grpc.httpAddr", s.grpcAddr),
		zapl.String("fgprof.httpAddr", s.fgprofAddr),
		zapl.String("prometheus.httpAddr", defaultPromAddr),
	)

	switch {
	case s.ChiServer != &chicore.Mux{} && s.GRPCServer != &grpccore.Server{} && s.parallelMode:
		s.DefaultLogger.Info("Initialized chi and grpc servers, parallel mode")
		grpc_health_v1.RegisterHealthServer(s.GRPCServer, health.NewServer())
		pb.RegisterEchoServer(s.GRPCServer, &echoServer{})
		mh := mixHTTPAndGRPC(s.ChiServer, s.GRPCServer)
		http2Server := &http2.Server{}
		http1Server := &http.Server{Handler: h2c.NewHandler(mh, http2Server)}
		s.ChiServer.Get("/health", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("OK"))
		})
		lis, err := net.Listen("tcp", s.httpAddr)
		if err != nil {
			panic(err)
		}

		g.Go(func() error {
			defer s.DefaultLogger.Info("Server stopped.")

			if err = http1Server.Serve(lis); err != nil && !errors.Is(err, http.ErrServerClosed) {
				s.DefaultLogger.Fatal("start chi and grpc server", zapl.Error(err))
			}
			return nil
		})
	case s.GinServer != &gincore.Engine{} && s.GRPCServer != &grpccore.Server{} && s.parallelMode:
		s.DefaultLogger.Info("Initialized gin and grpc servers, parallel mode")
		grpc_health_v1.RegisterHealthServer(s.GRPCServer, health.NewServer())
		//pb.RegisterEchoServer(s.GRPCServer, &echoServer{})
		mh := mixHTTPAndGRPC(s.GinServer, s.GRPCServer)
		http2Server := &http2.Server{}
		http1Server := &http.Server{Handler: h2c.NewHandler(mh, http2Server)}
		lis, err := net.Listen("tcp", s.httpAddr)
		if err != nil {
			panic(err)
		}
		g.Go(func() error {
			defer s.DefaultLogger.Info("Server stopped.")
			if err := http1Server.Serve(lis); err != nil && !errors.Is(err, http.ErrServerClosed) {
				s.DefaultLogger.Fatal("start gin and grpc server", zapl.Error(err))
			}
			return nil
		})
	case s.ChiServer != &chicore.Mux{} && s.GRPCServer != &grpccore.Server{} && !s.parallelMode:
		s.DefaultLogger.Info("Initialized chi and grpc servers, not parallel mode")
		s.ChiServer.Get("/health", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("OK"))
		})
		lis, err := net.Listen("tcp", s.grpcAddr)
		if err != nil {
			panic(err)
		}
		g.Go(func() error {
			defer s.DefaultLogger.Info("Server stopped.")
			grpc_health_v1.RegisterHealthServer(s.GRPCServer, health.NewServer())
			pb.RegisterEchoServer(s.GRPCServer, &echoServer{})
			if err := s.GRPCServer.Serve(lis); err != nil && !errors.Is(err, http.ErrServerClosed) {
				s.DefaultLogger.Fatal("start grpc server", zapl.Error(err))
			}
			s.DefaultLogger.Info("Init chi and grpc")
			return nil
		})
		g.Go(func() error {
			defer s.DefaultLogger.Info("Server stopped.")
			if err := http.ListenAndServe(s.httpAddr, s.ChiServer); err != nil && !errors.Is(err, http.ErrServerClosed) {
				s.DefaultLogger.Fatal("start chi server", zapl.Error(err))
			}
			return nil
		})
	case s.GinServer != &gincore.Engine{} && s.GRPCServer != &grpccore.Server{} && !s.parallelMode:
		s.DefaultLogger.Info("Initialized gin and grpc servers, not parallel mode")
		lis, err := net.Listen("tcp", s.grpcAddr)
		if err != nil {
			panic(err)
		}
		g.Go(func() error {
			defer s.DefaultLogger.Info("Server stopped.")
			grpc_health_v1.RegisterHealthServer(s.GRPCServer, health.NewServer())
			pb.RegisterEchoServer(s.GRPCServer, &echoServer{})
			if err = s.GRPCServer.Serve(lis); err != nil && !errors.Is(err, http.ErrServerClosed) {
				s.DefaultLogger.Fatal("start grpc server", zapl.Error(err))
			}
			return nil
		})
		g.Go(func() error {
			defer s.DefaultLogger.Info("Server stopped.")
			if err = s.GinServer.Run(s.httpAddr); err != nil && !errors.Is(err, http.ErrServerClosed) {
				s.DefaultLogger.Fatal("start gin server", zapl.Error(err))
			}
			return nil
		})
	case s.GRPCServer != &grpccore.Server{} && !s.parallelMode:
		s.DefaultLogger.Info("Initialized grpc server")
		lis, err := net.Listen("tcp", s.grpcAddr)
		if err != nil {
			panic(err)
		}
		g.Go(func() error {
			defer s.DefaultLogger.Info("Server stopped.")
			grpc_health_v1.RegisterHealthServer(s.GRPCServer, health.NewServer())
			pb.RegisterEchoServer(s.GRPCServer, &echoServer{})
			if err = s.GRPCServer.Serve(lis); err != nil && !errors.Is(err, http.ErrServerClosed) {
				s.DefaultLogger.Fatal("start grpc server", zapl.Error(err))
			}
			return nil
		})
	case s.GinServer != &gincore.Engine{} && !s.parallelMode:
		s.DefaultLogger.Info("Initialized gin server")
		g.Go(func() error {
			defer s.DefaultLogger.Info("Server stopped.")
			if err := s.GinServer.Run(s.httpAddr); err != nil && !errors.Is(err, http.ErrServerClosed) {
				s.DefaultLogger.Fatal("start gin server", zapl.Error(err))
			}
			return nil
		})
	case s.ChiServer != &chicore.Mux{} && !s.parallelMode:
		s.DefaultLogger.Info("Initialized chi server")
		s.ChiServer.Get("/health", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("OK"))
		})
		g.Go(func() error {
			defer s.DefaultLogger.Info("Server stopped.")
			if err := http.ListenAndServe(s.httpAddr, s.ChiServer); err != nil && !errors.Is(err, http.ErrServerClosed) {
				s.DefaultLogger.Fatal("start chi server", zapl.Error(err))
			}
			return nil
		})
	default:
		s.DefaultLogger.Info("No servers evaluated in service options or something goes wrong.")
	}

	if s.fgprofServer != nil {
		http.DefaultServeMux.Handle(fgprofUrl, *s.fgprofServer)
		g.Go(func() error {
			if err := http.ListenAndServe(s.fgprofAddr, nil); err != nil {
				s.DefaultLogger.Error("init fgprof server", zapl.Error(err))
			}
			return nil
		})
	}

	g.Go(func() error {
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
		time.Sleep(time.Second * 5)

		// Probably deadlock, forcing shutdown.
		s.DefaultLogger.Fatal("Service terminated gracefully!")
		return nil
	})

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

	s.validateServers()
	return g.Wait()
}

// WithServerName sets name of service.
func WithServerName(name string) func(*Server) {
	return func(s *Server) {
		switch {
		case name == "":
			panic("server name evaluated, but not defined")
		default:
			s.serverName = name
		}
	}
}

// WithHTTPServerPort sets provided port to http server.
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

// WithGRPCServerPort sets provided port to grpc server.
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

// WithGinServer provides gin http server and runs it after Start.
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
	}
}

// WithChiServer provides chi http server and runs it after Start.
func WithChiServer(cfg chi.Config) func(*Server) {
	return func(s *Server) {
		switch {
		case cfg.Default:
			s.ChiServer = cfg.NewDefaultChi(s.serverName)
		default:
			s.ChiServer = cfg.NewDefaultChi(s.serverName)
		}
	}
}

// WithGRPCServer provides grpc server and runs it after Start.
func WithGRPCServer(cfg grpc.Config) func(*Server) {
	return func(s *Server) {
		switch {
		case cfg.Default:
			s.GRPCServer = cfg.NewDefaultGRPCServer()
		default:
			s.GRPCServer = cfg.NewDefaultGRPCServer()
		}
	}
}

// WithFgprofServer provides http server with fgprof handler on 6060 port(by default) and runs it after Start.
func WithFgprofServer(cfg fgprof.Config) func(*Server) {
	return func(s *Server) {
		if cfg.Port != "" {
			s.fgprofAddr = "0.0.0.0:" + cfg.Port
		}
		s.fgprofServer = cfg.NewDefaultFgprof()
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

// WithCustomGoroutines adds goroutines to main errgroup instance of server.
func WithCustomGoroutines(funcs []func() error) func(*Server) {
	return func(s *Server) {
		s.customGoroutines = funcs
	}
}
