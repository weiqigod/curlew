package httpexec

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strings"
	"sync"
	"time"

	apierrors "github.com/weiqigod/curlew/internal/errors"
)

// Result holds the outcome of an executed HTTP request.
type Result struct {
	StatusCode int
	// Duration spans Do() entry to body fully read — the whole exchange. It is
	// what `duration_ms` reports and what `timing.max_duration_ms` asserts
	// against, so it has to include the download (§11C.6).
	Duration time.Duration
	Body     []byte
	Headers  http.Header
	// Timing carries connection-phase durations measured via net/http/httptrace.
	// Nil for protocols without phase capture (WebSocket, GraphQL transports
	// that bypass Execute). Added for events schema v1.3.
	Timing *Timing
}

// Timing captures connection-phase durations for a single HTTP exchange,
// measured via net/http/httptrace. Zero-value phase fields with Reused=true
// mean the phase did not occur (pooled connection).
type Timing struct {
	DNS      time.Duration // DNSStart → DNSDone
	Connect  time.Duration // ConnectStart → ConnectDone (final address)
	TLS      time.Duration // TLSHandshakeStart → TLSHandshakeDone
	TTFB     time.Duration // WroteRequest → GotFirstResponseByte
	Download time.Duration // GotFirstResponseByte → response body fully read
	Total    time.Duration // Do() entry → body fully read; agrees with Result.Duration
	Reused   bool          // GotConnInfo.Reused
}

// Request defines the HTTP request to execute.
type Request struct {
	Method      string
	URL         string
	Headers     map[string]string
	Body        any
	QueryParams map[string]string
}

// Execute sends an HTTP request and returns the result.
func Execute(ctx context.Context, req *Request) (*Result, error) {
	reqURL, err := applyQueryParams(req.URL, req.QueryParams)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNetwork, err)
	}

	bodyReader, contentType, err := prepareBody(req.Body)
	if err != nil {
		return nil, fmt.Errorf("preparing request body: %w", err)
	}

	trace := newTraceCapture()
	httpReq, err := http.NewRequestWithContext(httptrace.WithClientTrace(ctx, trace.clientTrace()), req.Method, reqURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNetwork, err)
	}

	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	if contentType != "" && httpReq.Header.Get("Content-Type") == "" {
		httpReq.Header.Set("Content-Type", contentType)
	}

	start := time.Now()
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		// Do() returned without a response, so there is no body to wait for and
		// this is the whole of the exchange.
		duration := time.Since(start)
		netErr := apierrors.ClassifyNetworkError(err)
		netErr.Duration = duration
		if netErr.Kind == apierrors.NetworkTimeout {
			netErr.Message = fmt.Sprintf("request timed out after %s", duration.Truncate(time.Millisecond))
			netErr.Hint = fmt.Sprintf("The request exceeded %s — consider increasing the timeout duration", duration.Truncate(time.Millisecond))
		}
		netErr.Inner = fmt.Errorf("%w: %w", ErrNetwork, err)
		return nil, netErr
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	// Measured after the body read, not around Do() alone. Do() returns when the
	// HEADERS arrive, so the earlier placement excluded the download entirely:
	// a one-second stream reported 0ms, and `timing.max_duration_ms` — a
	// documented assertion — could not fail on a slow body (§11C.6). For an
	// ordinary small response the two are indistinguishable, which is why this
	// went unnoticed.
	end := time.Now()
	duration := end.Sub(start)
	if err != nil {
		return nil, classifyBodyError(err, duration)
	}

	return &Result{
		StatusCode: resp.StatusCode,
		Duration:   duration,
		Body:       body,
		Headers:    resp.Header,
		Timing:     trace.finalize(start, end),
	}, nil
}

// traceCapture records httptrace callback timestamps. DNS and connect
// callbacks may fire on other goroutines, so all access is mutex-guarded.
type traceCapture struct {
	mu                        sync.Mutex
	dnsStart, dnsDone         time.Time
	connectStart, connectDone time.Time
	tlsStart, tlsDone         time.Time
	wroteRequest, firstByte   time.Time
	reused                    bool
}

func newTraceCapture() *traceCapture { return &traceCapture{} }

func (t *traceCapture) clientTrace() *httptrace.ClientTrace {
	return &httptrace.ClientTrace{
		DNSStart:             func(httptrace.DNSStartInfo) { t.stamp(&t.dnsStart) },
		DNSDone:              func(httptrace.DNSDoneInfo) { t.stamp(&t.dnsDone) },
		ConnectStart:         func(string, string) { t.stampIfZero(&t.connectStart) },
		ConnectDone:          func(string, string, error) { t.stamp(&t.connectDone) },
		TLSHandshakeStart:    func() { t.stamp(&t.tlsStart) },
		TLSHandshakeDone:     func(tls.ConnectionState, error) { t.stamp(&t.tlsDone) },
		WroteRequest:         func(httptrace.WroteRequestInfo) { t.stamp(&t.wroteRequest) },
		GotFirstResponseByte: func() { t.stamp(&t.firstByte) },
		GotConn: func(info httptrace.GotConnInfo) {
			t.mu.Lock()
			t.reused = info.Reused
			t.mu.Unlock()
		},
	}
}

func (t *traceCapture) stamp(field *time.Time) {
	t.mu.Lock()
	*field = time.Now()
	t.mu.Unlock()
}

// stampIfZero records only the first occurrence (ConnectStart can fire per
// address when dialing happy-eyeballs style; the phase starts at the first).
func (t *traceCapture) stampIfZero(field *time.Time) {
	t.mu.Lock()
	if field.IsZero() {
		*field = time.Now()
	}
	t.mu.Unlock()
}

// finalize converts captured timestamps into a Timing. doStart is the instant
// Execute entered http.Client.Do; bodyDone is the instant the response body
// was fully read.
func (t *traceCapture) finalize(doStart, bodyDone time.Time) *Timing {
	t.mu.Lock()
	defer t.mu.Unlock()
	tm := &Timing{Reused: t.reused, Total: bodyDone.Sub(doStart)}
	if !t.dnsStart.IsZero() && !t.dnsDone.IsZero() {
		tm.DNS = t.dnsDone.Sub(t.dnsStart)
	}
	if !t.connectStart.IsZero() && !t.connectDone.IsZero() {
		tm.Connect = t.connectDone.Sub(t.connectStart)
	}
	if !t.tlsStart.IsZero() && !t.tlsDone.IsZero() {
		tm.TLS = t.tlsDone.Sub(t.tlsStart)
	}
	if !t.wroteRequest.IsZero() && !t.firstByte.IsZero() {
		tm.TTFB = t.firstByte.Sub(t.wroteRequest)
	}
	if !t.firstByte.IsZero() {
		tm.Download = bodyDone.Sub(t.firstByte)
	}
	return tm
}

func applyQueryParams(rawURL string, params map[string]string) (string, error) {
	if len(params) == 0 {
		return rawURL, nil
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parsing URL: %w", err)
	}
	q := u.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func prepareBody(body any) (io.Reader, string, error) {
	if body == nil {
		return nil, "", nil
	}
	switch b := body.(type) {
	case string:
		return strings.NewReader(b), "", nil
	case []byte:
		// Raw bytes (body_binary_file path). Sent verbatim; Content-Type is
		// applied upstream via the Headers map.
		return bytes.NewReader(b), "", nil
	default:
		data, err := json.Marshal(b)
		if err != nil {
			return nil, "", fmt.Errorf("serializing request body to JSON: %w", err)
		}
		return bytes.NewReader(data), "application/json", nil
	}
}

// classifyBodyError distinguishes a body that could not be decoded from a
// connection that failed while the body was being read.
//
// The distinction decides whether retrying is worth anything. A truncated or
// reset connection may well succeed on a second attempt, so it is returned as a
// *errors.NetworkError, which is what retry_on.network_errors keys on. A body
// whose Content-Encoding is a lie will fail identically every time, so it is
// returned as a plain ErrDecode that no retry rule matches.
//
// Go's transport only decompresses gzip transparently, and it strips the
// Content-Encoding header when it does, so the encoding cannot be read back off
// the response here — the error identity is what names it.
func classifyBodyError(err error, duration time.Duration) error {
	if isDecodeError(err) {
		return fmt.Errorf(
			"%w: the server declared Content-Encoding: gzip but the body is not valid gzip: %w",
			ErrDecode, err)
	}

	netErr := apierrors.ClassifyNetworkError(err)
	netErr.Duration = duration
	if netErr.Kind == apierrors.NetworkOther {
		netErr.Message = fmt.Sprintf("connection failed while reading the response body: %s", err)
		netErr.Hint = "The response headers arrived but the body did not complete — the connection was closed early"
	}
	netErr.Inner = fmt.Errorf("%w: reading response body: %w", ErrNetwork, err)
	return netErr
}

// isDecodeError reports whether err came from decompressing the body rather
// than from transporting it.
//
// io.ErrUnexpectedEOF is deliberately absent: a compressed stream that stops
// early almost always means the connection died, which is the retriable case.
func isDecodeError(err error) bool {
	if errors.Is(err, gzip.ErrHeader) || errors.Is(err, gzip.ErrChecksum) {
		return true
	}
	var corrupt flate.CorruptInputError
	return errors.As(err, &corrupt)
}
