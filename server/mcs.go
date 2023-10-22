package mcs

import (
	"context"
	"errors"
	"log"
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
	slogcore "log/slog"

	"kit/server/logger/slog"
	"kit/server/logger/zap"
	"kit/server/servers/chi"
	"kit/server/servers/gin"
	"kit/server/servers/grpc"
)

const (
	httpHostPort = "0.0.0.0:8080"
	grpcHostPort = "0.0.0.0:8081"
	grpcHeader   = "application/grpc"
)

func mixHTTPAndGRPC(httpHandler http.Handler, grpcHandler *grpccore.Server) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor == 2 && strings.HasPrefix(r.Header.Get("content-type"), grpcHeader) {
			grpcHandler.ServeHTTP(w, r)
			return
		}
		httpHandler.ServeHTTP(w, r)
	})
}

type Server struct {
	// !ATTENTION! Options must be set before Start.
	port     string
	grpcPort string

	// Starts 2 services on different ports, 8080 and 8081 if any others not provided in Server options.
	parallelRoutes bool

	// Servers
	GinServer  *gincore.Engine
	ChiServer  *chicore.Mux
	GRPCServer *grpccore.Server

	// Loggers
	SlogLogger *slogcore.Logger
	ZapLogger  *zapl.Logger

	// Before and After funcs
	beforeStart []func() error
	afterStart  []func() error
	afterStop   []func() error
	beforeStop  []func() error
}

func New(options ...func(*Server)) *Server {
	srv := &Server{}
	for _, o := range options {
		o(srv)
	}
	return srv
}

type echoServer struct {
	pb.UnimplementedEchoServer
}

func (s *Server) Start() error {
	l, err := zapl.NewDevelopment(
		zapl.WithCaller(true),
		zapl.AddStacktrace(zapl.InfoLevel),
	)
	if err != nil {
		panic(err)
	}

	l.Debug("Starting service...")

	rootCtx := context.Background()

	if len(s.beforeStart) > 0 {
		for _, fn := range s.beforeStart {
			if err := fn(); err != nil {
				s.ZapLogger.Error("before start func", zapl.Error(err))
			}
		}
	}

	ctx, cancel := signal.NotifyContext(rootCtx, os.Interrupt)
	defer cancel()

	l.Debug("Starting errgroup...")
	g, ctx := errgroup.WithContext(ctx)

	port := httpHostPort
	if s.port != "" {
		port = "0.0.0.0:" + s.port
	}
	grpcPort := grpcHostPort
	if s.grpcPort != "" {
		port = "0.0.0.0:" + s.grpcPort
	}
	l.Debug("Initialized with ports", zapl.String("http.port", port), zapl.String("grpc.port", grpcPort))

	switch {
	case s.ChiServer != &chicore.Mux{} && s.GRPCServer != &grpccore.Server{} && !s.parallelRoutes:
		l.Debug("Initialized chi and grpc servers, not parallel mode")
		lis, err := net.Listen("tcp", grpcPort)
		if err != nil {
			panic(err)
		}
		g.Go(func() error {
			defer l.Info("Server stopped.")
			grpc_health_v1.RegisterHealthServer(s.GRPCServer, health.NewServer())
			pb.RegisterEchoServer(s.GRPCServer, &echoServer{})
			if err := s.GRPCServer.Serve(lis); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Fatal("start grpc server", err)
			}
			l.Debug("Init chi and grpc")
			return nil
		})
		g.Go(func() error {
			defer l.Info("Server stopped.")
			s.ChiServer.Get("/health", func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte("OK"))
			})
			if err := http.ListenAndServe(port, s.ChiServer); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Fatal("start chi server", err)
			}
			return nil
		})
	case s.GinServer != &gincore.Engine{} && s.GRPCServer != &grpccore.Server{} && !s.parallelRoutes:
		l.Debug("Initialized gin and grpc servers, not parallel mode")
		lis, err := net.Listen("tcp", grpcPort)
		if err != nil {
			panic(err)
		}
		g.Go(func() error {
			defer l.Info("Server stopped.")
			grpc_health_v1.RegisterHealthServer(s.GRPCServer, health.NewServer())
			pb.RegisterEchoServer(s.GRPCServer, &echoServer{})
			if err := s.GRPCServer.Serve(lis); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Fatal("start grpc server", err)
			}
			return nil
		})
		g.Go(func() error {
			defer l.Info("Server stopped.")
			if err := s.GinServer.Run(port); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Fatal("start gin server", err)
			}
			return nil
		})

	case s.ChiServer != &chicore.Mux{} && s.GRPCServer != &grpccore.Server{} && s.parallelRoutes:
		l.Debug("Initialized chi and grpc servers, parallel mode")
		grpc_health_v1.RegisterHealthServer(s.GRPCServer, health.NewServer())
		pb.RegisterEchoServer(s.GRPCServer, &echoServer{})
		mh := mixHTTPAndGRPC(s.ChiServer, s.GRPCServer)
		http2Server := &http2.Server{}
		http1Server := &http.Server{Handler: h2c.NewHandler(mh, http2Server)}
		lis, err := net.Listen("tcp", port)
		if err != nil {
			panic(err)
		}
		g.Go(func() error {
			defer l.Info("Server stopped.")
			grpc_health_v1.RegisterHealthServer(s.GRPCServer, health.NewServer())
			pb.RegisterEchoServer(s.GRPCServer, &echoServer{})
			if err := http1Server.Serve(lis); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Fatal("start chi and grpc server", err)
			}
			return nil
		})
	case s.GinServer != &gincore.Engine{} && s.GRPCServer != &grpccore.Server{} && s.parallelRoutes:
		l.Debug("Initialized gin and grpc servers, parallel mode")
		grpc_health_v1.RegisterHealthServer(s.GRPCServer, health.NewServer())
		pb.RegisterEchoServer(s.GRPCServer, &echoServer{})
		mh := mixHTTPAndGRPC(s.GinServer, s.GRPCServer)
		http2Server := &http2.Server{}
		http1Server := &http.Server{Handler: h2c.NewHandler(mh, http2Server)}
		lis, err := net.Listen("tcp", port)
		if err != nil {
			panic(err)
		}
		g.Go(func() error {
			defer l.Info("Server stopped.")
			if err := http1Server.Serve(lis); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Fatal("start gin and grpc server", err)
			}
			return nil
		})
	case s.GRPCServer != &grpccore.Server{}:
		l.Debug("Initialized grpc server")
		lis, err := net.Listen("tcp", grpcPort)
		if err != nil {
			panic(err)
		}
		g.Go(func() error {
			defer l.Info("Server stopped.")
			grpc_health_v1.RegisterHealthServer(s.GRPCServer, health.NewServer())
			pb.RegisterEchoServer(s.GRPCServer, &echoServer{})
			if err := s.GRPCServer.Serve(lis); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Fatal("start grpc server", err)
			}
			return nil
		})
	case s.GinServer != &gincore.Engine{}:
		l.Debug("Initialized gin server")
		g.Go(func() error {
			defer l.Info("Server stopped.")
			if err := s.GinServer.Run(port); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Fatal("start gin server", err)
			}
			return nil
		})
	case s.ChiServer != &chicore.Mux{}:
		l.Debug("Initialized chi server")
		g.Go(func() error {
			defer l.Info("Server stopped.")
			if err := http.ListenAndServe(port, s.ChiServer); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Fatal("start chi server", err)
			}
			return nil
		})
	default:
		l.Warn("No servers evaluated in service options.")
	}

	g.Go(func() error {
		// Guaranteed way to kill application.
		if len(s.beforeStop) > 0 {
			for _, fn := range s.beforeStop {
				if err := fn(); err != nil {
					s.ZapLogger.Error("before stop func", zapl.Error(err))
				}
			}
		}
		<-ctx.Done()
		if len(s.afterStop) > 0 {
			for _, fn := range s.afterStop {
				if err := fn(); err != nil {
					s.ZapLogger.Error("after stop func", zapl.Error(err))
				}
			}
		}
		// Context is canceled, giving application time to shut down gracefully.
		l.Warn("Waiting for application shutdown")
		time.Sleep(time.Second * 5)

		// Probably deadlock, forcing shutdown.
		l.Fatal("Graceful shutdown watchdog triggered: forcing shutdown")
		return nil
	})

	if len(s.afterStart) > 0 {
		for _, fn := range s.afterStart {
			if err := fn(); err != nil {
				s.ZapLogger.Error("after start func", zapl.Error(err))
			}
		}
	}
	return g.Wait()
}

func WithServerPort(port string) func(*Server) {
	return func(s *Server) {
		switch {
		case port != "":
			s.port = port
		default:
			s.port = "8080"
		}
	}
}

func WithGRPCServerPort(port string) func(*Server) {
	return func(s *Server) {
		switch {
		case port != "":
			s.grpcPort = port
		default:
			s.port = "8081"
		}
	}
}

func WithZapLogger(cfg zap.Config) func(*Server) {
	return func(s *Server) {
		switch {
		case cfg.Development:
			s.ZapLogger = cfg.NewDevelopmentLogger()
		case cfg.Production:
			s.ZapLogger = cfg.NewProductionLogger()
		default:
			s.ZapLogger = cfg.NewProductionLogger()
		}
	}
}

func WithSlogLogger(cfg slog.Config) func(*Server) {
	return func(s *Server) {
		switch {
		case cfg.Default:
			s.SlogLogger = cfg.NewDefaultLogger()
		case cfg.JSON:
			s.SlogLogger = cfg.NewJSONLogger()
		case cfg.Text:
			s.SlogLogger = cfg.NewJSONLogger()
		default:
			s.SlogLogger = cfg.NewJSONLogger()
		}
	}
}

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

func WithChiServer(cfg chi.Config) func(*Server) {
	return func(s *Server) {
		switch {
		case cfg.Default:
			s.ChiServer = cfg.NewDefaultChi()
		default:
			s.ChiServer = cfg.NewDefaultChi()
		}
	}
}

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

func WithBeforeStart(funcs []func() error) func(*Server) {
	return func(s *Server) {
		s.beforeStart = funcs
	}
}

func WithAfterStart(funcs []func() error) func(*Server) {
	return func(s *Server) {
		s.afterStart = funcs
	}
}

func WithAfterStop(funcs []func() error) func(*Server) {
	return func(s *Server) {
		s.afterStop = funcs
	}
}

func WithBeforeStop(funcs []func() error) func(*Server) {
	return func(s *Server) {
		s.beforeStop = funcs
	}
}
