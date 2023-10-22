package grpc

import (
	"google.golang.org/grpc"
)

type Config struct {
	Default bool
}

func (c *Config) NewDefaultGRPCServer() *grpc.Server {
	gs := grpc.NewServer()
	return gs
}
