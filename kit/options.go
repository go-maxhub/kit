package kit

// WithServerName sets name of service.
func WithServerName(name string) func(*Server) {
	return func(s *Server) {
		switch {
		case name == "":
			panic("server name option evaluated, but not defined")
		default:
			s.ServerName = name
		}
	}
}

// WithEndToEndTests enables server to run end-to-end tests in cli mode.
func WithEndToEndTests(configPath string) func(*Server) {
	return func(s *Server) {
		switch {
		case configPath == "":
			panic("path to config name evaluated, but not defined")
		default:
			s.addEndToEndTests(configPath)
		}
	}
}

// WithHTTPServerPort sets provided port to http kit.
func WithHTTPServerPort(port string) func(*Server) {
	return func(s *Server) {
		switch {
		case port == "":
			panic("http port evaluated, but not defined")
		default:
			s.httpAddr = "0.0.0.0:" + port
		}
	}
}

// WithGRPCServerPort sets provided port to grpc kit.
func WithGRPCServerPort(port string) func(*Server) {
	return func(s *Server) {
		switch {
		case port == "":
			panic("grpc port evaluated, but not defined")
		default:
			s.grpcAddr = "0.0.0.0:" + port
		}
	}
}

// WithParallelMode sets http and grpc engine to bind on one port and segregate requests by headers.
func WithParallelMode() func(*Server) {
	return func(s *Server) {
		s.parallelMode = true
	}
}

// WithGinServer provides gin http kit and runs it after Start.
func WithGinServer(cfg GinConfig) func(*Server) {
	return func(s *Server) {
		switch {
		case cfg.Blank:
			s.GinServer = cfg.NewBlankGin()
		case cfg.Default:
			s.GinServer = cfg.NewDefaultGin()
		default:
			s.GinServer = cfg.NewDefaultGin()
		}
		s.addGinServer()
	}
}

// WithChiServer provides metrics http kit and runs it after Start.
func WithChiServer(cfg ChiConfig) func(*Server) {
	return func(s *Server) {
		srv, tp := cfg.NewDefaultChi(s.RootCtx, s.DefaultLogger, s.ServerName, s.ServerVersion, s.envVars.OTELJaegerHost, s.envVars.DebugHeaders)
		s.ChiServer = srv
		s.tp = tp
		s.addChiServer()
	}
}

// WithChiAndGRPCServer provides metrics http and grpc engine and runs them after Start.
func WithChiAndGRPCServer(cfg ChiConfig, gcfg GRPCConfig) func(*Server) {
	return func(s *Server) {
		srv, tp := cfg.NewDefaultChi(s.RootCtx, s.DefaultLogger, s.ServerName, s.ServerVersion, s.envVars.OTELJaegerHost, s.envVars.DebugHeaders)
		s.ChiServer = srv
		s.tp = tp

		switch {
		case cfg.Default:
			s.GRPCServer = gcfg.NewDefaultGRPCServer()
		default:
			s.GRPCServer = gcfg.NewDefaultGRPCServer()
		}

		if s.parallelMode {
			s.addChiAndGRPCMixedServer()
		} else {
			s.addGRPCServer()
			s.addChiServer()
		}
	}
}

// WithGinAndGRPCServer provides gin http and grpc engine and runs them after Start.
func WithGinAndGRPCServer(cfg GinConfig, gcfg GRPCConfig) func(*Server) {
	return func(s *Server) {
		switch {
		case cfg.Blank:
			s.GinServer = cfg.NewBlankGin()
		case cfg.Default:
			s.GinServer = cfg.NewDefaultGin()
		default:
			s.GinServer = cfg.NewDefaultGin()
		}

		switch {
		case cfg.Default:
			s.GRPCServer = gcfg.NewDefaultGRPCServer()
		default:
			s.GRPCServer = gcfg.NewDefaultGRPCServer()
		}

		if s.parallelMode {
			s.addGinAndGRPCMixedServer()
		} else {
			s.addGRPCServer()
			s.addGinServer()
		}
	}
}

// WithGRPCServer provides grpc kit and runs it after Start.
func WithGRPCServer(cfg GRPCConfig) func(*Server) {
	return func(s *Server) {
		switch {
		case cfg.Default:
			s.GRPCServer = cfg.NewDefaultGRPCServer()
		default:
			s.GRPCServer = cfg.NewDefaultGRPCServer()
		}
		s.addGRPCServer()
	}
}

// WithBeforeStart executes functions before engine start.
func WithBeforeStart(funcs []func() error) func(*Server) {
	return func(s *Server) {
		s.beforeStart = funcs
	}
}

// WithAfterStart executes functions after engine start.
func WithAfterStart(funcs []func() error) func(*Server) {
	return func(s *Server) {
		s.afterStart = funcs
	}
}

// WithAfterStop executes functions after engine stop.
func WithAfterStop(funcs []func() error) func(*Server) {
	return func(s *Server) {
		s.afterStop = funcs
	}
}

// WithBeforeStop executes functions before engine stop.
func WithBeforeStop(funcs []func() error) func(*Server) {
	return func(s *Server) {
		s.beforeStop = funcs
	}
}

// WithCustomGoroutines adds goroutines to main errgroup instance of kit.
func WithCustomGoroutines(funcs []func() error) func(*Server) {
	return func(s *Server) {
		s.customGoroutines = funcs
	}
}
