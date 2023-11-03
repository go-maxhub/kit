package kit

import (
	"time"

	envParser "github.com/caarlos0/env/v9"
	"github.com/go-faster/errors"
	"go.uber.org/zap"
)

type Env struct {
	GracefulShutdownTimeout time.Duration `env:"GRACEFUL_SHUTDOWN" envDefault:"5s"`
}

func InitEnvVars(lg *zap.Logger) (Env, error) {
	envVars := Env{}
	if err := envParser.Parse(&envVars); err != nil {
		lg.Error("parse env vars", zap.Error(err))
		return envVars, errors.Wrap(err, "parse env vars")
	}
	return envVars, nil
}
