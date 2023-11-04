package metric

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-faster/errors"
	"github.com/go-faster/sdk/autometer"
	"github.com/go-faster/sdk/autotracer"
	"github.com/go-faster/sdk/profiler"
	"github.com/go-logr/zapr"
	"github.com/prometheus/client_golang/prometheus"
	promClient "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/contrib/propagators/autoprop"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	promexporter "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

const (
	instrumentationName = "go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func include[S []E, E any](s S, v ...E) S {
	out := make(S, len(s)+len(v))
	copy(out, s)
	copy(out[len(s):], v)
	return out
}

func newResource() *sdkresource.Resource {
	var (
		newResourcesOnce sync.Once
		r                *sdkresource.Resource
	)

	newResourcesOnce.Do(func() {
		extraResources, _ := sdkresource.New(
			context.Background(),
			sdkresource.WithOS(),
			sdkresource.WithProcess(),
			sdkresource.WithContainer(),
			sdkresource.WithHost(),
			sdkresource.WithAttributes(
				attribute.String("environment", "prod"),
			),
		)

		r, _ = sdkresource.Merge(
			sdkresource.Default(),
			extraResources,
		)

	})
	return r
}

func InitPrometheusConfiguration() *prometheus.Registry {
	exporter, err := promexporter.New()
	if err != nil {
		panic(err)
	}

	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(newResource()),
		sdkmetric.WithReader(exporter),
	)

	provider.Meter(instrumentationName)

	otel.SetMeterProvider(provider)

	reg := prometheus.NewRegistry()
	defaultPromCollectors := []prometheus.Collector{
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	}

	reg.MustRegister(defaultPromCollectors...)
	return reg
}

type httpEndpoint struct {
	srv      *http.Server
	mux      *http.ServeMux
	services []string
	addr     string
}

type Metrics struct {
	lg *zap.Logger

	prom *promClient.Registry

	http []httpEndpoint

	tracerProvider trace.TracerProvider
	meterProvider  metric.MeterProvider

	resource   *resource.Resource
	propagator propagation.TextMapPropagator

	shutdowns []shutdown
}

func (m *Metrics) registerShutdown(name string, fn func(ctx context.Context) error) {
	m.shutdowns = append(m.shutdowns, shutdown{name: name, fn: fn})
}

type shutdown struct {
	name string
	fn   func(ctx context.Context) error
}

func (m *Metrics) String() string {
	return "metrics"
}

func (m *Metrics) MeterProvider() metric.MeterProvider {
	if m.meterProvider == nil {
		return otel.GetMeterProvider()
	}
	return m.meterProvider
}

func (m *Metrics) TracerProvider() trace.TracerProvider {
	if m.tracerProvider == nil {
		return trace.NewNoopTracerProvider()
	}
	return m.tracerProvider
}

func (m *Metrics) TextMapPropagator() propagation.TextMapPropagator {
	return m.propagator
}

func prometheusAddr() string {
	host := "localhost"
	port := "9464"
	if v := os.Getenv("OTEL_EXPORTER_PROMETHEUS_HOST"); v != "" {
		host = v
	}
	if v := os.Getenv("OTEL_EXPORTER_PROMETHEUS_PORT"); v != "" {
		port = v
	}
	return net.JoinHostPort(host, port)
}

func (m *Metrics) registerProfiler(mux *http.ServeMux) {
	var routes []string
	if v := os.Getenv("PPROF_ROUTES"); v != "" {
		routes = strings.Split(v, ",")
	}
	if len(routes) == 1 && routes[0] == "none" {
		return
	}
	opt := profiler.Options{
		Routes: routes,
		UnknownRoute: func(route string) {
			m.lg.Warn("Unknown pprof route", zap.String("route", route))
		},
	}
	mux.Handle("/debug/pprof/", profiler.New(opt))
}

type zapErrorHandler struct {
	lg *zap.Logger
}

func (z zapErrorHandler) Handle(err error) {
	z.lg.Error("Error", zap.Error(err))
}

func Resource(ctx context.Context) (*resource.Resource, error) {
	opts := []resource.Option{
		resource.WithProcessRuntimeDescription(),
		resource.WithProcessRuntimeVersion(),
		resource.WithProcessRuntimeName(),
		resource.WithTelemetrySDK(),
	}
	r, err := resource.New(ctx, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "new")
	}
	return resource.Merge(resource.Default(), r)
}

func NewMetrics(
	ctx context.Context,
	lg *zap.Logger,
	meterOptions []autometer.Option,
	tracerOptions []autotracer.Option,
) (*Metrics, error) {
	{
		// Setup global OTEL logger and error handler.
		logger := lg.Named("otel")
		otel.SetLogger(zapr.NewLogger(logger))
		otel.SetErrorHandler(zapErrorHandler{lg: logger})
	}
	res, err := Resource(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "resource")
	}
	m := &Metrics{
		lg:       lg,
		resource: res,
	}
	{
		provider, stop, err := autotracer.NewTracerProvider(ctx,
			include(tracerOptions,
				autotracer.WithResource(res),
			)...,
		)
		if err != nil {
			return nil, errors.Wrap(err, "tracer provider")
		}
		m.tracerProvider = provider
		m.registerShutdown("tracer", stop)
	}
	{
		provider, stop, err := autometer.NewMeterProvider(ctx,
			include(meterOptions,
				autometer.WithResource(res),
				autometer.WithOnPrometheusRegistry(func(reg *promClient.Registry) {
					m.prom = reg
				}),
			)...,
		)
		if err != nil {
			return nil, errors.Wrap(err, "meter provider")
		}
		m.meterProvider = provider
		m.registerShutdown("meter", stop)
	}

	// Automatically composited from the OTEL_PROPAGATORS environment variable.
	m.propagator = autoprop.NewTextMapPropagator()

	// Setting up go runtime metrics.
	if err := runtime.Start(
		runtime.WithMeterProvider(m.MeterProvider()),
		runtime.WithMinimumReadMemStatsInterval(time.Second), // export as env?
	); err != nil {
		return nil, errors.Wrap(err, "runtime metrics")
	}

	// Register global OTEL providers.
	otel.SetMeterProvider(m.MeterProvider())
	otel.SetTracerProvider(m.TracerProvider())
	otel.SetTextMapPropagator(m.TextMapPropagator())

	// Initialize and register HTTP servers if required.
	//
	// Adding prometheus.
	if m.prom != nil {
		promAddr := prometheusAddr()
		if v := os.Getenv("METRICS_ADDR"); v != "" {
			promAddr = v
		}
		mux := http.NewServeMux()
		e := httpEndpoint{
			srv:      &http.Server{Addr: promAddr, Handler: mux},
			services: []string{"prometheus"},
			addr:     promAddr,
			mux:      mux,
		}
		mux.Handle("/metrics",
			promhttp.HandlerFor(m.prom, promhttp.HandlerOpts{}),
		)
		m.http = append(m.http, e)
	}
	// Adding pprof.
	if v := os.Getenv("PPROF_ADDR"); v != "" {
		const serviceName = "pprof"
		// Search for existing endpoint.
		var he httpEndpoint
		for i, e := range m.http {
			if e.addr != v {
				continue
			}
			// Using existing endpoint
			he = e
			he.services = append(he.services, serviceName)
			m.http[i] = he
		}
		if he.srv == nil {
			// Creating new endpoint.
			mux := http.NewServeMux()
			he = httpEndpoint{
				srv:      &http.Server{Addr: v, Handler: mux},
				addr:     v,
				mux:      mux,
				services: []string{serviceName},
			}
			m.http = append(m.http, he)
		}
		m.registerProfiler(he.mux)
	}
	fields := []zap.Field{
		zap.Stringer("otel.resource", res),
	}
	for _, e := range m.http {
		for _, s := range e.services {
			fields = append(fields, zap.String("http."+s, e.addr))
		}
		name := fmt.Sprintf("http %v", e.services)
		m.registerShutdown(name, e.srv.Shutdown)
	}
	lg.Info("Metrics initialized", fields...)
	return m, nil
}
