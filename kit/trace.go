package kit

import (
	"context"
	"log"

	jaegerpropagator "go.opentelemetry.io/contrib/propagators/jaeger"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.uber.org/zap"
)

type opentelemetryTracerProviderStore struct {
	exporters []sdktrace.SpanExporter
	res       *resource.Resource
	sampler   sdktrace.Sampler
}

// RegisterExporter adds a Span Exporter for registration with open telemetry global trace provider
func (s *opentelemetryTracerProviderStore) RegisterExporter(exporter sdktrace.SpanExporter) {
	s.exporters = append(s.exporters, exporter)
}

// HasExporter returns whether at least one Span Exporter has been registered
func (s *opentelemetryTracerProviderStore) HasExporter() bool {
	return len(s.exporters) > 0
}

// RegisterResource adds a otelResource for registration with open telemetry global trace provider
func (s *opentelemetryTracerProviderStore) RegisterResource(res *resource.Resource) {
	s.res = res
}

// RegisterSampler adds a custom sampler for registration with open telemetry global trace provider
func (s *opentelemetryTracerProviderStore) RegisterSampler(sampler sdktrace.Sampler) {
	s.sampler = sampler
}

// RegisterTracerProvider RegisterTraceProvider registers a trace provider as per the tracer options in the store
func (s *opentelemetryTracerProviderStore) RegisterTracerProvider() *sdktrace.TracerProvider {
	if len(s.exporters) != 0 {
		var tracerOptions []sdktrace.TracerProviderOption
		for _, exporter := range s.exporters {
			tracerOptions = append(tracerOptions, sdktrace.WithBatcher(exporter))
		}

		if s.res != nil {
			tracerOptions = append(tracerOptions, sdktrace.WithResource(s.res))
		}

		if s.sampler != nil {
			tracerOptions = append(tracerOptions, sdktrace.WithSampler(s.sampler))
		}

		tp := sdktrace.NewTracerProvider(tracerOptions...)

		otel.SetTracerProvider(tp)
		return tp
	}
	return nil
}

func newOTELTracerProviderStore() *opentelemetryTracerProviderStore {
	var exps []sdktrace.SpanExporter
	return &opentelemetryTracerProviderStore{exps, nil, nil}
}

func initTracerProvider(ctx context.Context, lg *zap.Logger, serverName, serverVersion, jaegerHost string) *sdktrace.TracerProvider {
	var (
		client  otlptrace.Client
		tpStore *opentelemetryTracerProviderStore
	)

	tpStore = newOTELTracerProviderStore()

	clientOptions := []otlptracehttp.Option{otlptracehttp.WithEndpoint(jaegerHost), otlptracehttp.WithInsecure()}
	client = otlptracehttp.NewClient(clientOptions...)

	otelExporter, err := otlptrace.New(ctx, client)
	if err != nil {
		lg.Fatal("init new otel exporter", zap.Error(err))
	}

	tpStore.RegisterExporter(otelExporter)

	res, err := resource.New(
		context.Background(),
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serverName),
			semconv.ServiceVersionKey.String(serverVersion),
		),
		resource.WithProcessRuntimeDescription(),
		resource.WithTelemetrySDK(),
		resource.WithContainer(),
		resource.WithHost(),
		resource.WithOS(),
		resource.WithProcess(),
		resource.WithProcessCommandArgs(),
		resource.WithProcessExecutableName(),
		resource.WithProcessOwner(),
		resource.WithProcessExecutablePath(),
		resource.WithOSDescription(),
		resource.WithFromEnv(),
		resource.WithOSType(),
		resource.WithProcessRuntimeDescription(),
		resource.WithProcessRuntimeName(),
		resource.WithProcessRuntimeVersion(),
	)
	if err != nil {
		log.Fatalf("unable to initialize resource due: %v", err)
	}

	tpStore.RegisterResource(res)
	tpStore.RegisterSampler(sdktrace.AlwaysSample())
	tp := tpStore.RegisterTracerProvider()

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
			jaegerpropagator.Jaeger{}),
	)
	return tp
}
