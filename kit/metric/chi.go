package metric

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
)

// Default bucket values for histogram metric.
var (
	dflBuckets = []float64{300, 1200, 5000}
)

// Constant metric names used throughout the middleware.
const (
	reqsName    = "requests_total"
	latencyName = "request_duration_milliseconds"
)

// Middleware encapsulates the counters and histograms for monitoring
// the number of requests, their latency, and the response size.
type Middleware struct {
	reqs    *prometheus.CounterVec
	latency *prometheus.HistogramVec
	params  *prometheus.CounterVec
}

// NewMiddleware constructs a Middleware that records basic request metric.
// Name parameter identifies the service, and buckets customizes latency histograms.
// It wraps the next HTTP handler, instrumenting how requests are processed.
func NewMiddleware(name string, buckets ...float64) func(next http.Handler) http.Handler {
	var m Middleware
	m.reqs = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:        reqsName,
			Help:        "How many HTTP requests processed, partitioned by status code, method and HTTP path.",
			ConstLabels: prometheus.Labels{"service": name},
		},
		[]string{"code", "method", "path", "remote_addr", "proto", "user_agent"},
	)
	prometheus.MustRegister(m.reqs)

	if len(buckets) == 0 {
		buckets = dflBuckets
	}

	m.latency = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:        latencyName,
		Help:        "How long it took to process the request, partitioned by status code, method and HTTP path.",
		ConstLabels: prometheus.Labels{"service": name},
		Buckets:     buckets,
	},
		[]string{"code", "method", "path"},
	)
	prometheus.MustRegister(m.latency)

	m.params = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "query_params",
			Help: "All query params",
		},
		[]string{"param_name", "param_value"},
	)
	prometheus.MustRegister(m.params)
	return m.handler
}

func (c Middleware) handler(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		c.reqs.WithLabelValues(fmt.Sprintf("%d", ww.Status()), r.Method, r.URL.Path, r.RemoteAddr, r.Proto, r.UserAgent()).Inc()
		c.latency.WithLabelValues(fmt.Sprintf("%d", ww.Status()), r.Method, r.URL.Path).Observe(float64(time.Since(start).Nanoseconds()) / 1000000)
		for k, v := range r.URL.Query() {
			c.params.WithLabelValues(k, strings.Join(v, " ")).Inc()
		}
	}
	return http.HandlerFunc(fn)
}
