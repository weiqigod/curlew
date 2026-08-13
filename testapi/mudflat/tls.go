package mudflat

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"time"
)

// TLS (§9.N) — and what is deliberately not here.
//
// The specification describes nine ports: a valid chain from mudflat's own CA,
// an expired leaf, a self-signed one, a wrong hostname, a missing intermediate,
// client certificates, a TLS 1.2 ceiling, cleartext, and h2c. §14.3 required
// one thing to be confirmed on the real toolchain before any of it was built:
// whether SSL_CERT_FILE overrides the platform verifier, since that is the only
// mechanism by which a client with no CA option could trust mudflat's CA.
//
// It was measured on go1.25.5 darwin/arm64 rather than assumed. It does not:
//
//	CONTROL (explicit RootCAs):  TRUSTED — 200 OK
//	SSL_CERT_FILE:               FAILED — x509: certificate signed by unknown authority
//	SSL_CERT_DIR:                FAILED — x509: certificate signed by unknown authority
//	SSL_CERT_FILE + GODEBUG=x509usefallbackroots=1: FAILED
//
// The control passing is what makes that attributable: the certificates are
// fine, the trust mechanism is not.
//
// So the eight remaining postures collapse into one outcome for curlew. Chain
// building fails before expiry, hostname or intermediate are ever considered,
// so an expired leaf, a self-signed leaf and a wrong-hostname leaf all produce
// "certificate signed by unknown authority" — the same string, from three
// different defects. Eight endpoints that a client cannot tell apart is exactly
// what §16 deletes, so they are not built. What is built is the one posture
// with a curlew-facing question attached: does a TLS failure produce a legible,
// correctly classified error?
//
// The rest is not lost, only blocked, on a capability §11.3 already names as
// missing. When curlew gains a CA option the postures become distinguishable
// and worth writing; until then they would be eight ways of asserting one
// string.

// tlsHost is the name mudflat's certificate is issued for.
const tlsHost = "mudflat.test"

// TLSMaterial is a generated CA and the leaf it signs. Generated per process
// rather than written to disk: §14.2 keeps certificate generation out of the
// startup path, and an in-memory pair keeps `mudflat serve` a single command
// with no files to stale.
type TLSMaterial struct {
	CAPEM       []byte
	Certificate tls.Certificate
}

// NewTLSMaterial builds a CA and a leaf valid for localhost and 127.0.0.1.
func NewTLSMaterial() (*TLSMaterial, error) {
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("ca key: %w", err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "mudflat test CA", Organization: []string{"mudflat"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, fmt.Errorf("ca certificate: %w", err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		return nil, fmt.Errorf("parse ca: %w", err)
	}

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("leaf key: %w", err)
	}
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: tlsHost},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{tlsHost, "localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		return nil, fmt.Errorf("leaf certificate: %w", err)
	}

	return &TLSMaterial{
		CAPEM: pemEncodeCert(caDER),
		Certificate: tls.Certificate{
			Certificate: [][]byte{leafDER, caDER},
			PrivateKey:  leafKey,
		},
	}, nil
}

// TLSConfig returns a server config offering h2 and http/1.1 over ALPN, in that
// preference order.
//
// Offering h2 is what makes §11.4 precise. curlew cannot select an HTTP version
// — but it does not need to over TLS: Go's default transport offers h2 in ALPN,
// so a client that reaches this listener negotiates HTTP/2 without asking, and
// /protocol reports it. What curlew cannot reach is h2c, cleartext HTTP/2,
// which has no ALPN to negotiate with and needs an explicit upgrade the client
// never sends. The gap is narrower than "no HTTP/2", and this is where the
// difference is visible.
func (m *TLSMaterial) TLSConfig() *tls.Config {
	return &tls.Config{
		Certificates: []tls.Certificate{m.Certificate},
		MinVersion:   tls.VersionTLS12,
		NextProtos:   []string{"h2", "http/1.1"},
	}
}

func pemEncodeCert(der []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func (s *Server) registerTLS() {
	s.register(Endpoint{
		Pattern:   "/protocol",
		Methods:   []string{http.MethodGet},
		Family:    "N",
		Summary:   "Reports the negotiated TLS version, cipher suite, ALPN protocol and SNI — or says the connection was cleartext.",
		Exercises: "Whether curlew reached this over TLS at all, and what it negotiated. Over cleartext it reports tls: null, which is what makes the TLS answer meaningful rather than assumed.",
		Handler:   s.handleProtocol,
	})
}

type protocolReport struct {
	HTTPVersion string      `json:"http_version"`
	TLS         *tlsDetails `json:"tls"`
}

type tlsDetails struct {
	Version     string `json:"version"`
	CipherSuite string `json:"cipher_suite"`
	ALPN        string `json:"alpn"`
	ServerName  string `json:"server_name"`
	Resumed     bool   `json:"resumed"`
}

func (s *Server) handleProtocol(w http.ResponseWriter, r *http.Request) {
	report := protocolReport{HTTPVersion: r.Proto}
	if state := r.TLS; state != nil {
		report.TLS = &tlsDetails{
			Version:     tlsVersionName(state.Version),
			CipherSuite: tls.CipherSuiteName(state.CipherSuite),
			ALPN:        state.NegotiatedProtocol,
			ServerName:  state.ServerName,
			Resumed:     state.DidResume,
		}
	}
	writeJSON(w, http.StatusOK, report)
}

func tlsVersionName(v uint16) string {
	switch v {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	default:
		return fmt.Sprintf("unknown (0x%04X)", v)
	}
}
