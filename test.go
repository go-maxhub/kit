package main

import (
	"log"

	mcs "kit/server"

	"kit/server/logger/slog"
	"kit/server/servers/chi"
	"kit/server/servers/grpc"
)

func main() {
	svc := mcs.New(
		mcs.WithChiServer(chi.Config{
			Default: true,
		}),
		mcs.WithGRPCServer(grpc.Config{Default: true}),
		mcs.WithSlogLogger(slog.Config{
			JSON: true,
		}),
	)

	if err := svc.Start(); err != nil {
		log.Fatal(err)
	}
}
