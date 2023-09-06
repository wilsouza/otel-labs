// Package semconv provides utilities for working with OpenTelemetry semantic conventions
// related to HTTP and network operations.
package semconv

import (
	"net"
	"net/http"
	"strconv"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

const (
	// HistogramMeasureUnitSeconds represents the unit (UCUM) convention for
	// measuring durations in seconds.
	HistogramMeasureUnitSeconds = "s"
	// ClientRequestDuration is the key representing the histogram unit for
	// outbound request durations in seconds.
	ClientRequestDuration = "http.client.duration"
)

// httpConv defines the HTTP semantic convention attributes for outbound
// requests duration.
type httpConv struct {
	httpRequestMethodKey      attribute.Key
	httpResponseStatusCodeKey attribute.Key
	serverAddressKey          attribute.Key
}

var hc = &httpConv{
	httpRequestMethodKey:      semconv.HTTPRequestMethodKey,
	httpResponseStatusCodeKey: semconv.HTTPResponseStatusCodeKey,
	serverAddressKey:          semconv.ServerAddressKey,
}

// ClientResponse returns attributes for an HTTP response received by a client
// from a server. It includes the "http.response.status_code" attribute if the
// response status code is defined.
func ClientResponse(res *http.Response) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 1)
	if res.StatusCode > 0 {
		attrs = append(attrs, hc.httpRequestMethodKey.Int(res.StatusCode))
	}

	return attrs
}

// ClientRequest returns trace attributes for an HTTP request made by a client.
// It always includes "http.request.method" and optionally the "server.address".
func ClientRequest(req *http.Request) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 2)
	attrs = append(attrs, hc.httpRequestMethodKey.String(req.Method))

	peer, _ := splitHostPort(req.Host)
	if peer != "" {
		attrs = append(attrs, hc.peerName(peer)...)
	}

	return attrs
}

func (c *httpConv) peerName(address string) []attribute.KeyValue {
	h, p := splitHostPort(address)
	var n int
	if h != "" {
		n++
		if p > 0 {
			n++
		}
	}

	if n == 0 {
		return nil
	}
	return []attribute.KeyValue{c.serverAddressKey.String(h)}
}

// splitHostPort splits a network address hostport of the form "host",
// "host%zone", "[host]", "[host%zone], "host:port", "host%zone:port",
// "[host]:port", "[host%zone]:port", or ":port" into host or host%zone and
// port.
//
// An empty host is returned if it is not provided or unparsable. A negative
// port is returned if it is not provided or unparsable.
func splitHostPort(hostport string) (host string, port int) {
	port = -1

	if strings.HasPrefix(hostport, "[") {
		addrEnd := strings.LastIndex(hostport, "]")
		if addrEnd < 0 {
			// Invalid hostport.
			return
		}
		if i := strings.LastIndex(hostport[addrEnd:], ":"); i < 0 {
			host = hostport[1:addrEnd]
			return
		}
	} else {
		if i := strings.LastIndex(hostport, ":"); i < 0 {
			host = hostport
			return
		}
	}

	host, pStr, err := net.SplitHostPort(hostport)
	if err != nil {
		return
	}

	p, err := strconv.ParseUint(pStr, 10, 16)
	if err != nil {
		return
	}
	return host, int(p)
}
