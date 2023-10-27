package chi

import (
	"github.com/go-chi/chi/v5"
	"kit/server/metrics"
)

type Config struct {
	Default bool
}

func (c *Config) NewDefaultChi(serverName string) *chi.Mux {
	cl := chi.NewRouter()
	cl.Use(metrics.NewMiddleware(serverName))
	return cl
}
