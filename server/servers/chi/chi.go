package chi

import (
	"github.com/go-chi/chi/v5"
	"kit/server/metric"
	"net/http"
)

type Config struct {
	Default bool
}

func (c *Config) NewDefaultChi(serverName string) *chi.Mux {
	cl := chi.NewRouter()
	cl.Use(metric.NewMiddleware(serverName))
	cl.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})
	return cl
}
