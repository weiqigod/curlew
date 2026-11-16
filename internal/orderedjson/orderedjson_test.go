package orderedjson

import (
	"encoding/json"
	"fmt"
	"testing"

	"gopkg.in/yaml.v3"
)

func decode(t *testing.T, src string) any {
	t.Helper()
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		t.Fatal(err)
	}
	v, err := FromYAML(&doc)
	if err != nil {
		t.Fatalf("FromYAML: %v", err)
	}
	return v
}

func TestFromYAML_KeepsOrderThroughAliasesAndPrints(t *testing.T) {
	v := decode(t, "base: &b {zeta: 1, alpha: [ {y: 1, x: 2} ]}\ncopy: *b\n")
	got, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"base":{"zeta":1,"alpha":[{"y":1,"x":2}]},"copy":{"zeta":1,"alpha":[{"y":1,"x":2}]}}`
	if string(got) != want {
		t.Errorf("JSON = %s, want %s", got, want)
	}
	if s := fmt.Sprintf("%v", v); s != want {
		t.Errorf("printed = %s, want %s", s, want)
	}
}

func TestFromYAML_MergeKeyFallsBackToPlainMap(t *testing.T) {
	v := decode(t, "base: &b {zeta: 1}\nmerged:\n  <<: *b\n  alpha: 2\n")
	obj, ok := v.(Object)
	if !ok {
		t.Fatalf("top level is %T, want Object", v)
	}
	merged, ok := obj.Values["merged"].(map[string]any)
	if !ok || merged["zeta"] != 1 || merged["alpha"] != 2 {
		t.Errorf("merged = %#v, want the merged map {zeta:1 alpha:2}", obj.Values["merged"])
	}
}
