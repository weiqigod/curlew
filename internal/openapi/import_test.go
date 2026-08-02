package openapi

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/peterlindqvist/apitest/internal/parser"
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
