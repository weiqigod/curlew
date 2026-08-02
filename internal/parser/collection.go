package parser

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/weiqigod/curlew/internal/assertion"
	"github.com/weiqigod/curlew/internal/datadriven"
	"github.com/weiqigod/curlew/internal/output"
	"github.com/weiqigod/curlew/internal/retry"
	"github.com/weiqigod/curlew/internal/variable"
)

// Options holds collection-level execution options.
type Options struct {
	StopOnFailure bool `yaml:"stop_on_failure"`
}

// SigningSpec is the parsed `signing:` field on a Collection or RequestItem.
// Type names a registered signer in internal/signer; Params is forwarded
// verbatim to the signer factory.
//
// To distinguish "field absent" from "field present but explicitly null"
// (which disables signing for that request even if the collection has a
// default), the parser uses a custom UnmarshalYAML that sets explicitNull
// when the YAML value was explicitly ~ or null.
//
// In practice, runner code calls resolveSigningSpec(item, col) which
// folds the precedence and returns either a normalised *SigningSpec or
// nil ("no signing for this request").
type SigningSpec struct {
	Type   string         `yaml:"type"`
	Params map[string]any `yaml:"params,omitempty"`

	// explicitNull is set by UnmarshalYAML when the YAML node was
	// explicitly null (`signing: null` or `signing: ~`). Distinguished
	// from "field absent" (*SigningSpec is nil) and from a normal
	// mapping (explicitNull is false, Type/Params are populated).
	explicitNull bool `yaml:"-"`
}

// IsExplicitNull reports whether the YAML field was explicitly null,
// disabling signing for that request even if the collection defaults to
// signing. Returns false on a nil receiver.
func (s *SigningSpec) IsExplicitNull() bool {
	if s == nil {
		return false
	}
	return s.explicitNull
}

// UnmarshalYAML decodes a signing: node into a SigningSpec. A null node
// sets explicitNull = true; a mapping decodes Type and Params normally
// and rejects an empty Type.
func (s *SigningSpec) UnmarshalYAML(value *yaml.Node) error {
	// yaml.v3 represents explicit null as Tag "!!null".
	if value.Tag == "!!null" {
		s.explicitNull = true
		return nil
	}
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("signing: expected mapping or null, got %v", value.Kind)
	}
	type signingRaw struct {
		Type   string         `yaml:"type"`
		Params map[string]any `yaml:"params,omitempty"`
	}
	var raw signingRaw
	if err := value.Decode(&raw); err != nil {
		return fmt.Errorf("signing: %w", err)
	}
	if raw.Type == "" {
		return fmt.Errorf("signing.type is required")
	}
	s.Type = raw.Type
	s.Params = raw.Params
	return nil
}

// CommandVar represents a variable sourced from a shell command.
type CommandVar struct {
	Command   string // shell command to execute
	Sensitive bool   // whether value should be redacted
	Cache     int    // TTL in seconds; 0 = no caching
}

// SensitiveVars holds a variable map and tracks which names were tagged !sensitive.
type SensitiveVars struct {
	Values    map[string]string
	Sensitive *variable.SensitiveSet
	Commands  map[string]CommandVar // from_command entries
}

// UnmarshalYAML detects the !sensitive tag on individual variable values
// and handles object-form variables with from_command, value, sensitive, and cache fields.
func (sv *SensitiveVars) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("variables: expected mapping, got %v", value.Tag)
	}
	sv.Values = make(map[string]string)
	sv.Sensitive = variable.NewSensitiveSet()
	sv.Commands = make(map[string]CommandVar)
	for i := 0; i+1 < len(value.Content); i += 2 {
		keyNode := value.Content[i]
		valNode := value.Content[i+1]
		key := keyNode.Value

		switch valNode.Kind {
		case yaml.ScalarNode:
			sv.Values[key] = valNode.Value
			if valNode.Tag == "!sensitive" {
				sv.Sensitive.Add(key)
			}
		case yaml.MappingNode:
			if err := sv.parseObjectVar(key, valNode); err != nil {
				return fmt.Errorf("variable %q: %w", key, err)
			}
		default:
			sv.Values[key] = valNode.Value
		}
	}
	return nil
}

// parseObjectVar handles the object-form variable syntax (from_command, value, sensitive, cache).
func (sv *SensitiveVars) parseObjectVar(key string, node *yaml.Node) error {
	var fromCommand, val string
	var hasCommand, hasValue, sensitive bool
	var cache int

	for j := 0; j+1 < len(node.Content); j += 2 {
		fieldKey := node.Content[j].Value
		fieldVal := node.Content[j+1]
		switch fieldKey {
		case "from_command":
			hasCommand = true
			fromCommand = fieldVal.Value
		case "value":
			hasValue = true
			val = fieldVal.Value
		case "sensitive":
			if err := fieldVal.Decode(&sensitive); err != nil {
				return fmt.Errorf("sensitive: %w", err)
			}
		case "cache":
			var c int
			if err := fieldVal.Decode(&c); err != nil {
				return fmt.Errorf("cache: %w", err)
			}
			cache = c
		}
	}

	if hasCommand && hasValue {
		return fmt.Errorf("'value' and 'from_command' are mutually exclusive")
	}
	if hasCommand && fromCommand == "" {
		return fmt.Errorf("'from_command' must not be empty")
	}

	if hasCommand {
		sv.Commands[key] = CommandVar{
			Command:   fromCommand,
			Sensitive: sensitive,
			Cache:     cache,
		}
		if sensitive {
			sv.Sensitive.Add(key)
		}
		return nil
	}

	// Object-form value (or no recognized source)
	sv.Values[key] = val
	if sensitive {
		sv.Sensitive.Add(key)
	}
	return nil
}

// Section represents a phase (setup, requests, teardown) with optional retry config.
// Supports two YAML forms:
//   - Array form: setup: [{name: ..., request: ...}]
//   - Object form: setup: { retry: {...}, items: [{name: ..., request: ...}] }
type Section struct {
	Retry *retry.FullConfig `yaml:"retry,omitempty"`
	Items []RequestItem     `yaml:"items,omitempty"`
}

// UnmarshalYAML handles both array and object forms.
func (s *Section) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.SequenceNode:
		return value.Decode(&s.Items)
	case yaml.MappingNode:
		type sectionRaw struct {
			Retry *retry.FullConfig `yaml:"retry,omitempty"`
			Items []RequestItem     `yaml:"items,omitempty"`
		}
		var raw sectionRaw
		if err := value.Decode(&raw); err != nil {
			return err
		}
		s.Retry = raw.Retry
		s.Items = raw.Items
		return nil
	default:
		return fmt.Errorf("section: expected sequence or mapping, got %v", value.Kind)
	}
}

// Collection represents a parsed collection file.
//
// The Include field (M3-003) lists other collection files to splice into this
// collection at parse time. Items from included files are appended to
// Setup/Requests/Teardown in declaration order. The parent's variables form a
// read-only snapshot that each included child inherits; child variable
// declarations override locally but never leak back to the parent scope.
// A child collection's RateLimitRPS is
// ignored (only the root parent's value is used).
// CollectionConfigBlock is the optional config: block in a collection file.
// It mirrors config.ConfigBlock but avoids a parser→config package import.
// Currently holds locale for the M20-001 locale precedence chain.
type CollectionConfigBlock struct {
	Locale string `yaml:"locale,omitempty"`
}

type Collection struct {
	Name         string                `yaml:"name"`
	Description  string                `yaml:"description,omitempty"`
	Variables    SensitiveVars         `yaml:"variables,omitempty"`
	Retry        *retry.FullConfig     `yaml:"retry,omitempty"`
	Include      []string              `yaml:"include,omitempty"` // M3-003
	Setup        Section               `yaml:"setup,omitempty"`
	Requests     Section               `yaml:"requests"`
	Teardown     Section               `yaml:"teardown,omitempty"`
	Options      Options               `yaml:"options,omitempty"`
	RateLimitRPS int                   `yaml:"rate_limit_rps,omitempty"` // 0 = unlimited; >0 = token-bucket throttle
	Output       *output.Config        `yaml:"output,omitempty"`         // M8-003: per-collection output override
	Config       CollectionConfigBlock `yaml:"config,omitempty"`         // M20-001: config block (locale, etc.)

	// M17-001: collection-level default signing applied to every request
	// that does not set its own `signing:` (and does not explicitly null
	// it). Nil means "no collection-level default".
	Signing *SigningSpec `yaml:"signing,omitempty"`

	ExternalFiles []string `yaml:"-"` // resolved external file paths, populated by ParseFile
}

// RequestItem is a single test entry in a collection.
type RequestItem struct {
	Name       string             `yaml:"name"`
	Path       string             `yaml:"path,omitempty"`
	Auth       string             `yaml:"auth,omitempty"`        // references an auth profile by name
	Required   *bool              `yaml:"required,omitempty"`    // nil = false (default)
	Retry      *retry.FullConfig  `yaml:"retry,omitempty"`       // per-request override (nil = inherit collection)
	DataDriven *datadriven.Config `yaml:"data_driven,omitempty"` // data-driven testing config (nil = normal request)
	Request    Request            `yaml:"request"`
	Variables  SensitiveVars      `yaml:"variables,omitempty"`
	Assertions Assertions         `yaml:"assertions,omitempty"`
	Extract    map[string]string  `yaml:"extract,omitempty"`

	// M17-001: per-request signing override. nil + collection has Signing
	// → use collection's. Non-nil + IsExplicitNull → disable for this
	// request. Non-nil + not explicit-null → use this spec.
	Signing *SigningSpec `yaml:"signing,omitempty"`

	// If is a CEL boolean expression evaluated by the runner before templating.
	// A false result skips the request with reason "if: false". Validated at
	// collection-load time only as a non-empty scalar; CEL parse/type-check
	// happens in the runner and in `curlew validate`.
	If string `yaml:"if,omitempty"`

	// DependsOn lists request names that must have run (any outcome other than
	// skipped) before this item. If any named item was skipped in the same
	// phase, this item is also marked skipped with reason "parent skipped:
	// <name>". Names must match a parsed item; unknown names produce
	// ErrUnknownDependsOn at parse time.
	DependsOn []string `yaml:"depends_on,omitempty"`

	// SourceFile is the absolute path of the file that declared this item.
	// Populated by the parser after unmarshal (collections: the parent file;
	// includes: the included child file; external request_file: the external
	// file path). Zero value means the item was synthesised (e.g. OpenAPI
	// import) and has no source YAML origin.
	SourceFile string `yaml:"-"`

	// SourceLine is the 1-based YAML line number of this item's mapping node
	// inside SourceFile. For external request files SourceLine is 1 (the
	// external file is a single request at the top of the document). Zero
	// value means unknown.
	SourceLine int `yaml:"-"`

	// Slug is the URL-safe identifier derived from Name at parse time via
	// parser.Slug(). Used by the W4 markdown formatter for filenames and
	// sentinel tags. Not user-configurable; populated by populateSlugs.
	Slug string `yaml:"-"`
}

// UnmarshalYAML captures the YAML node line for source-location plumbing and
// handles the explicit-null case for the `signing:` field.
//
// yaml.v3 does not call a pointer receiver's UnmarshalYAML when the value is
// null — it simply leaves the pointer nil. To distinguish "signing: ~" (explicit
// null, disables signing for this request) from "signing:" absent (inherit from
// collection), we scan the mapping node for the signing key before the alias
// decode, allocate a SigningSpec, and call its UnmarshalYAML manually.
func (r *RequestItem) UnmarshalYAML(node *yaml.Node) error {
	// First pass: check whether the signing: key is explicitly null so we can
	// set the sentinel before the alias decode overwrites Signing.
	var signingNode *yaml.Node
	if node.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value == "signing" {
				signingNode = node.Content[i+1]
				break
			}
		}
	}

	type requestItemRaw RequestItem
	var raw requestItemRaw
	if err := node.Decode(&raw); err != nil {
		return err
	}
	*r = RequestItem(raw)
	r.SourceLine = node.Line

	// If the signing: node was explicitly null, allocate a SigningSpec with
	// explicitNull=true. The alias decode above left r.Signing nil.
	if signingNode != nil && signingNode.Tag == "!!null" {
		spec := &SigningSpec{}
		if err := spec.UnmarshalYAML(signingNode); err != nil {
			return err
		}
		r.Signing = spec
	}
	return nil
}

// IsRequired returns whether this item is marked required.
// Defaults to false when not explicitly set.
func (ri *RequestItem) IsRequired() bool {
	return ri.Required != nil && *ri.Required
}

// externalRequest represents a standalone request file referenced via path:.
type externalRequest struct {
	Name       string            `yaml:"name"`
	Request    Request           `yaml:"request"`
	Assertions Assertions        `yaml:"assertions,omitempty"`
	Extract    map[string]string `yaml:"extract,omitempty"`
}

// Assertions holds the assertion definitions for a request.
type Assertions struct {
	Status  StatusCodes      `yaml:"status,omitempty"`
	Headers HeaderAssertions `yaml:"headers,omitempty"`
	Body    BodyAssertions   `yaml:"body,omitempty"`
	Timing  TimingAssertion  `yaml:"timing,omitempty"`
	// Schema is a relative path to a JSON Schema file.
	// Resolved and compiled to CompiledSchema by the parser at load time.
	Schema string `yaml:"schema,omitempty"`
	// CompiledSchema is the compiled form of Schema, populated by parseCollectionBytes.
	// Nil when Schema is empty. Safe for concurrent use.
	CompiledSchema *assertion.CompiledSchema `yaml:"-"`
	// CEL holds any cel: assertions defined under assertions:. Each item is a
	// CEL boolean expression evaluated at runtime against the standard activation
	// {response, previous, vars, env}. Mutually exclusive with operator assertions
	// on a per-entry basis (see CELAssertions.UnmarshalYAML).
	CEL CELAssertions `yaml:"cel,omitempty"`
}

// CELAssertion describes a single CEL boolean assertion.
type CELAssertion struct {
	// Source is the CEL expression string.
	Source string
}

// CELAssertions holds the list of cel: assertion entries under assertions:.
// Accepts two YAML forms per list item:
//   - plain string:  - "response.status == 200"
//   - mapping form:  - cel: "response.status == 200"
//
// The mapping form enforces mutual exclusion: an entry with cel: may not carry
// any additional operator key (e.g. eq:, equals:). A plain string is always
// interpreted as the CEL expression.
type CELAssertions struct {
	Items []CELAssertion
}

// UnmarshalYAML decodes the cel: list. Each element may be a plain scalar
// (the expression) or a mapping with a single cel: key. Any mapping entry that
// carries a cel: key alongside another key is rejected with
// ErrCelAndOperatorMutuallyExclusive.
func (c *CELAssertions) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.SequenceNode {
		return fmt.Errorf("cel assertions must be a list")
	}
	for _, item := range value.Content {
		switch item.Kind {
		case yaml.ScalarNode:
			// Simple form: - "<expr>"
			if item.Value == "" {
				return fmt.Errorf("cel assertion entry: expression must not be empty")
			}
			c.Items = append(c.Items, CELAssertion{Source: item.Value})
		case yaml.MappingNode:
			// Explicit form: - { cel: "<expr>" } — reject extra keys.
			var src string
			var extraKey string
			for j := 0; j+1 < len(item.Content); j += 2 {
				k := item.Content[j].Value
				if k == "cel" {
					src = item.Content[j+1].Value
					continue
				}
				if extraKey == "" {
					extraKey = k
				}
			}
			if extraKey != "" {
				return fmt.Errorf(
					"cel assertion entry: cel: and operator key %q are mutually exclusive on the same entry: %w",
					extraKey, ErrCelAndOperatorMutuallyExclusive,
				)
			}
			if src == "" {
				return fmt.Errorf("cel assertion entry: cel: expression must not be empty")
			}
			c.Items = append(c.Items, CELAssertion{Source: src})
		default:
			return fmt.Errorf("cel assertion entry: expected string or mapping, got kind %v", item.Kind)
		}
	}
	return nil
}

// BodyAssertion represents a single body assertion on a JSONPath.
type BodyAssertion struct {
	Path     string // JSONPath expression, e.g. "$.data.id"
	Operator string // "equals", "exists", "not_exists", "type"
	Value    any    // expected value (meaning depends on operator)
}

// BodyAssertions handles the YAML map-of-maps format for body assertions.
type BodyAssertions struct {
	Items []BodyAssertion
}

// UnmarshalYAML parses the body assertion map format:
//
//	$.data.id:
//	  equals: 1
//	$.token:
//	  exists: true
func (b *BodyAssertions) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("body assertions must be a mapping")
	}

	// Mapping nodes alternate between key and value nodes
	for i := 0; i < len(value.Content)-1; i += 2 {
		pathNode := value.Content[i]
		opsNode := value.Content[i+1]

		path := pathNode.Value

		if opsNode.Kind != yaml.MappingNode {
			return fmt.Errorf("body assertion for %q must be a mapping", path)
		}

		// Each path has exactly one operator
		for j := 0; j < len(opsNode.Content)-1; j += 2 {
			opNode := opsNode.Content[j]
			valNode := opsNode.Content[j+1]

			var v any
			if err := valNode.Decode(&v); err != nil {
				return fmt.Errorf("invalid value for %s.%s: %w", path, opNode.Value, err)
			}

			b.Items = append(b.Items, BodyAssertion{
				Path:     path,
				Operator: opNode.Value,
				Value:    v,
			})
		}
	}

	return nil
}

// HeaderAssertion represents a single assertion on a response header.
type HeaderAssertion struct {
	Name     string // Header name (case-insensitive matching at evaluation)
	Operator string // "equals", "exists", "matches"
	Value    any    // expected value (operator-dependent)
}

// HeaderAssertions handles the YAML map-of-maps format for header assertions.
type HeaderAssertions struct {
	Items []HeaderAssertion
}

// UnmarshalYAML parses the header assertion map format:
//
//	Content-Type:
//	  equals: "application/json"
//	X-Request-Id:
//	  exists: true
func (h *HeaderAssertions) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("header assertions must be a mapping")
	}

	for i := 0; i < len(value.Content)-1; i += 2 {
		nameNode := value.Content[i]
		opsNode := value.Content[i+1]

		name := nameNode.Value

		if opsNode.Kind != yaml.MappingNode {
			return fmt.Errorf("header assertion for %q must be a mapping", name)
		}

		for j := 0; j < len(opsNode.Content)-1; j += 2 {
			opNode := opsNode.Content[j]
			valNode := opsNode.Content[j+1]

			var v any
			if err := valNode.Decode(&v); err != nil {
				return fmt.Errorf("invalid value for %s.%s: %w", name, opNode.Value, err)
			}

			h.Items = append(h.Items, HeaderAssertion{
				Name:     name,
				Operator: opNode.Value,
				Value:    v,
			})
		}
	}

	return nil
}

// TimingAssertion holds timing-related assertion configuration.
type TimingAssertion struct {
	MaxDurationMs int `yaml:"max_duration_ms"`
}

// StatusCodes handles both `status: 200` and `status: [200, 201]` in YAML.
type StatusCodes struct {
	Codes []int
}

// UnmarshalYAML handles scalar int and sequence forms for status codes.
func (s *StatusCodes) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		var code int
		if err := value.Decode(&code); err != nil {
			return fmt.Errorf("invalid status code %q: %w", value.Value, err)
		}
		s.Codes = []int{code}
		return nil
	case yaml.SequenceNode:
		var codes []int
		if err := value.Decode(&codes); err != nil {
			return fmt.Errorf("invalid status code list: %w", err)
		}
		s.Codes = codes
		return nil
	default:
		return fmt.Errorf("status must be an integer or list of integers")
	}
}

// GraphQLConfig holds the GraphQL-specific request configuration.
type GraphQLConfig struct {
	Query         string         `yaml:"query,omitempty"`
	QueryFile     string         `yaml:"query_file,omitempty"` // path to an external .graphql file (mutually exclusive with query)
	Fragments     []string       `yaml:"fragments,omitempty"`  // paths to .graphql fragment files, concatenated onto the query
	Variables     map[string]any `yaml:"variables,omitempty"`
	ErrorHandling string         `yaml:"error_handling,omitempty"` // "fail" (default), "warn"
}

// ReconnectConfig controls auto-reconnection behaviour for a WebSocket request.
// Reconnection fires only on connection-level read failures during an expect
// step; assertion failures, expect timeouts, and context cancellation are not
// retried.
type ReconnectConfig struct {
	Enabled        bool   `yaml:"enabled"`
	MaxAttempts    int    `yaml:"max_attempts,omitempty"`     // default 3
	InitialDelayMs int    `yaml:"initial_delay_ms,omitempty"` // default 1000
	Backoff        string `yaml:"backoff,omitempty"`          // "exponential" (default)
}

// HeartbeatConfig controls periodic ping/pong keepalive for a WebSocket
// request. When Message is nil the adapter sends a gorilla PingMessage
// control frame and relies on the pong handler. When Message is set, the
// adapter sends a data frame and matches the pong against Expect assertions.
type HeartbeatConfig struct {
	Enabled    bool           `yaml:"enabled"`
	IntervalMs int            `yaml:"interval_ms,omitempty"` // default 30000
	Message    map[string]any `yaml:"message,omitempty"`
	Expect     BodyAssertions `yaml:"expect,omitempty"`
}

// WebSocketConfig holds the WebSocket-specific request configuration.
// Steps execute sequentially within a single WebSocket connection lifecycle.
type WebSocketConfig struct {
	Steps     []WebSocketStep  `yaml:"steps"`
	Reconnect *ReconnectConfig `yaml:"reconnect,omitempty"`
	Heartbeat *HeartbeatConfig `yaml:"heartbeat,omitempty"`
}

// WebSocketStep is a single action within a WebSocket request lifecycle.
// Action determines which fields are meaningful:
//   - "send":   Message (JSON object), MessageRaw (literal string), or MessageTemplate (external file)
//   - "expect": ExpectAssertions (JSONPath assertions), AnyOf (multi-pattern), Count (collect N), TimeoutMs, Extract
//   - "wait":   DurationMs
//   - "close":  Code, Reason
type WebSocketStep struct {
	Action             string            // "send" | "expect" | "wait" | "close"
	Message            map[string]any    // send: literal JSON object
	MessageRaw         string            // send: literal string payload
	MessageTemplate    string            // send: path to external template file (parser-resolved)
	MessageRawTemplate string            // send: resolved template file contents (populated at parse time)
	StepVariables      map[string]string // send: scoped variables merged in for message_template interpolation
	TimeoutMs          int               // expect: per-step wait for a matching message
	ExpectAssertions   BodyAssertions    // expect: single-pattern JSONPath assertions ("message:")
	AnyOf              []BodyAssertions  // expect: alternative JSONPath assertion sets ("any_of:")
	Count              int               // expect: collect N matching messages (default 1)
	Extract            map[string]string // expect: variable name -> JSONPath
	DurationMs         int               // wait: pause duration
	Code               int               // close: WebSocket close code (default 1000)
	Reason             string            // close: optional reason string
}

// UnmarshalYAML decodes a WebSocketStep, resolving the overloaded "message:"
// field based on the step's Action. For send steps, "message:" is a literal
// JSON map; for expect steps, "message:" is a BodyAssertions map.
func (s *WebSocketStep) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("websocket step: expected mapping, got %v", value.Kind)
	}
	// First pass: capture action so we know how to decode message:
	for i := 0; i+1 < len(value.Content); i += 2 {
		if value.Content[i].Value == "action" {
			s.Action = value.Content[i+1].Value
			break
		}
	}
	// Second pass: decode remaining fields, respecting the action.
	for i := 0; i+1 < len(value.Content); i += 2 {
		k := value.Content[i].Value
		v := value.Content[i+1]
		switch k {
		case "action":
			// already captured
		case "message":
			if s.Action == "expect" {
				if err := v.Decode(&s.ExpectAssertions); err != nil {
					return fmt.Errorf("expect message: %w", err)
				}
			} else {
				if err := v.Decode(&s.Message); err != nil {
					return fmt.Errorf("send message: %w", err)
				}
			}
		case "message_raw":
			s.MessageRaw = v.Value
		case "message_template":
			s.MessageTemplate = v.Value
		case "variables":
			if err := v.Decode(&s.StepVariables); err != nil {
				return fmt.Errorf("variables: %w", err)
			}
		case "timeout_ms":
			if err := v.Decode(&s.TimeoutMs); err != nil {
				return fmt.Errorf("timeout_ms: %w", err)
			}
		case "duration_ms":
			if err := v.Decode(&s.DurationMs); err != nil {
				return fmt.Errorf("duration_ms: %w", err)
			}
		case "code":
			if err := v.Decode(&s.Code); err != nil {
				return fmt.Errorf("code: %w", err)
			}
		case "reason":
			s.Reason = v.Value
		case "extract":
			if err := v.Decode(&s.Extract); err != nil {
				return fmt.Errorf("extract: %w", err)
			}
		case "any_of":
			if v.Kind != yaml.SequenceNode {
				return fmt.Errorf("any_of: expected sequence, got %v", v.Kind)
			}
			for _, item := range v.Content {
				if item.Kind != yaml.MappingNode {
					return fmt.Errorf("any_of: each entry must be a mapping")
				}
				var inner BodyAssertions
				for j := 0; j+1 < len(item.Content); j += 2 {
					if item.Content[j].Value == "message" {
						if decErr := item.Content[j+1].Decode(&inner); decErr != nil {
							return fmt.Errorf("any_of.message: %w", decErr)
						}
					}
				}
				s.AnyOf = append(s.AnyOf, inner)
			}
		case "count":
			if err := v.Decode(&s.Count); err != nil {
				return fmt.Errorf("count: %w", err)
			}
		}
	}
	return nil
}

// Request defines the HTTP request to execute.
type Request struct {
	Method  string            `yaml:"method"`
	URL     string            `yaml:"url"`
	Headers map[string]string `yaml:"headers,omitempty"`

	// Body is the request body. A string is sent as-is; a map is serialized as JSON.
	// Mutually exclusive with BodyFile and BodyBinaryFile.
	Body any `yaml:"body,omitempty"`

	// BodyFile is the path to a text file whose contents are sent as the request
	// body. Variable placeholders ({{var}}) in the file are interpolated at
	// request time. Content-Type is auto-detected from the file extension and
	// set when no explicit Content-Type header is present. Relative paths are
	// resolved against the collection file's directory. Mutually exclusive with
	// Body and BodyBinaryFile.
	BodyFile string `yaml:"body_file,omitempty"`

	// BodyBinaryFile is the path to a file whose raw bytes are sent as the
	// request body. No interpolation is performed. Content-Type is auto-detected
	// from the file extension (falling back to application/octet-stream) and set
	// when no explicit Content-Type header is present. Relative paths are
	// resolved against the collection file's directory. Mutually exclusive with
	// Body and BodyFile.
	BodyBinaryFile string `yaml:"body_binary_file,omitempty"`

	// BodyFileContent holds the bytes loaded from BodyFile at parse time.
	// Placeholders remain in place for execution-time interpolation.
	// Not serialised to YAML.
	BodyFileContent string `yaml:"-"`

	// BodyBinaryFileContent holds the bytes loaded from BodyBinaryFile at parse
	// time. Sent verbatim, no interpolation. Not serialised to YAML.
	BodyBinaryFileContent []byte `yaml:"-"`

	// BodyFileContentType is the auto-detected MIME type derived from the file
	// extension of BodyFile or BodyBinaryFile. Applied only when no explicit
	// Content-Type header is set on the request. Not serialised to YAML.
	BodyFileContentType string `yaml:"-"`

	// QueryParams are appended to the URL as query parameters.
	QueryParams map[string]string `yaml:"query,omitempty"`

	// Protocol specifies the request protocol. Defaults to "http" when empty.
	// Supported values: "http", "graphql", "websocket".
	Protocol string `yaml:"protocol,omitempty"`

	// GraphQL holds GraphQL-specific configuration (only used when Protocol is "graphql").
	GraphQL *GraphQLConfig `yaml:"graphql,omitempty"`

	// WebSocket holds WebSocket-specific configuration (only used when Protocol is "websocket").
	WebSocket *WebSocketConfig `yaml:"websocket,omitempty"`
}
