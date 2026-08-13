// Command mudflat runs the curlew test API: a deliberately difficult HTTP
// server for exercising the CLI against something curlew did not write.
//
// See docs/TESTAPI_SPECIFICATION.md.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/weiqigod/curlew/testapi/mudflat"
)

const usage = `mudflat — the curlew test API (Phase 1)

Usage:
  mudflat serve [flags]    Run the server
  mudflat index            Print the endpoint index as JSON and exit

Serve flags:
  --port <n>        Structured-layer port (default 8080). The raw adversarial
                    layer listens on --port+1.
  --bind-unsafe     Bind 0.0.0.0 instead of 127.0.0.1. Refuses without this
                    flag: mudflat ships fixed credentials and deliberate
                    protocol violations, and does not belong on a network.

Phase 1 implements the session model, the echo envelope, status codes, body
encodings and content types, deterministic failure injection, and stateful
resources. Phase 2 adds the raw adversarial layer (a second listener that uses
no HTTP library at all), signature verification, the concurrency barrier and
rate limiting. TLS postures, WebSocket, GraphQL and streaming are Phase 3; GET
/capabilities reports what is absent and why.
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "mudflat: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return fmt.Errorf("no command given")
	}

	switch args[0] {
	case "serve":
		return runServe(args[1:])
	case "index":
		return runIndex()
	case "-h", "--help", "help":
		fmt.Print(usage)
		return nil
	default:
		fmt.Fprint(os.Stderr, usage)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	port := fs.Int("port", 8080, "port to listen on")
	bindUnsafe := fs.Bool("bind-unsafe", false, "bind 0.0.0.0 instead of loopback")
	noTLS := fs.Bool("no-tls", false, "skip the TLS listener and its certificate generation")
	caOut := fs.String("ca-out", "", "write the generated CA to this path (for curl --cacert)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	host := "127.0.0.1"
	if *bindUnsafe {
		host = "0.0.0.0"
		fmt.Fprintln(os.Stderr,
			"mudflat: WARNING binding all interfaces. This server ships fixed test\n"+
				"         credentials and serves deliberate protocol violations. Do not\n"+
				"         expose it to an untrusted network.")
	}

	addr := net.JoinHostPort(host, fmt.Sprint(*port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}

	// The raw layer needs its own listener: its responses cannot be produced by
	// net/http, so they cannot share a server with the structured layer.
	rawAddr := net.JoinHostPort(host, fmt.Sprint(*port+1))
	rawLn, err := net.Listen("tcp", rawAddr)
	if err != nil {
		_ = ln.Close()
		return fmt.Errorf("listen on %s for the raw layer: %w", rawAddr, err)
	}

	srv := mudflat.New(mudflat.Options{})
	rawSrv := mudflat.NewRaw(mudflat.RawOptions{})

	// The TLS listener is the one posture §9.N builds — see the note at the top
	// of testapi/mudflat/tls.go for why the other eight are not. curlew cannot
	// trust the generated CA (no CA option, §11.3; and SSL_CERT_FILE was
	// measured not to override the platform verifier on macOS, §14.3), so this
	// exists for clients that can: Go tests, and curl with --cacert.
	var tlsLn net.Listener
	var material *mudflat.TLSMaterial
	if !*noTLS {
		material, err = mudflat.NewTLSMaterial()
		if err != nil {
			_ = ln.Close()
			_ = rawLn.Close()
			return fmt.Errorf("generating certificates: %w", err)
		}
		tlsAddr := net.JoinHostPort(host, fmt.Sprint(*port+2))
		tlsLn, err = net.Listen("tcp", tlsAddr)
		if err != nil {
			_ = ln.Close()
			_ = rawLn.Close()
			return fmt.Errorf("listen on %s for TLS: %w", tlsAddr, err)
		}
		if *caOut != "" {
			if err := os.WriteFile(*caOut, material.CAPEM, 0o600); err != nil {
				_ = ln.Close()
				_ = rawLn.Close()
				_ = tlsLn.Close()
				return fmt.Errorf("writing CA to %s: %w", *caOut, err)
			}
		}
	}

	fmt.Printf("mudflat %s listening on http://%s (%d endpoints)\n",
		mudflat.Version, ln.Addr(), len(srv.Endpoints()))
	fmt.Printf("  index:        http://%s/\n", ln.Addr())
	fmt.Printf("  capabilities: http://%s/capabilities\n", ln.Addr())
	fmt.Printf("  raw layer:    http://%s/raw/… (%d endpoints, no HTTP library)\n",
		rawLn.Addr(), len(rawSrv.Index()))
	if tlsLn != nil {
		fmt.Printf("  tls:          https://%s/protocol (CA generated per process; curlew cannot trust it — §11.3)\n",
			tlsLn.Addr())
		if *caOut != "" {
			fmt.Printf("  ca:           %s\n", *caOut)
		}
	}

	errs := make(chan error, 3)
	go func() { errs <- srv.Serve(ln) }()
	go func() { errs <- rawSrv.Serve(rawLn) }()
	if tlsLn != nil {
		go func() { errs <- srv.ServeTLS(tlsLn, material) }()
	}

	// Shut down on a signal so a CI harness that kills the process gets a clean
	// exit rather than a dropped connection mid-assertion.
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errs:
		if err != nil {
			return fmt.Errorf("serve: %w", err)
		}
		return nil
	case sig := <-signals:
		fmt.Fprintf(os.Stderr, "\nmudflat: %s, shutting down\n", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = rawSrv.Close()
		if tlsLn != nil {
			_ = tlsLn.Close()
		}
		if err := srv.Shutdown(ctx); err != nil {
			return fmt.Errorf("shutdown: %w", err)
		}
		return nil
	}
}

// runIndex prints the registry without binding a port, so the parity test and a
// developer reading the surface do not need a running server.
func runIndex() error {
	srv := mudflat.New(mudflat.Options{})
	rawSrv := mudflat.NewRaw(mudflat.RawOptions{})

	// One index across both layers: the parity test (§16) must see every
	// endpoint, and it should not have to know how many listeners there are.
	out, err := json.MarshalIndent(append(srv.Index(), rawSrv.Index()...), "", "  ")
	if err != nil {
		return fmt.Errorf("marshal index: %w", err)
	}
	fmt.Println(string(out))
	return nil
}
