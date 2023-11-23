# GOMAXHUB / kit
![kit.jpg](logo.jpg)
Friendly framework with high-performance and strong-extensibility for building micro-services.

## Features
- one and only logger uber.Zap, initialized and configured for production in stock
- chi http server
- optional grpc server (parallel and single mode)
- optional profiling for server (fgprof, pprof)
- optional after/before funcs
- add any number of your goroutines to server easily (WithCustomGoroutines option)
- autoinstrumented with prometheus metrics handler on 0.0.0.0:9090/metrics
- can easily register any prometheus collectors with same-named option
- just use promauto.* to register any metrics you like, it will work immediately

## Environment variables to use

| Name                              | Description                  | Example            | Default          |
|-----------------------------------|------------------------------|--------------------|------------------|
| `KIT_SERVER_VERSION`              | Server version(for logs)     | `0.0.1`            | `0.0.0`          |
| `KIT_GRACEFUL_SHUTDOWN`           | Shutdown timeout             | `10s`              | `5s`             |
| `KIT_TRACING_JAEGER_HOST`         | Jaeger host to export traces | `jaeger_host:4318` | `localhost:4318` |
| `KIT_METRICS_FGPROF`              | Enable fgprof                | `true`             | `false`          |
| `KIT_METRICS_PPROF`               | Enable pprof                 | `true`             | `false`          | 

