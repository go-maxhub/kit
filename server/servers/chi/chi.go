package chi

import (
	"github.com/go-chi/chi/v5"
)

type Config struct {
	Default bool
	Blank   bool
}

func (c *Config) NewDefaultChi() *chi.Mux {
	cl := chi.NewRouter()
	return cl
}
