package httpclient

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func TestTransportFormatter(t *testing.T) {
	var httpMethods = []struct {
		name     string
		method   string
		expected string
	}{
		{
			"GET method",
			http.MethodGet,
			"HTTP GET",
		},
		{
			"HEAD method",
			http.MethodHead,
			"HTTP HEAD",
		},
		{
			"POST method",
			http.MethodPost,
			"HTTP POST",
		},
		{
			"PUT method",
			http.MethodPut,
			"HTTP PUT",
		},
		{
			"PATCH method",
			http.MethodPatch,
			"HTTP PATCH",
		},
		{
			"DELETE method",
			http.MethodDelete,
			"HTTP DELETE",
		},
		{
			"CONNECT method",
			http.MethodConnect,
			"HTTP CONNECT",
		},
		{
			"OPTIONS method",
			http.MethodOptions,
			"HTTP OPTIONS",
		},
		{
			"TRACE method",
			http.MethodTrace,
			"HTTP TRACE",
		},
	}

	for _, tc := range httpMethods {
		t.Run(tc.name, func(t *testing.T) {
			r, err := http.NewRequest(tc.method, "http://localhost/", nil)
			if err != nil {
				t.Fatal(err)
			}
			formattedName := "HTTP " + r.Method

			if formattedName != tc.expected {
				t.Fatalf("unexpected name: got %s, expected %s", formattedName, tc.expected)
			}
		})
	}
}

func TestTransportBasics(t *testing.T) {
	prop := propagation.TraceContext{}
	content := []byte("Hello, world!")

	ctx := context.Background()
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{0x01},
		SpanID:  trace.SpanID{0x01},
	})
	ctx = trace.ContextWithRemoteSpanContext(ctx, sc)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := prop.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		span := trace.SpanContextFromContext(ctx)
		if span.SpanID() != sc.SpanID() {
			t.Fatalf("testing remote SpanID: got %s, expected %s", span.SpanID(), sc.SpanID())
		}
		if _, err := w.Write(content); err != nil {
			t.Fatal(err)
		}
	}))
	defer ts.Close()

	r, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL, nil)
	if err != nil {
		t.Fatal(err)
	}

	tr := NewTransport(http.DefaultTransport)

	c := http.Client{Transport: tr}
	res, err := c.Do(r)
	if err != nil {
		t.Fatal(err)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(body, content) {
		t.Fatalf("unexpected content: got %s, expected %s", body, content)
	}
}

func TestNilTransport(t *testing.T) {
	prop := propagation.TraceContext{}
	content := []byte("Hello, world!")

	ctx := context.Background()
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{0x01},
		SpanID:  trace.SpanID{0x01},
	})
	ctx = trace.ContextWithRemoteSpanContext(ctx, sc)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := prop.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		span := trace.SpanContextFromContext(ctx)
		if span.SpanID() != sc.SpanID() {
			t.Fatalf("testing remote SpanID: got %s, expected %s", span.SpanID(), sc.SpanID())
		}
		if _, err := w.Write(content); err != nil {
			t.Fatal(err)
		}
	}))
	defer ts.Close()

	r, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL, nil)
	if err != nil {
		t.Fatal(err)
	}

	tr := NewTransport(nil)

	c := http.Client{Transport: tr}
	res, err := c.Do(r)
	if err != nil {
		t.Fatal(err)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(body, content) {
		t.Fatalf("unexpected content: got %s, expected %s", body, content)
	}
}

// see if the tests bellow makes sense

func TestTransportProtocolSwitch(t *testing.T) {
	// This test validates the fix to #1329.

	// Simulate a "101 Switching Protocols" response from the test server.
	response := []byte(strings.Join([]string{
		"HTTP/1.1 101 Switching Protocols",
		"Upgrade: WebSocket",
		"Connection: Upgrade",
		"", "", // Needed for extra CRLF.
	}, "\r\n"))

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		conn, buf, err := w.(http.Hijacker).Hijack()
		require.NoError(t, err)

		_, err = buf.Write(response)
		require.NoError(t, err)
		require.NoError(t, buf.Flush())
		require.NoError(t, conn.Close())
	}))
	defer ts.Close()

	ctx := context.Background()
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL, http.NoBody)
	require.NoError(t, err)

	c := http.Client{Transport: NewTransport(http.DefaultTransport)}
	res, err := c.Do(r)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, res.Body.Close()) })

	assert.Implements(t, (*io.ReadWriteCloser)(nil), res.Body, "invalid body returned for protocol switch")
}

func TestTransportOriginRequestNotModify(t *testing.T) {
	ctx := context.Background()
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{0x01},
		SpanID:  trace.SpanID{0x01},
	})
	ctx = trace.ContextWithRemoteSpanContext(ctx, sc)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	r, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL, http.NoBody)
	require.NoError(t, err)

	expectedRequest := r.Clone(r.Context())

	c := http.Client{Transport: NewTransport(http.DefaultTransport)}
	res, err := c.Do(r)
	require.NoError(t, err)

	t.Cleanup(func() { require.NoError(t, res.Body.Close()) })

	assert.Equal(t, expectedRequest, r)
}
