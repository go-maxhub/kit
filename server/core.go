package kit

import (
	"go.uber.org/zap"
)

func initDefaultZapLogger() *zap.Logger {
	return zap.Must(zap.NewProduction(
		zap.AddCaller(),
	))
}
