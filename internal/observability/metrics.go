package observability

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	GatewayServiceRequestsTotal   *prometheus.CounterVec
	GatewayServiceRequestDuration *prometheus.HistogramVec
	GatewayHTTPRequestsTotal      *prometheus.CounterVec
	GatewayHTTPRequestDuration    *prometheus.HistogramVec
	GatewayGRPCRequestsTotal      *prometheus.CounterVec
	GatewayGRPCRequestDuration    *prometheus.HistogramVec
}

var (
	metricsInstance *Metrics
	metricsOnce     sync.Once
)

func MustMetrics() *Metrics {
	metricsOnce.Do(func() {
		metricsInstance = newMetrics()
	})
	return metricsInstance
}

func newMetrics() *Metrics {
	m := &Metrics{
		GatewayServiceRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "gateway",
				Subsystem: "service",
				Name:      "requests_total",
				Help:      "Total number of gateway service requests.",
			},
			[]string{"method", "status"},
		),
		GatewayServiceRequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "gateway",
				Subsystem: "service",
				Name:      "request_duration_seconds",
				Help:      "Gateway service request duration in seconds.",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"method"},
		),
		GatewayHTTPRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "gateway",
				Subsystem: "http",
				Name:      "requests_total",
				Help:      "Total number of HTTP requests handled by gateway.",
			},
			[]string{"method", "path", "status"},
		),
		GatewayHTTPRequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "gateway",
				Subsystem: "http",
				Name:      "request_duration_seconds",
				Help:      "HTTP request duration in seconds for gateway.",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"method", "path"},
		),
		GatewayGRPCRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "gateway",
				Subsystem: "grpc",
				Name:      "requests_total",
				Help:      "Total number of gRPC requests handled by gateway transport.",
			},
			[]string{"method", "code"},
		),
		GatewayGRPCRequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "gateway",
				Subsystem: "grpc",
				Name:      "request_duration_seconds",
				Help:      "gRPC request duration in seconds for gateway transport.",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"method"},
		),
	}

	prometheus.MustRegister(
		m.GatewayServiceRequestsTotal,
		m.GatewayServiceRequestDuration,
		m.GatewayHTTPRequestsTotal,
		m.GatewayHTTPRequestDuration,
		m.GatewayGRPCRequestsTotal,
		m.GatewayGRPCRequestDuration,
	)

	return m
}
