package main

import (
	"fmt"
	"github.com/go-maxhub/kit/examples/chi_and_grpc/proto"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	zapl "go.uber.org/zap"
	"google.golang.org/grpc/reflection"

	kit "github.com/go-maxhub/kit/kit"
	"github.com/go-maxhub/kit/kit/servers/chi"
	"github.com/go-maxhub/kit/kit/servers/grpc"
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
		kit.WithChiAndGRPCServer(chi.Config{
			Default: true,
		}, grpc.Config{Default: true}),
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
		_, err := w.Write([]byte("welcome"))
		if err != nil {
			return
		}
	})

	if err := svc.Start(); err != nil {
		svc.DefaultLogger.Error("start kit", zapl.Error(err))
	}
}
