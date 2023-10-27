# GOMAXHUB / kit
![kit.jpg](logo.jpg)
Friendly framework with high-performance and strong-extensibility for building micro-services.

## Features
- one and only logger uber.Zap, initialized and configured for production in stock
- optional http server (chi, gin, etc.)
- optional grpc server (parallel and single mode)
- optional profiling for server (fgprof, pprof) (if supported)
- optional after/before funcs
- add any number of your goroutines to server easily (WithCustomGoroutines option)
- autoinstrumented with prometheus metrics handler on 0.0.0.0:9090/metrics
- can easily register any prometheus collectors with same-named option
- just use promauto.* to register any metrics you like, it will work immidiately
