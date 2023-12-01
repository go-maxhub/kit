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
- e2e mode for testing your service

## Environment variables to use

| Name                              | Description                              | Example            | Default          |
|-----------------------------------|------------------------------------------|--------------------|------------------|
| `KIT_SERVER_VERSION`              | Server version(for logs)                 | `0.0.1`            | `0.0.0`          |
| `KIT_GRACEFUL_SHUTDOWN`           | Shutdown timeout                         | `10s`              | `5s`             |
| `KIT_TRACING_JAEGER_HOST`         | Jaeger host to export traces             | `jaeger_host:4318` | `localhost:4318` |
| `KIT_METRICS_FGPROF`              | Enable fgprof                            | `true`             | `false`          |
| `KIT_METRICS_PPROF`               | Enable pprof                             | `true`             | `false`          | 
| `KIT_DEBUG_HEADERS`               | Prints in logs !all! headers of requests | `true`             | `false`          |

## Default configuration

| Feature             | Host           | Notes              |
|---------------------|----------------|--------------------|
| HTTP Server(chi)    | 0.0.0.0:8080   |                    |
| GRPC Server         | 0.0.0.0:8081   |                    |
| Default jaeger host | localhost:4318 |                    |
| Default GRPC header |                | `application/grpc` |
| Fgprof address      | 0.0.0.0:6060   | /debug/fgprof      |
| Pprof address       | 0.0.0.0:8080   | /debug/pprof/*     |
| Prometheus address  | 0.0.0.0:9090   | /metrics           |

## E2E testing mode
* set **WithEndToEndTests** option and provide path to e2e.yaml file
* use flag `-mode=e2e` to run e2e tests with your binary
* see example in **examples/chi_with_e2e** folder
* if you don't set requests timeout, it will be 10s by default
* default timeout before tests are run is 5s, it's time to give your server wake up and init all connections
* Timeout, Headers, Body of requests are optional