package zap

import (
	"go.uber.org/zap"
)

type Config struct {
	Development bool
	Production  bool
}

func (c *Config) NewDevelopmentLogger() *zap.Logger {
	l, err := zap.NewDevelopment(
		zap.AddCaller(),
		zap.AddStacktrace(zap.DebugLevel),
	)
	if err != nil {
		panic(err)
	}
	return l
}

func (c *Config) NewProductionLogger() *zap.Logger {
	l, err := zap.NewProduction(
		zap.AddCaller(),
		zap.AddStacktrace(zap.DebugLevel),
	)
	if err != nil {
		panic(err)
	}
	return l
}
