package chi

import (
	"context"
	"net/http"
	"time"
	"unsafe"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/riandyrn/otelchi"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/go-maxhub/kit/kit/metric"
	"github.com/go-maxhub/kit/kit/trace"
)

type Config struct {
	Default bool
}

type Middleware func(next http.Handler) http.Handler

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

func byteToString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

func TraceMiddleware(lg *zap.Logger, m *metric.Metrics, t oteltrace.Tracer, debugHeaders bool) Middleware {
	const nanosecInMillisec = float64(time.Millisecond)

	var key struct{}
	var p = m.TextMapPropagator()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			ctx = context.WithValue(ctx, key, logger{base: lg})

			start := time.Now()
			w := &writerProxy{ResponseWriter: rw}

			ctx = p.Extract(ctx, propagation.HeaderCarrier(r.Header))
			ctx, span := t.Start(ctx, "HTTP")
			defer span.End()

			if sc := oteltrace.SpanContextFromContext(ctx); sc.IsValid() {
				w.Header().Set("trace-id", sc.TraceID().String())
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
						oteltrace.WithStackTrace(false),
					)
					span.SetStatus(codes.Error, "Panic recovered")
				}

				timeEnd := time.Now()

				zFields := []zap.Field{
					zap.String("type", "access"),
					zap.String("request_id", middleware.GetReqID(r.Context())),
					zap.String("trace_id", oteltrace.SpanContextFromContext(ctx).TraceID().String()),
					zap.String("span_id", oteltrace.SpanContextFromContext(ctx).SpanID().String()),
					zap.String("remote_ip", r.RemoteAddr),
					zap.String("url.full", r.URL.Path),
					zap.String("proto", r.Proto),
					zap.String("method", r.Method),
					zap.String("user_agent.os.full_name", r.Header.Get("User-Agent")),
					zap.Float64("latency_ms", float64(timeEnd.Sub(start).Nanoseconds())/nanosecInMillisec),
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

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func (c *Config) NewDefaultChi(ctx context.Context, lg *zap.Logger, serverName, serverVersion, jaegerHost string, debugHeaders bool) (*chi.Mux, *sdktrace.TracerProvider) {
	tracer, tp := trace.InitChiTracerProvider(ctx, lg, serverName, serverVersion, jaegerHost)

	m, err := metric.NewMetrics(ctx, lg.Named("kit.metrics"))
	if err != nil {
		lg.Fatal("init metrics", zap.Error(err))
	}

	cl := chi.NewRouter()

	cl.Use(
		metric.NewMiddleware(serverName),
		middleware.RequestID,
		middleware.Timeout(60*time.Second),
		middleware.RealIP,
		TraceMiddleware(lg, m, *tracer, debugHeaders),
		otelchi.Middleware(serverName, otelchi.WithChiRoutes(cl)),
	)
	return cl, tp
}
