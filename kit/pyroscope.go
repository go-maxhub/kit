package kit

import (
	"runtime"

	"github.com/grafana/pyroscope-go"
	"go.uber.org/zap"
)

// addPyroscopeExporter adds pyroscope auto-exporter to server.
func (s *Server) addPyroscopeExporter() {
	runtime.SetMutexProfileFraction(5)
	runtime.SetBlockProfileRate(5)

	pl := newPyroscopeLogger(s.DefaultLogger)

	_, err := pyroscope.Start(pyroscope.Config{
		ApplicationName: s.ServerName,
		ServerAddress:   s.envVars.PyroscopeHost,
		Logger:          pl,
		Tags: map[string]string{
			"service_name":    s.ServerName,
			"service_version": s.ServerVersion,
		},
		ProfileTypes: []pyroscope.ProfileType{
			pyroscope.ProfileCPU,
			pyroscope.ProfileAllocObjects,
			pyroscope.ProfileAllocSpace,
			pyroscope.ProfileInuseObjects,
			pyroscope.ProfileInuseSpace,
			pyroscope.ProfileGoroutines,
			pyroscope.ProfileMutexCount,
			pyroscope.ProfileMutexDuration,
			pyroscope.ProfileBlockCount,
			pyroscope.ProfileBlockDuration,
		},
	})
	if err != nil {
		s.DefaultLogger.Fatal("init pyroscope exporter", zap.Error(err))
	}
	s.DefaultLogger.Info("Starting pyroscope exporter...", zap.String("pyroscope.host", s.envVars.PyroscopeHost))

}

type pyroscopeLogger struct {
	zap.Logger
}

// newPyroscopeLogger initializes a new logger exactly for pyroscope
func newPyroscopeLogger(lg *zap.Logger) *pyroscopeLogger {
	return &pyroscopeLogger{*lg}
}

func (l *pyroscopeLogger) Infof(msg string, fields ...interface{}) {
	var fl []zap.Field
	for _, f := range fields {
		fl = append(fl, zap.Any("field", f))
	}
	l.Info(msg, fl...)
}

func (l *pyroscopeLogger) Debugf(msg string, fields ...interface{}) {
	var fl []zap.Field
	for _, f := range fields {
		fl = append(fl, zap.Any("field", f))
	}
	l.Debug(msg, fl...)
}

func (l *pyroscopeLogger) Errorf(msg string, fields ...interface{}) {
	var fl []zap.Field
	for _, f := range fields {
		fl = append(fl, zap.Any("field", f))
	}
	l.Error(msg, fl...)
}
