// Package httpclient provides utilities for collect http metrics context into
// the outbound request headers.
package httpclient

import (
	"net/http"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"

	"github.com/wilsouza/otel-labs/otelhttpclient/httpclient/semconv"
)

// Transport implements the http.RoundTripper interface and wraps
// outbound HTTP(S) requests with a meter.
type Transport struct {
	rt          http.RoundTripper
	httpLatency metric.Float64Histogram
}

var _ http.RoundTripper = &Transport{}

const instrumentationName string = "github.com/wilsouza/otel-labs/otelhttpclient/httpclient"

// NewTransport wraps the provided http.RoundTripper with one that and includes
// http metrics context into the outbound request headers.
//
// If the provided http.RoundTripper is nil, http.DefaultTransport will be used
// as the base http.RoundTripper. It allows for collecting metrics, specifically
// measuring outbound request durations in seconds, for HTTP client requests made
// using the wrapped RoundTripper.
//
//	base: The base http.RoundTripper to be wrapped. If nil, http.DefaultTransport
//	      is used as the default base RoundTripper.
func NewTransport(base http.RoundTripper) *Transport {
	if base == nil {
		base = http.DefaultTransport
	}

	t := Transport{
		rt: base,
	}

	h, err := otel.Meter(instrumentationName).Float64Histogram(
		semconv.ClientRequestDuration,
		metric.WithUnit(semconv.HistogramMeasureUnitSeconds),
	)
	if err != nil {
		panic(err)
	}

	t.httpLatency = h

	return &t
}

// RoundTrip creates a Span and propagates its context via the provided request's headers
// before handing the request to the configured base RoundTripper. The created span will
// end when the response body is closed or when a read from the body returns io.EOF.
func (t *Transport) RoundTrip(r *http.Request) (*http.Response, error) {
	startTime := time.Now()
	res, err := t.rt.RoundTrip(r)
	if err != nil {
		return res, err
	}

	attrs := append(semconv.ClientRequest(r), semconv.ClientResponse(res)...)

	measureOptions := metric.WithAttributes(attrs...)
	// Use floating point division here for higher precision (instead of Millisecond method).
	elapsedTime := float64(time.Since(startTime)) / float64(time.Millisecond)
	t.httpLatency.Record(r.Context(), elapsedTime, measureOptions)

	return res, err
}
