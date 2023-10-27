package main

import (
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	zapl "go.uber.org/zap"
	"google.golang.org/grpc/reflection"

	kitgrpc "kit/examples/proto"
	kit "kit/server"
	"kit/server/servers/chi"
	"kit/server/servers/grpc"
)

type server struct {
	kitgrpc.UnimplementedSenderServer
}

func main() {

	testGoroutine := func() error {
		for {
			fmt.Println("boogie woogie")
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

	ng := promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "myapp",
		Name:      "connected_devices",
		Help:      "Number of currently connected devices.",
	})
	svc.ChiServer.Get("/test", func(w http.ResponseWriter, r *http.Request) {
		ng.Add(1)
		w.Write([]byte("welcome"))
	})

	if err := svc.Start(); err != nil {
		svc.DefaultLogger.Error("start server", zapl.Error(err))
	}
}