package parser

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/weiqigod/curlew/internal/assertion"
	apierrors "github.com/weiqigod/curlew/internal/errors"
	graphqlfiles "github.com/weiqigod/curlew/internal/graphql/files"
	"github.com/weiqigod/curlew/internal/httpbody"
	wstemplates "github.com/weiqigod/curlew/internal/websocket/templates"
	"gopkg.in/yaml.v3"
)

// The parser validates four fields against closed sets of values. Each set is
// duplicated as an enum in schemas/collection-v1.json and quoted in the error
// hint the user sees, so the lists live here and the duplicates are pinned to
// them by TestSchema_enums_match_parser and the tests in closedsets_test.go.
// An empty value always means "unset" and resolves to the documented default.
var (
	// SupportedProtocols lists the values request.protocol accepts. Unset
	// resolves to http.
	SupportedProtocols = []string{"http", "graphql", "websocket"}

	// WebSocketActions lists the values a websocket step's action: accepts.
	WebSocketActions = []string{"send", "expect", "wait", "close"}

	// WebSocketBackoffStrategies lists the values websocket.reconnect.backoff
	// accepts. Unset resolves to exponential.
	WebSocketBackoffStrategies = []string{"exponential"}

	// GraphQLErrorHandlingValues lists the values graphql.error_handling
	// accepts. Unset resolves to fail.
	GraphQLErrorHandlingValues = []string{"fail", "warn", "ignore"}
)

// inClosedSet reports whether value is unset or a member of set.
func inClosedSet(set []string, value string) bool {
	return value == "" || slices.Contains(set, value)
}

// ParseFile reads and parses a collection YAML file.
func ParseFile(path string) (*Collection, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, &apierrors.Structured{
				Category: apierrors.CategoryParse,
				FilePath: path,
				Message:  fmt.Sprintf("file not found: %s", path),
				Inner:    ErrFileNotFound,
			}
		}
		return nil, &apierrors.Structured{
			Category: apierrors.CategoryParse,
			FilePath: path,
			Message:  fmt.Sprintf("reading file: %s", err),
			Inner:    err,
		}
	}

	col, err := parseCollectionBytes(path, data)
	if err != nil {
		return nil, err
	}

	// M3-004: JSON Schema assertion compilation.
	if collectionHasSchema(col) {
		if compileErr := compileSchemas(col, path); compileErr != nil {
			return nil, compileErr
		}
	}

	// M3-003: include directive resolution.
	if len(col.Include) > 0 {
		absParent, absErr := filepath.Abs(path)
		if absErr != nil {
			return nil, fmt.Errorf("resolving parent path %q: %w", path, absErr)
		}
		var includeExt []string
		incCtx := &includeContext{
			cumulativeVars: nil,
			visited:        map[string]bool{absParent: true},
			extFiles:       &includeExt,
		}
		if err := resolveIncludes(col, path, incCtx); err != nil {
			return nil, err
		}
		col.ExternalFiles = append(col.ExternalFiles, includeExt...)
	}

	// M8-004: reject duplicate main request names. Runs after include resolution
	// so items spliced via include: are covered.
	if err := checkDuplicateNames(col, path); err != nil {
		return nil, err
	}

	// M9-001: derive request slugs and reject names that slugify to empty.
	// Runs after duplicate-name validation so duplicate detection always wins
	// when both apply. Walks every phase (setup, main, teardown) because every
	// emitted request.start/request.end event must carry a non-empty slug.
	if err := populateSlugs(col, path); err != nil {
		return nil, err
	}

	// M19-001: validate depends_on: references after slugs are populated so
	// all names are final.
	if err := validateDependsOn(col, path); err != nil {
		return nil, err
	}

	return col, nil
}

// populateSlugs derives Slug for every RequestItem in setup/main/teardown
// and writes the result back via in-place mutation through the section
// pointer. Returns a structured error on the first item whose name
// slugifies to empty.
func populateSlugs(col *Collection, rootPath string) error {
	sections := []*[]RequestItem{
		&col.Setup.Items,
		&col.Requests.Items,
		&col.Teardown.Items,
	}
	for _, section := range sections {
		for i := range *section {
			item := &(*section)[i]
			if item.Name == "" {
				// Empty names are caught by other validators (duplicate-name
				// skips them, request-validation rejects them). Skip to keep
				// error attribution clean.
				continue
			}
			s, err := Slug(item.Name)
			if err != nil {
				return &apierrors.Structured{
					Category: apierrors.CategoryParse,
					FilePath: item.SourceFile,
					Line:     item.SourceLine,
					Message: fmt.Sprintf(
						"request name %q slugifies to empty (no alphanumeric runes after Unicode normalization)",
						item.Name,
					),
					Hint:  "Rename the request to include at least one ASCII letter or digit. Slugs are used as filenames for per-request markdown reports.",
					Inner: ErrSlugEmpty,
				}
			}
			item.Slug = s
		}
	}
	return nil
}

// checkDuplicateNames returns an error if any two main-phase items share the
// same name. Setup and teardown items are excluded — per spec, duplicate names
// across phases are intentional (e.g. a setup "Create user" and a main
// "Create user" are distinct test stages).
func checkDuplicateNames(col *Collection, rootPath string) error {
	type loc struct {
		file string
		line int
	}
	first := make(map[string]loc, len(col.Requests.Items))
	for _, item := range col.Requests.Items {
		if item.Name == "" {
			continue // empty names are caught by other validators
		}
		l := loc{file: item.SourceFile, line: item.SourceLine}
		if prev, dup := first[item.Name]; dup {
			return &apierrors.Structured{
				Category: apierrors.CategoryParse,
				FilePath: rootPath,
				Line:     prev.line,
				Message: fmt.Sprintf(
					"duplicate request name %q at %s:%d and %s:%d",
					item.Name, prev.file, prev.line, l.file, l.line,
				),
				Hint:  "Rename one of the duplicated requests. Main request names must be unique because --only, per-request markdown files, and CI report rows all address requests by name.",
				Inner: ErrDuplicateRequestName,
			}
		}
		first[item.Name] = l
	}
	return nil
}

// parseCollectionBytes parses and fully validates a collection from raw YAML
// bytes. It resolves external file references, GraphQL query files, WebSocket
// templates, and validates all request fields. It does NOT resolve include:
// directives — that is the caller's responsibility (ParseFileWithOptions).
func parseCollectionBytes(path string, data []byte) (*Collection, error) {
	var col Collection
	if err := yaml.Unmarshal(data, &col); err != nil {
		return nil, &apierrors.Structured{
			Category: apierrors.CategoryParse,
			FilePath: path,
			Line:     extractYAMLLine(err),
			Message:  fmt.Sprintf("invalid YAML syntax: %s", err),
			Inner:    fmt.Errorf("%w: %w", ErrInvalidYAML, err),
		}
	}

	if col.Name == "" {
		return nil, &apierrors.Structured{
			Category: apierrors.CategoryParse,
			FilePath: path,
			Message:  "collection has no name",
			Inner:    ErrEmptyCollection,
		}
	}

	// Resolve external file references before validation
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolving collection path %q: %w", path, err)
	}
	visited := map[string]bool{absPath: true}
	for _, section := range []*[]RequestItem{&col.Setup.Items, &col.Requests.Items, &col.Teardown.Items} {
		if len(*section) == 0 {
			continue
		}
		var extFiles []string
		*section, extFiles, err = resolveExternalReferences(path, *section, visited)
		if err != nil {
			return nil, &apierrors.Structured{
				Category: apierrors.CategoryParse,
				FilePath: path,
				Message:  err.Error(),
				Inner:    err,
			}
		}
		col.ExternalFiles = append(col.ExternalFiles, extFiles...)
	}

	// M6-002: stamp SourceFile on every item that does not already have one.
	// Items loaded via `path:` (resolveExternalReferences) are already stamped
	// with their own external file path and must not be overwritten.
	stampSourceFile(&col, absPath)

	// Resolve external GraphQL query and fragment files. Runs after external
	// request resolution (so requests loaded via `path:` are also processed)
	// and before the GraphQL validation block below, which expects
	// GraphQLConfig.Query to be populated.
	collectionDir := filepath.Dir(absPath)
	for _, section := range []*[]RequestItem{&col.Setup.Items, &col.Requests.Items, &col.Teardown.Items} {
		for i := range *section {
			gql := (*section)[i].Request.GraphQL
			if gql == nil {
				continue
			}
			if gql.QueryFile == "" && len(gql.Fragments) == 0 {
				continue
			}
			res, loadErr := graphqlfiles.LoadQuery(graphqlfiles.LoadQueryInput{
				BaseDir:     collectionDir,
				InlineQuery: gql.Query,
				QueryFile:   gql.QueryFile,
				Fragments:   gql.Fragments,
			})
			if loadErr != nil {
				return nil, &apierrors.Structured{
					Category: apierrors.CategoryParse,
					FilePath: path,
					Message:  fmt.Sprintf("graphql request %q: %s", (*section)[i].Name, loadErr),
					Hint:     "Check graphql.query_file and graphql.fragments paths",
					Inner:    loadErr,
				}
			}
			gql.Query = res.Query
			col.ExternalFiles = append(col.ExternalFiles, res.FilePaths...)
		}
	}

	// Load external WebSocket message templates, then validate WebSocket step fields.
	for _, section := range []*[]RequestItem{&col.Setup.Items, &col.Requests.Items, &col.Teardown.Items} {
		for i := range *section {
			ws := (*section)[i].Request.WebSocket
			if ws == nil {
				continue
			}
			for j := range ws.Steps {
				step := &ws.Steps[j]
				if step.MessageTemplate != "" {
					body, abs, loadErr := wstemplates.LoadTemplate(collectionDir, step.MessageTemplate)
					if loadErr != nil {
						return nil, &apierrors.Structured{
							Category: apierrors.CategoryParse,
							FilePath: path,
							Message:  fmt.Sprintf("websocket request %q step %d: %s", (*section)[i].Name, j+1, loadErr),
							Hint:     "Check the message_template path",
							Inner:    loadErr,
						}
					}
					step.MessageRawTemplate = body
					col.ExternalFiles = append(col.ExternalFiles, abs)
				}
				// Validate send mutual exclusion.
				if step.Action == "send" {
					hasMsg := step.Message != nil
					hasRaw := step.MessageRaw != ""
					hasTpl := step.MessageTemplate != ""
					if moreThanOneTrue(hasMsg, hasRaw, hasTpl) {
						return nil, &apierrors.Structured{
							Category: apierrors.CategoryParse,
							FilePath: path,
							Message:  fmt.Sprintf("websocket request %q step %d: message, message_raw, and message_template are mutually exclusive", (*section)[i].Name, j+1),
							Inner:    ErrInvalidFieldValue,
						}
					}
					if len(step.StepVariables) > 0 && !hasTpl {
						return nil, &apierrors.Structured{
							Category: apierrors.CategoryParse,
							FilePath: path,
							Message:  fmt.Sprintf("websocket request %q step %d: variables only allowed with message_template", (*section)[i].Name, j+1),
							Hint:     "Use message_template: to specify a template file when using variables",
							Inner:    ErrInvalidFieldValue,
						}
					}
				}
				// Validate expect mutual exclusion.
				if step.Action == "expect" && len(step.AnyOf) > 0 && len(step.ExpectAssertions.Items) > 0 {
					return nil, &apierrors.Structured{
						Category: apierrors.CategoryParse,
						FilePath: path,
						Message:  fmt.Sprintf("websocket request %q step %d: message and any_of are mutually exclusive", (*section)[i].Name, j+1),
						Inner:    ErrInvalidFieldValue,
					}
				}
				// Validate count.
				if step.Action == "expect" && step.Count < 0 {
					return nil, &apierrors.Structured{
						Category: apierrors.CategoryParse,
						FilePath: path,
						Message:  fmt.Sprintf("websocket request %q step %d: expect.count must be >= 0", (*section)[i].Name, j+1),
						Inner:    ErrInvalidFieldValue,
					}
				}
			}
		}
	}

	// Load external body files for HTTP requests. Mirrors the graphql.query_file
	// and websocket.message_template patterns: parse-time load so `validate`
	// catches missing files; placeholders are preserved for runtime
	// interpolation (text variant only). Binary variant is sent verbatim.
	for _, section := range []*[]RequestItem{&col.Setup.Items, &col.Requests.Items, &col.Teardown.Items} {
		for i := range *section {
			req := &(*section)[i].Request
			// GraphQL and WebSocket populate Body differently; body_file is
			// meaningless for them.
			if req.Protocol == "graphql" || req.Protocol == "websocket" {
				continue
			}

			hasBody := req.Body != nil
			hasText := req.BodyFile != ""
			hasBin := req.BodyBinaryFile != ""
			if moreThanOneTrue(hasBody, hasText, hasBin) {
				return nil, &apierrors.Structured{
					Category: apierrors.CategoryParse,
					FilePath: path,
					Message:  fmt.Sprintf("request %q: body, body_file, and body_binary_file are mutually exclusive", (*section)[i].Name),
					Hint:     "Use at most one of body, body_file, or body_binary_file",
					Inner:    ErrInvalidFieldValue,
				}
			}

			if hasText {
				res, loadErr := httpbody.LoadBody(httpbody.LoadBodyInput{
					BaseDir:  collectionDir,
					BodyFile: req.BodyFile,
				})
				if loadErr != nil {
					return nil, &apierrors.Structured{
						Category: apierrors.CategoryParse,
						FilePath: path,
						Message:  fmt.Sprintf("request %q: %s", (*section)[i].Name, loadErr),
						Hint:     "Check the body_file path and file size",
						Inner:    loadErr,
					}
				}
				req.BodyFileContent = string(res.Body)
				req.BodyFileContentType = res.ContentType
				col.ExternalFiles = append(col.ExternalFiles, res.FilePath)
			}
			if hasBin {
				res, loadErr := httpbody.LoadBody(httpbody.LoadBodyInput{
					BaseDir:  collectionDir,
					BodyFile: req.BodyBinaryFile,
				})
				if loadErr != nil {
					return nil, &apierrors.Structured{
						Category: apierrors.CategoryParse,
						FilePath: path,
						Message:  fmt.Sprintf("request %q: %s", (*section)[i].Name, loadErr),
						Hint:     "Check the body_binary_file path and file size",
						Inner:    loadErr,
					}
				}
				req.BodyBinaryFileContent = res.Body
				ct := res.ContentType
				if ct == "" {
					ct = "application/octet-stream"
				}
				req.BodyFileContentType = ct
				col.ExternalFiles = append(col.ExternalFiles, res.FilePath)
			}
		}
	}

	// Auto-detect WebSocket protocol from URL scheme before method validation.
	// When protocol is unset and the URL starts with ws:// or wss://, set
	// Protocol to "websocket" so the existing validation path handles it.
	// An explicit protocol is always honoured — auto-detection never overrides.
	for _, section := range []*[]RequestItem{&col.Setup.Items, &col.Requests.Items, &col.Teardown.Items} {
		for i := range *section {
			req := &(*section)[i].Request
			if req.Protocol == "" {
				lower := strings.ToLower(req.URL)
				if strings.HasPrefix(lower, "ws://") || strings.HasPrefix(lower, "wss://") {
					req.Protocol = "websocket"
				}
			}
		}
	}

	for _, section := range []*[]RequestItem{&col.Setup.Items, &col.Requests.Items, &col.Teardown.Items} {
		for i := range *section {
			// WebSocket requests do not use HTTP method validation; the default
			// "WS" display method is assigned during protocol validation below.
			if (*section)[i].Request.Protocol == "websocket" {
				continue
			}
			method, err := normalizeAndValidateMethod((*section)[i].Request.Method)
			if err != nil {
				return nil, &apierrors.Structured{
					Category: apierrors.CategoryParse,
					FilePath: path,
					Message:  fmt.Sprintf("unsupported HTTP method %q in request %q", (*section)[i].Request.Method, (*section)[i].Name),
					Hint:     "Allowed methods: GET, POST, PUT, DELETE, PATCH, HEAD, OPTIONS",
					Inner:    err,
				}
			}
			(*section)[i].Request.Method = method
		}
	}

	// Validate protocol and graphql fields
	for _, section := range []*[]RequestItem{&col.Setup.Items, &col.Requests.Items, &col.Teardown.Items} {
		for i := range *section {
			protocol := (*section)[i].Request.Protocol
			if !inClosedSet(SupportedProtocols, protocol) {
				return nil, &apierrors.Structured{
					Category: apierrors.CategoryParse,
					FilePath: path,
					Message:  fmt.Sprintf("unsupported protocol %q in request %q", protocol, (*section)[i].Name),
					Hint:     "Allowed protocols: " + strings.Join(SupportedProtocols, ", "),
					Inner:    ErrUnsupportedProtocol,
				}
			}
			if protocol == "websocket" {
				ws := (*section)[i].Request.WebSocket
				if ws == nil || len(ws.Steps) == 0 {
					return nil, &apierrors.Structured{
						Category: apierrors.CategoryParse,
						FilePath: path,
						Message:  fmt.Sprintf("websocket request %q must have websocket.steps with at least one action", (*section)[i].Name),
						Hint:     "Add websocket: steps: [...] to your request",
						Inner:    ErrMissingRequiredField,
					}
				}
				for j, step := range ws.Steps {
					if !slices.Contains(WebSocketActions, step.Action) {
						return nil, &apierrors.Structured{
							Category: apierrors.CategoryParse,
							FilePath: path,
							Message:  fmt.Sprintf("unsupported websocket action %q in request %q step %d", step.Action, (*section)[i].Name, j+1),
							Hint:     "Allowed actions: " + strings.Join(WebSocketActions, ", "),
							Inner:    ErrInvalidFieldValue,
						}
					}
				}
				// Default method for display purposes (WebSocket has no HTTP verb).
				if (*section)[i].Request.Method == "" {
					(*section)[i].Request.Method = "WS"
				}
				// Validate optional reconnect config.
				if ws.Reconnect != nil {
					backoff := ws.Reconnect.Backoff
					if !inClosedSet(WebSocketBackoffStrategies, backoff) {
						return nil, &apierrors.Structured{
							Category: apierrors.CategoryParse,
							FilePath: path,
							Message:  fmt.Sprintf("unsupported reconnect.backoff %q in request %q", backoff, (*section)[i].Name),
							Hint:     "Allowed values: exponential (default)",
							Inner:    ErrInvalidFieldValue,
						}
					}
				}
				// Validate optional heartbeat config.
				if ws.Heartbeat != nil && ws.Heartbeat.Enabled {
					if ws.Heartbeat.IntervalMs <= 0 {
						return nil, &apierrors.Structured{
							Category: apierrors.CategoryParse,
							FilePath: path,
							Message:  fmt.Sprintf("heartbeat.interval_ms must be > 0 in request %q", (*section)[i].Name),
							Hint:     "Set interval_ms to a positive value (e.g. 30000 for 30 seconds)",
							Inner:    ErrInvalidFieldValue,
						}
					}
				}
			}
			if protocol == "graphql" && ((*section)[i].Request.GraphQL == nil || (*section)[i].Request.GraphQL.Query == "") {
				return nil, &apierrors.Structured{
					Category: apierrors.CategoryParse,
					FilePath: path,
					Message:  fmt.Sprintf("graphql request %q must have a graphql.query field", (*section)[i].Name),
					Hint:     "Add graphql: query: |... to your request",
					Inner:    ErrMissingRequiredField,
				}
			}
			// Validate graphql.error_handling values
			if protocol == "graphql" && (*section)[i].Request.GraphQL != nil {
				eh := (*section)[i].Request.GraphQL.ErrorHandling
				if !inClosedSet(GraphQLErrorHandlingValues, eh) {
					return nil, &apierrors.Structured{
						Category: apierrors.CategoryParse,
						FilePath: path,
						Message:  fmt.Sprintf("invalid error_handling value %q in request %q", eh, (*section)[i].Name),
						Hint:     "Allowed values: " + strings.Join(GraphQLErrorHandlingValues, ", "),
						Inner:    ErrInvalidFieldValue,
					}
				}
			}
			// GraphQL requests don't require method validation - BuildRequest forces POST
			if protocol == "graphql" && (*section)[i].Request.Method == "" {
				(*section)[i].Request.Method = "POST"
			}
		}
	}

	for _, section := range [][]RequestItem{col.Setup.Items, col.Requests.Items, col.Teardown.Items} {
		if err := validateRequests(path, section); err != nil {
			return nil, err
		}
	}

	// Validate rate_limit_rps: must be >= 0.
	if col.RateLimitRPS < 0 {
		line := findTopLevelKeyLine(data, "rate_limit_rps")
		return nil, &apierrors.Structured{
			Category: apierrors.CategoryParse,
			FilePath: path,
			Line:     line,
			Message:  fmt.Sprintf("rate_limit_rps must be >= 0, got %d", col.RateLimitRPS),
			Hint:     "Use 0 (or omit the field) to disable throttling",
			Inner:    ErrInvalidFieldValue,
		}
	}

	return &col, nil
}

var allowedMethods = map[string]bool{
	"GET": true, "POST": true, "PUT": true, "DELETE": true,
	"PATCH": true, "HEAD": true, "OPTIONS": true,
}

func normalizeAndValidateMethod(method string) (string, error) {
	if method == "" {
		return "GET", nil
	}
	upper := strings.ToUpper(method)
	if !allowedMethods[upper] {
		return "", fmt.Errorf("%w: %s", ErrUnsupportedMethod, method)
	}
	return upper, nil
}

func validateRequests(path string, items []RequestItem) error {
	for _, item := range items {
		if item.Request.URL == "" {
			return &apierrors.Structured{
				Category: apierrors.CategoryParse,
				FilePath: path,
				Message:  fmt.Sprintf("request %q is missing required field 'url'", item.Name),
				Hint:     "Every request must specify a url field",
				Inner:    ErrMissingRequiredField,
			}
		}
	}
	return nil
}

// moreThanOneTrue reports whether more than one of the given booleans is true.
func moreThanOneTrue(vals ...bool) bool {
	count := 0
	for _, v := range vals {
		if v {
			count++
		}
	}
	return count > 1
}

// yamlLineRe matches "yaml: line N:" from yaml.v3 error messages.
var yamlLineRe = regexp.MustCompile(`line (\d+)`)

func extractYAMLLine(err error) int {
	m := yamlLineRe.FindStringSubmatch(err.Error())
	if len(m) < 2 {
		return 0
	}
	n, parseErr := strconv.Atoi(m[1])
	if parseErr != nil {
		return 0
	}
	return n
}

// findTopLevelKeyLine re-parses data as a YAML document node and returns the
// line number of the given top-level mapping key. Returns 0 if not found or
// on any parse error. Used to attach line numbers to post-unmarshal validation
// errors without decoding the full document into a yaml.Node on the hot path.
func findTopLevelKeyLine(data []byte, key string) int {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return 0
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return 0
	}
	mapping := doc.Content[0]
	if mapping.Kind != yaml.MappingNode {
		return 0
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i].Line
		}
	}
	return 0
}

// validateDependsOn checks that every depends_on: reference names an item that
// exists in the collection (across all phases). Unknown names produce a
// structured ErrUnknownDependsOn error. Called after populateSlugs so all
// item names are final.
func validateDependsOn(col *Collection, rootPath string) error {
	// Build a set of all known request item names across every phase.
	names := make(map[string]struct{})
	for _, section := range [][]RequestItem{col.Setup.Items, col.Requests.Items, col.Teardown.Items} {
		for _, item := range section {
			if item.Name != "" {
				names[item.Name] = struct{}{}
			}
		}
	}
	// Validate all depends_on lists.
	for _, section := range [][]RequestItem{col.Setup.Items, col.Requests.Items, col.Teardown.Items} {
		for _, item := range section {
			for _, dep := range item.DependsOn {
				if _, ok := names[dep]; !ok {
					return &apierrors.Structured{
						Category: apierrors.CategoryParse,
						FilePath: item.SourceFile,
						Line:     item.SourceLine,
						Message: fmt.Sprintf(
							"request %q: depends_on %q does not match any request name",
							item.Name, dep,
						),
						Hint:  "depends_on must reference an item by its name (case-sensitive). Names are unique within main. A setup or teardown name parses, but it orders nothing — phases already run in order and skip propagation is within a phase — and under --parallel it is rejected (§11.4).",
						Inner: ErrUnknownDependsOn,
					}
				}
			}
		}
	}
	return nil
}

// collectionHasSchema reports whether any request item in the collection has
// a non-empty assertions.schema field.
func collectionHasSchema(col *Collection) bool {
	for _, section := range [][]RequestItem{col.Setup.Items, col.Requests.Items, col.Teardown.Items} {
		for _, item := range section {
			if item.Assertions.Schema != "" {
				return true
			}
		}
	}
	return false
}

// compileSchemas walks all sections of col and compiles each request's
// assertions.schema path into a *assertion.CompiledSchema. Schema paths
// are resolved relative to the directory containing the collection file at
// collectionPath. Any compilation failure returns a structured parse error.
func compileSchemas(col *Collection, collectionPath string) error {
	collectionDir := filepath.Dir(collectionPath)
	for _, section := range []*[]RequestItem{&col.Setup.Items, &col.Requests.Items, &col.Teardown.Items} {
		for i := range *section {
			raw := (*section)[i].Assertions.Schema
			if raw == "" {
				continue
			}
			absPath := raw
			if !filepath.IsAbs(absPath) {
				absPath = filepath.Join(collectionDir, raw)
			}
			compiled, err := assertion.CompileSchemaFile(absPath)
			if err != nil {
				return &apierrors.Structured{
					Category: apierrors.CategoryParse,
					FilePath: collectionPath,
					Message:  fmt.Sprintf("request %q: compiling schema %q: %s", (*section)[i].Name, raw, err),
					Inner:    err,
				}
			}
			(*section)[i].Assertions.CompiledSchema = compiled
			col.ExternalFiles = append(col.ExternalFiles, absPath)
		}
	}
	return nil
}

// stampSourceFile sets SourceFile on every RequestItem that does not already
// have one. Called by parseCollectionBytes with the absolute path of the
// collection file being parsed. Items loaded from external request files
// (resolveExternalReferences) or from included collections (resolveIncludes)
// will already have their own SourceFile set and must be left alone.
func stampSourceFile(col *Collection, absPath string) {
	for _, section := range []*[]RequestItem{&col.Setup.Items, &col.Requests.Items, &col.Teardown.Items} {
		for i := range *section {
			if (*section)[i].SourceFile == "" {
				(*section)[i].SourceFile = absPath
			}
		}
	}
}
