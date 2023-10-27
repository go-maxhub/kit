package main

import (
	"fmt"
	kitgrpc "kit/examples/proto"
	kit "kit/server"
	"time"

	"kit/server/servers/chi"
	"kit/server/servers/grpc"

	"github.com/go-chi/chi/v5/middleware"
	zapl "go.uber.org/zap"
	"google.golang.org/grpc/reflection"
)

type server struct {
	kitgrpc.UnimplementedSenderServer
}

func main() {

	testGoroutine := func() error {
		for {
			fmt.Println("test goroutine")
			time.Sleep(time.Second * 2)
		}
	}

	svc := kit.New(
		kit.WithServerName("shuttle"),
		kit.WithChiServer(chi.Config{
			Default: true,
		}),
		kit.WithGRPCServer(grpc.Config{Default: true}),
		kit.WithParallelMode(),
		kit.WithCustomGoroutines([]func() error{testGoroutine}),
	)
	kitgrpc.RegisterSenderServer(svc.GRPCServer, &server{})
	reflection.Register(svc.GRPCServer)

	svc.ChiServer.Use(
		middleware.Recoverer,
		middleware.NoCache,
	)

	svc.DefaultLogger.Info("Describe your server logic down here ↓↓↓")

	if err := svc.Start(); err != nil {
		svc.DefaultLogger.Error("start server", zapl.Error(err))
	}
}
