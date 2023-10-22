package slog

import (
	"log/slog"
	"os"
)

type Config struct {
	Default bool
	JSON    bool
	Text    bool
}

func (c *Config) NewDefaultLogger() *slog.Logger {
	l := slog.Default()
	return l
}

func (c *Config) NewJSONLogger() *slog.Logger {
	l := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	return l
}

func (c *Config) NewTextLogger() *slog.Logger {
	l := slog.New(slog.NewTextHandler(os.Stdout, nil))
	return l
}
