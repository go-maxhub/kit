package trace

import (
	"context"
	"log"

	"go.opentelemetry.io/otel"
	stdout "go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
)

func InitChiTracerProvider() *trace.Tracer {
	exporter, err := stdout.New(stdout.WithPrettyPrint())
	if err != nil {
		log.Fatal(err)
	}
	res, err := resource.New(
		context.Background(),
		resource.WithAttributes(
			// the service name used to display traces in backends
			semconv.ServiceNameKey.String("kit.server"),
		),
	)
	if err != nil {
		log.Fatalf("unable to initialize resource due: %v", err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			log.Printf("shutdown tracer provider: %v", err)
		}
	}()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	// initialize tracer
	tracer := otel.Tracer("kit.tracer")
	return &tracer
}
