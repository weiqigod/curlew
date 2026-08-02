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
	apierrors "github.com/peterlindqvist/apitest/internal/errors"
	"github.com/peterlindqvist/apitest/internal/parser"
	"github.com/peterlindqvist/apitest/internal/variable"
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
	doc, err := loader.LoadFromFile(specPath)
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
			url, headers, newVars := collectParameters(op, pi, "{{base_url}}"+interpolatePath(path), collVarsSeen)
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
			// "path" params are handled by interpolatePath; "cookie" ignored.
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
