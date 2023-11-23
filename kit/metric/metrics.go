package metric

import (
	"context"
	"sync"

	"github.com/go-faster/errors"
	"github.com/go-logr/zapr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
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

type Metrics struct {
	lg *zap.Logger

	tracerProvider trace.TracerProvider
	meterProvider  metric.MeterProvider

	resource   *resource.Resource
	propagator propagation.TextMapPropagator
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
) (*Metrics, error) {
	{
		logger := lg.Named("kit.otel")
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
	m.propagator = autoprop.NewTextMapPropagator()

	otel.SetMeterProvider(m.MeterProvider())
	otel.SetTracerProvider(m.TracerProvider())
	otel.SetTextMapPropagator(m.TextMapPropagator())

	lg.Info("Metrics initialized")
	return m, nil
}
