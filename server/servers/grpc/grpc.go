package grpc

import (
	"google.golang.org/grpc"
	pb "google.golang.org/grpc/examples/features/proto/echo"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

type Config struct {
	Default bool
}

type echoServer struct {
	pb.UnimplementedEchoServer
}

func (c *Config) NewDefaultGRPCServer() *grpc.Server {
	gs := grpc.NewServer()
	grpc_health_v1.RegisterHealthServer(gs, health.NewServer())
	pb.RegisterEchoServer(gs, &echoServer{})
	return gs
}
