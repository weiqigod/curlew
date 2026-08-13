package openapi

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/getkin/kin-openapi/openapi3"
	apierrors "github.com/weiqigod/curlew/internal/errors"
	"github.com/weiqigod/curlew/internal/parser"
	"github.com/weiqigod/curlew/internal/variable"
)

// methodOrder is the fixed iteration order for HTTP methods within a path.
// Keeping this stable makes generated collections deterministic.
var methodOrder = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS", "TRACE"}

var pathParamRE = regexp.MustCompile(`\{([^{}/]+)\}`)

var reMultiUnderscore = regexp.MustCompile(`_+`)

// stderrWarn writes a warning message to os.Stderr.
func stderrWarn(msg string) {
	fmt.Fprintln(os.Stderr, "warning:", msg)
}

// Import parses an OpenAPI 3.0 or 3.1 spec file and returns a collection with
// headers, request bodies, and status-code assertions derived from the spec.
func Import(specPath string) (*parser.Collection, error) {
	return importWithWarn(specPath, stderrWarn)
}

// importWithWarn is the testable core of Import; warn receives warning messages
// instead of writing to os.Stderr.
func importWithWarn(specPath string, warn func(string)) (*parser.Collection, error) {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = false

	raw, err := os.ReadFile(specPath)
	if err != nil {
		return nil, &apierrors.Structured{
			Category: apierrors.CategoryParse,
			FilePath: specPath,
			Message:  fmt.Sprintf("reading openapi spec: %v", err),
			Inner:    fmt.Errorf("%w: %v", ErrSpecInvalid, err),
		}
	}
	// A 3.1 document is translated into the 3.0 spelling of the same meaning
	// before it reaches kin-openapi, whose validator implements 3.0 (§11C.9).
	// A 3.0 document passes through untouched.
	relaxed, err := relax31(raw, warn)
	if err != nil {
		return nil, &apierrors.Structured{
			Category: apierrors.CategoryParse,
			FilePath: specPath,
			Message:  err.Error(),
			Inner:    fmt.Errorf("%w: %v", ErrSpecInvalid, err),
		}
	}

	doc, err := loader.LoadFromData(relaxed)
	if err != nil {
		return nil, &apierrors.Structured{
			Category: apierrors.CategoryParse,
			FilePath: specPath,
			Message:  fmt.Sprintf("loading openapi spec: %v", err),
			Inner:    fmt.Errorf("%w: %v", ErrSpecInvalid, err),
		}
	}
	if err := doc.Validate(loader.Context); err != nil {
		return nil, &apierrors.Structured{
			Category: apierrors.CategoryParse,
			FilePath: specPath,
			Message:  fmt.Sprintf("validating openapi spec: %v", err),
			Inner:    fmt.Errorf("%w: %v", ErrSpecInvalid, err),
		}
	}

	baseURL := ""
	if len(doc.Servers) > 0 {
		baseURL = doc.Servers[0].URL
	}

	name := "Imported Collection"
	if doc.Info != nil && doc.Info.Title != "" {
		name = doc.Info.Title
	}

	col := &parser.Collection{
		Name: name,
		Variables: parser.SensitiveVars{
			Values:    map[string]string{"base_url": baseURL},
			Sensitive: variable.NewSensitiveSet(),
			Commands:  map[string]parser.CommandVar{},
		},
	}

	// Sort paths for determinism.
	var paths []string
	if doc.Paths != nil {
		for p := range doc.Paths.Map() {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)

	walker := &schemaWalker{warn: warn, warnedRefs: map[string]bool{}}
	used := map[string]int{}
	collVarsSeen := map[string]bool{"base_url": true} // pre-seed to avoid clobbering base_url
	var items []parser.RequestItem

	for _, path := range paths {
		pi := doc.Paths.Find(path)
		if pi == nil {
			continue
		}
		ops := pi.Operations()
		for _, method := range methodOrder {
			op, ok := ops[method]
			if !ok {
				continue
			}
			url, headers, newVars := collectParameters(op, pi, "{{base_url}}"+interpolatePath(path), collVarsSeen, walker)
			for k, v := range newVars {
				col.Variables.Values[k] = v
			}
			body := buildRequestBody(op, walker)
			statusCodes := extractStatusCodes(op.Responses)

			item := parser.RequestItem{
				Name: chooseName(op, method, path, used),
				Request: parser.Request{
					Method:  method,
					URL:     url,
					Headers: headers,
					Body:    body,
				},
			}
			if len(statusCodes) > 0 {
				item.Assertions.Status = parser.StatusCodes{Codes: statusCodes}
			}
			items = append(items, item)
		}
	}

	if len(items) == 0 {
		return nil, &apierrors.Structured{
			Category: apierrors.CategoryParse,
			FilePath: specPath,
			Message:  "openapi spec contains no operations",
			Inner:    ErrNoOperations,
		}
	}
	col.Requests = parser.Section{Items: items}
	return col, nil
}

// collectParameters extracts header and query parameters from an operation
// (merging path-level and operation-level params, operation wins on conflict).
// Returns the (possibly query-appended) URL, request headers map (nil when
// empty), and new collection variables to add. collVarsSeen tracks
// already-added variable names so shared params across operations become a
// single collection variable.
func collectParameters(
	op *openapi3.Operation,
	pi *openapi3.PathItem,
	baseURL string,
	collVarsSeen map[string]bool,
	w *schemaWalker,
) (url string, headers, newVars map[string]string) {
	// Merge path-level params with operation-level (operation overrides).
	merged := map[string]*openapi3.Parameter{}
	for _, ref := range pi.Parameters {
		if ref != nil && ref.Value != nil {
			merged[ref.Value.Name] = ref.Value
		}
	}
	for _, ref := range op.Parameters {
		if ref != nil && ref.Value != nil {
			merged[ref.Value.Name] = ref.Value
		}
	}

	hdrs := map[string]string{}
	newVars = map[string]string{}
	var queryParts []string

	// Sort for determinism.
	var names []string
	for n := range merged {
		names = append(names, n)
	}
	sort.Strings(names)

	for _, name := range names {
		p := merged[name]
		switch p.In {
		case "header":
			varName := snakeCase(name)
			hdrs[name] = "{{" + varName + "}}"
			if !collVarsSeen[varName] {
				collVarsSeen[varName] = true
				newVars[varName] = ""
			}
		case "query":
			varName := snakeCase(name)
			queryParts = append(queryParts, name+"={{"+varName+"}}")
			if !collVarsSeen[varName] {
				collVarsSeen[varName] = true
				newVars[varName] = ""
			}
		case "path":
			// interpolatePath has already turned {code} in the URL into
			// {{code}}. Without a matching variable the generated collection
			// cannot run at all: it exits 5 with `undefined variable "code"`,
			// at run time rather than at import time, and §9.P's criterion —
			// that `curlew run` on the import's own output passes — was
			// unreachable for any path with a parameter (§11C.10).
			//
			// A path parameter has no sensible empty value: an empty string
			// yields /status/ rather than /status/200. The default comes from
			// the document — example, enum, default, then the schema's type —
			// which is the same order buildRequestBody uses for bodies.
			//
			// The variable keeps the parameter's own name rather than being
			// snake-cased like header and query parameters, because
			// interpolatePath has already written that exact name into the URL
			// as {{petId}}. The two have to agree, and the URL is the side that
			// is already published.
			varName := name
			if !collVarsSeen[varName] {
				collVarsSeen[varName] = true
				newVars[varName] = pathParamDefault(p, w)
			}
			// "cookie" is ignored.
		}
	}

	if len(hdrs) > 0 {
		headers = hdrs
	}
	url = baseURL
	if len(queryParts) > 0 {
		sort.Strings(queryParts)
		url += "?" + strings.Join(queryParts, "&")
	}
	return url, headers, newVars
}

// buildRequestBody generates a placeholder request body from the operation's
// requestBody definition. Returns nil if no requestBody is defined.
// Prefers explicit examples over schema-derived placeholders.
// Only application/json (or json-like) media types are used.
func buildRequestBody(op *openapi3.Operation, w *schemaWalker) any {
	if op.RequestBody == nil || op.RequestBody.Value == nil {
		return nil
	}
	content := op.RequestBody.Value.Content
	mt := content.Get("application/json")
	if mt == nil {
		contentKeys := make([]string, 0, len(content))
		for k := range content {
			contentKeys = append(contentKeys, k)
		}
		sort.Strings(contentKeys)
		for _, key := range contentKeys {
			if strings.Contains(key, "json") {
				mt = content[key]
				break
			}
		}
	}
	if mt == nil || mt.Schema == nil {
		return nil
	}
	// Prefer explicit example on the MediaType.
	if mt.Example != nil {
		return mt.Example
	}
	// Prefer named example — pick lexicographically first key for determinism.
	if len(mt.Examples) > 0 {
		exKeys := make([]string, 0, len(mt.Examples))
		for k := range mt.Examples {
			exKeys = append(exKeys, k)
		}
		sort.Strings(exKeys)
		for _, k := range exKeys {
			ex := mt.Examples[k]
			if ex != nil && ex.Value != nil {
				return ex.Value.Value
			}
		}
	}
	return w.placeholder(mt.Schema, nil)
}

// extractStatusCodes returns the sorted numeric HTTP status codes from an
// operation's responses map. Non-numeric keys like "default" and pattern keys
// like "2XX" are ignored.
func extractStatusCodes(responses *openapi3.Responses) []int {
	if responses == nil {
		return nil
	}
	var codes []int
	for key := range responses.Map() {
		code, err := strconv.Atoi(key)
		if err != nil {
			continue // skip "default", "2XX", etc.
		}
		codes = append(codes, code)
	}
	sort.Ints(codes)
	return codes
}

// snakeCase converts an arbitrary parameter name into a snake_case variable
// name suitable for collection variable placeholders.
//
// Rules:
//  1. Explicit separators (-, ., space) become _.
//  2. CamelCase transitions (lowercase→uppercase) become _<lower>.
//  3. Acronym boundaries (uppercase run followed by lowercase) insert _ before
//     the last uppercase in the run: HTTPMethod → http_method.
//
// Examples: "X-API-Key" → "x_api_key", "userId" → "user_id", "limit" → "limit"
func snakeCase(s string) string {
	runes := []rune(s)
	var b strings.Builder
	for i, r := range runes {
		switch {
		case r == '-' || r == '.' || r == ' ':
			b.WriteRune('_')
		case unicode.IsUpper(r) && i > 0 && runes[i-1] != '_':
			prevIsLower := unicode.IsLower(runes[i-1])
			nextIsLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
			if prevIsLower || (unicode.IsUpper(runes[i-1]) && nextIsLower) {
				b.WriteRune('_')
			}
			b.WriteRune(unicode.ToLower(r))
		default:
			b.WriteRune(unicode.ToLower(r))
		}
	}
	result := strings.Trim(reMultiUnderscore.ReplaceAllString(b.String(), "_"), "_")
	if result == "" {
		return "param"
	}
	return result
}

// interpolatePath rewrites OpenAPI path parameters {name} into {{name}}.
func interpolatePath(path string) string {
	return pathParamRE.ReplaceAllString(path, "{{$1}}")
}

// chooseName returns the operationId if set; otherwise synthesises a name from
// method and path. Collisions are disambiguated with _2, _3, ... suffixes.
func chooseName(op *openapi3.Operation, method, path string, used map[string]int) string {
	name := op.OperationID
	if name == "" {
		name = synthName(method, path)
	}
	if n := used[name]; n > 0 {
		disambiguated := fmt.Sprintf("%s_%d", name, n+1)
		used[name] = n + 1
		used[disambiguated] = 1
		return disambiguated
	}
	used[name] = 1
	return name
}

// synthName converts "GET /pets/{petId}" -> "get_pets_by_petId".
func synthName(method, path string) string {
	var b strings.Builder
	b.WriteString(strings.ToLower(method))
	b.WriteByte('_')
	trimmed := strings.TrimPrefix(path, "/")
	for i, seg := range strings.Split(trimmed, "/") {
		if i > 0 {
			b.WriteByte('_')
		}
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			b.WriteString("by_")
			b.WriteString(seg[1 : len(seg)-1])
			continue
		}
		b.WriteString(seg)
	}
	return b.String()
}

// pathParamDefault produces the value a generated collection uses for a path
// parameter, so that the import runs unaided (§11C.10).
//
// The document decides wherever it can: an explicit example on the parameter or
// its schema, then an enum member, then a declared default. Only when the
// document says nothing does this fall back to the schema's type, and a numeric
// parameter respects a declared minimum — a status-code parameter constrained
// to 100..599 must not default to 0, which is not a status code at all.
//
// The value is always rendered as a string because it is substituted into a URL.
func pathParamDefault(p *openapi3.Parameter, w *schemaWalker) string {
	if p == nil {
		return "1"
	}
	if p.Example != nil {
		return scalarToString(p.Example)
	}
	if len(p.Examples) > 0 {
		for _, k := range sortedExampleKeys(p.Examples) {
			if ex := p.Examples[k]; ex != nil && ex.Value != nil && ex.Value.Value != nil {
				return scalarToString(ex.Value.Value)
			}
		}
	}
	if p.Schema == nil || p.Schema.Value == nil {
		return "1"
	}
	s := p.Schema.Value
	if s.Example != nil {
		return scalarToString(s.Example)
	}
	if len(s.Enum) > 0 {
		return scalarToString(s.Enum[0])
	}
	if s.Default != nil {
		return scalarToString(s.Default)
	}

	switch {
	case s.Type.Is("integer") || s.Type.Is("number"):
		if s.Min != nil && *s.Min > 0 {
			return strconv.FormatFloat(*s.Min, 'f', -1, 64)
		}
		return "1"
	case s.Type.Is("boolean"):
		return "true"
	case s.Type.Is("string"):
		return defaultForStringFormat(s.Format)
	}
	// Anything else — an untyped or composed schema. The walker already knows
	// how to pick a representative value.
	if v := w.placeholder(p.Schema, map[string]bool{}); v != nil {
		if str := scalarToString(v); str != "" {
			return str
		}
	}
	return "1"
}

// scalarToString renders a schema example/enum/default as URL path text.
// Composite values have no meaningful path form and yield "".
func scalarToString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case map[string]any, []any, nil:
		return ""
	default:
		return fmt.Sprintf("%v", t)
	}
}

func sortedExampleKeys(m openapi3.Examples) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
