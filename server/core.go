package kit

import (
	"go.uber.org/zap"
)

func initLogger() *zap.Logger {
	l, err := zap.NewDevelopment()

	if err != nil {
		panic(err)
	}
	return l
}
