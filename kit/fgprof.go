package kit

import (
	"net/http"

	"go.uber.org/zap"
)

// addFgprofServer adds fgprof endpoint to server.
func (s *Server) addFgprofServer() {
	s.DefaultLogger.Info("Starting fgprof endpoint...")
	http.DefaultServeMux.Handle(fgprofUrl, s.fgprofServer)
	fs := func() error {
		if err := http.ListenAndServe(defaultFgrpofAddr, nil); err != nil {
			s.DefaultLogger.Error("init fgprof kit", zap.Error(err))
		}
		return nil
	}
	s.servers = append(s.servers, fs)
}
