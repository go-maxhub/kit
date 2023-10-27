# GOMAXHUB / kit
![kit.jpg](logo.jpg)
Friendly framework with high-performance and strong-extensibility for building micro-services.

## Features
- optional loggers (zap, slog, logrus, etc.)
- optional http server (chi, gin, etc.)
- optional grpc server (parallel and single mode)
- optional metrics and tracing for server (fgprof, pprof) (if supported)
- optional after/before funcs
- add any number of your goroutines to server easily (WithCustomGoroutines option)
