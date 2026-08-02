package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/weiqigod/curlew/internal/config"
	"github.com/weiqigod/curlew/internal/uiserver"
)

// uiFlags holds parsed `curlew ui` arguments.
type uiFlags struct {
	port       int  // -1 = unset (config/default precedence applies)
	portSet    bool // true when --port was given explicitly
	env        string
	collection string
	noOpen     bool
	noColor    bool
}

// parseUIArgs parses the ui command's flags. --allow-sensitive is rejected by
// design: the UI always redacts sensitive values.
func parseUIArgs(args []string) (uiFlags, bool, error) {
	f := uiFlags{port: -1}
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; arg {
		case "--help", "-h":
			return f, true, nil
		case "--port":
			if i+1 >= len(args) {
				return f, false, fmt.Errorf("--port requires a value")
			}
			i++
			var port int
			if _, err := fmt.Sscanf(args[i], "%d", &port); err != nil || port < 0 || port > 65535 {
				return f, false, fmt.Errorf("--port: invalid port %q", args[i])
			}
			f.port = port
			f.portSet = true
		case "--env":
			if i+1 >= len(args) {
				return f, false, fmt.Errorf("--env requires a value")
			}
			i++
			f.env = args[i]
		case "--collection":
			if i+1 >= len(args) {
				return f, false, fmt.Errorf("--collection requires a value")
			}
			i++
			f.collection = args[i]
		case "--no-open":
			f.noOpen = true
		case "--no-color":
			f.noColor = true
		case "--allow-sensitive":
			return f, false, fmt.Errorf("the UI always redacts sensitive values")
		default:
			return f, false, fmt.Errorf("unknown flag: %s", arg)
		}
	}
	return f, false, nil
}

// printUIHelpTo writes the ui command help text.
func printUIHelpTo(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: curlew ui [--port <n>] [--env <name>] [--collection <file>] [--no-open] [--no-color]")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Start the local web UI: a runner and inspector over this project's")
	_, _ = fmt.Fprintln(w, "collections. Files stay the source of truth — the UI never edits them.")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Options:")
	_, _ = fmt.Fprintln(w, "  --port <n>           Listen port (default: ui.port from curlew.yaml, else 8765).")
	_, _ = fmt.Fprintln(w, "                       0 picks an ephemeral port. An explicitly chosen busy port")
	_, _ = fmt.Fprintln(w, "                       fails; the default scans up to 19 ports upward.")
	_, _ = fmt.Fprintln(w, "  --env <name>         Environment preselected in the UI (validated against environments/).")
	_, _ = fmt.Fprintln(w, "  --collection <file>  Restrict the tree to one collection file inside the project.")
	_, _ = fmt.Fprintln(w, "  --no-open            Do not launch the browser. The URL is always printed.")
	_, _ = fmt.Fprintln(w, "  --no-color           Disable color on stderr output.")
	_, _ = fmt.Fprintln(w, "  --help, -h           Show this help.")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "The server binds 127.0.0.1 only. Remote access is your own SSH tunnel.")
	_, _ = fmt.Fprintln(w, "Sensitive values are always redacted; there is no --allow-sensitive.")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Exit codes:")
	_, _ = fmt.Fprintln(w, "  0  clean shutdown (signal)")
	_, _ = fmt.Fprintln(w, "  1  usage error; port bind failure; fatal server error")
	_, _ = fmt.Fprintln(w, "  3  invalid config (bad ui: block, unknown --env, --collection outside root)")
	_, _ = fmt.Fprintln(w, "  5  no project found")
}

// mintUIToken returns 32 hex chars from crypto/rand (spec §9.3).
func mintUIToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// openBrowser launches the platform browser for url, best-effort.
func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "windows":
		return exec.Command("cmd", "/c", "start", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}

// uiCmdOut implements `curlew ui` (UI_SPECIFICATION.md §2).
func uiCmdOut(args []string, stdout, stderr io.Writer) int {
	flags, showHelp, err := parseUIArgs(args)
	if showHelp {
		printUIHelpTo(stdout)
		return 0
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
		_, _ = fmt.Fprintln(stderr, usageSynopsis("ui"))
		return 1
	}

	wd, err := os.Getwd()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	root, found := config.FindProjectRoot(wd)
	if !found {
		_, _ = fmt.Fprintln(stderr, "no curlew project found (no curlew.yaml in current or parent directories)")
		return 5
	}
	projectCfg, _, err := config.LoadProjectConfig(root)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
		return 3
	}

	// Resolve per-field precedence: flag > ui: block > built-in default.
	uiCfg := projectCfg.UI
	port := uiserver.DefaultPort
	portDefaulted := true
	if uiCfg != nil && uiCfg.Port != 0 {
		port = uiCfg.Port
		portDefaulted = false
	}
	if flags.portSet {
		port = flags.port
		portDefaulted = false
	}
	host := "127.0.0.1"
	if uiCfg != nil && uiCfg.Host != "" {
		host = uiCfg.Host
		if host == "localhost" {
			host = "127.0.0.1"
		}
	}
	openBrowserWanted := !flags.noOpen
	if uiCfg != nil && uiCfg.OpenBrowser != nil && !*uiCfg.OpenBrowser {
		openBrowserWanted = false
	}
	historyEnabled := true
	maxRuns := uiserver.DefaultMaxRuns
	if uiCfg != nil && uiCfg.History != nil {
		if uiCfg.History.Enabled != nil {
			historyEnabled = *uiCfg.History.Enabled
		}
		if uiCfg.History.MaxRuns > 0 {
			maxRuns = uiCfg.History.MaxRuns
		}
	}

	// Validate --env against the project's environments.
	if flags.env != "" {
		available := config.ListAvailableEnvironments(root)
		if !slices.Contains(available, flags.env) {
			_, _ = fmt.Fprintf(stderr, "Error: unknown environment %q (available: %v)\n", flags.env, available)
			return 3
		}
	}
	// Validate --collection resolves inside the project root.
	collectionFilter := ""
	if flags.collection != "" {
		abs, err := filepath.Abs(flags.collection)
		if err != nil || !strings.HasPrefix(abs, root+string(filepath.Separator)) {
			_, _ = fmt.Fprintf(stderr, "Error: --collection must resolve inside the project root %s\n", root)
			return 3
		}
		rel, _ := filepath.Rel(root, abs)
		collectionFilter = rel
	}

	// Bind loopback. Explicit busy port → exit 1; defaulted busy port → scan +1…+19.
	listener, boundPort, err := listenUI(host, port, portDefaulted && !flags.portSet)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}

	token, err := mintUIToken()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "Error: minting session token: %v\n", err)
		return 1
	}

	srv, err := uiserver.NewServer(uiserver.Options{
		Root:             root,
		ProjectName:      projectCfg.ProjectName,
		Version:          version,
		DefaultEnv:       flags.env,
		CollectionFilter: collectionFilter,
		Token:            token,
		HistoryEnabled:   historyEnabled,
		MaxRuns:          maxRuns,
		EditorCommand:    resolveUIEditor(uiCfg),
		DevProxy:         os.Getenv("CURLEW_UI_DEV_PROXY"),
		Diagnostics: func(format string, args ...any) {
			_, _ = fmt.Fprintf(stderr, format+"\n", args...)
		},
	})
	if err != nil {
		_ = listener.Close()
		_, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}

	url := fmt.Sprintf("http://127.0.0.1:%d/?token=%s", boundPort, token)
	_, _ = fmt.Fprintf(stdout, "curlew ui listening on %s\n", url)
	if openBrowserWanted {
		if err := openBrowser(url); err != nil {
			_, _ = fmt.Fprintf(stderr, "warning: could not open browser: %v\n", err)
		}
	}

	httpSrv := &http.Server{Handler: srv.Handler()}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serveErr := make(chan error, 1)
	go func() { serveErr <- httpSrv.Serve(listener) }()

	select {
	case <-ctx.Done():
		// Cancel any active run, flush the store, drain HTTP with a 5 s cap.
		srv.Shutdown()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutdownCtx)
		return 0
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return 0
		}
		_, _ = fmt.Fprintf(stderr, "Error: server: %v\n", err)
		return 1
	}
}

// listenUI binds host:port. When scan is true (port came from the built-in
// default) and the port is busy, ports +1…+19 are tried before failing.
func listenUI(host string, port int, scan bool) (net.Listener, int, error) {
	tryPorts := []int{port}
	if scan && port != 0 {
		for i := 1; i <= 19; i++ {
			tryPorts = append(tryPorts, port+i)
		}
	}
	var lastErr error
	for _, p := range tryPorts {
		l, err := net.Listen("tcp", net.JoinHostPort(host, fmt.Sprintf("%d", p)))
		if err == nil {
			return l, l.Addr().(*net.TCPAddr).Port, nil
		}
		lastErr = err
	}
	if scan && port != 0 {
		return nil, 0, fmt.Errorf("ports %d-%d are all busy: %w (hint: pass --port to choose one)", port, port+19, lastErr)
	}
	return nil, 0, fmt.Errorf("port %d is busy: %w (hint: pass --port to choose another)", port, lastErr)
}

// resolveUIEditor returns the configured editor command template for /open.
// Resolution order: $CURLEW_EDITOR > ui.editor > "" (server falls back to
// `code --goto`).
func resolveUIEditor(uiCfg *config.UIConfig) string {
	if v := os.Getenv("CURLEW_EDITOR"); v != "" {
		return v
	}
	if uiCfg != nil {
		return uiCfg.Editor
	}
	return ""
}
