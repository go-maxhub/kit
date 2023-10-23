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
	)
	if err != nil {
		panic(err)
	}
	return l
}

func (c *Config) NewProductionLogger() *zap.Logger {
	l, err := zap.NewProduction(
		zap.AddCaller(),
	)
	if err != nil {
		panic(err)
	}
	return l
}
