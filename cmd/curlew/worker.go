package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"time"

	"github.com/weiqigod/curlew/internal/appdir"
	"github.com/weiqigod/curlew/internal/backend"
	teamtmpl "github.com/weiqigod/curlew/internal/vault/teamtemplate"
	"github.com/weiqigod/curlew/internal/worker"
	schedpkg "github.com/weiqigod/curlew/internal/worker/schedule"
)

// workerSchedulePullCfg holds flags specific to the --schedule-pull mode.
type workerSchedulePullCfg struct {
	enabled      bool
	backendURL   string        // --backend (or CURLEW_BACKEND_URL); defaults to https://api.apitool.dev
	accessToken  string        // resolved from secure storage or CURLEW_BACKEND_TOKEN
	managedToken bool          // true when the login session can be refreshed locally
	pollInterval time.Duration // --poll-interval, default 30s
	once         bool          // --once: run RunOnce and exit (smoke test / CI)
	workingDir   string        // defaults to os.Getwd()
	refreshVault bool          // M16-018: --refresh-vault: bypass team vault TTL on startup
	teamEnv      string        // --env / CURLEW_TEAM_ENV: shared-vault environment for scheduled runs
}

// workerCmdOut implements the worker subcommand: claim-execute-submit loop
// against the M5-008 coordinator service, or the M16 schedule-pull loop.
//
// Exit codes:
//
//	0  = success (all shards completed or no more shards)
//	1  = usage error
//	10 = unauthorized (401 from coordinator) or not logged in
//	2  = network failure (retries exhausted on claim/heartbeat)
func workerCmdOut(args []string, stdout, stderr io.Writer) int {
	cfg, schedCfg, showHelp, err := parseWorkerArgs(args)
	if showHelp {
		printWorkerHelpTo(stdout)
		return 0
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
		_, _ = fmt.Fprintln(stderr, usageSynopsis("worker"))
		return 1
	}

	if schedCfg.enabled {
		return runSchedulePull(schedCfg, stdout, stderr)
	}

	if validateErr := cfg.Validate(); validateErr != nil {
		_, _ = fmt.Fprintf(stderr, "Error: %v\n", validateErr)
		return 1
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	_, runErr := worker.Run(ctx, cfg, worker.RunOptions{Stdout: stdout, Stderr: stderr})
	if runErr != nil {
		if errors.Is(runErr, worker.ErrUnauthorized) {
			_, _ = fmt.Fprintln(stderr, "error: unauthorized")
			return 10
		}
		if errors.Is(runErr, worker.ErrNetworkExhausted) {
			_, _ = fmt.Fprintf(stderr, "error: %v\n", runErr)
			return 2
		}
		_, _ = fmt.Fprintf(stderr, "error: %v\n", runErr)
		return 1
	}
	return 0
}

// runSchedulePull runs the M16 schedule-pull worker.
func runSchedulePull(sc workerSchedulePullCfg, stdout, stderr io.Writer) int {
	backendURL := sc.backendURL
	if backendURL == "" {
		backendURL = os.Getenv("CURLEW_BACKEND_URL")
	}
	if backendURL == "" {
		backendURL = "https://api.apitool.dev"
	}
	cfgDir, dirErr := appdir.ResolveConfigDir()
	if dirErr != nil {
		cfgDir = ""
	}
	if sc.accessToken == "" && sc.managedToken && cfgDir != "" {
		refreshed, refreshErr := refreshStoredBackendAccessToken(context.Background(), cfgDir, backendURL)
		if refreshErr != nil {
			_, _ = fmt.Fprintf(stderr, "error: refresh login session: %v\n", refreshErr)
			return 10
		}
		sc.accessToken = refreshed
	}
	if sc.accessToken == "" {
		_, _ = fmt.Fprintln(stderr, "error: not logged in (run `curlew login` first)")
		return 10
	}

	bc, err := backend.NewClient(backend.Options{BaseURL: backendURL})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	wd := sc.workingDir
	if wd == "" {
		wd, err = os.Getwd()
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "error: get working directory: %v\n", err)
			return 1
		}
	}

	// OrgID only selects the backend route. The backend still authorizes the
	// bearer token, so an unverified routing claim cannot grant vault access.
	orgID := orgIDFromJWT(sc.accessToken)

	var vaultCache *teamtmpl.Cache
	if cfgDir != "" {
		vaultCache = teamtmpl.NewCache(cfgDir)
	}
	scheduleClient := &schedpkg.Client{HTTP: bc, AccessToken: sc.accessToken}
	var refreshAccessToken func(context.Context) (string, error)
	if sc.managedToken && cfgDir != "" {
		refreshAccessToken = func(ctx context.Context) (string, error) {
			return refreshStoredBackendAccessToken(ctx, cfgDir, backendURL)
		}
		scheduleClient.RefreshAccessToken = refreshAccessToken
	}
	fetcher := &backendVaultFetcher{client: bc}
	if refreshAccessToken != nil {
		fetcher.refreshAccessToken = func(ctx context.Context) (string, error) {
			token, refreshErr := refreshAccessToken(ctx)
			if refreshErr == nil {
				scheduleClient.SetAccessToken(token)
			}
			return token, refreshErr
		}
	}
	localTeamConfig := os.Getenv("CURLEW_TEAM_CONFIG")
	loadTeamTemplate := func(ctx context.Context) (*teamtmpl.TeamTemplate, error) {
		loaded, loadErr := teamtmpl.Load(ctx, teamtmpl.LoadOptions{
			Cache:       vaultCache,
			Fetcher:     fetcher,
			OrgID:       orgID,
			AccessToken: scheduleClient.CurrentAccessToken(),
			LocalPath:   localTeamConfig,
			Warn:        stderr,
		})
		if loadErr != nil || loaded == nil {
			return nil, loadErr
		}
		return loaded.Template, nil
	}

	r := &schedpkg.Runner{
		Client: scheduleClient,
		Queue:  &schedpkg.Queue{}, // Dir resolved lazily to ~/.config/curlew/pending-uploads
		Executor: schedpkg.NewRunnerExecutorWithOptions(schedpkg.RunnerExecutorOptions{
			TeamEnv:          sc.teamEnv,
			LoadTeamTemplate: loadTeamTemplate,
		}),
		PollInterval: sc.pollInterval,
		WorkingDir:   wd,
		Stdout:       stdout,
		Stderr:       stderr,
	}

	if cfgDir != "" {
		r.Queue.Dir = filepath.Join(cfgDir, "pending-uploads")
	}

	// M16-018: when --refresh-vault is set, force-refresh the team vault cache
	// on startup before the worker begins polling. Non-fatal — a failure emits a
	// warning but does not prevent the worker from starting.
	if sc.refreshVault && vaultCache != nil && orgID != "" {
		if refreshErr := vaultCache.Refresh(context.Background(), fetcher, orgID, scheduleClient.CurrentAccessToken(), stderr); refreshErr != nil {
			_, _ = fmt.Fprintf(stderr, "warning: team vault refresh failed: %v\n", refreshErr)
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if sc.once {
		if runErr := r.RunOnce(ctx); runErr != nil {
			_, _ = fmt.Fprintf(stderr, "error: %v\n", runErr)
			return 1
		}
		return 0
	}

	if runErr := r.Run(ctx); runErr != nil && !errors.Is(runErr, context.Canceled) {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", runErr)
		return 1
	}
	return 0
}

// parseWorkerArgs extracts flags for the worker subcommand.
// Env vars: CURLEW_COORDINATOR_URL, CURLEW_BACKEND_TOKEN.
// Returns: (coordinator config, schedule-pull config, showHelp, error).
func parseWorkerArgs(args []string) (cfg worker.Config, schedCfg workerSchedulePullCfg, showHelp bool, err error) {
	cfg = worker.Config{
		CoordinatorURL:    os.Getenv("CURLEW_COORDINATOR_URL"),
		Token:             os.Getenv("CURLEW_BACKEND_TOKEN"),
		Concurrency:       1,
		HeartbeatInterval: 15 * time.Second,
	}
	schedCfg = workerSchedulePullCfg{
		pollInterval: 30 * time.Second,
		teamEnv:      os.Getenv("CURLEW_TEAM_ENV"),
	}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return cfg, schedCfg, true, nil
		case "--schedule-pull":
			schedCfg.enabled = true
		case "--backend":
			i++
			if i >= len(args) {
				return cfg, schedCfg, false, fmt.Errorf("--backend requires a value")
			}
			schedCfg.backendURL = args[i]
		case "--poll-interval":
			i++
			if i >= len(args) {
				return cfg, schedCfg, false, fmt.Errorf("--poll-interval requires a value (e.g. 30s)")
			}
			d, parseErr := time.ParseDuration(args[i])
			if parseErr != nil {
				return cfg, schedCfg, false, fmt.Errorf("--poll-interval invalid: %w", parseErr)
			}
			schedCfg.pollInterval = d
		case "--once":
			schedCfg.once = true
		case "--env":
			i++
			if i >= len(args) {
				return cfg, schedCfg, false, fmt.Errorf("--env requires a shared-vault environment name")
			}
			schedCfg.teamEnv = args[i]
		case "--job":
			i++
			if i >= len(args) {
				return cfg, schedCfg, false, fmt.Errorf("--job requires a value")
			}
			cfg.JobID = args[i]
		case "--org":
			i++
			if i >= len(args) {
				return cfg, schedCfg, false, fmt.Errorf("--org requires a value")
			}
			cfg.Org = args[i]
		case "--coordinator-url":
			i++
			if i >= len(args) {
				return cfg, schedCfg, false, fmt.Errorf("--coordinator-url requires a value")
			}
			cfg.CoordinatorURL = args[i]
		case "--token":
			i++
			if i >= len(args) {
				return cfg, schedCfg, false, fmt.Errorf("--token requires a value")
			}
			cfg.Token = args[i]
		case "--worker-id":
			i++
			if i >= len(args) {
				return cfg, schedCfg, false, fmt.Errorf("--worker-id requires a value")
			}
			cfg.WorkerID = args[i]
		case "--concurrency":
			i++
			if i >= len(args) {
				return cfg, schedCfg, false, fmt.Errorf("--concurrency requires a value")
			}
			n, parseErr := strconv.Atoi(args[i])
			if parseErr != nil {
				return cfg, schedCfg, false, fmt.Errorf("--concurrency must be an integer: %w", parseErr)
			}
			cfg.Concurrency = n
		case "--heartbeat-interval":
			i++
			if i >= len(args) {
				return cfg, schedCfg, false, fmt.Errorf("--heartbeat-interval requires a value (e.g. 15s)")
			}
			d, parseErr := time.ParseDuration(args[i])
			if parseErr != nil {
				return cfg, schedCfg, false, fmt.Errorf("--heartbeat-interval invalid: %w", parseErr)
			}
			cfg.HeartbeatInterval = d
		case "--events":
			return cfg, schedCfg, false, fmt.Errorf("--events is supported only on run; use --format jsonl for streaming samples")
		case "--refresh-vault":
			// M16-018: force team-vault cache refresh on startup.
			schedCfg.refreshVault = true
		default:
			return cfg, schedCfg, false, fmt.Errorf("unknown flag: %s", args[i])
		}
	}

	// Mutual exclusion: --schedule-pull cannot be combined with --job.
	if schedCfg.enabled && cfg.JobID != "" {
		return cfg, schedCfg, false, fmt.Errorf("--schedule-pull cannot be combined with --job")
	}

	if cfg.WorkerID == "" {
		cfg.WorkerID = workerDefaultID()
	}

	// Resolve access token for schedule-pull mode.
	if schedCfg.enabled {
		// Prefer env var (for CI/CD pipelines).
		if t := os.Getenv("CURLEW_BACKEND_TOKEN"); t != "" {
			schedCfg.accessToken = t
		} else {
			cfgDir, dirErr := appdir.ResolveConfigDir()
			if dirErr == nil {
				schedCfg.accessToken, _ = loadStoredBackendAccessToken(cfgDir)
				schedCfg.managedToken = schedCfg.accessToken != "" || hasStoredBackendRefreshToken(cfgDir)
			}
		}
	}

	return cfg, schedCfg, false, nil
}

// workerDefaultID generates a unique worker identifier from hostname, PID, and nanoseconds.
func workerDefaultID() string {
	host, _ := os.Hostname()
	return fmt.Sprintf("wkr_%s_%d_%d", host, os.Getpid(), time.Now().UnixNano())
}

// printWorkerHelpTo writes worker help to w.
func printWorkerHelpTo(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: curlew worker [options]")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Run as a distributed worker against an Curlew coordinator.")
	_, _ = fmt.Fprintln(w, "The worker repeatedly claims a shard, executes its requests, submits results,")
	_, _ = fmt.Fprintln(w, "and exits cleanly when no more shards are available.")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Required (coordinator mode):")
	_, _ = fmt.Fprintln(w, "  --job <id>           Coordinator job id (job_<hex>)")
	_, _ = fmt.Fprintln(w, "  --org <slug-or-id>   Organization slug or org_<hex> wire id")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Options (coordinator mode):")
	_, _ = fmt.Fprintln(w, "  --coordinator-url <url>   Coordinator base URL (or set CURLEW_COORDINATOR_URL)")
	_, _ = fmt.Fprintln(w, "  --token <token>           Bearer token (or set CURLEW_BACKEND_TOKEN)")
	_, _ = fmt.Fprintln(w, "  --worker-id <id>          Worker identifier (default: wkr_<host>_<pid>_<nanos>)")
	_, _ = fmt.Fprintln(w, "  --concurrency <n>         Parallel requests per shard (default: 1)")
	_, _ = fmt.Fprintln(w, "  --heartbeat-interval <d>  Heartbeat cadence (default: 15s)")
	_, _ = fmt.Fprintln(w, "  --help, -h                Show this help message")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Schedule pull (M16):")
	_, _ = fmt.Fprintln(w, "  --schedule-pull            Poll the schedule queue and execute due runs locally.")
	_, _ = fmt.Fprintln(w, "  --backend <url>            Backend base URL (default: $CURLEW_BACKEND_URL, then https://api.apitool.dev).")
	_, _ = fmt.Fprintln(w, "  --poll-interval <d>        Poll cadence (default: 30s).")
	_, _ = fmt.Fprintln(w, "  --once                     Drain the queue and execute one due run, then exit.")
	_, _ = fmt.Fprintln(w, "  --env <name>               Shared-vault environment for scheduled runs (auto-selected when only one exists).")
	_, _ = fmt.Fprintln(w, "  --refresh-vault            Force refresh of team vault cache on startup (bypasses TTL).")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "  See docs/SPECIFICATION.md \"Schedule Execution Model\".")
	_, _ = fmt.Fprintln(w, "  Combining --schedule-pull with --perf-pull / --all-modes is reserved for a")
	_, _ = fmt.Fprintln(w, "  future release; this slice supports --schedule-pull alone.")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Environment Variables:")
	_, _ = fmt.Fprintln(w, "  CURLEW_COORDINATOR_URL  Default coordinator base URL")
	_, _ = fmt.Fprintln(w, "  CURLEW_BACKEND_TOKEN    Default bearer token (also used by --schedule-pull)")
	_, _ = fmt.Fprintln(w, "  CURLEW_BACKEND_URL      Default backend URL for --schedule-pull")
	_, _ = fmt.Fprintln(w, "  CURLEW_TEAM_ENV         Default shared-vault environment for --schedule-pull")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Exit codes: 0=ok, 1=usage/error, 2=network, 10=unauthorized/not-logged-in")
}
