package kit

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/riandyrn/otelchi"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/semconv/v1.12.0"
	trace2 "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type ChiConfig struct {
	Default bool
}

type defaultMiddleware func(next http.Handler) http.Handler

type writerProxy struct {
	http.ResponseWriter
	wrote  int64
	status int
}

func (w *writerProxy) Write(bytes []byte) (n int, err error) {
	n, err = w.ResponseWriter.Write(bytes)
	w.wrote += int64(n)
	return n, err
}

type logger struct {
	base *zap.Logger
}

func (w *writerProxy) WriteHeader(statusCode int) {
	w.ResponseWriter.WriteHeader(statusCode)
	w.status = statusCode
}

func TraceMiddleware(lg *zap.Logger, m *otelMetrics, tp *trace.TracerProvider, debugHeaders bool) defaultMiddleware {
	const nanosecInMillisec = float64(time.Millisecond)

	var key struct{}
	var p = m.TextMapPropagator()

	return func(next http.Handler) http.Handler {
		t := tp.Tracer("http")
		propagator := otel.GetTextMapPropagator()
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			ctx = context.WithValue(ctx, key, logger{base: lg})

			start := time.Now()
			w := &writerProxy{ResponseWriter: rw, status: http.StatusOK}

			ctx = p.Extract(ctx, propagation.HeaderCarrier(r.Header))
			ctx, span := t.Start(
				ctx,
				"HTTP",
				trace2.WithSpanKind(trace2.SpanKindServer),
				trace2.WithAttributes(semconv.HTTPServerAttributesFromHTTPRequest(
					"kit", "HTTP", r)...))

			defer span.End()

			if sc := trace2.SpanContextFromContext(ctx); sc.IsValid() {
				w.Header().Set("trace-id", sc.TraceID().String())
				w.Header().Set("span-id", sc.SpanID().String())
			}

			defer func() {
				if r := recover(); r != nil {
					lg.Error("Panic", zap.Stack("stack"))
					if w.status == 0 {
						lg.Debug("Writing error response")
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusInternalServerError)
						w.Header().Set("Message", "Internal kit error: panic recovered")
					}
					span.AddEvent("Panic recovered",
						trace2.WithStackTrace(false),
					)
					span.SetStatus(codes.Error, "Panic recovered")
				}

				timeEnd := time.Now()

				zFields := []zap.Field{
					zap.String("type", "access"),
					zap.String("request_id", middleware.GetReqID(r.Context())),
					zap.String("trace_id", trace2.SpanContextFromContext(ctx).TraceID().String()),
					zap.String("span_id", trace2.SpanContextFromContext(ctx).SpanID().String()),
					zap.String("remote_ip", r.RemoteAddr),
					zap.String("url.full", r.URL.Path),
					zap.String("proto", r.Proto),
					zap.String("method", r.Method),
					zap.String("user_agent.os.full_name", r.Header.Get("User-Agent")),
					zap.Float64("latency_ms", float64(timeEnd.Sub(start).Nanoseconds())/nanosecInMillisec),
					zap.Int("status_code", w.status),
				}

				if debugHeaders {
					for name, values := range r.Header {
						for _, value := range values {
							zFields = append(zFields, zap.String("header."+name, value))
						}
					}
				}
				lg.Info("incoming_request", zFields...)
			}()

			propagator.Inject(ctx, propagation.HeaderCarrier(w.Header()))

			next.ServeHTTP(w, r.WithContext(ctx))

		})
	}
}

func (c *ChiConfig) NewDefaultChi(ctx context.Context, lg *zap.Logger, serverName, serverVersion, jaegerHost string, debugHeaders bool) (*chi.Mux, *trace.TracerProvider) {
	tp := initTracerProvider(ctx, lg, serverName, serverVersion, jaegerHost)

	m, err := newMetrics(ctx, lg.Named("kit.metrics"))
	if err != nil {
		lg.Fatal("init metrics", zap.Error(err))
	}

	cl := chi.NewRouter()

	cl.Use(
		newMiddleware(serverName),
		middleware.RequestID,
		middleware.Timeout(60*time.Second),
		middleware.RealIP,
		TraceMiddleware(lg, m, tp, debugHeaders),
		otelchi.Middleware(serverName, otelchi.WithChiRoutes(cl)),
	)
	return cl, tp
}
