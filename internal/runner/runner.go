package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	celgo "github.com/google/cel-go/cel"

	"github.com/weiqigod/curlew/internal/assertion"
	"github.com/weiqigod/curlew/internal/auth"
	apicel "github.com/weiqigod/curlew/internal/cel"
	"github.com/weiqigod/curlew/internal/config"
	"github.com/weiqigod/curlew/internal/datadriven"
	apierrors "github.com/weiqigod/curlew/internal/errors"
	"github.com/weiqigod/curlew/internal/graphql"
	"github.com/weiqigod/curlew/internal/httpexec"
	"github.com/weiqigod/curlew/internal/output/ids"
	"github.com/weiqigod/curlew/internal/parallel"
	"github.com/weiqigod/curlew/internal/parser"
	"github.com/weiqigod/curlew/internal/plugin/hooks"
	"github.com/weiqigod/curlew/internal/ratelimit"
	"github.com/weiqigod/curlew/internal/requtil"
	"github.com/weiqigod/curlew/internal/retry"
	"github.com/weiqigod/curlew/internal/signer"
	"github.com/weiqigod/curlew/internal/variable"
	"github.com/weiqigod/curlew/internal/vault"
	"github.com/weiqigod/curlew/internal/vault/teamtemplate"
	"github.com/weiqigod/curlew/internal/websocket"
)

// HooksDispatcher is the interface used by the runner to invoke plugin lifecycle
// hooks. It is satisfied by *hooks.Dispatcher. Exposed as an interface for testability.
type HooksDispatcher interface {
	OnRequest(ctx context.Context, req hooks.RequestPayload) (hooks.RequestPayload, error)
	OnResponse(ctx context.Context, resp hooks.ResponsePayload) (hooks.ResponsePayload, error)
	OnResult(ctx context.Context, res hooks.ResultPayload) error
	Close() error
}

// ErrAuthProfileNotFound is returned when a request references an auth profile
// that does not exist in the configured auth profiles.
var ErrAuthProfileNotFound = errors.New("auth profile not found")

// ErrNoMatchingRequests is returned when --only names at least one request but
// none of the provided names match any main-phase item. The error message
// lists the available main request names.
var ErrNoMatchingRequests = errors.New("no main requests matched --only")

// resolveAuthProfile looks up authName in profiles and returns the header name
// and value to inject. Returns ("", "", nil) when authName is empty.
// Returns a descriptive ErrAuthProfileNotFound when the profile is not found,
// listing all available profile names.
func resolveAuthProfile(authName string, profiles []auth.Profile, scope *variable.Scope) (headerName, headerValue string, err error) {
	if authName == "" {
		return "", "", nil
	}

	var found *auth.Profile
	available := make([]string, 0, len(profiles))
	for i := range profiles {
		available = append(available, profiles[i].Name)
		if profiles[i].Name == authName {
			found = &profiles[i]
		}
	}

	if found == nil {
		if len(available) == 0 {
			return "", "", fmt.Errorf("%w: %q; no auth profiles configured (add auth_profiles: to curlew.yaml)",
				ErrAuthProfileNotFound, authName)
		}
		sort.Strings(available)
		return "", "", fmt.Errorf("%w: %q; available profiles: %s",
			ErrAuthProfileNotFound, authName, strings.Join(available, ", "))
	}

	varName := found.Extract
	if varName == "" {
		varName = found.Name
	}

	resolved := scope.Resolved()
	val, ok := resolved[varName]
	if !ok {
		return "", "", fmt.Errorf("auth profile %q: variable %q not found in scope (did the profile execute successfully?)",
			authName, varName)
	}

	return "Authorization", "Bearer " + val, nil
}

// findAuthProfile returns a pointer to the profile with the given name, or nil if not found.
func findAuthProfile(name string, profiles []auth.Profile) *auth.Profile {
	for i := range profiles {
		if profiles[i].Name == name {
			return &profiles[i]
		}
	}
	return nil
}

// hasHeaderCaseInsensitive reports whether headers contains name (case-insensitive).
func hasHeaderCaseInsensitive(headers map[string]string, name string) bool {
	lower := strings.ToLower(name)
	for k := range headers {
		if strings.ToLower(k) == lower {
			return true
		}
	}
	return false
}

// MaxRequests is the guard rail limit for abuse prevention.
// Tests may override this value; restore via t.Cleanup.
var MaxRequests = 1000

// ExecuteFunc is the function signature for executing a single request.
type ExecuteFunc = requtil.ExecuteFunc

// EventSink receives per-request lifecycle notifications from the runner so
// callers (cmd/curlew) can translate them into an NDJSON event stream.
// Every method is non-blocking — implementations must not return errors that
// halt execution; emission failures are logged to stderr by the adapter.
//
// Contract:
//   - RequestStart fires exactly once per request, before the first exec call.
//   - AssertionResult fires once per individual assertion item, after Evaluate.
//   - RequestEnd fires exactly once per request, after assertion evaluation,
//     retries, and variable extraction — regardless of outcome.
type EventSink interface {
	RequestStart(ev RequestEvent)
	RequestEnd(ev RequestEndEvent)
	AssertionResult(ev AssertionEvent)
}

// RequestEvent describes a request lifecycle start for event emission.
type RequestEvent struct {
	RequestID   string
	RequestSlug string // M9-001: derived from Name at parse time; pairs with RequestEndEvent
	Name        string
	Method      string
	URL         string
	Phase       string
	SourceFile  string
	SourceLine  int
}

// RequestEndEvent describes a completed request for event emission.
type RequestEndEvent struct {
	RequestID    string
	RequestSlug  string // M9-001: same value as the paired RequestEvent
	Outcome      string // "passed" | "failed" | "skipped" | "error"
	StatusCode   int
	Duration     time.Duration
	WaveIndex    int
	RequestBody  []byte
	ResponseBody []byte
	Err          error
	// Additive fields for mid-run inspection (curlew ui, events schema v1.3).
	// The NDJSON emitter ignores the header fields — headers stay out of the
	// events schema; the UI's REST detail endpoint covers them.
	RequestHeaders  map[string]string // interpolated request headers (same data as RequestResult.RequestHeaders)
	ResponseHeaders http.Header       // from httpexec.Result.Headers; nil on error/skip
	Timing          *httpexec.Timing  // connection-phase breakdown; nil when unavailable
	Attempts        int               // retry.Outcome.Attempts; 1 when no retry
}

// AssertionEvent describes one evaluated assertion for event emission.
type AssertionEvent struct {
	RequestID string
	// RequestSlug is the paired RequestEvent's slug, carried so consumers can
	// name the request without joining on the positional RequestID.
	RequestSlug string
	// SourceFile and SourceLine locate the assertion in the collection.
	SourceFile string
	SourceLine int
	// Type, Target and Operator mirror assertion.Result's identity triple.
	// Type is a discriminator only; use Label for anything a human reads.
	Type     string
	Target   string
	Operator string
	Expected string
	Actual   string
	Passed   bool
}

// Label returns the human-readable description of the assertion, e.g.
// "body $.user.name equals".
func (e AssertionEvent) Label() string {
	return assertion.Label(e.Type, e.Target, e.Operator)
}

// Phase identifies the execution phase of a request.
type Phase string

const (
	// PhaseSetup is the setup phase, executed before main requests.
	PhaseSetup Phase = "setup"
	// PhaseMain is the main execution phase.
	PhaseMain Phase = "main"
	// PhaseTeardown is the teardown phase, executed after main requests unconditionally.
	PhaseTeardown Phase = "teardown"
)

// RequestResult holds the outcome of a single request execution.
type RequestResult struct {
	Name             string
	Phase            Phase             // empty string treated as PhaseMain for backward compat
	Method           string            // HTTP method (after interpolation); empty for context-cancelled skips
	URL              string            // Full URL (after interpolation); empty for context-cancelled skips
	RequestHeaders   map[string]string // interpolated request headers (for -v/-vv output)
	RequestBody      any               // interpolated request body (for -vv output)
	Result           *httpexec.Result
	Err              error
	Skipped          bool
	SkipReason       string // human-readable reason when Skipped is true
	AssertionResults *assertion.Results
	RetryCount       int                   // number of retries (0 = no retries, Attempts-1)
	RetryWarnings    []string              // warnings from retry conditions (e.g., non-idempotent method)
	AttemptDetails   []retry.AttemptDetail // per-attempt details (populated when retries occurred)
	WaveIndex        int                   // parallel execution wave index (0-based); -1 when not parallel
	Warnings         []string              // non-fatal warnings (e.g. GraphQL partial success in warn mode)
	// Data-driven iteration metadata (populated only for data-driven results)
	IsDataDriven   bool              // true when this result is from a data-driven iteration
	DataDrivenName string            // base request name (without [X/Y] suffix)
	IterationIndex int               // 0-based iteration index
	IterationTotal int               // total number of iterations in this data-driven group
	IterationData  map[string]string // row data values for this iteration (nil when not data-driven)

	// SourceFile / SourceLine are copied verbatim from the originating
	// parser.RequestItem (M6-002). For data-driven iterations every iteration
	// shares the base item's values; iteration identity is encoded by
	// IsDataDriven / IterationIndex / IterationTotal.
	// Zero values mean the item was synthesised (e.g. OpenAPI import) with no
	// source YAML origin.
	SourceFile string
	SourceLine int

	// RequestID is the per-run identifier minted by Run; matches the
	// request_id field in --events NDJSON when events are enabled.
	// Always populated for non-skipped requests; empty for context-cancelled
	// skips. M9-002.
	RequestID string

	// RequestSlug is the URL-safe slug (parser.Slug). For top-level items
	// this is item.Slug; for data-driven iterations this is the slug of
	// the iteration name (e.g. "create-user-2-3"). M9-002.
	RequestSlug string
}

// Summary holds aggregate execution results.
type Summary struct {
	Total                   int
	Passed                  int
	Failed                  int
	Skipped                 int
	AssertionFailures       int // subset of Failed caused by assertion failures (not network errors)
	TeardownErrors          int // failures in teardown phase (do not affect exit code)
	TeardownAssertionErrors int // assertion failures within teardown (subset of TeardownErrors)
	Duration                time.Duration
	LimitExceeded           bool                   // true when guard rail stopped execution
	RequestsExecuted        int                    // HTTP requests actually sent (not skipped)
	AuthSensitive           *variable.SensitiveSet // variables from auth profiles (always sensitive)
	RuntimeSensitive        *variable.SensitiveSet // values registered at runtime by dynamic-fn eval (e.g. resolved $hmacSha256 keys when sourced from a sensitive variable)
	Impact                  []parallel.ImpactEntry // parallel execution impact analysis
	// Parallel execution metadata (only populated when --parallel is used)
	WaveCount      int             // total number of execution waves
	MaxParallelism int             // largest wave size (max concurrent requests)
	WaveDurations  []time.Duration // duration of each wave
	IsParallel     bool            // true when parallel execution was used
	// SharedSecretsResolved is the number of aliases resolved from the team
	// vault template. Zero when no team template is configured.
	SharedSecretsResolved int

	// RunID is the per-run identifier minted by Run. 32-char lowercase
	// hex. Stable across all phases. M9-002.
	RunID string
}

// VarSources bundles all variable sources for a collection run.
type VarSources struct {
	Project             map[string]string       // curlew.yaml global variables (precedence 2)
	EnvFile             map[string]string       // --env file variables (precedence 3)
	DotEnv              map[string]string       // .env variables (precedence 4)
	EnvVar              map[string]string       // --env-var OS imports (precedence 9)
	CLI                 map[string]string       // --var variables (precedence 10)
	Seed                *int64                  // nil = real randomness; non-nil = deterministic seed
	Secrets             *vault.SecretsConfig    // vault provider config from curlew.yaml (nil = no vault)
	VaultExecutor       vault.CommandExecutor   // nil = use variable.ExecuteCommand
	AuthProfiles        []auth.Profile          // from curlew.yaml auth_profiles: block
	ProjectRoot         string                  // directory containing curlew.yaml (for resolving relative paths)
	AuthExecuteFunc     auth.ExecuteFunc        // nil = use RunForExtraction; non-nil = use directly (for tests)
	CacheStore          auth.CacheStore         // nil = auto-detect from ProjectRoot; non-nil for test injection
	GlobalRetry         *retry.FullConfig       // from curlew.yaml defaults.retry
	GlobalGraphQL       *config.GraphQLDefaults // from curlew.yaml defaults.graphql
	Parallel            bool                    // true = parallel execution for main phase
	CollectionDir       string                  // directory of the collection file (for resolving relative data file paths)
	ConfirmLargeDataset bool                    // true when user confirmed large dataset execution (>10,000 rows)
	WebSocketDialer     websocket.Dialer        // nil = use websocket.DefaultDialer (injection point for tests)

	// TeamTemplate is the parsed shared vault configuration template loaded from
	// CURLEW_TEAM_CONFIG. When non-nil the runner resolves {{secrets.X}}
	// references via the active environment (TeamEnv) before the first request.
	TeamTemplate *teamtemplate.TeamTemplate
	// TeamEnv is the --env name that selects which environment in TeamTemplate
	// to use. Required when the collection references {{secrets.X}} tokens.
	TeamEnv string
	// TeamStub, when true, uses an in-memory stub provider (CURLEW_VAULT_STUB=1)
	// instead of real AWS/Azure credentials.
	TeamStub bool

	// Signer is the registry of named signer factories. When non-nil, the
	// runner allocates an exec-wrap that invokes the resolved signer between
	// variable templating and HTTP execution for every request that has a
	// signing: field (from the request or as a collection-level default).
	// When nil, signer.NewWithBuiltins() is used — a registry preloaded with
	// all signers registered at init time (e.g. aws-sigv4, oauth1).
	// The signer wrap is skipped entirely when no request or collection in the
	// run sets signing: (fast path, zero overhead).
	Signer *signer.Registry

	// Hooks is the plugin hook dispatcher. When non-nil, the runner wraps each
	// HTTP exec call to invoke on_request (before) and on_response (after),
	// and fires on_result once after all phases complete. When nil, no hooks
	// are invoked (zero overhead for runs without plugins).
	Hooks HooksDispatcher

	// OnEvent receives per-request lifecycle callbacks. When nil, the runner
	// incurs zero overhead. M6-005 adds this to support the --events NDJSON
	// stream in cmd/curlew; no internal package imports output/events.
	OnEvent EventSink

	// RuntimeSensitive is the set that receives values discovered during the
	// run — dynamic-function credentials, and extracted values whose names are
	// sensitive. When nil the runner allocates its own and returns it on
	// Summary.RuntimeSensitive.
	//
	// A caller passes its own set when something is already reading it *during*
	// the run: the --events sink redacts each request.end as it is emitted, so
	// a token extracted at request 1 must be known before request 1's own
	// response body reaches the stream. A set handed back at the end is too
	// late for that.
	RuntimeSensitive *variable.SensitiveSet

	// Selection is the list of main request names passed via --only. When
	// non-empty, the runner filters col.Requests.Items to items whose Name
	// matches exactly (case-sensitive). Setup and teardown are never filtered.
	// An empty or nil Selection means "run all main requests" (the default).
	// Whitespace trimming is the caller's responsibility.
	Selection []string

	// RunID overrides the auto-generated run identifier. When empty,
	// Run mints one via crypto/rand. Used by deterministic tests to pin
	// run_id for byte-stable goldens. M9-002.
	RunID string

	// RequestIDPrefix prefixes every minted request id ("<prefix>req-N").
	// Empty for single runs (existing id format unchanged); multi-collection
	// batch runs (curlew ui) pass "c1-", "c2-", … per collection so ids stay
	// unique across the shared event stream and detail map.
	RequestIDPrefix string

	// globalLimiter is internal state populated by Run to share the collection-level
	// token-bucket across all phases and workers. Callers should leave this nil.
	globalLimiter *ratelimit.Limiter

	// reqIDCounter is a per-Run atomic counter shared with parallel execution.
	// Lazily initialised inside Run so external callers never touch it.
	reqIDCounter *atomic.Int64

	// fullMainForCliff is internal state populated by Run so the --only
	// variable-cliff diagnostic can look up producers over the un-filtered main
	// items list. Callers should leave this nil.
	fullMainForCliff []parser.RequestItem

	// fullSetupForCliff is internal state populated by Run so the --only
	// variable-cliff diagnostic can look up setup-phase producers after a
	// minimal-setup prune. Callers should leave this nil. M8-005.
	fullSetupForCliff []parser.RequestItem

	// Locale is the --locale flag value (precedence 5, highest in the chain).
	// Empty string means "unset at CLI level"; the resolver applies the full
	// Default < Project < Environment < Collection < CLI chain.
	Locale string

	// ProjectLocale is the config.locale from curlew.yaml (precedence 2).
	// Empty means unset or the project file had no config: block.
	ProjectLocale string

	// EnvironmentLocale is config.locale from the selected environment file
	// (precedence 3). Empty when no environment or config block is selected.
	EnvironmentLocale string

	// CollectionLocale is the config.locale from the collection file
	// (precedence 4). Empty when the collection has no config: block.
	CollectionLocale string

	// LocaleVerbose gates the locale fallback warning (e.g. "locale en-GB not
	// available; falling back to en-US"). When false (the default), the warning
	// is suppressed even when Diagnostics is set. Set true only when the CLI
	// verbosity is >= VerbosityVerbose. Mirrors the exec subcommand's
	// opts.Verbosity >= VerbosityVerbose gate. M20-001.
	LocaleVerbose bool

	// Diagnostics receives one-line, human-readable warnings the runner emits
	// outside the structured error path (e.g. "--only minimal-setup analysis
	// failed, falling back to full setup"). When nil (default), no diagnostics
	// are emitted. The cmd layer threads stderr here; tests pass a *bytes.Buffer.
	// M8-005.
	Diagnostics io.Writer

	// CelEvaluator compiles `if:` expressions (M19-001). When nil, no if: gate
	// is active. Pre-initialised by runPhases when any item has if: set.
	// Set explicitly in tests for injection.
	CelEvaluator apicel.Evaluator

	// ifProgCache caches compiled CEL programs keyed by expression source string.
	// Lazily initialised on first if: evaluation. Per-VarSources (per-run) so
	// two runs get independent caches and concurrency is simple.
	ifProgCache map[string]apicel.Program

	// assertProgCache caches compiled CEL programs for cel: assertions keyed
	// by expression source string. Shares the same lifecycle and lazy-init
	// pattern as ifProgCache. Nil until runPhases detects cel: assertions.
	assertProgCache map[string]apicel.Program
}

// resolveLocale applies the locale precedence chain and validates the winner.
// Sources, lowest to highest: project config, environment config, collection
// config, CLI flag. Empty string means "unset" at each level. Returns "" when
// nothing is set (registry defaults to en-US). Returns ERR_LOCALE_UNKNOWN for
// an unsupported winning value.
func resolveLocale(vars VarSources) (string, error) {
	chosen := ""
	for _, src := range []string{
		vars.ProjectLocale,     // precedence 2
		vars.EnvironmentLocale, // precedence 3 (seam: empty until env config parsed)
		vars.CollectionLocale,  // precedence 4
		vars.Locale,            // precedence 5 — CLI flag, highest
	} {
		if strings.TrimSpace(src) != "" {
			chosen = src
		}
	}
	if chosen == "" {
		return "", nil
	}
	if err := variable.ValidateLocale(chosen); err != nil {
		return "", err
	}
	return chosen, nil
}

// nextRequestID atomically increments and returns the next request ID string.
// It is safe to call concurrently across goroutines. When RequestIDPrefix is
// non-empty, ids are "<prefix>req-N" — used by multi-collection batch runs
// (curlew ui) to keep request ids unique across per-collection Run calls
// sharing one event stream. request_id is an opaque pairing string per the
// events schema, so prefixed ids are schema-legal.
func (v VarSources) nextRequestID() string {
	return fmt.Sprintf("%sreq-%d", v.RequestIDPrefix, v.reqIDCounter.Add(1))
}

// ensureCelEvaluator returns the CEL evaluator for this run. When CelEvaluator
// is set (e.g. by a test or by runPhases' pre-init), that value is returned
// directly. Otherwise a new evaluator is constructed. The caller (runPhases)
// stores the result back into vars.CelEvaluator before passing vars by value
// to executePhase so each copy sees the same evaluator.
func (v *VarSources) ensureCelEvaluator() (apicel.Evaluator, error) {
	if v.CelEvaluator != nil {
		return v.CelEvaluator, nil
	}
	return apicel.NewEvaluator()
}

// ensureIfProgCache lazily initialises the if-expression compile cache.
func (v *VarSources) ensureIfProgCache() map[string]apicel.Program {
	if v.ifProgCache == nil {
		v.ifProgCache = make(map[string]apicel.Program)
	}
	return v.ifProgCache
}

// compileIfProgram looks up src in the cache; if absent, compiles it with the
// evaluator and stores the result. Returned programs are shared across uses of
// the same source expression within a run (safe because Programs are immutable).
func compileIfProgram(cache map[string]apicel.Program, ev apicel.Evaluator, src string) (apicel.Program, error) {
	if p, ok := cache[src]; ok {
		return p, nil
	}
	p, err := ev.Compile(src, celgo.BoolType)
	if err != nil {
		return nil, err
	}
	cache[src] = p
	return p, nil
}

// NewRunID produces a 32-char lowercase hex identifier. Exposed so cmd/curlew
// can generate a single run_id and pass it to both the events emitter and the
// runner. Delegates to internal/output/ids so events, exec --log, and markdown
// sentinels share one canonical implementation. M9-002, M11-004.
func NewRunID() string {
	return ids.NewRunID()
}

// Run executes all requests in the collection sequentially and returns
// per-request results, a summary, and any pre-execution error (e.g. variable resolution).
// Precedence: Project (2) < EnvFile (3) < DotEnv (4) < collection (7) < EnvVar (9) < CLI (10).
// Execution order: auth profiles → setup → main → teardown (teardown always runs).
func Run(ctx context.Context, col *parser.Collection, exec ExecuteFunc, vars VarSources) ([]RequestResult, *Summary, error) {
	total := len(col.Setup.Items) + len(col.Requests.Items) + len(col.Teardown.Items)
	emptySummary := &Summary{Total: total}

	// Initialise the per-run request ID counter so it is shared across all
	// phases (sequential and parallel) via the vars value copy.
	var idc atomic.Int64
	vars.reqIDCounter = &idc

	// M9-002: mint run_id once. Empty vars.RunID generates a fresh value;
	// a non-empty value is honoured (deterministic tests pin run_id for goldens).
	runID := vars.RunID
	if runID == "" {
		runID = NewRunID()
	}
	emptySummary.RunID = runID

	scope, sharedSecretsResolved, err := buildScope(ctx, col, vars)
	if err != nil {
		return nil, emptySummary, err
	}

	// Allocate the per-run runtime SensitiveSet and attach it to the scope.
	// Dynamic-function dispatch (Scope.Interpolate) mutates this set when a
	// credential-bearing argument (e.g. the key of $hmacSha256) resolves from
	// a sensitive source. The set is exposed on the summary so cmd/curlew can
	// merge it into the post-run redaction set.
	runtimeSensitive := vars.RuntimeSensitive
	if runtimeSensitive == nil {
		runtimeSensitive = variable.NewSensitiveSet()
	}
	scope = scope.WithRuntimeSensitive(runtimeSensitive)

	// Build the shared global limiter and wrap exec so every HTTP dispatch
	// (sequential, parallel, and data-driven) waits on the same token bucket.
	globalLimiter := ratelimit.New(col.RateLimitRPS)
	if globalLimiter != nil {
		unwrapped := exec
		exec = func(ctx context.Context, req *httpexec.Request) (*httpexec.Result, error) {
			if waitErr := globalLimiter.Wait(ctx); waitErr != nil {
				return nil, waitErr
			}
			return unwrapped(ctx, req)
		}
		vars.globalLimiter = globalLimiter
	}

	// Hook wrap: on_request before exec, on_response after. When vars.Hooks is
	// nil (no plugins) the fast path leaves exec unchanged.
	if vars.Hooks != nil {
		unwrapped := exec
		hd := vars.Hooks
		exec = func(ctx context.Context, req *httpexec.Request) (*httpexec.Result, error) {
			inPayload := hooks.RequestPayload{
				Method:      req.Method,
				URL:         req.URL,
				Headers:     req.Headers,
				Body:        req.Body,
				QueryParams: req.QueryParams,
			}
			outPayload, hErr := hd.OnRequest(ctx, inPayload)
			if hErr != nil {
				// Plugin aborted the request; surface as request error.
				return nil, hErr
			}
			// Apply mutations from plugin (only non-zero fields override).
			if outPayload.Method != "" {
				req.Method = outPayload.Method
			}
			if outPayload.URL != "" {
				req.URL = outPayload.URL
			}
			if outPayload.Headers != nil {
				req.Headers = outPayload.Headers
			}
			if outPayload.Body != nil {
				req.Body = outPayload.Body
			}
			if outPayload.QueryParams != nil {
				req.QueryParams = outPayload.QueryParams
			}

			start := time.Now()
			resp, execErr := unwrapped(ctx, req)
			if execErr != nil {
				return resp, execErr
			}
			respPayload := hooks.ResponsePayload{
				StatusCode: resp.StatusCode,
				Headers:    flattenHeader(resp.Headers),
				Body:       resp.Body,
				DurationMs: time.Since(start).Milliseconds(),
			}
			// Annotations from plugins are informational; they are not
			// threaded into the result in this task (future work).
			_, _ = hd.OnResponse(ctx, respPayload)
			return resp, nil
		}
	}

	// Signer wrap: invoke the resolved signer between templating and exec.
	// Fast path mirrors the vars.Hooks != nil branch — when no request and
	// no collection sets `signing:`, exec is left unchanged (no allocation,
	// no overhead).
	// Signing sits OUTSIDE the hooks wrap: the signer fires first (mutates
	// the request), then the hooks dispatcher sees the already-signed request.
	if collectionUsesSigning(col) {
		signerReg := vars.Signer
		if signerReg == nil {
			signerReg = signer.NewWithBuiltins()
		}
		runtimeSensitiveForSigner := runtimeSensitive
		unwrapped := exec
		exec = func(ctx context.Context, req *httpexec.Request) (*httpexec.Result, error) {
			spec := signer.RequestSpecFromContext(ctx)
			if spec == nil {
				return unwrapped(ctx, req)
			}
			factory, lookupErr := signerReg.Lookup(spec.Type)
			if lookupErr != nil {
				return nil, lookupErr
			}
			s, buildErr := factory(spec.Params)
			if buildErr != nil {
				return nil, fmt.Errorf("building signer %q: %w", spec.Type, buildErr)
			}
			if signErr := s.Sign(ctx, req, runtimeSensitiveForSigner); signErr != nil {
				return nil, fmt.Errorf("signer %q: %w", spec.Type, signErr)
			}
			return unwrapped(ctx, req)
		}
	}

	// Build auth exec func and cache store (used for both profile execution and refresh-on-failure).
	authExec := vars.AuthExecuteFunc
	if authExec == nil {
		authExec = func(ctx context.Context, collPath string) (map[string]string, error) {
			return RunForExtraction(ctx, collPath, exec, vars)
		}
	}
	var cacheStore auth.CacheStore
	switch {
	case vars.CacheStore != nil:
		cacheStore = vars.CacheStore
	case vars.ProjectRoot != "":
		cacheStore = auth.NewFileCacheStore(vars.ProjectRoot)
	default:
		cacheStore = auth.NopCacheStore{}
	}

	// Auth profile execution (before all phases)
	var authSensitive *variable.SensitiveSet
	if len(vars.AuthProfiles) > 0 {
		profileResult, authErr := auth.ExecuteProfiles(ctx, vars.AuthProfiles, vars.ProjectRoot, authExec, cacheStore)
		if authErr != nil {
			return nil, emptySummary, fmt.Errorf("auth profile: %w", authErr)
		}
		for k, v := range profileResult.Variables {
			scope.Set(k, v)
		}
		authSensitive = profileResult.Sensitive
	}

	// M8-004 + M8-005: apply --only selection to main-phase items and prune the
	// setup phase to the transitive closure of {{variable}} references from the
	// selection. Teardown is never pruned. When Selection yields zero matches,
	// fail with a structured error before any HTTP runs. Capture the full main
	// items so the variable-cliff diagnostic can scan producers over the
	// unfiltered list.
	if len(vars.Selection) > 0 {
		vars.fullMainForCliff = col.Requests.Items
		vars.fullSetupForCliff = col.Setup.Items
		filtered, filterErr := filterMainItemsBySelection(col.Requests.Items, vars.Selection)
		if filterErr != nil {
			// No main items matched: total for run.end should reflect only
			// setup + teardown (zero main items would have run).
			emptySummary.Total = len(col.Setup.Items) + len(col.Teardown.Items)
			return nil, emptySummary, filterErr
		}

		// M8-005: prune setup to the transitive closure of {{variable}}
		// references from the selected main items. Items with an empty Extract:
		// map are included unconditionally (pure seeders the analyzer cannot
		// prove unneeded).
		prunedSetup := col.Setup.Items
		if len(col.Setup.Items) > 0 {
			preExecVars := buildPreExecVarSet(scope)
			combined := make([]parser.RequestItem, 0, len(col.Setup.Items)+len(filtered))
			combined = append(combined, col.Setup.Items...)
			combined = append(combined, filtered...)
			graph := parallel.Analyze(combined, preExecVars)
			if !graph.IsValid {
				// Fallback: analyzer rejected the combined DAG (e.g. cycles,
				// collisions, duplicate producers). Run full setup and emit a
				// one-line diagnostic on the Diagnostics writer.
				if vars.Diagnostics != nil {
					_, _ = fmt.Fprintf(vars.Diagnostics,
						"curlew: --only minimal-setup analysis failed (%s); running full setup\n",
						strings.Join(graph.Errors, "; "),
					)
				}
			} else {
				// Filtered main items come after setup items in the combined slice,
				// so their combined-graph indices are:
				//   len(col.Setup.Items) + i  for i in 0..len(filtered)-1
				starts := make([]int, 0, len(filtered))
				base := len(col.Setup.Items)
				for i := range filtered {
					starts = append(starts, base+i)
				}
				reachable := parallel.AncestorClosure(graph, starts)
				pruned := make([]parser.RequestItem, 0, len(col.Setup.Items))
				for i, item := range col.Setup.Items {
					// Keep if the analyzer placed it in the closure, OR if it has
					// no extract: block (pure seeder — always run conservatively).
					if reachable[i] || len(item.Extract) == 0 {
						pruned = append(pruned, item)
					}
				}
				prunedSetup = pruned
			}
		}

		// Shallow-copy col so we replace Setup.Items and Requests.Items without
		// mutating the caller's parsed collection. Teardown remains shared.
		shallow := *col
		shallow.Setup = parser.Section{Retry: col.Setup.Retry, Items: prunedSetup}
		shallow.Requests = parser.Section{Retry: col.Requests.Retry, Items: filtered}
		col = &shallow
		// Update total to reflect the pruned setup count + filtered main count.
		emptySummary.Total = len(prunedSetup) + len(filtered) + len(col.Teardown.Items)
	}

	results, summary, err := runPhases(ctx, col, exec, scope, vars, cacheStore, authExec)
	if summary != nil {
		if authSensitive != nil {
			summary.AuthSensitive = authSensitive
		}
		summary.RuntimeSensitive = runtimeSensitive
		summary.SharedSecretsResolved = sharedSecretsResolved
		summary.RunID = runID // M9-002: carry run_id to the caller
	}

	// Fire on_result once at run completion, regardless of outcome.
	if vars.Hooks != nil && summary != nil {
		_ = vars.Hooks.OnResult(ctx, buildResultPayload(results, summary))
	}

	return results, summary, err
}

// FilterMainItems returns the subset of items whose Name is in the selection set
// (exact, case-sensitive match). When no item matches, it returns
// ErrNoMatchingRequests with the available names listed in the error message.
// This is the exported form of filterMainItemsBySelection for callers outside
// the runner package (e.g. the cmd layer's --show-dependencies path, M8-004).
func FilterMainItems(items []parser.RequestItem, selection []string) ([]parser.RequestItem, error) {
	return filterMainItemsBySelection(items, selection)
}

// filterMainItemsBySelection returns the subset of items whose Name is in the
// selection set (exact, case-sensitive match). When no item matches, it returns
// ErrNoMatchingRequests with the available names listed in the error message.
func filterMainItemsBySelection(items []parser.RequestItem, selection []string) ([]parser.RequestItem, error) {
	selected := make(map[string]struct{}, len(selection))
	for _, n := range selection {
		selected[n] = struct{}{}
	}
	out := make([]parser.RequestItem, 0, len(selection))
	for _, it := range items {
		if _, ok := selected[it.Name]; ok {
			out = append(out, it)
		}
	}
	if len(out) == 0 {
		available := make([]string, 0, len(items))
		for _, it := range items {
			available = append(available, it.Name)
		}
		// Build the unmatched-names portion of the error message. When multiple
		// --only values are given and all fail to match, list all of them so the
		// user can see every mistyped name at once (not just the first one).
		var msg string
		if len(selection) == 1 {
			msg = fmt.Sprintf("no request named %q; available: %s", selection[0], quotedJoin(available))
		} else {
			msg = fmt.Sprintf("no requests named %s; available: %s", quotedJoin(selection), quotedJoin(available))
		}
		return nil, &apierrors.Structured{
			Category: apierrors.CategoryInput,
			Code:     "ONLY_NO_MATCH",
			Message:  msg,
			Hint:     "Pass --only <name> with a name that matches one of the available main requests. --only does not target setup or teardown items.",
			Inner:    ErrNoMatchingRequests,
		}
	}
	return out, nil
}

// quotedJoin joins names as a comma-separated list of double-quoted strings,
// e.g. `"A", "B", "C"`. Used in user-facing error messages.
func quotedJoin(names []string) string {
	if len(names) == 0 {
		return "(none)"
	}
	parts := make([]string, len(names))
	for i, n := range names {
		parts[i] = fmt.Sprintf("%q", n)
	}
	return strings.Join(parts, ", ")
}

// buildResultPayload converts run results and summary into the on_result hook payload.
func buildResultPayload(results []RequestResult, summary *Summary) hooks.ResultPayload {
	rows := make([]hooks.ResultTestRow, 0, len(results))
	for _, r := range results {
		status := "pass"
		switch {
		case r.Skipped:
			status = "skip"
		case r.Err != nil:
			status = "error"
		case r.AssertionResults != nil && !r.AssertionResults.Passed:
			status = "fail"
		}
		var dur int64
		if r.Result != nil {
			dur = r.Result.Duration.Milliseconds()
		}
		errStr := ""
		if r.Err != nil {
			errStr = r.Err.Error()
		}
		rows = append(rows, hooks.ResultTestRow{
			Name:       r.Name,
			Status:     status,
			DurationMs: dur,
			Error:      errStr,
		})
	}
	return hooks.ResultPayload{
		PassCount:  summary.Passed,
		FailCount:  summary.Failed,
		SkipCount:  summary.Skipped,
		DurationMs: summary.Duration.Milliseconds(),
		Tests:      rows,
	}
}

// flattenHeader converts http.Header (multi-value) to map[string]string by
// taking the first value for each header name, as plugins receive a flat map.
func flattenHeader(h http.Header) map[string]string {
	if len(h) == 0 {
		return nil
	}
	out := make(map[string]string, len(h))
	for k, vs := range h {
		if len(vs) > 0 {
			out[k] = vs[0]
		}
	}
	return out
}

// ensureAssertProgCache lazily initialises the cel: assertion compile cache.
func (v *VarSources) ensureAssertProgCache() map[string]apicel.Program {
	if v.assertProgCache == nil {
		v.assertProgCache = make(map[string]apicel.Program)
	}
	return v.assertProgCache
}

// collectionHasIf reports whether any request item in the collection has a
// non-empty if: field. Used to decide whether to initialise the CEL evaluator
// and fall back from parallel to sequential execution.
func collectionHasIf(col *parser.Collection) bool {
	for _, section := range [][]parser.RequestItem{col.Setup.Items, col.Requests.Items, col.Teardown.Items} {
		for _, item := range section {
			if item.If != "" {
				return true
			}
		}
	}
	return false
}

// collectionHasCelAssertions reports whether any request item in the
// collection has at least one cel: assertion entry.
func collectionHasCelAssertions(col *parser.Collection) bool {
	for _, section := range [][]parser.RequestItem{col.Setup.Items, col.Requests.Items, col.Teardown.Items} {
		for _, item := range section {
			if len(item.Assertions.CEL.Items) > 0 {
				return true
			}
		}
	}
	return false
}

// enrichInterpErr enriches an interpolation error by copying the RequestItem's
// SourceFile and SourceLine into the error chain's *Structured value, if it
// does not already carry location information. This makes the error diagnosable
// from the event stream without requiring callers to thread location data manually.
func enrichInterpErr(err error, item parser.RequestItem) error {
	var s *apierrors.Structured
	if !errors.As(err, &s) {
		return err
	}
	if s.FilePath == "" {
		s.FilePath = item.SourceFile
	}
	if s.Line == 0 {
		s.Line = item.SourceLine
	}
	return err
}

// undefinedVarRe captures the variable name from a structured error produced
// by variable.Scope.Interpolate. The producing format is owned by
// internal/variable/variable.go: fmt.Sprintf("undefined variable %q", name).
// TestRunner_UndefinedVarMessageFormatStable guards this contract.
var undefinedVarRe = regexp.MustCompile(`^undefined variable "([^"]+)"$`)

// enrichSelectionCliff looks for an undefined-variable error in err's chain and,
// when --only is active, checks whether the missing variable's producer is a
// filtered-out main item. When the producer is not found in fullMainItems, it
// additionally scans fullSetupItems — in that case the analyzer missed a
// {{variable}} reference, which is a likely curlew bug; the enriched error
// notes that. Returns err unchanged when Selection is empty, when the error is
// not an undefined-variable error, or when no producer is found.
func enrichSelectionCliff(err error, fullMainItems, fullSetupItems []parser.RequestItem, selection []string) error {
	if err == nil || len(selection) == 0 {
		return err
	}
	if len(fullMainItems) == 0 && len(fullSetupItems) == 0 {
		return err
	}
	var s *apierrors.Structured
	if !errors.As(err, &s) {
		return err
	}
	if !errors.Is(err, variable.ErrUndefinedVariable) {
		return err
	}
	m := undefinedVarRe.FindStringSubmatch(s.Message)
	if len(m) != 2 {
		return err
	}
	varName := m[1]

	// Build the set of selected names.
	sel := make(map[string]struct{}, len(selection))
	for _, n := range selection {
		sel[n] = struct{}{}
	}

	// First, scan full main items (existing behaviour).
	for _, it := range fullMainItems {
		produced, _ := parallel.ExtractProducedVars(it.Extract)
		if !produced[varName] {
			continue
		}
		if _, inSelection := sel[it.Name]; inSelection {
			// Producer IS selected — different failure mode; leave err alone.
			return err
		}
		// Producer is filtered out by --only. Return a new enriched error that
		// wraps the original (preserves the full error chain for errors.Is/As).
		msg := fmt.Sprintf(
			"variable {{%s}} is not defined; normally extracted from %q which was not included by --only",
			varName, it.Name,
		)
		hint := fmt.Sprintf(
			"Add the producer to --only (e.g. --only %q --only %q) or pass the variable explicitly via --var %s=<value>.",
			it.Name, selection[0], varName,
		)
		return &apierrors.Structured{
			Category: s.Category,
			FilePath: s.FilePath,
			Line:     s.Line,
			Code:     s.Code,
			Message:  msg,
			Hint:     hint,
			Inner:    err,
		}
	}

	// M8-005 safety net: scan setup items. A producer found here indicates
	// the minimal-setup analyser missed a {{variable}} reference — likely a
	// bug (the scanner should have kept the setup item in the closure).
	for _, it := range fullSetupItems {
		produced, _ := parallel.ExtractProducedVars(it.Extract)
		if !produced[varName] {
			continue
		}
		msg := fmt.Sprintf(
			"variable {{%s}} is not defined; normally extracted from setup request %q which was pruned by --only (the minimal-setup analyser did not detect a reference; this is likely an curlew bug — please report)",
			varName, it.Name,
		)
		hint := fmt.Sprintf(
			"Workaround: pass --var %s=<value> or disable minimal-setup pruning by omitting --only.",
			varName,
		)
		return &apierrors.Structured{
			Category: s.Category,
			FilePath: s.FilePath,
			Line:     s.Line,
			Code:     s.Code,
			Message:  msg,
			Hint:     hint,
			Inner:    err,
		}
	}
	return err
}

// RunForExtraction executes a collection to extract variables, used by auth profile execution.
// Requests in the collection do NOT count toward the caller's guard rail counter.
// Auth profiles are not forwarded to prevent recursive execution.
func RunForExtraction(ctx context.Context, collectionPath string, exec ExecuteFunc, vars VarSources) (map[string]string, error) {
	col, err := parser.ParseFile(collectionPath)
	if err != nil {
		return nil, fmt.Errorf("auth collection: %w", err)
	}

	// Prevent recursion and isolate auth collection execution
	innerVars := vars
	innerVars.AuthProfiles = nil
	innerVars.ProjectRoot = ""
	innerVars.AuthExecuteFunc = nil

	// Initialise the per-run request ID counter for this extraction sub-run.
	// Required because executePhase now always calls nextRequestID. M9-002.
	var idcExtract atomic.Int64
	innerVars.reqIDCounter = &idcExtract

	scope, _, scopeErr := buildScope(ctx, col, innerVars)
	if scopeErr != nil {
		return nil, scopeErr
	}

	before := scope.Resolved()

	results, _, runErr := runPhases(ctx, col, exec, scope, innerVars, auth.NopCacheStore{}, nil)
	if runErr != nil {
		return nil, runErr
	}

	// Check for any request failures — auth collection must succeed fully
	for _, r := range results {
		if r.Err != nil {
			return nil, fmt.Errorf("auth request %q: %w", r.Name, r.Err)
		}
		if r.AssertionResults != nil && !r.AssertionResults.Passed {
			return nil, fmt.Errorf("auth request %q: assertions failed", r.Name)
		}
	}

	after := scope.Resolved()
	extracted := make(map[string]string)
	for k, v := range after {
		if old, ok := before[k]; !ok || old != v {
			extracted[k] = v
		}
	}
	return extracted, nil
}

// buildScope merges all variable sources and returns a fully resolved scope
// and the number of shared secrets resolved from a team template (0 when none).
// Runs from_command and vault resolution as needed.
func buildScope(ctx context.Context, col *parser.Collection, vars VarSources) (*variable.Scope, int, error) {
	// Build merged variable map: project (2) < env (3) < .env (4) < collection (7)
	merged := make(map[string]string, len(vars.Project)+len(vars.EnvFile)+len(vars.DotEnv)+len(col.Variables.Values))
	for k, v := range vars.Project {
		merged[k] = v
	}
	for k, v := range vars.EnvFile {
		merged[k] = v // env file overrides project
	}
	for k, v := range vars.DotEnv {
		merged[k] = v // .env overrides environment file
	}

	// Precedence 5: from_command variables
	if len(col.Variables.Commands) > 0 {
		// Cache is scoped to this buildScope() invocation. Each variable name is iterated
		// once per run, so the cache does not serve hits within a single run. The
		// cache field on CommandVar is parsed and validated, ready for future
		// multi-run scenarios (e.g., watch mode or repeated execution).
		cache := variable.NewCommandCache()
		for name, cmd := range col.Variables.Commands {
			// Higher precedence sources skip command execution
			if _, ok := vars.CLI[name]; ok {
				continue
			}
			if _, ok := vars.EnvVar[name]; ok {
				continue
			}
			// Collection values (precedence 7) override from_command (precedence 5)
			if _, ok := col.Variables.Values[name]; ok {
				continue
			}

			if cmd.Cache > 0 {
				if val, ok := cache.Get(name); ok {
					merged[name] = val
					continue
				}
			}

			val, err := variable.ExecuteCommand(ctx, cmd.Command)
			if err != nil {
				return nil, 0, err
			}

			if cmd.Cache > 0 {
				cache.Set(name, val, cmd.Cache)
			}
			merged[name] = val
		}
	}

	// Resolve vault secrets (precedence 6)
	if vars.Secrets != nil {
		vaultExec := vars.VaultExecutor
		if vaultExec == nil {
			vaultExec = variable.ExecuteCommand
		}
		result, err := vault.Resolve(ctx, vars.Secrets, vaultExec)
		if err != nil {
			return nil, 0, err
		}
		for k, v := range result.Variables {
			merged[k] = v
		}
	}

	for k, v := range col.Variables.Values {
		merged[k] = v // collection overrides .env and from_command
	}

	// Variable resolution (before any HTTP execution)
	scope := variable.NewScope(merged)
	if err := scope.Resolve(); err != nil {
		return nil, 0, err
	}

	// --env-var imports (precedence 9)
	for k, v := range vars.EnvVar {
		scope.Set(k, v)
	}

	// CLI --var overrides (precedence 10 — highest)
	for k, v := range vars.CLI {
		scope.Set(k, v)
	}

	// Wire dynamic function registry (precedence 1 — lowest; evaluated at interpolation time)
	locale, localeErr := resolveLocale(vars)
	if localeErr != nil {
		return nil, 0, localeErr // ERR_LOCALE_UNKNOWN surfaces as structured/non-zero exit
	}
	var localeOpts []variable.Option
	if locale != "" {
		localeOpts = append(localeOpts, variable.WithLocale(locale))
		// Only emit the fallback warning in verbose mode (Behaviour 5). The
		// exec subcommand gates on opts.Verbosity >= VerbosityVerbose; the run
		// subcommand signals verbose intent via LocaleVerbose so both paths are
		// consistent. M20-001.
		if vars.Diagnostics != nil && vars.LocaleVerbose {
			localeOpts = append(localeOpts, variable.WithLocaleWarning(func(msg string) {
				_, _ = fmt.Fprintln(vars.Diagnostics, "curlew: "+msg)
			}))
		}
	}
	registry := variable.NewRegistry(vars.Seed, localeOpts...)
	scope = scope.WithDynamic(registry)

	// Shared vault template (Layer 4).
	var sharedSecretsResolved int
	if vars.TeamTemplate != nil {
		// Scan the collection for {{secrets.X}} tokens.
		needed := collectSecretReferences(col)
		if len(needed) > 0 && vars.TeamEnv == "" {
			return nil, 0, &apierrors.Structured{
				Category: apierrors.CategoryConfig,
				Message:  "shared template requires --env <name>",
				Hint:     "Add --env <envname> to your curlew run invocation",
				Inner:    teamtemplate.ErrEnvFlagRequired,
			}
		}

		if vars.TeamEnv != "" {
			// Build resolver with the chosen provider factory (stub vs real).
			makeProvider := realTeamProviderFactory(vars.VaultExecutor)
			if vars.TeamStub {
				makeProvider = stubTeamProviderFactory()
			}
			resolver, resolverErr := teamtemplate.NewSecretsResolver(vars.TeamTemplate, vars.TeamEnv, makeProvider)
			if resolverErr != nil {
				return nil, 0, resolverErr
			}

			// Validate that every referenced alias exists in the active env.
			for _, alias := range needed {
				if !resolver.HasAlias(alias) {
					return nil, 0, fmt.Errorf("%w %q for environment %q",
						teamtemplate.ErrUnknownAlias, alias, vars.TeamEnv)
				}
			}

			resolved, resolveErr := resolver.Resolve(ctx)
			if resolveErr != nil {
				return nil, 0, resolveErr
			}

			scope = scope.WithSecrets(resolved)
			sharedSecretsResolved = resolver.Count()
		}
	}

	return scope, sharedSecretsResolved, nil
}

// collectSecretReferences walks all string fields of a collection and returns
// the sorted unique set of {{secrets.ALIAS}} alias names referenced.
func collectSecretReferences(col *parser.Collection) []string {
	seen := make(map[string]struct{})
	visit := func(s string) {
		for _, alias := range variable.SecretReferences(s) {
			seen[alias] = struct{}{}
		}
	}
	visitItems := func(items []parser.RequestItem) {
		for i := range items {
			item := &items[i]
			visit(item.Request.URL)
			visit(item.Request.Method)
			for _, v := range item.Request.Headers {
				visit(v)
			}
			for _, v := range item.Request.QueryParams {
				visit(v)
			}
			visitBody(item.Request.Body, visit)
			for _, ba := range item.Assertions.Body.Items {
				visit(ba.Path)
				if s, ok := ba.Value.(string); ok {
					visit(s)
				}
			}
			for _, ha := range item.Assertions.Headers.Items {
				if s, ok := ha.Value.(string); ok {
					visit(s)
				}
			}
		}
	}
	visitItems(col.Setup.Items)
	visitItems(col.Requests.Items)
	visitItems(col.Teardown.Items)

	if len(seen) == 0 {
		return nil
	}
	out := make([]string, 0, len(seen))
	for alias := range seen {
		out = append(out, alias)
	}
	sort.Strings(out)
	return out
}

// visitBody recursively walks a body value and calls visit on every string.
func visitBody(body any, visit func(string)) {
	switch v := body.(type) {
	case string:
		visit(v)
	case map[string]any:
		for _, val := range v {
			visitBody(val, visit)
		}
	case map[string]string:
		for _, val := range v {
			visit(val)
		}
	case []any:
		for _, val := range v {
			visitBody(val, visit)
		}
	}
}

// stubTeamProviderFactory returns a provider factory that creates a StubProvider.
func stubTeamProviderFactory() func(*teamtemplate.ResolvedEnv) (vault.Provider, error) {
	return func(env *teamtemplate.ResolvedEnv) (vault.Provider, error) {
		return teamtemplate.NewStubProvider(env.Name, env.Provider), nil
	}
}

// realTeamProviderFactory returns a provider factory that dispatches on
// env.Provider and constructs the appropriate real vault provider.
func realTeamProviderFactory(exec vault.CommandExecutor) func(*teamtemplate.ResolvedEnv) (vault.Provider, error) {
	if exec == nil {
		exec = variable.ExecuteCommand
	}
	return func(env *teamtemplate.ResolvedEnv) (vault.Provider, error) {
		switch env.Provider {
		case vault.ProviderAWS:
			return vault.NewAWSProvider(env.Region, exec), nil
		case vault.ProviderAzure:
			return vault.NewAzureProvider(env.VaultName, exec), nil
		default:
			return nil, fmt.Errorf("%w: %q", teamtemplate.ErrUnknownProvider, env.Provider)
		}
	}
}

// runPhases executes setup, main, and teardown phases on the provided scope.
func runPhases(ctx context.Context, col *parser.Collection, exec ExecuteFunc, scope *variable.Scope, vars VarSources, cacheStore auth.CacheStore, authExec auth.ExecuteFunc) ([]RequestResult, *Summary, error) {
	total := len(col.Setup.Items) + len(col.Requests.Items) + len(col.Teardown.Items)
	summary := &Summary{Total: total}
	all := make([]RequestResult, 0, total)
	start := time.Now()
	counter := 0

	// M19-001/M19-004: If any item uses `if:` or `cel:` assertions,
	// pre-initialise the CEL evaluator and compile-caches once so they are
	// shared across all phases (setup→main→teardown). This avoids repeated
	// lazy-init inside executePhase where vars is copied by value each call.
	if collectionHasIf(col) || collectionHasCelAssertions(col) {
		ev, evErr := vars.ensureCelEvaluator()
		if evErr != nil {
			return nil, &Summary{Total: total}, fmt.Errorf("cel evaluator: %w", evErr)
		}
		vars.CelEvaluator = ev
		vars.ifProgCache = vars.ensureIfProgCache()
		vars.assertProgCache = vars.ensureAssertProgCache()

		// Parallel fallback: both `if:` and `cel:` assertions require sequential
		// execution because `previous` is updated in sequential order and CEL
		// evaluation may depend on it. When Parallel=true, fall back to sequential
		// main phase and emit a diagnostics line.
		if vars.Parallel {
			if vars.Diagnostics != nil {
				if collectionHasIf(col) {
					_, _ = fmt.Fprintln(vars.Diagnostics, "curlew: if: gate active; falling back to sequential main phase")
				} else {
					_, _ = fmt.Fprintln(vars.Diagnostics, "curlew: cel: gate active; falling back to sequential main phase")
				}
			}
			vars.Parallel = false
		}
	}

	// Phase 1: Setup
	setupFailed := false
	if len(col.Setup.Items) > 0 {
		setupResults, reqFailed, fatalErr := executePhase(ctx, col, col.Setup.Items, scope, exec, vars, cacheStore, authExec, PhaseSetup, true, false, &counter, MaxRequests, col.Retry, col.Setup.Retry, vars.GlobalRetry)
		all = append(all, setupResults...)
		if fatalErr != nil {
			summary.Duration = time.Since(start)
			computeSummary(all, summary)
			return nil, summary, fatalErr
		}
		setupFailed = reqFailed
	}

	// Phase 2: Main (skipped entirely if a required setup item failed)
	if setupFailed {
		for _, item := range col.Requests.Items {
			all = append(all, RequestResult{Name: item.Name, Phase: PhaseMain, Method: item.Request.Method, URL: item.Request.URL, Skipped: true, WaveIndex: -1, SourceFile: item.SourceFile, SourceLine: item.SourceLine, RequestSlug: item.Slug})
		}
	} else if vars.Parallel && len(col.Requests.Items) > 0 {
		// Parallel execution: analyze dependencies and execute in waves
		mainResults, parallelCounter, impact, waveCount, maxPar, waveDurs, fatalErr := executeParallelMain(ctx, col, scope, exec, vars, &counter)
		all = append(all, mainResults...)
		counter += parallelCounter
		summary.Impact = impact
		summary.IsParallel = true
		summary.WaveCount = waveCount
		summary.MaxParallelism = maxPar
		summary.WaveDurations = waveDurs
		if fatalErr != nil {
			summary.Duration = time.Since(start)
			computeSummary(all, summary)
			return nil, summary, fatalErr
		}
	} else {
		mainResults, _, fatalErr := executePhase(ctx, col, col.Requests.Items, scope, exec, vars, cacheStore, authExec, PhaseMain, false, col.Options.StopOnFailure, &counter, MaxRequests, col.Retry, col.Requests.Retry, vars.GlobalRetry)
		all = append(all, mainResults...)
		if fatalErr != nil {
			summary.Duration = time.Since(start)
			computeSummary(all, summary)
			return nil, summary, fatalErr
		}
	}

	// Phase 3: Teardown — ALWAYS runs regardless of setup or main outcome
	if len(col.Teardown.Items) > 0 {
		tdResults, _, fatalErr := executePhase(ctx, col, col.Teardown.Items, scope, exec, vars, cacheStore, authExec, PhaseTeardown, false, false, &counter, MaxRequests, col.Retry, col.Teardown.Retry, vars.GlobalRetry)
		all = append(all, tdResults...)
		if fatalErr != nil {
			summary.Duration = time.Since(start)
			computeSummary(all, summary)
			return nil, summary, fatalErr
		}
	}

	summary.Duration = time.Since(start)
	summary.RequestsExecuted = counter
	if counter >= MaxRequests && total > counter {
		summary.LimitExceeded = true
	}
	computeSummary(all, summary)
	return all, summary, nil
}

// buildPreExecVarSet returns the name set of all variables already resolved
// on scope. Used to seed parallel.Analyze so that consumer->producer edges
// only form for variables that are not already available at phase start.
func buildPreExecVarSet(scope *variable.Scope) map[string]bool {
	resolved := scope.Resolved()
	out := make(map[string]bool, len(resolved))
	for k := range resolved {
		out[k] = true
	}
	return out
}

// executeParallelMain runs the main phase requests using parallel wave-based execution.
// Returns the results tagged with PhaseMain, the number of HTTP requests executed, impact entries,
// wave count, max parallelism, wave durations, and any error.
func executeParallelMain(ctx context.Context, col *parser.Collection, scope *variable.Scope, exec ExecuteFunc, vars VarSources, counter *int) ([]RequestResult, int, []parallel.ImpactEntry, int, int, []time.Duration, error) {
	// Build pre-execution variable set from the scope's resolved vars
	preExecVars := buildPreExecVarSet(scope)

	graph := parallel.Analyze(col.Requests.Items, preExecVars)
	if !graph.IsValid {
		return nil, 0, nil, 0, 0, nil, fmt.Errorf("parallel analysis: %s", strings.Join(graph.Errors, "; "))
	}

	remaining := MaxRequests - *counter
	if remaining < 0 {
		remaining = 0
	}

	// Build a DataDrivenFunc that delegates data-driven items to executeDataDriven,
	// treating each data-driven request as an atomic unit within the parallel wave.
	ddFunc := buildDataDrivenFunc(exec, vars, col)

	// Build a WebSocketFunc that runs WebSocket items concurrently in a wave,
	// mirroring the sequential WebSocket path in executePhase.
	wsFunc := buildWebSocketFunc(vars)

	// Build a RetryConfigFunc that resolves retry config per request item.
	retryConfigFunc := func(item parser.RequestItem) retry.Config {
		return resolveRetryConfig(vars.GlobalRetry, col.Retry, col.Requests.Retry, item.Retry)
	}

	// Wire EventSink into the parallel executor via the adapter, sharing the
	// same atomic counter as the sequential phases so IDs are globally monotonic.
	var pSink parallel.EventSink
	if vars.OnEvent != nil {
		pSink = &parallelSinkAdapter{inner: vars.OnEvent}
	}

	// Build PreExec to attach signing spec on ctx for each parallel item.
	// When no request uses signing, PreExec is nil (fast path, no overhead).
	var pPreExec func(context.Context, parser.RequestItem) context.Context
	if collectionUsesSigning(col) {
		capturedCol := col
		capturedScope := scope
		pPreExec = func(ctx context.Context, item parser.RequestItem) context.Context {
			if spec := resolveSigningSpec(item, capturedCol); spec != nil {
				ctx = signer.WithRequestSpec(ctx, spec)
				ctx = signer.WithScope(ctx, capturedScope)
				return ctx
			}
			return ctx
		}
	}

	execResult, err := parallel.ExecuteWaves(ctx, parallel.Config{
		Graph:           graph,
		Items:           col.Requests.Items,
		Scope:           scope,
		ExecFunc:        exec,
		MaxRequests:     remaining,
		DataDrivenFunc:  ddFunc,
		WebSocketFunc:   wsFunc,
		RetryConfigFunc: retryConfigFunc,
		EventSink:       pSink,
		NextRequestID:   vars.nextRequestID,
		PreExec:         pPreExec,
	})
	if err != nil {
		return nil, 0, nil, 0, 0, nil, fmt.Errorf("parallel execution: %w", err)
	}

	// Convert parallel outcomes to runner results
	var results []RequestResult
	executed := 0
	for _, wave := range execResult.Waves {
		for _, outcome := range wave.Outcomes {
			rr := RequestResult{
				Name:             outcome.Name,
				Phase:            PhaseMain,
				RequestID:        outcome.RequestID,
				RequestSlug:      outcome.RequestSlug,
				Method:           outcome.Method,
				URL:              outcome.URL,
				RequestHeaders:   outcome.RequestHeaders,
				RequestBody:      outcome.RequestBody,
				Result:           outcome.Result,
				Err:              outcome.Err,
				Skipped:          outcome.Skipped,
				SkipReason:       outcome.SkipReason,
				AssertionResults: outcome.AssertionResults,
				RetryCount:       outcome.RetryCount,
				RetryWarnings:    outcome.RetryWarnings,
				AttemptDetails:   outcome.AttemptDetails,
				WaveIndex:        outcome.WaveIndex,
				Warnings:         outcome.Warnings,
				SourceFile:       outcome.SourceFile,
				SourceLine:       outcome.SourceLine,
			}
			results = append(results, rr)
			if !outcome.Skipped {
				executed++
			}
		}
	}

	// Compute wave metadata
	waveCount := len(execResult.Waves)
	maxPar := 0
	waveDurations := make([]time.Duration, waveCount)
	for i, wave := range execResult.Waves {
		size := len(wave.Outcomes)
		if size > maxPar {
			maxPar = size
		}
		waveDurations[i] = wave.Duration
	}

	return results, executed, execResult.Impact, waveCount, maxPar, waveDurations, nil
}

// buildDataDrivenFunc creates a parallel.DataDrivenFunc that delegates data-driven
// items to the runner's data-driven execution path, treating each item as an atomic unit.
func buildDataDrivenFunc(exec ExecuteFunc, vars VarSources, col *parser.Collection) parallel.DataDrivenFunc {
	return func(ctx context.Context, item parser.RequestItem, scope *variable.Scope, waveIdx int) (*parallel.DataDrivenOutcome, error) {
		// Execute data-driven iterations via the sequential/parallel data-driven path
		dummyCounter := 0
		ddResults, _, ddErr := executeDataDriven(ctx, col, item, scope, exec, vars, PhaseMain, false, false, &dummyCounter, MaxRequests, col.Retry, col.Requests.Retry, vars.GlobalRetry)
		if ddErr != nil {
			return nil, ddErr
		}

		// Convert runner results to parallel outcomes
		out := &parallel.DataDrivenOutcome{
			Extracted: make(map[string]string),
		}
		for _, rr := range ddResults {
			out.Outcomes = append(out.Outcomes, parallel.RequestOutcome{
				Name:             rr.Name,
				RequestID:        rr.RequestID,
				RequestSlug:      rr.RequestSlug,
				Method:           rr.Method,
				URL:              rr.URL,
				RequestHeaders:   rr.RequestHeaders,
				RequestBody:      rr.RequestBody,
				Result:           rr.Result,
				Err:              rr.Err,
				Skipped:          rr.Skipped,
				SkipReason:       rr.SkipReason,
				AssertionResults: rr.AssertionResults,
				RetryCount:       rr.RetryCount,
				RetryWarnings:    rr.RetryWarnings,
				AttemptDetails:   rr.AttemptDetails,
				WaveIndex:        waveIdx,
				Warnings:         rr.Warnings,
				SourceFile:       rr.SourceFile,
				SourceLine:       rr.SourceLine,
			})
		}

		// Collect accumulated extracted variables from the scope.
		// executeDataDriven already sets accumulated vars into the scope,
		// so we capture them here for the parallel executor to propagate.
		resolved := scope.Resolved()
		for k, v := range resolved {
			out.Extracted[k] = v
		}

		return out, nil
	}
}

// buildWebSocketFunc creates a parallel.WebSocketFunc that runs a WebSocket
// request item concurrently within a wave, mirroring the sequential WebSocket
// path in executePhase. Interpolation and result synthesis are identical to
// the sequential path.
func buildWebSocketFunc(vars VarSources) parallel.WebSocketFunc {
	return func(ctx context.Context, item parser.RequestItem, scope *variable.Scope, waveIdx int) parallel.RequestOutcome {
		req := item.Request
		interpolated, interpErr := requtil.InterpolateRequest(scope, &req)
		if interpErr != nil {
			interpErr = enrichInterpErr(interpErr, item)
			// M8-004: WebSocket requests can only be in main phase; enrich cliff.
			interpErr = enrichSelectionCliff(interpErr, vars.fullMainForCliff, vars.fullSetupForCliff, vars.Selection)
			return parallel.RequestOutcome{
				Name:        item.Name,
				RequestSlug: item.Slug,
				Method:      req.Method,
				URL:         req.URL,
				WaveIndex:   waveIdx,
				Err:         fmt.Errorf("request %q: %w", item.Name, interpErr),
				SourceFile:  item.SourceFile,
				SourceLine:  item.SourceLine,
			}
		}
		req = *interpolated

		// Apply global rate limit before the WebSocket upgrade.
		if vars.globalLimiter != nil {
			if waitErr := vars.globalLimiter.Wait(ctx); waitErr != nil {
				return parallel.RequestOutcome{
					Name:        item.Name,
					RequestSlug: item.Slug,
					Method:      req.Method,
					URL:         req.URL,
					WaveIndex:   waveIdx,
					Skipped:     true,
					SkipReason:  "context cancelled",
					SourceFile:  item.SourceFile,
					SourceLine:  item.SourceLine,
				}
			}
		}

		dialer := vars.WebSocketDialer
		if dialer == nil {
			dialer = websocket.DefaultDialer
		}
		wsRes := websocket.Execute(ctx, &req, scope, dialer)

		return buildWSOutcome(item, req, wsRes, waveIdx)
	}
}

// buildWSOutcome synthesises a parallel.RequestOutcome from a WebSocket execution
// result, mirroring the shape produced by the sequential WebSocket path in
// executePhase so that downstream output formatters receive consistent data.
func buildWSOutcome(item parser.RequestItem, req parser.Request, wsRes *websocket.Result, waveIdx int) parallel.RequestOutcome {
	synth := &httpexec.Result{
		StatusCode: 101,
		Duration:   wsRes.Duration,
	}
	outcome := parallel.RequestOutcome{
		Name:           item.Name,
		RequestSlug:    item.Slug,
		Method:         req.Method,
		URL:            req.URL,
		RequestHeaders: req.Headers,
		RequestBody:    req.Body,
		Result:         synth,
		WaveIndex:      waveIdx,
		Warnings:       wsRes.Warnings,
		SourceFile:     item.SourceFile,
		SourceLine:     item.SourceLine,
	}
	if wsRes.Passed {
		outcome.AssertionResults = &assertion.Results{Passed: true}
	} else {
		outcome.Err = wsRes.Err
		actual := "unknown failure"
		if wsRes.Err != nil {
			actual = wsRes.Err.Error()
		}
		outcome.AssertionResults = &assertion.Results{
			Passed: false,
			Items: []assertion.Result{{
				Type:     "websocket",
				Expected: "all steps pass",
				Actual:   actual,
				Passed:   false,
			}},
		}
	}
	return outcome
}

// executePhase runs items sequentially, tagging each result with the given phase.
// checkRequired: if true, a failed item with Required==true causes all subsequent items to be skipped.
// stopOnFailure: if true, stop on the first failure (regardless of required flag).
// counter tracks total HTTP requests executed across all phases; maxRequests is the guard rail limit.
// Returns results, whether a required item failed, and any fatal error (e.g. interpolation failure).
func executePhase(
	ctx context.Context,
	col *parser.Collection,
	items []parser.RequestItem,
	scope *variable.Scope,
	exec ExecuteFunc,
	vars VarSources,
	cacheStore auth.CacheStore,
	authExec auth.ExecuteFunc,
	phase Phase,
	checkRequired bool,
	stopOnFailure bool,
	counter *int,
	maxRequests int,
	collectionRetry *retry.FullConfig,
	sectionRetry *retry.FullConfig,
	globalRetry *retry.FullConfig,
) ([]RequestResult, bool, error) {
	results := make([]RequestResult, 0, len(items))
	stopped := false
	requiredFailed := false

	// M19-001: previous tracks the last non-skipped response in this phase for
	// CEL if: expression evaluation. nil until at least one request succeeds.
	var previous *apicel.Response

	// M19-001: skippedSet tracks items that were skipped in this phase so
	// depends_on: skip-propagation works without needing a second pass.
	skippedSet := make(map[string]struct{})

	for _, item := range items {
		if stopped {
			rr := RequestResult{Name: item.Name, Phase: phase, Skipped: true, WaveIndex: -1, SourceFile: item.SourceFile, SourceLine: item.SourceLine, RequestSlug: item.Slug}
			results = append(results, rr)
			skippedSet[item.Name] = struct{}{}
			if vars.OnEvent != nil {
				emitRequestEnd(vars.OnEvent, "", item.Slug, rr, -1)
			}
			continue
		}

		// Guard rail: stop if request limit reached
		if *counter >= maxRequests {
			rr := RequestResult{Name: item.Name, Phase: phase, Skipped: true, WaveIndex: -1, SourceFile: item.SourceFile, SourceLine: item.SourceLine, RequestSlug: item.Slug}
			results = append(results, rr)
			skippedSet[item.Name] = struct{}{}
			stopped = true
			if vars.OnEvent != nil {
				emitRequestEnd(vars.OnEvent, "", item.Slug, rr, -1)
			}
			continue
		}

		if err := ctx.Err(); err != nil {
			rr := RequestResult{Name: item.Name, Phase: phase, Skipped: true, WaveIndex: -1, SourceFile: item.SourceFile, SourceLine: item.SourceLine, RequestSlug: item.Slug}
			results = append(results, rr)
			skippedSet[item.Name] = struct{}{}
			stopped = true
			if vars.OnEvent != nil {
				emitRequestEnd(vars.OnEvent, "", item.Slug, rr, -1)
			}
			continue
		}

		// M19-001: skip-on-skipped-parent. Walk depends_on: names in declaration
		// order. Only this phase's skippedSet is consulted. Cross-phase propagation
		// is not supported (teardown always runs regardless of prior phase outcomes).
		if len(item.DependsOn) > 0 {
			skippedParent := ""
			for _, dep := range item.DependsOn {
				if _, parentSkipped := skippedSet[dep]; parentSkipped {
					skippedParent = dep
					break
				}
			}
			if skippedParent != "" {
				reason := fmt.Sprintf("parent skipped: %s", skippedParent)
				rr := RequestResult{
					Name: item.Name, Phase: phase,
					Skipped: true, SkipReason: reason,
					WaveIndex:   -1,
					SourceFile:  item.SourceFile,
					SourceLine:  item.SourceLine,
					RequestSlug: item.Slug,
				}
				results = append(results, rr)
				skippedSet[item.Name] = struct{}{}
				if vars.OnEvent != nil {
					emitRequestEnd(vars.OnEvent, "", item.Slug, rr, -1)
				}
				continue
			}
		}

		// M19-001: if: gate. Evaluated BEFORE templating so that
		// {{from_command:}}, vault fetches, and faker seeds are never invoked
		// for skipped items. Uses the CEL evaluator pre-initialised in runPhases.
		// vars.CelEvaluator is always non-nil here: runPhases guarantees it is
		// set whenever collectionHasIf(col) returns true, which is the only path
		// that sets item.If non-empty. The guard is therefore item.If != "" only.
		if item.If != "" {
			prog, compileErr := compileIfProgram(vars.ifProgCache, vars.CelEvaluator, item.If)
			if compileErr != nil {
				var cerr *apicel.CelError
				if errors.As(compileErr, &cerr) {
					cerr.FieldPath = fmt.Sprintf("%s[%s].if", phase, item.Name)
				}
				return results, requiredFailed, fmt.Errorf("request %q: if: %w", item.Name, compileErr)
			}

			// Build the activation. Vars are the resolved scope values as map[string]any.
			resolvedVars := scope.Resolved()
			varsAny := make(map[string]any, len(resolvedVars))
			for k, v := range resolvedVars {
				varsAny[k] = v
			}
			// Build env map from --env-var OS imports so that if: expressions
			// can reference env.<name>. vars.EnvVar is already map[string]string
			// and matches the StandardActivation.Env type exactly.
			var envMap map[string]string
			if len(vars.EnvVar) > 0 {
				envMap = vars.EnvVar
			}
			act := apicel.StandardActivation{
				Previous: previous,
				Vars:     varsAny,
				Env:      envMap,
			}

			// Build sensitive name set from collection + item sensitive vars.
			sensNames := make(map[string]struct{})
			for _, name := range col.Variables.Sensitive.Names() {
				sensNames[name] = struct{}{}
			}
			for _, name := range item.Variables.Sensitive.Names() {
				sensNames[name] = struct{}{}
			}

			// Build runtimeSensitive observer from scope.
			rts := scope.RuntimeSensitiveSet()

			out, evalErr := prog.Eval(act, apicel.EvalOptions{
				SensitiveNames: sensNames,
				SensitiveObserver: func(_, value string) {
					if rts != nil {
						rts.AddValue(value)
					}
				},
			})
			if evalErr != nil {
				return results, requiredFailed, fmt.Errorf("request %q: if: %w", item.Name, evalErr)
			}

			// A falsy result (false bool or anything that is not true) skips the item.
			gateOpen := false
			if b, ok := out.(bool); ok {
				gateOpen = b
			}
			if !gateOpen {
				rr := RequestResult{
					Name: item.Name, Phase: phase,
					Skipped: true, SkipReason: "if: false",
					WaveIndex:   -1,
					SourceFile:  item.SourceFile,
					SourceLine:  item.SourceLine,
					RequestSlug: item.Slug,
				}
				results = append(results, rr)
				skippedSet[item.Name] = struct{}{}
				if vars.OnEvent != nil {
					emitRequestEnd(vars.OnEvent, "", item.Slug, rr, -1)
				}
				continue
			}
		}

		// Data-driven execution: run once per row from data source
		if item.DataDriven != nil {
			ddResults, ddFailed, ddErr := executeDataDriven(ctx, col, item, scope, exec, vars, phase, checkRequired, stopOnFailure, counter, maxRequests, collectionRetry, sectionRetry, globalRetry)
			results = append(results, ddResults...)
			if ddErr != nil {
				return results, requiredFailed, ddErr
			}
			if ddFailed {
				requiredFailed = true
				stopped = true
			}
			continue
		}

		// Per-request variable scoping (precedence 8)
		reqScope := scope
		if len(item.Variables.Values) > 0 {
			var oErr error
			reqScope, oErr = scope.WithOverrides(item.Variables.Values)
			if oErr != nil {
				return results, requiredFailed, fmt.Errorf("request %q variables: %w", item.Name, oErr)
			}
			// Re-apply higher-precedence overrides (9, 10) on top of request scope
			for k, v := range vars.EnvVar {
				reqScope.Set(k, v)
			}
			for k, v := range vars.CLI {
				reqScope.Set(k, v)
			}
		}

		// Interpolate request fields (on a copy, not mutating parsed collection)
		req := item.Request
		interpolated, interpErr := requtil.InterpolateRequest(reqScope, &req)
		if interpErr != nil {
			interpErr = enrichInterpErr(interpErr, item)
			// M8-004: enrich with variable-cliff diagnostic for main-phase items.
			if phase == PhaseMain {
				interpErr = enrichSelectionCliff(interpErr, vars.fullMainForCliff, vars.fullSetupForCliff, vars.Selection)
			}
			return results, requiredFailed, fmt.Errorf("request %q: %w", item.Name, interpErr)
		}
		req = *interpolated

		// GraphQL protocol handling: request transformation
		if req.Protocol == "graphql" {
			if req.GraphQL != nil {
				gqlReq, buildErr := graphql.BuildRequest(&req)
				if buildErr != nil {
					return results, requiredFailed, fmt.Errorf("request %q: %w", item.Name, buildErr)
				}
				req = *gqlReq
			}
		}

		// WebSocket protocol handling: full step lifecycle.
		// The WebSocket branch does not call exec; it owns the connection and
		// produces a synthesised RequestResult. All steps count as one request
		// against the guard rail.
		if req.Protocol == "websocket" {
			// Apply global rate limit before the WebSocket upgrade (counts as one request).
			if vars.globalLimiter != nil {
				if waitErr := vars.globalLimiter.Wait(ctx); waitErr != nil {
					rr := RequestResult{Name: item.Name, Phase: phase, Skipped: true, WaveIndex: -1, SourceFile: item.SourceFile, SourceLine: item.SourceLine, RequestSlug: item.Slug}
					results = append(results, rr)
					stopped = true
					if vars.OnEvent != nil {
						emitRequestEnd(vars.OnEvent, "", item.Slug, rr, -1)
					}
					continue
				}
			}

			*counter++

			// Emit request.start for WebSocket requests.
			// Always mint the request ID so markdown formatter can correlate. M9-002.
			wsReqID := vars.nextRequestID()
			if vars.OnEvent != nil {
				vars.OnEvent.RequestStart(RequestEvent{
					RequestID:   wsReqID,
					RequestSlug: item.Slug,
					Name:        item.Name,
					Method:      req.Method,
					URL:         req.URL,
					Phase:       string(phase),
					SourceFile:  item.SourceFile,
					SourceLine:  item.SourceLine,
				})
			}

			dialer := vars.WebSocketDialer
			if dialer == nil {
				dialer = websocket.DefaultDialer
			}
			wsRes := websocket.Execute(ctx, &req, reqScope, dialer)

			// Synthesise an httpexec.Result so downstream output formatters
			// (terminal, JSON, HTML) keep working without schema changes. The
			// status code is always 101 (the WS upgrade code) to distinguish
			// a WebSocket request from an HTTP network failure; callers use
			// rr.Err / rr.AssertionResults.Passed to detect failure.
			synth := &httpexec.Result{
				StatusCode: 101,
				Duration:   wsRes.Duration,
			}
			rr := RequestResult{
				Name: item.Name, Phase: phase, Method: req.Method, URL: req.URL,
				RequestHeaders: req.Headers, RequestBody: req.Body,
				Result:     synth,
				WaveIndex:  -1,
				Warnings:   wsRes.Warnings,
				SourceFile: item.SourceFile,
				SourceLine: item.SourceLine,
				RequestID:  wsReqID, RequestSlug: item.Slug, // M9-002
			}
			if wsRes.Passed {
				rr.AssertionResults = &assertion.Results{Passed: true}
			} else {
				// Populate rr.Err for parity with the HTTP path so verbose
				// output, JSON, and TAP formatters render a top-level error
				// line for connection / dial / timeout failures.
				rr.Err = wsRes.Err
				actual := "unknown failure"
				if wsRes.Err != nil {
					actual = wsRes.Err.Error()
				}
				rr.AssertionResults = &assertion.Results{
					Passed: false,
					Items: []assertion.Result{{
						Type:     "websocket",
						Expected: "all steps pass",
						Actual:   actual,
						Passed:   false,
					}},
				}
				if checkRequired && item.IsRequired() {
					requiredFailed = true
					stopped = true
				} else if stopOnFailure {
					stopped = true
				}
			}
			if vars.OnEvent != nil {
				emitAssertionResults(vars.OnEvent, wsReqID, item.Slug, item.SourceFile, rr.AssertionResults)
				emitRequestEnd(vars.OnEvent, wsReqID, item.Slug, rr, -1)
			}
			results = append(results, rr)
			continue
		}

		// Per-request auth profile injection
		if item.Auth != "" {
			headerName, headerValue, authErr := resolveAuthProfile(item.Auth, vars.AuthProfiles, reqScope)
			if authErr != nil {
				return results, requiredFailed, fmt.Errorf("request %q: %w", item.Name, authErr)
			}
			// Explicit header takes precedence (case-insensitive)
			if headerName != "" && !hasHeaderCaseInsensitive(req.Headers, headerName) {
				if req.Headers == nil {
					req.Headers = make(map[string]string)
				}
				req.Headers[headerName] = headerValue
			}
		}

		// Determine effective retry config: merge all precedence levels.
		retryCfg := resolveRetryConfig(globalRetry, collectionRetry, sectionRetry, item.Retry)

		// Always mint the request ID so it is available even without --events.
		// M9-002: the markdown formatter relies on RequestID regardless of events.
		reqID := vars.nextRequestID()
		if vars.OnEvent != nil {
			vars.OnEvent.RequestStart(RequestEvent{
				RequestID:   reqID,
				RequestSlug: item.Slug,
				Name:        item.Name,
				Method:      req.Method,
				URL:         req.URL,
				Phase:       string(phase),
				SourceFile:  item.SourceFile,
				SourceLine:  item.SourceLine,
			})
		}

		// Attach the per-request signing spec to ctx so the signer exec-wrap
		// (allocated above when collectionUsesSigning) can invoke the right
		// signer. MUST be set immediately before exec to avoid leaking state.
		execCtx := ctx
		if spec := resolveSigningSpec(item, col); spec != nil {
			execCtx = signer.WithRequestSpec(ctx, spec)
			execCtx = signer.WithScope(execCtx, reqScope)
		}

		outcome := retry.ExecuteWithRetry(execCtx, retryCfg, req.Method, func(rCtx context.Context) (*httpexec.Result, error) {
			hr := requtil.ToHTTPRequest(&req)
			r, e := exec(rCtx, hr)
			req.Headers = hr.Headers // sync signer-injected headers back to parser.Request
			*counter++
			return r, e
		}, retry.DefaultSleep)

		result := outcome.Result
		execErr := outcome.Err
		retryCount := outcome.Attempts - 1
		retryWarnings := outcome.Warnings
		retryDetails := outcome.AttemptDetails

		// Refresh-on-failure: re-execute auth profile and retry once on 401.
		if execErr == nil && result != nil && result.StatusCode == 401 && item.Auth != "" {
			if p := findAuthProfile(item.Auth, vars.AuthProfiles); p != nil && p.RefreshOnFailure {
				_ = cacheStore.Invalidate(p.Name)
				freshResult, refreshErr := auth.ExecuteProfiles(ctx, []auth.Profile{*p}, vars.ProjectRoot, authExec, cacheStore)
				if refreshErr == nil {
					for k, v := range freshResult.Variables {
						scope.Set(k, v)
						reqScope.Set(k, v)
					}
					// Re-interpolate with fresh credentials.
					retryReq := item.Request
					retryInterpolated, retryErr := requtil.InterpolateRequest(reqScope, &retryReq)
					if retryErr == nil {
						retryReq = *retryInterpolated
						if headerName, headerValue, hErr := resolveAuthProfile(item.Auth, vars.AuthProfiles, reqScope); hErr == nil && headerName != "" {
							if retryReq.Headers == nil {
								retryReq.Headers = make(map[string]string)
							}
							retryReq.Headers[headerName] = headerValue
						}
						// Re-exec with signing spec on ctx (same spec as the original attempt).
						hr401 := requtil.ToHTTPRequest(&retryReq)
						result, execErr = exec(execCtx, hr401)
						retryReq.Headers = hr401.Headers // sync signer-injected headers back
						*counter++
						req = retryReq
					}
				}
			}
		}

		if execErr != nil {
			rr := RequestResult{
				Name: item.Name, Phase: phase, Method: req.Method, URL: req.URL,
				RequestHeaders: req.Headers, RequestBody: req.Body,
				Err: execErr, RetryCount: retryCount, RetryWarnings: retryWarnings, AttemptDetails: retryDetails, WaveIndex: -1,
				SourceFile: item.SourceFile, SourceLine: item.SourceLine,
				RequestID: reqID, RequestSlug: item.Slug, // M9-002
			}
			results = append(results, rr)
			if vars.OnEvent != nil {
				emitRequestEnd(vars.OnEvent, reqID, item.Slug, rr, -1)
			}
			if checkRequired && item.IsRequired() {
				requiredFailed = true
				stopped = true
			} else if stopOnFailure {
				stopped = true
			}
			continue
		}

		// M19-001: update `previous` for the next item's if: expression.
		// We update here — after a confirmed HTTP response (execErr == nil && result != nil)
		// but BEFORE assertion evaluation — so that subsequent if: expressions see the
		// actual response regardless of whether assertions pass or fail.
		// Items that produced no response (execErr != nil) leave `previous` unchanged.
		prevResponse := previous // snapshot before update for CEL assertion context
		var celResp *apicel.Response
		if result != nil {
			celResp = buildCelResponse(result)
			previous = celResp
		}

		// M19-004: build CEL assertion context when cel: assertions are present.
		// The assertion context receives the current response as `response` and the
		// pre-update `previous` (i.e. the prior request's response) as `previous`.
		var celCtx assertion.CELContext
		if len(item.Assertions.CEL.Items) > 0 && vars.CelEvaluator != nil {
			celCtx = buildCELCtxForItem(item, col, reqScope, celResp, prevResponse, vars)
		}

		headerInputs, bodyInputs, assertErr := requtil.ToAssertionInputs(reqScope, item.Assertions)
		if assertErr != nil {
			return results, requiredFailed, fmt.Errorf("request %q: %w", item.Name, assertErr)
		}

		ar := assertion.Evaluate(assertion.EvalInput{
			StatusCodes:      item.Assertions.Status.Codes,
			ActualStatus:     result.StatusCode,
			HeaderAssertions: headerInputs,
			Headers:          result.Headers,
			BodyAssertions:   bodyInputs,
			Body:             result.Body,
			MaxDurationMs:    item.Assertions.Timing.MaxDurationMs,
			ActualDuration:   result.Duration,
			Schema:           item.Assertions.CompiledSchema,
			StatusLine:       item.Assertions.Status.Line,
			TimingLine:       item.Assertions.Timing.Line,
			SchemaLine:       item.Assertions.SchemaLine,
			CELInputs:        toCELInputs(item.Assertions.CEL.Items),
			CELCtx:           celCtx,
		})

		// GraphQL error checking (after assertions, so user assertions on $.errors still work)
		var graphqlWarnings []string
		if item.Request.Protocol == "graphql" && result != nil {
			// Resolve effective mode: per-request > global > built-in default (fail).
			effectiveMode := graphql.ErrorHandlingFail
			if vars.GlobalGraphQL != nil && vars.GlobalGraphQL.ErrorHandling.PartialSuccess != "" {
				m, mErr := graphql.ParseErrorHandling(vars.GlobalGraphQL.ErrorHandling.PartialSuccess)
				if mErr != nil {
					return results, requiredFailed, fmt.Errorf("global graphql defaults: %w", mErr)
				}
				effectiveMode = m
			}
			if item.Request.GraphQL != nil && item.Request.GraphQL.ErrorHandling != "" {
				m, mErr := graphql.ParseErrorHandling(item.Request.GraphQL.ErrorHandling)
				if mErr != nil {
					return results, requiredFailed, fmt.Errorf("request %q: %w", item.Name, mErr)
				}
				effectiveMode = m
			}

			check, checkErr := graphql.CheckResponse(result.Body)
			if checkErr != nil {
				// A response that is not a GraphQL document is a property of
				// THIS response — a gateway 500 with an HTML error page is the
				// ordinary case — so it fails this request and the run
				// continues. Aborting discarded every result collected so far
				// and reported "requests": [] beside a summary that still
				// counted the passes it had thrown away, under an exit code
				// reserved for variable resolution (§11C.1).
				//
				// checkErr already carries the "parsing graphql response"
				// prefix; adding it again produced the doubled message.
				if ar == nil {
					ar = &assertion.Results{Passed: false}
				}
				ar.Passed = false
				ar.Items = append(ar.Items, assertion.Result{
					Type:     assertion.TypeGraphQLError,
					Expected: "a GraphQL response document",
					Actual:   checkErr.Error(),
					Passed:   false,
				})
			}
			// Outcome classification only means something for a document that
			// parsed; an unparseable body has already failed the request above.
			outcome := graphql.OutcomeSuccess
			if checkErr == nil {
				outcome = graphql.ClassifyOutcome(check)
			}
			// The mode governs BOTH error outcomes, as docs/MANUAL.md §7.1
			// states them: a matrix of four outcomes against three modes, in
			// which partial-success and full-failure are each fail / warn /
			// pass. Full failure used to be handled before the mode was read,
			// under a comment claiming "per spec" — which
			// docs/CLI_SPECIFICATION.md §12.2 does not say — so `warn` and
			// `ignore` behaved exactly like `fail` and the setting was inert
			// for half the cases it documents (§11C.2).
			//
			// `ignore` does not hide a broken response: the request's own
			// assertions still run, so an author who opts out of GraphQL-level
			// error checking can still assert on $.data and $.errors.
			if outcome == graphql.OutcomeFullFailure || outcome == graphql.OutcomePartialSuccess {
				label := "GraphQL partial success"
				if outcome == graphql.OutcomeFullFailure {
					label = "GraphQL full failure"
				}
				switch effectiveMode {
				case graphql.ErrorHandlingFail:
					if ar == nil {
						ar = &assertion.Results{Passed: false}
					}
					ar.Passed = false
					errMsg := "GraphQL response contains errors"
					if len(check.Errors) > 0 {
						errMsg = fmt.Sprintf("GraphQL error: %s", check.Errors[0].Message)
					}
					ar.Items = append(ar.Items, assertion.Result{
						Type:     assertion.TypeGraphQLError,
						Expected: "no errors",
						Actual:   errMsg,
						Passed:   false,
					})
				case graphql.ErrorHandlingWarn:
					for _, e := range check.Errors {
						graphqlWarnings = append(graphqlWarnings, fmt.Sprintf("%s: %s", label, e.Message))
					}
				case graphql.ErrorHandlingIgnore:
					// no-op: the author opted out of GraphQL-level error
					// checking. Their own assertions still run.
				}
			}
		}

		rr := RequestResult{
			Name: item.Name, Phase: phase, Method: req.Method, URL: req.URL,
			RequestHeaders: req.Headers, RequestBody: req.Body,
			Result: result, AssertionResults: ar, RetryCount: retryCount, RetryWarnings: retryWarnings, AttemptDetails: retryDetails, WaveIndex: -1,
			Warnings:   graphqlWarnings,
			SourceFile: item.SourceFile,
			SourceLine: item.SourceLine,
			RequestID:  reqID, RequestSlug: item.Slug, // M9-002
		}

		if ar != nil && !ar.Passed {
			if vars.OnEvent != nil {
				emitAssertionResults(vars.OnEvent, reqID, item.Slug, item.SourceFile, ar)
				emitRequestEnd(vars.OnEvent, reqID, item.Slug, rr, -1)
			}
			if checkRequired && item.IsRequired() {
				requiredFailed = true
				stopped = true
			} else if stopOnFailure {
				stopped = true
			}
			results = append(results, rr)
			continue
		}

		// Extract variables from response (if configured)
		if len(item.Extract) > 0 {
			extResult, extErr := variable.Extract(variable.ExtractionInput{
				Extractions: item.Extract,
				Body:        result.Body,
			})
			if extErr != nil {
				rr.Err = extErr
				if vars.OnEvent != nil {
					emitAssertionResults(vars.OnEvent, reqID, item.Slug, item.SourceFile, ar)
					emitRequestEnd(vars.OnEvent, reqID, item.Slug, rr, -1)
				}
				if checkRequired && item.IsRequired() {
					requiredFailed = true
					stopped = true
				} else if stopOnFailure {
					stopped = true
				}
				results = append(results, rr)
				continue
			}
			// Register the sensitive ones before any event carrying them is
			// emitted, so the response body that produced the token is
			// redacted too — not just later requests that use it.
			variable.MarkExtractedSensitive(scope.RuntimeSensitiveSet(), extResult.Variables, item.ExtractSensitive)
			for k, v := range extResult.Variables {
				scope.Set(k, v)
			}
		}

		// Emit assertion results and request end for passing requests.
		if vars.OnEvent != nil {
			emitAssertionResults(vars.OnEvent, reqID, item.Slug, item.SourceFile, ar)
			emitRequestEnd(vars.OnEvent, reqID, item.Slug, rr, -1)
		}

		results = append(results, rr)
	}

	return results, requiredFailed, nil
}

// buildCelResponse converts an httpexec.Result into an apicel.Response for use
// in CEL if: expressions as `previous`. Body is best-effort JSON decode; nil
// on parse failure (CEL handles missing-key access gracefully via dyn type).
func buildCelResponse(result *httpexec.Result) *apicel.Response {
	if result == nil {
		return nil
	}
	var body any
	if len(result.Body) > 0 {
		// Best-effort JSON decode; leave nil if the body is not JSON.
		var decoded any
		if err := json.Unmarshal(result.Body, &decoded); err == nil {
			body = decoded
		}
	}
	return &apicel.Response{
		Status:  result.StatusCode,
		Headers: flattenHeader(result.Headers),
		Body:    body,
	}
}

// toCELInputs converts a slice of parser.CELAssertion into assertion.CELInput
// values ready for CheckCEL.
func toCELInputs(items []parser.CELAssertion) []assertion.CELInput {
	if len(items) == 0 {
		return nil
	}
	out := make([]assertion.CELInput, len(items))
	for i, it := range items {
		out[i] = assertion.CELInput{Index: i, Source: it.Source, Line: it.Line}
	}
	return out
}

// buildCELCtxForItem constructs an assertion.CELContext for a single request
// item using the per-request scope, the current response, the previous response,
// and the run's evaluator + cache.
func buildCELCtxForItem(
	item parser.RequestItem,
	col *parser.Collection,
	scope *variable.Scope,
	celResponse *apicel.Response,
	previous *apicel.Response,
	vars VarSources,
) assertion.CELContext {
	// Resolve variables as map[string]any for the CEL activation.
	resolvedVars := scope.Resolved()
	varsAny := make(map[string]any, len(resolvedVars))
	for k, v := range resolvedVars {
		varsAny[k] = v
	}

	// Build the sensitive-name set from collection + item sensitive vars.
	sensNames := make(map[string]struct{})
	for _, name := range col.Variables.Sensitive.Names() {
		sensNames[name] = struct{}{}
	}
	for _, name := range item.Variables.Sensitive.Names() {
		sensNames[name] = struct{}{}
	}

	// Snapshot current sensitive values for substring redaction in failure messages.
	// We combine the runtime sensitive set with the static sensitive sets so that
	// dynamically-resolved secrets are also masked.
	rts := scope.RuntimeSensitiveSet()
	sensValues := snapshotSensitiveValues(col.Variables.Sensitive, item.Variables.Sensitive, rts)

	return assertion.CELContext{
		Evaluator:       vars.CelEvaluator,
		ProgCache:       vars.assertProgCache,
		Response:        celResponse,
		Previous:        previous,
		Vars:            varsAny,
		Env:             vars.EnvVar,
		SensitiveNames:  sensNames,
		SensitiveValues: sensValues,
		SensitiveObserve: func(_, value string) {
			if rts != nil {
				rts.AddValue(value)
			}
		},
	}
}

// snapshotSensitiveValues collects the current set of concrete sensitive values
// from one or more SensitiveSets and returns them sorted longest-first (to
// ensure longer secrets take precedence in substring replacement).
func snapshotSensitiveValues(sets ...*variable.SensitiveSet) []string {
	seen := map[string]struct{}{}
	for _, s := range sets {
		if s == nil {
			continue
		}
		for _, v := range s.Values() {
			if v != "" {
				seen[v] = struct{}{}
			}
		}
	}
	out := make([]string, 0, len(seen))
	for v := range seen {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		if len(out[i]) != len(out[j]) {
			return len(out[i]) > len(out[j])
		}
		return out[i] < out[j]
	})
	return out
}

// executeDataDriven handles a data-driven request item. It checks the feature gate,
// loads the data source, and executes the request once per row.
// Each iteration counts toward the guard rail counter.
// Extracted variables accumulate as JSON arrays across iterations.
func executeDataDriven(
	ctx context.Context,
	col *parser.Collection,
	item parser.RequestItem,
	scope *variable.Scope,
	exec ExecuteFunc,
	vars VarSources,
	phase Phase,
	checkRequired bool,
	stopOnFailure bool,
	counter *int,
	maxRequests int,
	collectionRetry *retry.FullConfig,
	sectionRetry *retry.FullConfig,
	globalRetry *retry.FullConfig,
) ([]RequestResult, bool, error) {
	// Load data source relative to collection directory
	baseDir := vars.CollectionDir
	if baseDir == "" {
		baseDir = vars.ProjectRoot
	}
	ds, loadErr := datadriven.LoadWithControls(*item.DataDriven, baseDir)
	if loadErr != nil {
		if errors.Is(loadErr, datadriven.ErrEmptyDataFile) {
			return []RequestResult{{
				Name:       item.Name,
				Phase:      phase,
				Skipped:    true,
				SkipReason: "data file is empty",
				WaveIndex:  -1,
				SourceFile: item.SourceFile,
				SourceLine: item.SourceLine,
			}}, false, nil
		}
		return nil, false, fmt.Errorf("request %q data_driven: %w", item.Name, loadErr)
	}

	// Large dataset check: warn if >10,000 rows unless confirmed
	if info := datadriven.CheckLargeDataset(ds); info != nil && !vars.ConfirmLargeDataset {
		return nil, false, fmt.Errorf(
			"data file %q has %d rows (>%d). Performance estimate: %s, storage: %s. "+
				"Use --confirm-large-dataset to proceed or add store_results: summary|failed_only",
			item.DataDriven.Source, info.TotalRows, datadriven.LargeDatasetThreshold,
			info.EstimatedDuration, info.StorageEstimate)
	}

	// Parallel data-driven execution
	if item.DataDriven.Parallel {
		return executeDataDrivenParallel(ctx, col, item, scope, exec, vars, phase, checkRequired, stopOnFailure, counter, maxRequests, collectionRetry, sectionRetry, globalRetry, ds)
	}

	// Execute iterations sequentially
	var results []RequestResult
	accumulated := make(map[string][]string)
	total := len(ds.Rows)
	requiredFailed := false

	for idx, row := range ds.Rows {
		if *counter >= maxRequests {
			break
		}
		if err := ctx.Err(); err != nil {
			break
		}

		iterScope := datadriven.InjectIterationVars(scope, row, idx, total)

		// Per-request variable scoping (precedence 8) on top of iteration scope
		reqScope := iterScope
		if len(item.Variables.Values) > 0 {
			var oErr error
			reqScope, oErr = iterScope.WithOverrides(item.Variables.Values)
			if oErr != nil {
				return results, requiredFailed, fmt.Errorf("request %q [%d/%d] variables: %w", item.Name, idx+1, total, oErr)
			}
			for k, v := range vars.EnvVar {
				reqScope.Set(k, v)
			}
			for k, v := range vars.CLI {
				reqScope.Set(k, v)
			}
		}

		// Interpolate request fields
		req := item.Request
		interpolated, interpErr := requtil.InterpolateRequest(reqScope, &req)
		if interpErr != nil {
			interpErr = enrichInterpErr(interpErr, item)
			// M8-004: data-driven items are always in main phase; enrich cliff.
			interpErr = enrichSelectionCliff(interpErr, vars.fullMainForCliff, vars.fullSetupForCliff, vars.Selection)
			return results, requiredFailed, fmt.Errorf("request %q [%d/%d]: %w", item.Name, idx+1, total, interpErr)
		}
		req = *interpolated

		// Per-request auth profile injection
		if item.Auth != "" {
			headerName, headerValue, authErr := resolveAuthProfile(item.Auth, vars.AuthProfiles, reqScope)
			if authErr != nil {
				return results, requiredFailed, fmt.Errorf("request %q [%d/%d]: %w", item.Name, idx+1, total, authErr)
			}
			if headerName != "" && !hasHeaderCaseInsensitive(req.Headers, headerName) {
				if req.Headers == nil {
					req.Headers = make(map[string]string)
				}
				req.Headers[headerName] = headerValue
			}
		}

		// Determine retry config
		retryCfg := resolveRetryConfig(globalRetry, collectionRetry, sectionRetry, item.Retry)

		iterName := fmt.Sprintf("%s [%d/%d]", item.Name, idx+1, total)
		iterSlug, _ := parser.Slug(iterName)

		// Emit RequestStart for this iteration.
		// Always mint the request ID so markdown formatter can correlate. M9-002.
		iterReqID := vars.nextRequestID()
		if vars.OnEvent != nil {
			vars.OnEvent.RequestStart(RequestEvent{
				RequestID:   iterReqID,
				RequestSlug: iterSlug,
				Name:        iterName,
				Method:      req.Method,
				URL:         req.URL,
				Phase:       string(phase),
				SourceFile:  item.SourceFile,
				SourceLine:  item.SourceLine,
			})
		}

		// Attach signing spec to ctx for the data-driven sequential path.
		ddExecCtx := ctx
		if spec := resolveSigningSpec(item, col); spec != nil {
			ddExecCtx = signer.WithRequestSpec(ctx, spec)
			ddExecCtx = signer.WithScope(ddExecCtx, reqScope)
		}

		outcome := retry.ExecuteWithRetry(ddExecCtx, retryCfg, req.Method, func(rCtx context.Context) (*httpexec.Result, error) {
			hr := requtil.ToHTTPRequest(&req)
			r, e := exec(rCtx, hr)
			req.Headers = hr.Headers // sync signer-injected headers back to parser.Request
			*counter++
			return r, e
		}, retry.DefaultSleep)

		result := outcome.Result
		execErr := outcome.Err
		retryCount := outcome.Attempts - 1
		retryWarnings := outcome.Warnings
		retryDetails := outcome.AttemptDetails

		if execErr != nil {
			errRR := RequestResult{
				Name: iterName, Phase: phase, Method: req.Method, URL: req.URL,
				RequestHeaders: req.Headers, RequestBody: req.Body,
				Err: execErr, RetryCount: retryCount, RetryWarnings: retryWarnings, AttemptDetails: retryDetails, WaveIndex: -1,
				IsDataDriven: true, DataDrivenName: item.Name,
				IterationIndex: idx, IterationTotal: total,
				IterationData: cloneRow(row),
				SourceFile:    item.SourceFile,
				SourceLine:    item.SourceLine,
				RequestID:     iterReqID, RequestSlug: iterSlug, // M9-002
			}
			if vars.OnEvent != nil {
				emitRequestEnd(vars.OnEvent, iterReqID, iterSlug, errRR, -1)
			}
			results = append(results, errRR)
			if item.DataDriven.FailFast {
				break
			}
			continue
		}

		// M19-004: build CEL assertion context for data-driven iterations.
		var ddCelCtx assertion.CELContext
		if len(item.Assertions.CEL.Items) > 0 && vars.CelEvaluator != nil {
			ddCelResp := buildCelResponse(result)
			ddCelCtx = buildCELCtxForItem(item, col, reqScope, ddCelResp, nil, vars)
		}

		headerInputs, bodyInputs, assertErr := requtil.ToAssertionInputs(reqScope, item.Assertions)
		if assertErr != nil {
			return results, requiredFailed, fmt.Errorf("request %q [%d/%d]: %w", item.Name, idx+1, total, assertErr)
		}

		ar := assertion.Evaluate(assertion.EvalInput{
			StatusCodes:      item.Assertions.Status.Codes,
			ActualStatus:     result.StatusCode,
			HeaderAssertions: headerInputs,
			Headers:          result.Headers,
			BodyAssertions:   bodyInputs,
			Body:             result.Body,
			MaxDurationMs:    item.Assertions.Timing.MaxDurationMs,
			ActualDuration:   result.Duration,
			Schema:           item.Assertions.CompiledSchema,
			StatusLine:       item.Assertions.Status.Line,
			TimingLine:       item.Assertions.Timing.Line,
			SchemaLine:       item.Assertions.SchemaLine,
			CELInputs:        toCELInputs(item.Assertions.CEL.Items),
			CELCtx:           ddCelCtx,
		})
		rr := RequestResult{
			Name: iterName, Phase: phase, Method: req.Method, URL: req.URL,
			RequestHeaders: req.Headers, RequestBody: req.Body,
			Result: result, AssertionResults: ar, RetryCount: retryCount, RetryWarnings: retryWarnings, AttemptDetails: retryDetails, WaveIndex: -1,
			IsDataDriven: true, DataDrivenName: item.Name,
			IterationIndex: idx, IterationTotal: total,
			IterationData: cloneRow(row),
			SourceFile:    item.SourceFile,
			SourceLine:    item.SourceLine,
			RequestID:     iterReqID, RequestSlug: iterSlug, // M9-002
		}

		if ar != nil && !ar.Passed {
			if vars.OnEvent != nil {
				emitAssertionResults(vars.OnEvent, iterReqID, iterSlug, item.SourceFile, ar)
				emitRequestEnd(vars.OnEvent, iterReqID, iterSlug, rr, -1)
			}
			results = append(results, rr)
			if item.DataDriven.FailFast {
				break
			}
			continue
		}

		// Extract variables from response (accumulate across iterations)
		if len(item.Extract) > 0 {
			extResult, extErr := variable.Extract(variable.ExtractionInput{
				Extractions: item.Extract,
				Body:        result.Body,
			})
			if extErr != nil {
				rr.Err = extErr
				if vars.OnEvent != nil {
					emitRequestEnd(vars.OnEvent, iterReqID, iterSlug, rr, -1)
				}
				results = append(results, rr)
				continue
			}
			variable.MarkExtractedSensitive(scope.RuntimeSensitiveSet(), extResult.Variables, item.ExtractSensitive)
			for k, v := range extResult.Variables {
				accumulated[k] = append(accumulated[k], v)
			}
		}

		// Emit assertion results and request end for passing iterations.
		if vars.OnEvent != nil {
			emitAssertionResults(vars.OnEvent, iterReqID, iterSlug, item.SourceFile, ar)
			emitRequestEnd(vars.OnEvent, iterReqID, iterSlug, rr, -1)
		}

		results = append(results, rr)
	}

	// Set accumulated extraction values as JSON arrays into the outer scope
	setAccumulatedVars(scope, accumulated)

	// Check if any iteration failed. The caller uses this to decide whether
	// to skip subsequent items (consistent with non-data-driven path).
	requiredFailed = checkDataDrivenFailure(results, checkRequired, stopOnFailure, item.IsRequired())

	// Apply store_results filtering (after failure detection to not affect exit logic)
	results = filterDataDrivenResults(results, item.DataDriven.EffectiveStoreResults())

	return results, requiredFailed, nil
}

// executeDataDrivenParallel handles parallel data-driven execution using a worker pool.
// Each iteration runs concurrently with configurable concurrency and rate limiting.
func executeDataDrivenParallel(
	ctx context.Context,
	col *parser.Collection,
	item parser.RequestItem,
	scope *variable.Scope,
	exec ExecuteFunc,
	vars VarSources,
	phase Phase,
	checkRequired bool,
	stopOnFailure bool,
	counter *int,
	maxRequests int,
	collectionRetry *retry.FullConfig,
	sectionRetry *retry.FullConfig,
	globalRetry *retry.FullConfig,
	ds *datadriven.DataSet,
) ([]RequestResult, bool, error) {
	total := len(ds.Rows)

	// Build the per-iteration execution function
	execFn := func(iterCtx context.Context, iterScope *variable.Scope, idx int) (*datadriven.IterationResult, error) {
		// Per-request variable scoping (precedence 8)
		reqScope := iterScope
		if len(item.Variables.Values) > 0 {
			var oErr error
			reqScope, oErr = iterScope.WithOverrides(item.Variables.Values)
			if oErr != nil {
				return nil, fmt.Errorf("request %q [%d/%d] variables: %w", item.Name, idx+1, total, oErr)
			}
			for k, v := range vars.EnvVar {
				reqScope.Set(k, v)
			}
			for k, v := range vars.CLI {
				reqScope.Set(k, v)
			}
		}

		// Interpolate request fields
		req := item.Request
		interpolated, interpErr := requtil.InterpolateRequest(reqScope, &req)
		if interpErr != nil {
			interpErr = enrichInterpErr(interpErr, item)
			// M8-004: parallel data-driven items are always in main phase; enrich cliff.
			interpErr = enrichSelectionCliff(interpErr, vars.fullMainForCliff, vars.fullSetupForCliff, vars.Selection)
			return nil, fmt.Errorf("request %q [%d/%d]: %w", item.Name, idx+1, total, interpErr)
		}
		req = *interpolated

		// Auth profile injection
		if item.Auth != "" {
			headerName, headerValue, authErr := resolveAuthProfile(item.Auth, vars.AuthProfiles, reqScope)
			if authErr != nil {
				return nil, fmt.Errorf("request %q [%d/%d]: %w", item.Name, idx+1, total, authErr)
			}
			if headerName != "" && !hasHeaderCaseInsensitive(req.Headers, headerName) {
				if req.Headers == nil {
					req.Headers = make(map[string]string)
				}
				req.Headers[headerName] = headerValue
			}
		}

		// Retry config
		retryCfg := resolveRetryConfig(globalRetry, collectionRetry, sectionRetry, item.Retry)

		iterName := fmt.Sprintf("%s [%d/%d]", item.Name, idx+1, total)
		iterSlug, _ := parser.Slug(iterName)

		// Emit RequestStart before execution (M6-005: parallel data-driven event emission).
		// Always mint the request ID so markdown formatter can correlate. M9-002.
		iterReqID := vars.nextRequestID()
		if vars.OnEvent != nil {
			vars.OnEvent.RequestStart(RequestEvent{
				RequestID:   iterReqID,
				RequestSlug: iterSlug,
				Name:        iterName,
				Method:      req.Method,
				URL:         req.URL,
				Phase:       string(phase),
				SourceFile:  item.SourceFile,
				SourceLine:  item.SourceLine,
			})
		}

		// Attach signing spec to ctx for the parallel data-driven path.
		ddParallelExecCtx := iterCtx
		if spec := resolveSigningSpec(item, col); spec != nil {
			ddParallelExecCtx = signer.WithRequestSpec(iterCtx, spec)
			ddParallelExecCtx = signer.WithScope(ddParallelExecCtx, reqScope)
		}

		outcome := retry.ExecuteWithRetry(ddParallelExecCtx, retryCfg, req.Method, func(rCtx context.Context) (*httpexec.Result, error) {
			hr := requtil.ToHTTPRequest(&req)
			r, e := exec(rCtx, hr)
			req.Headers = hr.Headers // sync signer-injected headers back to parser.Request
			return r, e
		}, retry.DefaultSleep)

		result := outcome.Result
		execErr := outcome.Err
		retryCount := outcome.Attempts - 1
		retryWarnings := outcome.Warnings
		retryDetails := outcome.AttemptDetails

		if execErr != nil {
			errIR := &datadriven.IterationResult{
				Index:          idx,
				Name:           iterName,
				Err:            execErr,
				Method:         req.Method,
				URL:            req.URL,
				RequestHeaders: req.Headers,
				RequestBody:    req.Body,
				RetryCount:     retryCount,
				RetryWarnings:  retryWarnings,
				AttemptDetails: retryDetails,
				RequestID:      iterReqID, // M9-002: carry IDs so conversion loop can populate RequestResult
				RequestSlug:    iterSlug,  // M9-002
			}
			if vars.OnEvent != nil {
				errRR := RequestResult{
					Name:        iterName,
					Phase:       phase,
					Method:      req.Method,
					URL:         req.URL,
					RequestBody: req.Body,
					Err:         execErr,
				}
				emitRequestEnd(vars.OnEvent, iterReqID, iterSlug, errRR, -1)
			}
			return errIR, nil
		}

		// Evaluate assertions
		// M19-004: build CEL context for parallel data-driven iterations.
		var pddCelCtx assertion.CELContext
		if len(item.Assertions.CEL.Items) > 0 && vars.CelEvaluator != nil {
			pddCelResp := buildCelResponse(result)
			pddCelCtx = buildCELCtxForItem(item, col, reqScope, pddCelResp, nil, vars)
		}

		headerInputs, bodyInputs, assertErr := requtil.ToAssertionInputs(reqScope, item.Assertions)
		if assertErr != nil {
			return nil, fmt.Errorf("request %q [%d/%d]: %w", item.Name, idx+1, total, assertErr)
		}

		ar := assertion.Evaluate(assertion.EvalInput{
			StatusCodes:      item.Assertions.Status.Codes,
			ActualStatus:     result.StatusCode,
			HeaderAssertions: headerInputs,
			Headers:          result.Headers,
			BodyAssertions:   bodyInputs,
			Body:             result.Body,
			MaxDurationMs:    item.Assertions.Timing.MaxDurationMs,
			ActualDuration:   result.Duration,
			Schema:           item.Assertions.CompiledSchema,
			StatusLine:       item.Assertions.Status.Line,
			TimingLine:       item.Assertions.Timing.Line,
			SchemaLine:       item.Assertions.SchemaLine,
			CELInputs:        toCELInputs(item.Assertions.CEL.Items),
			CELCtx:           pddCelCtx,
		})

		ir := &datadriven.IterationResult{
			Index:            idx,
			Name:             iterName,
			HTTPResult:       result,
			AssertionResults: ar,
			Method:           req.Method,
			URL:              req.URL,
			RequestHeaders:   req.Headers,
			RequestBody:      req.Body,
			RetryCount:       retryCount,
			RetryWarnings:    retryWarnings,
			AttemptDetails:   retryDetails,
			RequestID:        iterReqID, // M9-002: carry IDs so conversion loop can populate RequestResult
			RequestSlug:      iterSlug,  // M9-002
		}

		if ar != nil && !ar.Passed {
			ir.Err = fmt.Errorf("assertion failed")
		}

		// Extract variables
		if len(item.Extract) > 0 && (ar == nil || ar.Passed) {
			extResult, extErr := variable.Extract(variable.ExtractionInput{
				Extractions: item.Extract,
				Body:        result.Body,
			})
			if extErr != nil {
				ir.Err = extErr
			} else {
				variable.MarkExtractedSensitive(scope.RuntimeSensitiveSet(), extResult.Variables, item.ExtractSensitive)
				ir.Extracted = extResult.Variables
			}
		}

		// Emit AssertionResult and RequestEnd after all processing (M6-005).
		if vars.OnEvent != nil {
			rr := RequestResult{
				Name:             iterName,
				Phase:            phase,
				Method:           req.Method,
				URL:              req.URL,
				RequestBody:      req.Body,
				Result:           result,
				AssertionResults: ar,
				Err:              ir.Err,
			}
			emitAssertionResults(vars.OnEvent, iterReqID, iterSlug, item.SourceFile, ar)
			emitRequestEnd(vars.OnEvent, iterReqID, iterSlug, rr, -1)
		}

		return ir, nil
	}

	// Determine rate limit
	rps := 0
	if item.DataDriven.RateLimitRPS != nil {
		rps = *item.DataDriven.RateLimitRPS
	}

	// For large datasets, process in chunks to reduce peak memory from concurrent results.
	// Each chunk is processed through the worker pool, and results are aggregated.
	chunks := datadriven.ChunkRows(ds.Rows, datadriven.DefaultChunkSize)

	var allIterResults []datadriven.IterationResult
	accumulated := make(map[string][]string)

	chunkOffset := 0
	for _, chunk := range chunks {
		if ctx.Err() != nil {
			break
		}

		chunkDS := &datadriven.DataSet{Rows: chunk, Columns: ds.Columns}
		chunkResults, chunkAccum, pErr := datadriven.ExecuteParallel(ctx, datadriven.ParallelConfig{
			DataSet:      chunkDS,
			Scope:        scope,
			MaxWorkers:   datadriven.DefaultMaxWorkers,
			RateLimitRPS: rps,
			FailFast:     item.DataDriven.FailFast,
			ExecFn:       execFn,
			ChunkOffset:  chunkOffset,
			TotalRows:    total,
		})
		if pErr != nil {
			return nil, false, fmt.Errorf("request %q parallel: %w", item.Name, pErr)
		}

		allIterResults = append(allIterResults, chunkResults...)
		for k, vals := range chunkAccum {
			accumulated[k] = append(accumulated[k], vals...)
		}

		chunkOffset += len(chunk)

		// Fail-fast across chunks: if any iteration in this chunk failed and fail_fast is set, stop
		if item.DataDriven.FailFast {
			hasFail := false
			for _, ir := range chunkResults {
				if ir.Err != nil {
					hasFail = true
					break
				}
			}
			if hasFail {
				break
			}
		}
	}

	// Convert iteration results to runner RequestResults
	results := make([]RequestResult, 0, len(allIterResults))
	for _, ir := range allIterResults {
		rr := RequestResult{
			Name:           ir.Name,
			Phase:          phase,
			Method:         ir.Method,
			URL:            ir.URL,
			RequestHeaders: ir.RequestHeaders,
			RequestBody:    ir.RequestBody,
			RetryCount:     ir.RetryCount,
			RetryWarnings:  ir.RetryWarnings,
			AttemptDetails: ir.AttemptDetails,
			WaveIndex:      -1,
			IsDataDriven:   true,
			DataDrivenName: item.Name,
			IterationIndex: ir.Index,
			IterationTotal: total,
			IterationData:  cloneRow(ir.Row),
			SourceFile:     item.SourceFile,
			SourceLine:     item.SourceLine,
			RequestID:      ir.RequestID,   // M9-002: propagate from IterationResult
			RequestSlug:    ir.RequestSlug, // M9-002
		}
		if ir.HTTPResult != nil {
			rr.Result = ir.HTTPResult
		}
		if ir.AssertionResults != nil {
			rr.AssertionResults = ir.AssertionResults
		}
		if ir.Err != nil {
			rr.Err = ir.Err
		}
		results = append(results, rr)
	}

	// Count executions toward guard rail
	*counter += len(allIterResults)

	// Set accumulated extraction values into the outer scope
	setAccumulatedVars(scope, accumulated)

	// Check failure status before filtering (filtering must not affect failure detection)
	requiredFailed := checkDataDrivenFailure(results, checkRequired, stopOnFailure, item.IsRequired())

	// Apply store_results filtering
	results = filterDataDrivenResults(results, item.DataDriven.EffectiveStoreResults())

	return results, requiredFailed, nil
}

// cloneRow returns a shallow copy of a datadriven.Row as a plain map. Returns nil for empty rows.
func cloneRow(r datadriven.Row) map[string]string {
	if len(r) == 0 {
		return nil
	}
	out := make(map[string]string, len(r))
	for k, v := range r {
		out[k] = v
	}
	return out
}

// setAccumulatedVars sets accumulated extraction values as JSON arrays into the scope.
func setAccumulatedVars(scope *variable.Scope, accumulated map[string][]string) {
	for k, vals := range accumulated {
		if len(vals) == 1 {
			scope.Set(k, vals[0])
		} else {
			b, _ := json.Marshal(vals)
			scope.Set(k, string(b))
		}
	}
}

// checkDataDrivenFailure examines results to determine if any iteration failed
// and whether that failure should propagate (based on required/stopOnFailure flags).
func checkDataDrivenFailure(results []RequestResult, checkRequired, stopOnFailure, isRequired bool) bool {
	anyFailed := false
	for _, rr := range results {
		if rr.Err != nil || (rr.AssertionResults != nil && !rr.AssertionResults.Passed) {
			anyFailed = true
			break
		}
	}
	if anyFailed && checkRequired && isRequired {
		return true
	}
	if anyFailed && stopOnFailure {
		return true
	}
	return false
}

// filterDataDrivenResults applies store_results policy to data-driven request results.
// Must be called AFTER checkDataDrivenFailure so failure detection is unaffected.
//   - "all": returns results unchanged
//   - "summary": strips Result, AssertionResults, RequestHeaders, and RequestBody
//     from all iterations, keeping only Name/Phase/Err and data-driven metadata
//   - "failed_only": keeps full details only for failed iterations; strips details
//     from passed iterations
func filterDataDrivenResults(results []RequestResult, policy string) []RequestResult {
	switch policy {
	case datadriven.StoreSummary:
		out := make([]RequestResult, len(results))
		for i, r := range results {
			out[i] = RequestResult{
				Name:           r.Name,
				Phase:          r.Phase,
				Method:         r.Method,
				URL:            r.URL,
				Err:            r.Err,
				Skipped:        r.Skipped,
				SkipReason:     r.SkipReason,
				WaveIndex:      r.WaveIndex,
				IsDataDriven:   r.IsDataDriven,
				DataDrivenName: r.DataDrivenName,
				IterationIndex: r.IterationIndex,
				IterationTotal: r.IterationTotal,
				IterationData:  r.IterationData,
				SourceFile:     r.SourceFile,
				SourceLine:     r.SourceLine,
			}
		}
		return out
	case datadriven.StoreFailedOnly:
		out := make([]RequestResult, len(results))
		for i, r := range results {
			isFailed := r.Err != nil || (r.AssertionResults != nil && !r.AssertionResults.Passed)
			if isFailed {
				out[i] = r // keep full details
			} else {
				// Strip details from passed iterations
				out[i] = RequestResult{
					Name:           r.Name,
					Phase:          r.Phase,
					Method:         r.Method,
					URL:            r.URL,
					Skipped:        r.Skipped,
					SkipReason:     r.SkipReason,
					WaveIndex:      r.WaveIndex,
					IsDataDriven:   r.IsDataDriven,
					DataDrivenName: r.DataDrivenName,
					IterationIndex: r.IterationIndex,
					IterationTotal: r.IterationTotal,
					IterationData:  r.IterationData,
					SourceFile:     r.SourceFile,
					SourceLine:     r.SourceLine,
				}
			}
		}
		return out
	default: // StoreAll or unrecognized
		return results
	}
}

// computeSummary populates s from all results across all phases.
func computeSummary(results []RequestResult, s *Summary) {
	for _, r := range results {
		switch {
		case r.Skipped:
			s.Skipped++
		case r.Err != nil:
			s.Failed++
			if r.Phase == PhaseTeardown {
				s.TeardownErrors++
			}
		case r.AssertionResults != nil && !r.AssertionResults.Passed:
			s.Failed++
			s.AssertionFailures++
			if r.Phase == PhaseTeardown {
				s.TeardownErrors++
				s.TeardownAssertionErrors++
			}
		default:
			s.Passed++
		}
	}
}

// resolveRetryConfig merges all precedence levels and produces a concrete Config.
func resolveRetryConfig(
	global *retry.FullConfig,
	collection *retry.FullConfig,
	section *retry.FullConfig,
	request *retry.FullConfig,
) retry.Config {
	configs := []retry.FullConfig{retry.BuiltinDefaults()}
	if global != nil {
		configs = append(configs, *global)
	}
	if collection != nil {
		configs = append(configs, *collection)
	}
	if section != nil {
		configs = append(configs, *section)
	}
	if request != nil {
		configs = append(configs, *request)
	}
	return retry.MergeAll(configs...).Resolve()
}

// outcomeString maps a RequestResult to one of "passed", "failed", "error", or "skipped".
func outcomeString(rr RequestResult) string {
	if rr.Skipped {
		return "skipped"
	}
	if rr.Err != nil {
		return "error"
	}
	if rr.AssertionResults != nil && !rr.AssertionResults.Passed {
		return "failed"
	}
	return "passed"
}

// emitRequestEnd fires RequestEnd on the sink with the appropriate fields from rr.
// waveIndex is -1 for sequential requests.
func emitRequestEnd(sink EventSink, reqID, reqSlug string, rr RequestResult, waveIndex int) {
	if sink == nil {
		return
	}
	var statusCode int
	var duration time.Duration
	var respBody []byte
	var respHeaders http.Header
	var timing *httpexec.Timing
	if rr.Result != nil {
		statusCode = rr.Result.StatusCode
		duration = rr.Result.Duration
		respBody = rr.Result.Body
		respHeaders = rr.Result.Headers
		timing = rr.Result.Timing
	}
	var reqBodyBytes []byte
	if rr.RequestBody != nil {
		switch b := rr.RequestBody.(type) {
		case []byte:
			reqBodyBytes = b
		case string:
			reqBodyBytes = []byte(b)
		default:
			// For map[string]any and other JSON-serialisable types, marshal to JSON.
			if encoded, err := json.Marshal(rr.RequestBody); err == nil {
				reqBodyBytes = encoded
			}
		}
	}
	sink.RequestEnd(RequestEndEvent{
		RequestID:       reqID,
		RequestSlug:     reqSlug,
		Outcome:         outcomeString(rr),
		StatusCode:      statusCode,
		Duration:        duration,
		WaveIndex:       waveIndex,
		RequestBody:     reqBodyBytes,
		ResponseBody:    respBody,
		Err:             rr.Err,
		RequestHeaders:  rr.RequestHeaders,
		ResponseHeaders: respHeaders,
		Timing:          timing,
		Attempts:        rr.RetryCount + 1,
	})
}

// emitAssertionResults fires one AssertionResult per assertion item in ar.
func emitAssertionResults(sink EventSink, reqID, reqSlug, sourceFile string, ar *assertion.Results) {
	if sink == nil || ar == nil {
		return
	}
	for _, a := range ar.Items {
		sink.AssertionResult(AssertionEvent{
			RequestID:   reqID,
			RequestSlug: reqSlug,
			SourceFile:  sourceFile,
			SourceLine:  a.SourceLine,
			Type:        a.Type,
			Target:      a.Target,
			Operator:    a.Operator,
			Expected:    a.Expected,
			Actual:      a.Actual,
			Passed:      a.Passed,
		})
	}
}

// parallelSinkAdapter wraps a runner.EventSink to satisfy parallel.EventSink,
// bridging the two interfaces without creating an import cycle.
type parallelSinkAdapter struct {
	inner EventSink
}

func (a *parallelSinkAdapter) RequestStart(requestID, requestSlug, name, method, url, phase, sourceFile string, sourceLine, waveIndex int) {
	a.inner.RequestStart(RequestEvent{
		RequestID:   requestID,
		RequestSlug: requestSlug,
		Name:        name,
		Method:      method,
		URL:         url,
		Phase:       phase,
		SourceFile:  sourceFile,
		SourceLine:  sourceLine,
	})
}

func (a *parallelSinkAdapter) RequestEnd(requestID, requestSlug, outcome string, statusCode int, duration time.Duration, waveIndex int, reqBody, respBody []byte, err error, extra *parallel.EndExtra) {
	ev := RequestEndEvent{
		RequestID:    requestID,
		RequestSlug:  requestSlug,
		Outcome:      outcome,
		StatusCode:   statusCode,
		Duration:     duration,
		WaveIndex:    waveIndex,
		RequestBody:  reqBody,
		ResponseBody: respBody,
		Err:          err,
		Attempts:     1,
	}
	if extra != nil {
		ev.RequestHeaders = extra.RequestHeaders
		ev.ResponseHeaders = extra.ResponseHeaders
		ev.Timing = extra.Timing
		if extra.Attempts > 0 {
			ev.Attempts = extra.Attempts
		}
	}
	a.inner.RequestEnd(ev)
}

func (a *parallelSinkAdapter) AssertionResult(ev parallel.AssertionEvent) {
	a.inner.AssertionResult(AssertionEvent{
		RequestID:   ev.RequestID,
		RequestSlug: ev.RequestSlug,
		SourceFile:  ev.SourceFile,
		SourceLine:  ev.SourceLine,
		Type:        ev.Type,
		Target:      ev.Target,
		Operator:    ev.Operator,
		Expected:    ev.Expected,
		Actual:      ev.Actual,
		Passed:      ev.Passed,
	})
}
