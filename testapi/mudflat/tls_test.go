package mudflat

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TLS (§9.N).
//
// These tests set RootCAs explicitly, which is the capability curlew does not
// have (§11.3) and the reason the full posture matrix is not built — see the
// note at the top of tls.go. What they establish is that the one posture which
// IS built works: the certificate chain is sound, the endpoint reports what was
// negotiated, and the same endpoint over cleartext says so rather than
// reporting a TLS version nobody negotiated.

func startTLSServer(t *testing.T) (base string, client *http.Client, caPEM []byte) {
	t.Helper()

	material, err := NewTLSMaterial()
	if err != nil {
		t.Fatalf("certificates: %v", err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := New(Options{})
	go func() { _ = srv.ServeTLS(ln, material) }()
	t.Cleanup(func() { _ = srv.Close(); _ = ln.Close() })

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(material.CAPEM) {
		t.Fatal("generated CA is not valid PEM")
	}
	// ForceAttemptHTTP2 makes this transport offer h2 in ALPN, which is what
	// Go's DefaultTransport does — so this client negotiates what curlew would
	// negotiate if it could reach the listener at all.
	client = &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig:   &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
			ForceAttemptHTTP2: true,
		},
	}
	return "https://" + ln.Addr().String(), client, material.CAPEM
}

func TestTLS_ProtocolReportsWhatWasNegotiated(t *testing.T) {
	base, client, _ := startTLSServer(t)

	resp, err := client.Get(base + "/protocol")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var report protocolReport
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if report.TLS == nil {
		t.Fatal("tls is null over a TLS connection")
	}
	if !strings.HasPrefix(report.TLS.Version, "TLS 1.") {
		t.Errorf("version = %q", report.TLS.Version)
	}
	if report.TLS.CipherSuite == "" {
		t.Error("no cipher suite reported")
	}
	// h2 is negotiated without anyone asking for it: the transport offers it in
	// ALPN, the server accepts, and HTTP/2 happens. That is the precise shape of
	// §11.4 — curlew cannot SELECT a version, but over TLS it does not need to.
	// What it cannot reach is h2c, which has no ALPN to negotiate with.
	if report.TLS.ALPN != "h2" {
		t.Errorf("alpn = %q, want h2", report.TLS.ALPN)
	}
	if report.HTTPVersion != "HTTP/2.0" {
		t.Errorf("http_version = %q, want HTTP/2.0", report.HTTPVersion)
	}
}

func TestTLS_ProtocolOverCleartextReportsNoTLS(t *testing.T) {
	// The negative control. Without it, "tls is not null" would be satisfied by
	// an endpoint that always reported a version.
	base, client := startServer(t)
	_, body := get(t, client, base+"/protocol")

	var report protocolReport
	if err := json.Unmarshal(body, &report); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
	if report.TLS != nil {
		t.Errorf("tls = %+v over a cleartext connection, want null", report.TLS)
	}
	if report.HTTPVersion != "HTTP/1.1" {
		t.Errorf("http_version = %q", report.HTTPVersion)
	}
}

func TestTLS_UntrustedClientIsRefused(t *testing.T) {
	// What curlew sees today: no CA option, so the system trust store decides,
	// and mudflat's CA is not in it. The failure is closed, which is right — the
	// gap is that every posture produces this same error, so an expired
	// certificate and a hostname mismatch are indistinguishable from it.
	base, _, _ := startTLSServer(t)

	plain := &http.Client{Timeout: 5 * time.Second}
	_, err := plain.Get(base + "/protocol")
	if err == nil {
		t.Fatal("a client with the system trust store accepted mudflat's CA")
	}
	if !strings.Contains(err.Error(), "certificate") {
		t.Errorf("err = %v, want a certificate verification failure", err)
	}
}

func TestTLS_MaterialIsUsableAsPEM(t *testing.T) {
	material, err := NewTLSMaterial()
	if err != nil {
		t.Fatalf("certificates: %v", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(material.CAPEM) {
		t.Error("CAPEM does not parse as PEM")
	}
	if len(material.Certificate.Certificate) != 2 {
		t.Errorf("chain has %d certificates, want leaf + CA", len(material.Certificate.Certificate))
	}
	leaf, err := x509.ParseCertificate(material.Certificate.Certificate[0])
	if err != nil {
		t.Fatalf("parse leaf: %v", err)
	}
	if err := leaf.VerifyHostname("127.0.0.1"); err != nil {
		t.Errorf("leaf is not valid for 127.0.0.1: %v", err)
	}
	if time.Now().After(leaf.NotAfter) {
		t.Error("leaf is already expired")
	}
}
