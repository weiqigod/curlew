// Package validator performs structural validation of collection files without
// executing HTTP requests. It collects all issues (errors and warnings) rather
// than stopping at the first problem.
package validator

import (
	"errors"
	"fmt"
	"os"

	celgo "github.com/google/cel-go/cel"
	"gopkg.in/yaml.v3"

	apicel "github.com/weiqigod/curlew/internal/cel"
	apierrors "github.com/weiqigod/curlew/internal/errors"
	"github.com/weiqigod/curlew/internal/parser"
	"github.com/weiqigod/curlew/internal/retry"
	"github.com/weiqigod/curlew/internal/variable"
	"github.com/weiqigod/curlew/internal/vault/teamtemplate"
)

// Severity classifies a validation finding.
type Severity int

const (
	SeverityError   Severity = iota // blocks execution — exit 3
	SeverityWarning                 // informational — exit 0
)

// String returns a human-readable severity label.
func (s Severity) String() string {
	switch s {
	case SeverityError:
		return "error"
	case SeverityWarning:
		return "warning"
	default:
		return "unknown"
	}
}

// Issue represents a single validation finding.
type Issue struct {
	Severity Severity
	FilePath string
	Line     int
	Message  string
	Hint     string
}

// ResultKind identifies what flavor of file was validated.
type ResultKind int

const (
	// KindCollection represents a regular collection file.
	KindCollection ResultKind = iota
	// KindTeamTemplate represents a shared vault configuration template.
	KindTeamTemplate
)

// Result holds all validation findings for a single file.
type Result struct {
	FilePath string
	Valid    bool       // true if no SeverityError issues
	Kind     ResultKind // KindCollection or KindTeamTemplate
	Issues   []Issue
	Summary  string // e.g. "2 environments, 4 secrets" for team templates; empty for collections
}

// ValidateAuto inspects the top-level YAML keys of path and dispatches to
// either collection or team-template validation. Files with a top-level
// "team_secrets:" key are treated as team templates.
func ValidateAuto(path string, knownVars map[string]string) *Result {
	if isTeam, _ := sniffTeamTemplate(path); isTeam {
		return validateTeamTemplate(path)
	}
	return Validate(path, knownVars)
}

// sniffTeamTemplate reads the file and checks whether it has a top-level
// "team_secrets:" key. Returns (false, err) on any read/parse failure so
// the caller can fall through to the existing collection validator.
func sniffTeamTemplate(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	var probe struct {
		TeamSecrets yaml.Node `yaml:"team_secrets"`
	}
	if err := yaml.Unmarshal(data, &probe); err != nil {
		return false, err
	}
	return probe.TeamSecrets.Kind != 0, nil
}

// validateTeamTemplate reads and validates a team template file, returning
// a *Result in the same shape as Validate.
func validateTeamTemplate(path string) *Result {
	result := &Result{FilePath: path, Valid: true, Kind: KindTeamTemplate}

	data, err := os.ReadFile(path)
	if err != nil {
		result.Valid = false
		result.Issues = append(result.Issues, Issue{
			Severity: SeverityError,
			FilePath: path,
			Message:  fmt.Sprintf("read file: %v", err),
		})
		return result
	}

	tpl, err := teamtemplate.Parse(data)
	if err != nil {
		result.Valid = false
		result.Issues = append(result.Issues, Issue{
			Severity: SeverityError,
			FilePath: path,
			Message:  err.Error(),
		})
		return result
	}

	tplIssues := tpl.Validate()
	if len(tplIssues) > 0 {
		result.Valid = false
		for _, ti := range tplIssues {
			result.Issues = append(result.Issues, Issue{
				Severity: SeverityError,
				FilePath: path,
				Line:     ti.Line,
				Message:  fmt.Sprintf("%s: %s", ti.Path, ti.Message),
			})
		}
	}

	if result.Valid {
		result.Summary = tpl.Summary()
	}
	return result
}

// Validate performs comprehensive validation on a collection file.
// It collects ALL issues rather than failing on the first one.
// knownVars optionally provides variable names known at validate time
// (e.g. from --var or --env-var flags), suppressing false-positive warnings.
func Validate(path string, knownVars map[string]string) *Result {
	result := &Result{FilePath: path, Valid: true}

	col, err := parser.ParseFile(path)
	if err != nil {
		line := 0
		message := err.Error()
		hint := ""
		var se *apierrors.Structured
		if errors.As(err, &se) {
			line = se.Line
			message = se.Message
			hint = se.Hint
		}
		result.Issues = append(result.Issues, Issue{
			Severity: SeverityError,
			FilePath: path,
			Line:     line,
			Message:  message,
			Hint:     hint,
		})
		result.Valid = false
		return result
	}

	// Additional structural checks not covered by ParseFile.
	allRequests := joinSections(col)
	for _, item := range allRequests {
		if item.Name == "" {
			result.Issues = append(result.Issues, Issue{
				Severity: SeverityError,
				FilePath: path,
				Message:  "request is missing required field 'name'",
				Hint:     "Every request must have a unique name for identification",
			})
			result.Valid = false
		}
	}
	if !result.Valid {
		return result
	}

	// Build known variable set: collection variables + caller-provided knownVars.
	known := knownCollectionVars(col)
	for k := range knownVars {
		known[k] = true
	}

	// Scan each request for potentially unresolved variable references.
	// Process requests in order so extract: keys from earlier requests are
	// available to later ones.
	warned := make(map[string]bool)
	for _, item := range allRequests {
		// Request-level variables are known within this request.
		for k := range item.Variables.Values {
			known[k] = true
		}

		refs := collectRequestRefs(&item)
		for _, ref := range refs {
			if !known[ref] && !warned[ref] {
				result.Issues = append(result.Issues, Issue{
					Severity: SeverityWarning,
					FilePath: path,
					Message:  fmt.Sprintf("variable %q may not be defined at runtime", ref),
					Hint:     "Variables can be defined via collection variables, --var, --env-var, --env, or .env file",
				})
				warned[ref] = true
			}
		}

		// Add extracted variables for downstream requests.
		for k := range item.Extract {
			known[k] = true
		}
	}

	// M19-001/M19-005: validate CEL if: and assertions: - cel: sites (parse + bool type-check).
	for _, iss := range validateCelSites(col) {
		result.Issues = append(result.Issues, iss)
		if iss.Severity == SeverityError {
			result.Valid = false
		}
	}

	// Retry status_ranges must parse; runtime skips unparsable ranges, so a
	// typo here would otherwise silently disable retries.
	for _, iss := range validateRetryRanges(col, path) {
		result.Issues = append(result.Issues, iss)
		if iss.Severity == SeverityError {
			result.Valid = false
		}
	}

	return result
}

// validateRetryRanges checks every retry status_ranges entry — at collection,
// section, and request level, in both retry_on and do_not_retry_on — against
// retry.ParseStatusRange and reports unparsable ranges as errors.
func validateRetryRanges(col *parser.Collection, filePath string) []Issue {
	var issues []Issue
	check := func(cfg *retry.FullConfig, fieldPath, srcFile string, srcLine int) {
		if cfg == nil {
			return
		}
		prefix := "retry"
		if fieldPath != "" {
			prefix = fieldPath + ".retry"
		}
		sites := map[string][]string{}
		if cfg.RetryOn != nil {
			sites["retry_on"] = cfg.RetryOn.StatusRanges
		}
		if cfg.DoNotRetryOn != nil {
			sites["do_not_retry_on"] = cfg.DoNotRetryOn.StatusRanges
		}
		for _, field := range []string{"retry_on", "do_not_retry_on"} {
			for _, r := range sites[field] {
				if _, _, err := retry.ParseStatusRange(r); err != nil {
					issues = append(issues, Issue{
						Severity: SeverityError,
						FilePath: srcFile,
						Line:     srcLine,
						Message:  fmt.Sprintf("%s.%s.status_ranges: %v", prefix, field, err),
						Hint:     `Status ranges use "min-max" (e.g. "500-599") or the "Nxx" shorthand (e.g. "5xx")`,
					})
				}
			}
		}
	}

	check(col.Retry, "", filePath, 0)
	sections := []struct {
		prefix string
		sec    *parser.Section
	}{
		{"setup", &col.Setup},
		{"requests", &col.Requests},
		{"teardown", &col.Teardown},
	}
	for _, s := range sections {
		check(s.sec.Retry, s.prefix, filePath, 0)
		for i := range s.sec.Items {
			item := &s.sec.Items[i]
			check(item.Retry, fmt.Sprintf("%s[%d]", s.prefix, i), item.SourceFile, item.SourceLine)
		}
	}
	return issues
}

// sectionDesc associates a field-path prefix with the request items in that section.
type sectionDesc struct {
	prefix string
	items  []parser.RequestItem
}

// validateCelSites compiles every CEL expression in the collection (if: on every
// section item; assertions.cel on every section item) and reports any parse or
// type error as a SeverityError Issue. Expressions must evaluate to bool;
// non-bool expressions produce ERR_CEL_TYPE. All sites are walked even if earlier
// sites fail — validate collects all issues rather than stopping at the first.
func validateCelSites(col *parser.Collection) []Issue {
	ev, err := apicel.NewEvaluator()
	if err != nil {
		return []Issue{{
			Severity: SeverityError,
			Message:  "internal: build CEL evaluator: " + err.Error(),
		}}
	}
	var issues []Issue
	sections := []sectionDesc{
		{"setup", col.Setup.Items},
		{"requests", col.Requests.Items},
		{"teardown", col.Teardown.Items},
	}
	for _, sec := range sections {
		for i, item := range sec.items {
			// if: site
			if item.If != "" {
				if iss, ok := compileCelSite(ev, item.If, fmt.Sprintf("%s[%d].if", sec.prefix, i), item.SourceFile, item.SourceLine); ok {
					issues = append(issues, iss)
				}
			}
			// assertions: - cel: sites
			for j, ca := range item.Assertions.CEL.Items {
				fieldPath := fmt.Sprintf("%s[%d].assertions[%d].cel", sec.prefix, i, j)
				if iss, ok := compileCelSite(ev, ca.Source, fieldPath, item.SourceFile, item.SourceLine); ok {
					issues = append(issues, iss)
				}
			}
		}
	}
	return issues
}

// compileCelSite compiles a single CEL expression and returns (issue, true) on
// failure or (zero, false) on success. Field path naming convention follows the
// v4.4 spec: section[i].if or section[i].assertions[j].cel.
func compileCelSite(ev apicel.Evaluator, src, fieldPath, srcFile string, srcLine int) (Issue, bool) {
	_, compileErr := ev.Compile(src, celgo.BoolType)
	if compileErr == nil {
		return Issue{}, false
	}
	var cerr *apicel.CelError
	if errors.As(compileErr, &cerr) {
		label := "ERR_CEL_PARSE"
		var body string
		if errors.Is(cerr.Sentinel, apicel.ErrCelType) {
			label = "ERR_CEL_TYPE"
			body = fmt.Sprintf("got %s, expected %s", cerr.Actual, cerr.Expected)
		} else {
			body = cerr.Inner
		}
		return Issue{
			Severity: SeverityError,
			FilePath: srcFile,
			Line:     srcLine,
			Message:  fmt.Sprintf("%s: %s: %s (source: %s)", fieldPath, label, body, cerr.Source),
			Hint:     celHint(cerr.Sentinel),
		}, true
	}
	return Issue{
		Severity: SeverityError,
		FilePath: srcFile,
		Line:     srcLine,
		Message:  fmt.Sprintf("%s: CEL compile error: %s", fieldPath, compileErr),
	}, true
}

// celHint returns a user-facing hint for the given CEL sentinel error.
func celHint(sentinel error) string {
	if errors.Is(sentinel, apicel.ErrCelType) {
		return "The expression must evaluate to bool. Example: response.body.status == 200"
	}
	return "Check the CEL expression syntax. See docs/MANUAL.md §3.10 Expression Language (CEL)."
}

// joinSections returns setup, requests, and teardown concatenated.
func joinSections(col *parser.Collection) []parser.RequestItem {
	total := len(col.Setup.Items) + len(col.Requests.Items) + len(col.Teardown.Items)
	all := make([]parser.RequestItem, 0, total)
	all = append(all, col.Setup.Items...)
	all = append(all, col.Requests.Items...)
	all = append(all, col.Teardown.Items...)
	return all
}

// knownCollectionVars returns a set of variable names statically defined in the collection.
func knownCollectionVars(col *parser.Collection) map[string]bool {
	known := make(map[string]bool, len(col.Variables.Values))
	for k := range col.Variables.Values {
		known[k] = true
	}
	return known
}

// collectRequestRefs returns all {{varName}} references found in a request item's
// string fields (URL, headers, query params, body).
func collectRequestRefs(item *parser.RequestItem) []string {
	var refs []string
	refs = append(refs, variable.FindReferences(item.Request.URL)...)
	for _, v := range item.Request.Headers {
		refs = append(refs, variable.FindReferences(v)...)
	}
	for _, v := range item.Request.QueryParams {
		refs = append(refs, variable.FindReferences(v)...)
	}
	refs = append(refs, collectAllStringRefs(item.Request.Body)...)
	return refs
}

// collectAllStringRefs recursively extracts {{varName}} references from any value.
func collectAllStringRefs(v any) []string {
	switch val := v.(type) {
	case string:
		return variable.FindReferences(val)
	case map[string]interface{}:
		var refs []string
		for _, elem := range val {
			refs = append(refs, collectAllStringRefs(elem)...)
		}
		return refs
	case []interface{}:
		var refs []string
		for _, elem := range val {
			refs = append(refs, collectAllStringRefs(elem)...)
		}
		return refs
	default:
		return nil
	}
}
