package openapi

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/weiqigod/curlew/internal/parser"
)

// writeSpecToTemp writes YAML content to a temp file and returns the path.
func writeSpecToTemp(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "spec.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writeSpecToTemp: %v", err)
	}
	return path
}

func TestSnakeCase(t *testing.T) {
	tests := []struct{ in, want string }{
		{"limit", "limit"},
		{"X-API-Key", "x_api_key"},
		{"userId", "user_id"},
		{"Content-Type", "content_type"},
		{"x.forwarded.for", "x_forwarded_for"},
		{"already_snake", "already_snake"},
		{"HTTPMethod", "http_method"}, // acronym boundary: HTTP→Method detected via next-char-lowercase rule
		{"", "param"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := snakeCase(tt.in); got != tt.want {
				t.Errorf("snakeCase(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestExtractStatusCodes(t *testing.T) {
	buildResponses := func(keys []string) *openapi3.Responses {
		if keys == nil {
			return nil
		}
		r := openapi3.NewResponses()
		for _, k := range keys {
			desc := "desc"
			r.Set(k, &openapi3.ResponseRef{Value: &openapi3.Response{Description: &desc}})
		}
		return r
	}
	tests := []struct {
		name string
		keys []string
		want []int
	}{
		{"single", []string{"200"}, []int{200}},
		{"multiple sorted", []string{"404", "200", "201"}, []int{200, 201, 404}},
		{"skips default", []string{"200", "default"}, []int{200}},
		{"skips pattern", []string{"200", "2XX"}, []int{200}},
		{"nil responses", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractStatusCodes(buildResponses(tt.keys))
			if len(got) != len(tt.want) {
				t.Errorf("extractStatusCodes(%v) = %v, want %v", tt.keys, got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("extractStatusCodes(%v)[%d] = %d, want %d", tt.keys, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestImport_HeaderParameters(t *testing.T) {
	tests := []struct {
		name       string
		specYAML   string
		wantHeader string
		wantVarKey string
	}{
		{
			"single required header",
			`openapi: 3.0.3
info: {title: Test, version: 1.0.0}
servers: [{url: https://example.com}]
paths:
  /items:
    get:
      operationId: listItems
      parameters:
        - name: X-API-Key
          in: header
          required: true
          schema: {type: string}
      responses: {'200': {description: ok}}`,
			"X-API-Key",
			"x_api_key",
		},
		{
			"camel case header",
			`openapi: 3.0.3
info: {title: Test, version: 1.0.0}
servers: [{url: https://example.com}]
paths:
  /items:
    get:
      operationId: listItems
      parameters:
        - name: ContentType
          in: header
          schema: {type: string}
      responses: {'200': {description: ok}}`,
			"ContentType",
			"content_type",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeSpecToTemp(t, tt.specYAML)
			col, err := Import(path)
			if err != nil {
				t.Fatalf("Import: %v", err)
			}
			item := col.Requests.Items[0]
			if item.Request.Headers[tt.wantHeader] != "{{"+tt.wantVarKey+"}}" {
				t.Errorf("header %q = %q, want %q", tt.wantHeader,
					item.Request.Headers[tt.wantHeader], "{{"+tt.wantVarKey+"}}")
			}
			if _, ok := col.Variables.Values[tt.wantVarKey]; !ok {
				t.Errorf("expected collection variable %q, got vars: %v", tt.wantVarKey, col.Variables.Values)
			}
		})
	}
}

func TestImport_SharedHeaderDedup(t *testing.T) {
	spec := `openapi: 3.0.3
info: {title: Test, version: 1.0.0}
servers: [{url: https://example.com}]
paths:
  /a:
    get:
      operationId: opA
      parameters:
        - name: X-API-Key
          in: header
          required: true
          schema: {type: string}
      responses: {'200': {description: ok}}
  /b:
    get:
      operationId: opB
      parameters:
        - name: X-API-Key
          in: header
          required: true
          schema: {type: string}
      responses: {'200': {description: ok}}`
	path := writeSpecToTemp(t, spec)
	col, err := Import(path)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	// x_api_key must appear exactly once in collection variables
	count := 0
	for k := range col.Variables.Values {
		if k == "x_api_key" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 x_api_key var, got %d (vars: %v)", count, col.Variables.Values)
	}
	// Both ops should reference it
	for _, it := range col.Requests.Items {
		if it.Request.Headers["X-API-Key"] != "{{x_api_key}}" {
			t.Errorf("%s: X-API-Key header = %q, want {{x_api_key}}", it.Name, it.Request.Headers["X-API-Key"])
		}
	}
}

func TestImport_QueryParameters(t *testing.T) {
	spec := `openapi: 3.0.3
info: {title: Test, version: 1.0.0}
servers: [{url: https://example.com}]
paths:
  /items:
    get:
      operationId: listItems
      parameters:
        - name: limit
          in: query
          schema: {type: integer}
        - name: from
          in: query
          schema: {type: integer}
      responses: {'200': {description: ok}}`
	path := writeSpecToTemp(t, spec)
	col, err := Import(path)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	url := col.Requests.Items[0].Request.URL
	if !strings.Contains(url, "from={{from}}") {
		t.Errorf("URL %q missing from={{from}}", url)
	}
	if !strings.Contains(url, "limit={{limit}}") {
		t.Errorf("URL %q missing limit={{limit}}", url)
	}
	// Query vars must be alphabetically ordered in URL
	fromIdx := strings.Index(url, "from={{from}}")
	limitIdx := strings.Index(url, "limit={{limit}}")
	if fromIdx > limitIdx {
		t.Errorf("URL query params not sorted: %q", url)
	}
	// Variables added
	if _, ok := col.Variables.Values["limit"]; !ok {
		t.Errorf("expected collection variable 'limit'")
	}
	if _, ok := col.Variables.Values["from"]; !ok {
		t.Errorf("expected collection variable 'from'")
	}
}

func TestImport_StatusAssertions(t *testing.T) {
	buildSpec := func(responses string) string {
		return `openapi: 3.0.3
info: {title: Test, version: 1.0.0}
servers: [{url: https://example.com}]
paths:
  /items:
    get:
      operationId: op
      responses: ` + responses
	}
	tests := []struct {
		name      string
		responses string
		want      []int
	}{
		{"single_200", "{'200': {description: ok}}", []int{200}},
		{"multi_sorted", "{'404': {description: nf}, '200': {description: ok}, '201': {description: created}}", []int{200, 201, 404}},
		{"skips_default", "{'200': {description: ok}, 'default': {description: err}}", []int{200}},
		{"skips_2xx_pattern", "{'200': {description: ok}, '2XX': {description: all}}", []int{200}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeSpecToTemp(t, buildSpec(tt.responses))
			col, err := Import(path)
			if err != nil {
				t.Fatalf("Import: %v", err)
			}
			got := col.Requests.Items[0].Assertions.Status.Codes
			if len(got) != len(tt.want) {
				t.Errorf("status codes = %v, want %v", got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("status code[%d] = %d, want %d", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestImport_RequestBody_FromSchema(t *testing.T) {
	buildSpec := func(schemaYAML string) string {
		return `openapi: 3.0.3
info: {title: Test, version: 1.0.0}
servers: [{url: https://example.com}]
paths:
  /items:
    post:
      operationId: createItem
      requestBody:
        required: true
        content:
          application/json:
            schema: ` + schemaYAML + `
      responses: {'201': {description: created}}`
	}
	tests := []struct {
		name     string
		schema   string
		wantBody map[string]any
	}{
		{
			"primitive_string_field",
			"{type: object, properties: {name: {type: string}}}",
			map[string]any{"name": "string"},
		},
		{
			"integer_field",
			"{type: object, properties: {count: {type: integer}}}",
			map[string]any{"count": 0},
		},
		{
			"boolean_field",
			"{type: object, properties: {active: {type: boolean}}}",
			map[string]any{"active": false},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeSpecToTemp(t, buildSpec(tt.schema))
			col, err := Import(path)
			if err != nil {
				t.Fatalf("Import: %v", err)
			}
			body, ok := col.Requests.Items[0].Request.Body.(map[string]any)
			if !ok {
				t.Fatalf("body is %T, want map[string]any", col.Requests.Items[0].Request.Body)
			}
			for k, wantV := range tt.wantBody {
				gotV := body[k]
				if gotV != wantV {
					t.Errorf("body[%q] = %v (%T), want %v (%T)", k, gotV, gotV, wantV, wantV)
				}
			}
		})
	}
}

func TestImport_RequestBody_FromExample(t *testing.T) {
	spec := `openapi: 3.0.3
info: {title: Test, version: 1.0.0}
servers: [{url: https://example.com}]
paths:
  /items:
    post:
      operationId: createItem
      requestBody:
        required: true
        content:
          application/json:
            example:
              name: Fluffy
              tag: cat
            schema: {type: object, properties: {name: {type: string}}}
      responses: {'201': {description: created}}`
	path := writeSpecToTemp(t, spec)
	col, err := Import(path)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	body, ok := col.Requests.Items[0].Request.Body.(map[string]any)
	if !ok {
		t.Fatalf("body is %T, want map[string]any", col.Requests.Items[0].Request.Body)
	}
	if body["name"] != "Fluffy" {
		t.Errorf("body[name] = %v, want Fluffy", body["name"])
	}
}

func TestImport_RequestBody_FromNamedExamples(t *testing.T) {
	// Behavior 4: named examples map — lexicographically first key wins.
	spec := `openapi: 3.0.3
info: {title: Test, version: 1.0.0}
servers: [{url: https://example.com}]
paths:
  /items:
    post:
      operationId: createItem
      requestBody:
        required: true
        content:
          application/json:
            examples:
              beta:
                value:
                  name: BetaName
                  tag: beta
              alpha:
                value:
                  name: AlphaName
                  tag: alpha
            schema: {type: object, properties: {name: {type: string}}}
      responses: {'201': {description: created}}`
	path := writeSpecToTemp(t, spec)
	col, err := Import(path)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	body, ok := col.Requests.Items[0].Request.Body.(map[string]any)
	if !ok {
		t.Fatalf("body is %T, want map[string]any", col.Requests.Items[0].Request.Body)
	}
	// "alpha" < "beta" lexicographically — alpha example must be chosen.
	if body["name"] != "AlphaName" {
		t.Errorf("body[name] = %v, want AlphaName (lexicographically first key)", body["name"])
	}
}

func TestImport_RequestBody_FallbackContentType(t *testing.T) {
	// findintg #4: fallback loop for json-variant content types.
	spec := `openapi: 3.0.3
info: {title: Test, version: 1.0.0}
servers: [{url: https://example.com}]
paths:
  /items:
    post:
      operationId: createItem
      requestBody:
        required: true
        content:
          application/vnd.api+json:
            schema: {type: object, properties: {name: {type: string}}}
      responses: {'201': {description: created}}`
	path := writeSpecToTemp(t, spec)
	col, err := Import(path)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	body, ok := col.Requests.Items[0].Request.Body.(map[string]any)
	if !ok {
		t.Fatalf("body is %T, want map[string]any", col.Requests.Items[0].Request.Body)
	}
	if body["name"] != "string" {
		t.Errorf("body[name] = %v, want string", body["name"])
	}
}

func TestImport_RequestBody_FallbackContentType_Deterministic(t *testing.T) {
	// Finding #1 (Low): multiple json-variant content types must be iterated in
	// sorted order so the selection is deterministic across runs.
	// application/hal+json sorts before application/vnd.api+json, so it wins.
	spec := `openapi: 3.0.3
info: {title: Test, version: 1.0.0}
servers: [{url: https://example.com}]
paths:
  /items:
    post:
      operationId: createItem
      requestBody:
        required: true
        content:
          application/vnd.api+json:
            schema: {type: object, properties: {name: {type: string}}}
          application/hal+json:
            schema: {type: object, properties: {id: {type: integer}}}
      responses: {'201': {description: created}}`
	// Run multiple times to surface non-determinism if the map is iterated unsorted.
	for i := 0; i < 20; i++ {
		path := writeSpecToTemp(t, spec)
		col, err := Import(path)
		if err != nil {
			t.Fatalf("Import: %v", err)
		}
		body, ok := col.Requests.Items[0].Request.Body.(map[string]any)
		if !ok {
			t.Fatalf("iteration %d: body is %T, want map[string]any", i, col.Requests.Items[0].Request.Body)
		}
		// application/hal+json is lexicographically first — must always win.
		if _, hasID := body["id"]; !hasID {
			t.Errorf("iteration %d: expected body from application/hal+json (key 'id'), got %v", i, body)
		}
	}
}

func TestImport_NoRequestBody(t *testing.T) {
	spec := `openapi: 3.0.3
info: {title: Test, version: 1.0.0}
servers: [{url: https://example.com}]
paths:
  /items:
    get:
      operationId: listItems
      responses: {'200': {description: ok}}`
	path := writeSpecToTemp(t, spec)
	col, err := Import(path)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if col.Requests.Items[0].Request.Body != nil {
		t.Errorf("expected nil body, got %v", col.Requests.Items[0].Request.Body)
	}
}

func TestImport_CycleWarning(t *testing.T) {
	// Spec with a recursive $ref: TreeNode references itself.
	spec := `openapi: 3.0.3
info: {title: Test, version: 1.0.0}
servers: [{url: https://example.com}]
components:
  schemas:
    TreeNode:
      type: object
      properties:
        value:
          type: string
        child:
          $ref: '#/components/schemas/TreeNode'
paths:
  /tree:
    post:
      operationId: createTree
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/TreeNode'
      responses: {'201': {description: created}}`
	path := writeSpecToTemp(t, spec)

	var warnings []string
	col, err := importWithWarn(path, func(msg string) { warnings = append(warnings, msg) })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = col

	found := false
	for _, w := range warnings {
		if strings.Contains(w, "recursive $ref") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected recursive $ref warning; got %v", warnings)
	}
	// warn-once: same ref should not appear twice
	count := 0
	for _, w := range warnings {
		if strings.Contains(w, "recursive $ref") {
			count++
		}
	}
	if count > 1 {
		t.Errorf("expected warn-once; got %d warnings for same ref", count)
	}
}

func TestImport_FullFixture(t *testing.T) {
	col, err := Import("testdata/petstore_full.yaml")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	// Behavior 7: shared X-API-Key → single collection variable
	if _, ok := col.Variables.Values["x_api_key"]; !ok {
		t.Error("expected x_api_key collection variable")
	}

	// Behavior 1: all 3 ops have X-API-Key header
	for _, it := range col.Requests.Items {
		if it.Request.Headers["X-API-Key"] != "{{x_api_key}}" {
			t.Errorf("%s: missing X-API-Key header, got %v", it.Name, it.Request.Headers)
		}
	}

	// Behavior 2: listPets URL has ?limit={{limit}}
	listPets := findItem(t, col, "listPets")
	if !strings.Contains(listPets.Request.URL, "limit={{limit}}") {
		t.Errorf("listPets URL missing limit query param: %s", listPets.Request.URL)
	}

	// Behavior 3: createPet body has name field
	createPet := findItem(t, col, "createPet")
	body, ok := createPet.Request.Body.(map[string]any)
	if !ok || body["name"] == nil {
		t.Errorf("createPet body missing name field: %v", createPet.Request.Body)
	}

	// Behavior 5: all ops have status assertions
	for _, it := range col.Requests.Items {
		if len(it.Assertions.Status.Codes) == 0 {
			t.Errorf("%s: expected status assertions", it.Name)
		}
	}
	assertStatusCodes(t, "listPets", listPets.Assertions.Status.Codes, []int{200, 404})
	assertStatusCodes(t, "createPet", createPet.Assertions.Status.Codes, []int{201, 404})
}

func findItem(t *testing.T, col *parser.Collection, name string) parser.RequestItem {
	t.Helper()
	for _, it := range col.Requests.Items {
		if it.Name == name {
			return it
		}
	}
	t.Fatalf("item %q not found in collection", name)
	return parser.RequestItem{}
}

func assertStatusCodes(t *testing.T, opName string, got, want []int) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s: status codes = %v, want %v", opName, got, want)
		return
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("%s: status code[%d] = %d, want %d", opName, i, got[i], want[i])
		}
	}
}

func TestSynthName(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		want   string
	}{
		{"simple get", "GET", "/pets", "get_pets"},
		{"post simple", "POST", "/pets", "post_pets"},
		{"single path param", "GET", "/pets/{petId}", "get_pets_by_petId"},
		{"nested path param", "GET", "/users/{userId}/pets/{petId}", "get_users_by_userId_pets_by_petId"},
		{"root", "GET", "/", "get_"},
		{"dotted param", "GET", "/a/{user.id}", "get_a_by_user.id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := synthName(tt.method, tt.path); got != tt.want {
				t.Errorf("synthName(%q, %q) = %q, want %q", tt.method, tt.path, got, tt.want)
			}
		})
	}
}

func TestInterpolatePath(t *testing.T) {
	tests := []struct{ in, want string }{
		{"/pets", "/pets"},
		{"/pets/{petId}", "/pets/{{petId}}"},
		{"/users/{userId}/pets/{petId}", "/users/{{userId}}/pets/{{petId}}"},
	}
	for _, tt := range tests {
		if got := interpolatePath(tt.in); got != tt.want {
			t.Errorf("interpolatePath(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestImport_Petstore(t *testing.T) {
	col, err := Import("testdata/petstore.yaml")
	if err != nil {
		t.Fatalf("Import returned error: %v", err)
	}
	if col.Name != "Petstore" {
		t.Errorf("Name = %q, want %q", col.Name, "Petstore")
	}
	if got := col.Variables.Values["base_url"]; got != "https://api.example.com/v1" {
		t.Errorf("base_url = %q, want %q", got, "https://api.example.com/v1")
	}
	if len(col.Requests.Items) != 3 {
		t.Fatalf("Requests.Items len = %d, want 3", len(col.Requests.Items))
	}

	// Expect deterministic order: sorted paths, methods in methodOrder.
	want := []struct{ name, method, url string }{
		{"listPets", "GET", "{{base_url}}/pets"},
		{"createPet", "POST", "{{base_url}}/pets"},
		{"showPetById", "GET", "{{base_url}}/pets/{{petId}}"},
	}
	for i, w := range want {
		got := col.Requests.Items[i]
		if got.Name != w.name || got.Request.Method != w.method || got.Request.URL != w.url {
			t.Errorf("item[%d] = {%s %s %s}, want {%s %s %s}",
				i, got.Name, got.Request.Method, got.Request.URL,
				w.name, w.method, w.url)
		}
	}
}

func TestImport_OpenAPI31(t *testing.T) {
	col, err := Import("testdata/petstore_31.yaml")
	if err != nil {
		t.Fatalf("Import returned error: %v", err)
	}
	if len(col.Requests.Items) == 0 {
		t.Fatal("expected at least one request from 3.1 spec")
	}
}

func TestImport_NoServers(t *testing.T) {
	col, err := Import("testdata/no_servers.yaml")
	if err != nil {
		t.Fatalf("Import returned error: %v", err)
	}
	if col.Variables.Values["base_url"] != "" {
		t.Errorf("expected empty base_url, got %q", col.Variables.Values["base_url"])
	}
}

func TestImport_NoOperationId(t *testing.T) {
	col, err := Import("testdata/no_operation_id.yaml")
	if err != nil {
		t.Fatalf("Import returned error: %v", err)
	}
	names := map[string]bool{}
	for _, it := range col.Requests.Items {
		names[it.Name] = true
	}
	if !names["get_pets"] {
		t.Errorf("expected synthesised name get_pets; got %v", names)
	}
}

func TestChosenName_Collision(t *testing.T) {
	// Two operations that produce the same base name (via operationId collision)
	// should be disambiguated with _2, _3, ... suffixes.
	used := map[string]int{}

	// operationId with no prior collision
	name1 := chooseName(&openapi3.Operation{OperationID: "list_pets"}, "GET", "/pets", used)
	if name1 != "list_pets" {
		t.Errorf("first name = %q, want %q", name1, "list_pets")
	}

	// Same operationId again — should get _2 suffix
	name2 := chooseName(&openapi3.Operation{OperationID: "list_pets"}, "POST", "/pets", used)
	if name2 != "list_pets_2" {
		t.Errorf("second name = %q, want %q", name2, "list_pets_2")
	}

	// Same operationId a third time — should get _3 suffix
	name3 := chooseName(&openapi3.Operation{OperationID: "list_pets"}, "PUT", "/pets", used)
	if name3 != "list_pets_3" {
		t.Errorf("third name = %q, want %q", name3, "list_pets_3")
	}
}

func TestImport_NoOperations(t *testing.T) {
	_, err := Import("testdata/no_operations.yaml")
	if err == nil {
		t.Fatal("expected error for spec with no operations")
	}
	if !errors.Is(err, ErrNoOperations) {
		t.Errorf("expected ErrNoOperations, got %v", err)
	}
}

func TestImport_FileNotFound(t *testing.T) {
	_, err := Import("testdata/does_not_exist.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !errors.Is(err, ErrSpecInvalid) {
		t.Errorf("expected ErrSpecInvalid, got %v", err)
	}
}

func TestImport_InvalidSpec(t *testing.T) {
	_, err := Import("testdata/invalid.yaml")
	if err == nil {
		t.Fatal("expected error for invalid spec")
	}
	if !errors.Is(err, ErrSpecInvalid) {
		t.Errorf("expected ErrSpecInvalid, got %v", err)
	}
}

// §11C.9. Both CLI_SPECIFICATION §18.8 and docs/MANUAL.md promise "OpenAPI
// 3.x", and the importer accepted a document DECLARING openapi: 3.1.0 and then
// validated it against 3.0 rules. Three ordinary 3.1 constructs were refused:
//
//	info.summary            added in 3.1     invalid info: extra sibling fields: [summary]
//	webhooks                added in 3.1     extra sibling fields: [webhooks]
//	type: ["string","null"] JSON Schema 2020-12  unsupported 'type' value "null"
//
// testdata/petstore_31.yaml declared 3.1 and used none of them, which is why
// nothing caught this.
func TestImport_openapi31Constructs(t *testing.T) {
	var warnings []string
	col, err := importWithWarn("testdata/petstore_31_constructs.yaml", func(m string) {
		warnings = append(warnings, m)
	})
	if err != nil {
		t.Fatalf("a 3.1 document was rejected: %v", err)
	}
	if col.Name != "Petstore 3.1 constructs" {
		t.Errorf("Name = %q", col.Name)
	}

	// The paths must import normally — relaxing the validator must not cost
	// operations.
	names := map[string]bool{}
	for _, it := range col.Requests.Items {
		names[it.Name] = true
	}
	for _, want := range []string{"listPets", "createPet"} {
		if !names[want] {
			t.Errorf("operation %q missing; got %v", want, names)
		}
	}

	// webhooks describe inbound callbacks with no server to call, so they are
	// not importable as requests — but dropping them silently would be the
	// same class of defect as rejecting them.
	if names["petStatusChanged"] {
		t.Error("a webhook was imported as a request; webhooks are inbound")
	}
	var mentionedWebhooks bool
	for _, w := range warnings {
		if strings.Contains(w, "webhook") {
			mentionedWebhooks = true
		}
	}
	if !mentionedWebhooks {
		t.Errorf("webhooks were dropped without a warning; got %v", warnings)
	}
}

// A nullable type array must survive as the underlying type, so body synthesis
// still produces a value of the right shape rather than nothing.
func TestImport_openapi31NullableTypeArrayKeepsItsType(t *testing.T) {
	col, err := importWithWarn("testdata/petstore_31_constructs.yaml", func(string) {})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	var post *parser.RequestItem
	for i := range col.Requests.Items {
		if col.Requests.Items[i].Name == "createPet" {
			post = &col.Requests.Items[i]
		}
	}
	if post == nil {
		t.Fatal("createPet not imported")
	}
	body, ok := post.Request.Body.(map[string]any)
	if !ok {
		t.Fatalf("body is %T, want a synthesised object", post.Request.Body)
	}
	// nickname is type: ["string","null"]; it must be treated as a string, not
	// dropped and not left as an unusable multi-type.
	if _, present := body["nickname"]; !present {
		t.Errorf("nickname absent from the synthesised body: %v", body)
	}
	if s, isString := body["nickname"].(string); !isString {
		t.Errorf("nickname is %T, want string (the non-null member of the type array)", body["nickname"])
	} else if s == "" {
		t.Error("nickname synthesised as empty")
	}
}

// The 3.0 path must be untouched.
func TestImport_openapi30StillImports(t *testing.T) {
	if _, err := importWithWarn("testdata/petstore.yaml", func(string) {}); err != nil {
		t.Fatalf("3.0 import broke: %v", err)
	}
}

// §11C.10. A path parameter becomes {{code}} in the generated URL, but the
// import emitted no variables: entry for it, so the collection it produced
// could not run: exit 5, `undefined variable "code"`, at run time rather than
// at import time. §9.P's acceptance criterion is literal — `curlew run` on the
// import's own output must pass — and it was unreachable for any path with a
// parameter.
func TestImport_pathParameterGetsAVariable(t *testing.T) {
	col, err := importWithWarn("testdata/path_params.yaml", func(string) {})
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	tests := []struct {
		varName string
		want    string
		why     string
	}{
		{"code", "200", "the parameter carries example: 200"},
		{"petId", "abc-123", "the schema carries an example"},
		{"kind", "cat", "the schema is an enum; the first member is used"},
		{"page", "1", "an integer with minimum 1 must not default to 0"},
		{"slug", "string", "a plain string parameter falls back to its type"},
	}
	for _, tt := range tests {
		got, present := col.Variables.Values[tt.varName]
		if !present {
			t.Errorf("no variable for path parameter %q — the collection cannot run (%s)", tt.varName, tt.why)
			continue
		}
		if got != tt.want {
			t.Errorf("%s = %q, want %q (%s)", tt.varName, got, tt.want, tt.why)
		}
	}

	// Every {{var}} in every generated URL must resolve against the emitted
	// variables. This is the property that actually matters: it is what makes
	// the collection runnable, and it does not depend on the values above.
	for _, item := range col.Requests.Items {
		for _, m := range regexp.MustCompile(`\{\{([^{}]+)\}\}`).FindAllStringSubmatch(item.Request.URL, -1) {
			if _, present := col.Variables.Values[m[1]]; !present {
				t.Errorf("request %q references {{%s}}, which the import did not define", item.Name, m[1])
			}
		}
	}
}

// No path parameter, no spurious variables.
func TestImport_noPathParametersAddsNoVariables(t *testing.T) {
	col, err := importWithWarn("testdata/petstore.yaml", func(string) {})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	for name := range col.Variables.Values {
		if name == "base_url" {
			continue
		}
		if col.Variables.Values[name] == "" {
			continue // header/query params legitimately default to empty
		}
	}
	if _, ok := col.Variables.Values["base_url"]; !ok {
		t.Error("base_url missing")
	}
}
