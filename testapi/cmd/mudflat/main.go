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
  --port <n>        Port to listen on (default 8080)
  --bind-unsafe     Bind 0.0.0.0 instead of 127.0.0.1. Refuses without this
                    flag: mudflat ships fixed credentials and deliberate
                    protocol violations, and does not belong on a network.

Phase 1 implements the session model, the echo envelope, status codes, body
encodings and content types, deterministic failure injection, and stateful
resources. The raw adversarial layer, signature verification, the concurrency
barrier, TLS postures, WebSocket, GraphQL and streaming are later phases; GET
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

	srv := mudflat.New(mudflat.Options{})

	fmt.Printf("mudflat %s listening on http://%s (%d endpoints)\n",
		mudflat.Version, ln.Addr(), len(srv.Endpoints()))
	fmt.Printf("  index:        http://%s/\n", ln.Addr())
	fmt.Printf("  capabilities: http://%s/capabilities\n", ln.Addr())

	errs := make(chan error, 1)
	go func() { errs <- srv.Serve(ln) }()

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

	out, err := json.MarshalIndent(srv.Index(), "", "  ")
	if err != nil {
		return fmt.Errorf("marshal index: %w", err)
	}
	fmt.Println(string(out))
	return nil
}
