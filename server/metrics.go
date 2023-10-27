package kit

import (
	"context"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	promexporter "go.opentelemetry.io/otel/exporters/prometheus"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
)

const (
	instrumentationName = "go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func newResource() *sdkresource.Resource {
	var (
		newResourcesOnce sync.Once
		resource         *sdkresource.Resource
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

		resource, _ = sdkresource.Merge(
			sdkresource.Default(),
			extraResources,
		)

	})
	return resource
}

func initPrometheusConfiguration() *prometheus.Registry {
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
