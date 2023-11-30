package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-maxhub/kit/kit"
	"github.com/go-maxhub/kit/kit/servers/chi"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	zapl "go.uber.org/zap"
)

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
		kit.WithCustomGoroutines([]func() error{testGoroutine}),
	)

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
		_, err := w.Write([]byte("welcome test"))
		if err != nil {
			return
		}
	})
	svc.ChiServer.Get("/test/poof/woof", func(w http.ResponseWriter, r *http.Request) {
		ng.Add(1)
		_, err := w.Write([]byte("welcome test"))
		if err != nil {
			return
		}
	})
	svc.ChiServer.Get("/", func(w http.ResponseWriter, r *http.Request) {
		ng.Add(1)
		_, err := w.Write([]byte("welcome root"))
		if err != nil {
			return
		}
	})
	svc.ChiServer.Get("/test/mcs1235/poof/pid3424", func(w http.ResponseWriter, r *http.Request) {
		ng.Add(1)
		_, err := w.Write([]byte("welcome poof"))
		if err != nil {
			return
		}
	})

	if err := svc.Start(); err != nil {
		svc.DefaultLogger.Error("start kit", zapl.Error(err))
	}
}
