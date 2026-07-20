// Copyright (c) The Thanos Authors.
// Licensed under the Apache License 2.0.

package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// ClientMetrics holds a collection of metrics that can be used to instrument a http client.
// By setting this field in HTTPClientConfig, NewHTTPClient will create an instrumented client.
type ClientMetrics struct {
	inFlightGauge            *prometheus.GaugeVec
	requestTotalCount        *prometheus.CounterVec
	requestDurationHistogram *prometheus.HistogramVec
}

// NewClientMetrics creates a new instance of ClientMetrics.
// It will also register the metrics with the included register.
// The metrics include a 'client' label to distinguish between different client types.
func NewClientMetrics(reg prometheus.Registerer) *ClientMetrics {
	if reg == nil {
		return nil
	}

	var m ClientMetrics
	const maxBucketNumber = 256
	const bucketFactor = 1.1

	m.inFlightGauge = promauto.With(reg).NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: "http_client",
		Name:      "in_flight_requests",
		Help:      "A gauge of in-flight requests.",
	}, []string{"client"})

	m.requestTotalCount = promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
		Subsystem: "http_client",
		Name:      "request_total",
		Help:      "Total http client request by code, method, and client.",
	}, []string{"client", "code", "method"})

	m.requestDurationHistogram = promauto.With(reg).NewHistogramVec(
		prometheus.HistogramOpts{
			Subsystem: "http_client",
			Name:      "request_duration_seconds",
			Help:      "A histogram of request latencies.",
			Buckets:   []float64{0.025, .05, .1, .5, 1, 5, 10},

			NativeHistogramBucketFactor:    bucketFactor,
			NativeHistogramMaxBucketNumber: maxBucketNumber,
		},
		[]string{"client", "code", "method"},
	)

	return &m
}

// InstrumentedRoundTripper instruments the given roundtripper with metrics that are
// registered in the provided ClientMetrics. The clientName parameter is used to set the
// 'client' label on all metrics.
func InstrumentedRoundTripper(tripper http.RoundTripper, m *ClientMetrics, clientName string) http.RoundTripper {
	if m == nil {
		return tripper
	}

	// Curry the metrics with the client label
	inFlightGauge := m.inFlightGauge.With(prometheus.Labels{"client": clientName})
	requestTotal := m.requestTotalCount.MustCurryWith(prometheus.Labels{"client": clientName})
	requestDuration := m.requestDurationHistogram.MustCurryWith(prometheus.Labels{"client": clientName})

	return promhttp.InstrumentRoundTripperInFlight(
		inFlightGauge,
		promhttp.InstrumentRoundTripperCounter(
			requestTotal,
			promhttp.InstrumentRoundTripperDuration(
				requestDuration,
				tripper,
			),
		),
	)
}
