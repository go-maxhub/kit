package kit

import (
	"time"

	envParser "github.com/caarlos0/env/v9"
	"github.com/go-faster/errors"
	"go.uber.org/zap"
)

type env struct {
	ServerVersion string `env:"KIT_SERVER_VERSION" envDefault:"0.0.0"`

	GracefulShutdownTimeout time.Duration `env:"KIT_GRACEFUL_SHUTDOWN" envDefault:"5s"`

	OTELJaegerHost string `env:"KIT_TRACING_JAEGER_HOST" envDefault:"localhost:4318"`

	FgprofEnable bool `env:"KIT_METRICS_FGPROF" envDefault:"false"`

	PprofEnable bool `env:"KIT_METRICS_PPROF" envDefault:"false"`

	DebugHeaders bool `env:"KIT_DEBUG_HEADERS" envDefault:"false"`

	PyroscopeHost string `env:"KIT_METRICS_PYROSCOPE_HOST" envDefault:"http://localhost:4040"`
}

func initEnvVars(lg *zap.Logger) (env, error) {
	envVars := env{}
	if err := envParser.Parse(&envVars); err != nil {
		lg.Error("parse env vars", zap.Error(err))
		return envVars, errors.Wrap(err, "parse env vars")
	}
	return envVars, nil
}
