package kit

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func initDefaultZapLogger() *zap.Logger {
	return zap.Must(zap.NewProduction(
		zap.AddCaller(),
		zap.AddStacktrace(zapcore.ErrorLevel),
	))
}
