package main

import (
	"log"

	"github.com/go-chi/chi/v5/middleware"
	"google.golang.org/grpc/reflection"

	kitgrpc "kit/examples/proto"
	kit "kit/server"

	"kit/server/logger/zap"
	"kit/server/servers/chi"
	"kit/server/servers/grpc"
)

type server struct {
	kitgrpc.UnimplementedSenderServer
}

func main() {
	svc := kit.New(
		kit.WithChiServer(chi.Config{
			Default: true,
		}),
		kit.WithGRPCServer(grpc.Config{Default: true}),
		kit.WithZapLogger(zap.Config{
			Development: true,
		}),
		kit.WithParallelMode(),
	)
	kitgrpc.RegisterSenderServer(svc.GRPCServer, &server{})
	reflection.Register(svc.GRPCServer)

	svc.ChiServer.Use(
		middleware.Recoverer,
		middleware.NoCache,
	)

	svc.ZapLogger.Info("Describe your server logic down here ↓↓↓")

	if err := svc.Start(); err != nil {
		log.Fatal(err)
	}
}
