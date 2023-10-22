package main

import (
	"kit/server/logger/zap"
	"log"

	mcs "kit/server"

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

	svc.ZapLogger.Info("started")
	svc.ChiServer.Middlewares()

	if err := svc.Start(); err != nil {
		log.Fatal(err)
	}

}
