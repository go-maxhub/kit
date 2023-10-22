package main

import (
	"log"

	"github.com/go-chi/chi/v5/middleware"

	mcs "kit/server"

	"kit/server/logger/zap"
	"kit/server/servers/chi"
	"kit/server/servers/grpc"
)

func main() {
	svc := mcs.New(
		mcs.WithChiServer(chi.Config{
			Default: true,
		}),
		mcs.WithGRPCServer(grpc.Config{Default: true}),
		mcs.WithZapLogger(zap.Config{
			Development: true,
		}),
	)

	svc.ChiServer.Use(
		middleware.Recoverer,
		middleware.NoCache,
	)

	svc.ZapLogger.Info("Describe your server logic down here ↓↓↓")

	if err := svc.Start(); err != nil {
		log.Fatal(err)
	}
}
