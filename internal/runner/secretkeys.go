package runner

import (
	"errors"
	"reflect"
	"regexp"
	"strings"

	apicel "github.com/weiqigod/curlew/internal/cel"
	"github.com/weiqigod/curlew/internal/parser"
	"github.com/weiqigod/curlew/internal/variable"
)

// celVarPattern matches CEL references to the vars map: vars.name and vars["name"].
var celVarPattern = regexp.MustCompile(`\bvars(?:\.([A-Za-z_][A-Za-z0-9_]*)|\[\s*["']([^"']+)["']\s*\])`)

// runtimeCELPattern matches CEL inputs that only exist once requests have run.
var runtimeCELPattern = regexp.MustCompile(`\b(previous|response|row)\b`)

var errUnfetchedSecret = errors.New("secret not fetched while planning the run")

// planSecretKeys splits the secret keys into those fetched before the first
// request and those fetched on first use. Up front are keys referenced by
// requests that can run: setup, teardown and the --only selection, minus
// requests whose if: is already false from the starting variables. Every other
// key is deferred, so a guarded or unselected request never asks the provider,
// and a key the scan missed is still fetched when a request uses it. Keys
// overridden by collection values, --env-var or --var are neither.
func planSecretKeys(col *parser.Collection, vars VarSources, merged map[string]string) (upFront, deferred map[string]bool) {
	referenced := map[string]bool{}
	var pending []string
	add := func(text string) {
		for _, name := range variable.FindReferences(text) {
			if !referenced[name] {
				referenced[name] = true
				pending = append(pending, name)
			}
		}
		for _, m := range celVarPattern.FindAllStringSubmatch(text, -1) {
			name := m[1] + m[2]
			if !referenced[name] {
				referenced[name] = true
				pending = append(pending, name)
			}
		}
	}

	runnable := runnableItemFilter(col, vars, merged)
	view := *col
	view.Setup.Items = runnable(col.Setup.Items)
	view.Requests.Items = runnable(selectedMainItems(col.Requests.Items, vars.Selection))
	view.Teardown.Items = runnable(col.Teardown.Items)
	visitStrings(reflect.ValueOf(&view), add, 0)

	sources := []map[string]string{vars.Project, vars.EnvFile, vars.DotEnv, merged, col.Variables.Values, vars.EnvVar, vars.CLI}
	for len(pending) > 0 {
		name := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		for _, src := range sources {
			if v, ok := src[name]; ok {
				add(v)
			}
		}
	}

	upFront, deferred = map[string]bool{}, map[string]bool{}
	for name := range vars.Secrets.Keys {
		if _, ok := col.Variables.Values[name]; ok {
			continue
		}
		if _, ok := vars.EnvVar[name]; ok {
			continue
		}
		if _, ok := vars.CLI[name]; ok {
			continue
		}
		if referenced[name] {
			upFront[name] = true
		} else {
			deferred[name] = true
		}
	}
	return upFront, deferred
}

func selectedMainItems(items []parser.RequestItem, selection []string) []parser.RequestItem {
	if len(selection) == 0 {
		return items
	}
	filtered, err := filterMainItemsBySelection(items, selection)
	if err != nil {
		return items
	}
	return filtered
}

// runnableItemFilter drops items whose if: is false from the starting
// variables. Expressions that use previous, response, a secret, or that do not
// evaluate cleanly are kept: they can only be decided during the run.
func runnableItemFilter(col *parser.Collection, vars VarSources, merged map[string]string) func([]parser.RequestItem) []parser.RequestItem {
	keepAll := func(items []parser.RequestItem) []parser.RequestItem { return items }
	if !collectionHasIf(col) {
		return keepAll
	}
	start := map[string]string{}
	for _, src := range []map[string]string{merged, col.Variables.Values, vars.EnvVar, vars.CLI} {
		for k, v := range src {
			start[k] = v
		}
	}
	names := make([]string, 0, len(vars.Secrets.Keys))
	for name := range vars.Secrets.Keys {
		names = append(names, name)
	}
	placeholders := variable.NewDeferred(names, func(string) (string, error) { return "", errUnfetchedSecret })
	scope := variable.NewScope(start).WithDeferred(placeholders)
	if err := scope.Resolve(); err != nil {
		return keepAll
	}
	resolved := scope.Resolved()
	varsAny := make(map[string]any, len(resolved))
	for k, v := range resolved {
		varsAny[k] = v
	}
	ev, err := vars.ensureCelEvaluator()
	if err != nil {
		return keepAll
	}
	cache := map[string]apicel.Program{}
	skips := func(expr string) bool {
		if expr == "" || runtimeCELPattern.MatchString(expr) {
			return false
		}
		for _, m := range celVarPattern.FindAllStringSubmatch(expr, -1) {
			name := m[1] + m[2]
			if vars.Secrets.Keys[name] != "" || strings.Contains(resolved[name], "{{") {
				return false
			}
		}
		prog, err := compileIfProgram(cache, ev, expr)
		if err != nil {
			return false
		}
		out, err := prog.Eval(apicel.StandardActivation{Vars: varsAny, Env: vars.EnvVar}, apicel.EvalOptions{})
		if err != nil {
			return false
		}
		open, ok := out.(bool)
		return ok && !open
	}
	return func(items []parser.RequestItem) []parser.RequestItem {
		kept := make([]parser.RequestItem, 0, len(items))
		for _, item := range items {
			if !skips(item.If) {
				kept = append(kept, item)
			}
		}
		return kept
	}
}

// visitStrings calls visit for every string reachable from v.
func visitStrings(v reflect.Value, visit func(string), depth int) {
	if depth > 64 || !v.IsValid() {
		return
	}
	switch v.Kind() {
	case reflect.String:
		visit(v.String())
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			visitStrings(v.Elem(), visit, depth+1)
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			visitStrings(v.Field(i), visit, depth+1)
		}
	case reflect.Map:
		iter := v.MapRange()
		for iter.Next() {
			visitStrings(iter.Key(), visit, depth+1)
			visitStrings(iter.Value(), visit, depth+1)
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			visitStrings(v.Index(i), visit, depth+1)
		}
	}
}
