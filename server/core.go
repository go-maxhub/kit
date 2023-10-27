package kit

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func initDefaultZapLogger(name string) *zap.Logger {
	var serviceName string
	if name != "" {
		serviceName = name
	} else {
		serviceName = "kit.service"
	}

	return zap.Must(zap.NewProduction(
		zap.AddCaller(),
		zap.AddStacktrace(zapcore.ErrorLevel),
		zap.Fields(
			zap.String("service.name", serviceName),
		),
	))
}
