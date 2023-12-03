package kit

import (
	"errors"
	"net"
	"net/http"
	"strings"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/examples/features/proto/echo"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// addGRPCServer adds GRPC engine to server.
func (s *Server) addGRPCServer() {
	lis, err := net.Listen("tcp", s.grpcAddr)
	if err != nil {
		panic(err)
	}
	grpcs := func() error {
		defer s.DefaultLogger.Info("Server stopped.")
		if err = s.GRPCServer.Serve(lis); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.DefaultLogger.Fatal("start grpc kit", zap.Error(err))
		}
		return nil
	}
	s.servers = append(s.servers, grpcs)
}

// mixHTTPAndGRPC configures mixed http and grpc handler.
func mixHTTPAndGRPC(httpHandler http.Handler, grpcHandler *grpc.Server) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor == 2 && strings.HasPrefix(r.Header.Get("content-type"), grpcHeader) {
			grpcHandler.ServeHTTP(w, r)
			return
		}
		httpHandler.ServeHTTP(w, r)
	})
}

type GRPCConfig struct {
	Default bool
}

type echoServer struct {
	echo.UnimplementedEchoServer
}

func (c *GRPCConfig) NewDefaultGRPCServer() *grpc.Server {
	gs := grpc.NewServer()
	grpc_health_v1.RegisterHealthServer(gs, health.NewServer())
	echo.RegisterEchoServer(gs, &echoServer{})
	return gs
}
