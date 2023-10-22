package fgprof

import (
	"net/http"
	_ "net/http/pprof"

	"github.com/felixge/fgprof"
)

type Config struct {
	Port string
}

func (c *Config) NewDefaultFgprof() *http.Handler {
	fgs := fgprof.Handler()
	return &fgs
}
