package runner

import (
	"os"
	"reflect"
	"regexp"

	"github.com/weiqigod/curlew/internal/parser"
	"github.com/weiqigod/curlew/internal/variable"
)

// celVarPattern matches CEL references to the vars map: vars.name and vars["name"].
var celVarPattern = regexp.MustCompile(`\bvars(?:\.([A-Za-z_][A-Za-z0-9_]*)|\[\s*["']([^"']+)["']\s*\])`)

// usedSecretKeys returns the secret key names the run can reference, so only
// those are fetched. It is deliberately generous: every string in the parsed
// collection and its external files counts, plus everything reachable through
// variable values. Keys overridden by a higher-precedence source (collection
// values, --env-var, --var) are left out because their secret value is unused.
func usedSecretKeys(col *parser.Collection, vars VarSources, merged map[string]string) map[string]bool {
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

	visitStrings(reflect.ValueOf(col), add, 0)
	for _, path := range col.ExternalFiles {
		if data, err := os.ReadFile(path); err == nil {
			add(string(data))
		}
	}

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

	used := map[string]bool{}
	for name := range vars.Secrets.Keys {
		if !referenced[name] {
			continue
		}
		if _, ok := col.Variables.Values[name]; ok {
			continue
		}
		if _, ok := vars.EnvVar[name]; ok {
			continue
		}
		if _, ok := vars.CLI[name]; ok {
			continue
		}
		used[name] = true
	}
	return used
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
