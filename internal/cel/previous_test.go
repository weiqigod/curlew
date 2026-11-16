package cel

import "testing"

func TestUsesPrevious(t *testing.T) {
	for src, want := range map[string]bool{
		"previous.status == 200":                                    true,
		"has(previous.body) && response.status == 200":              true,
		"response.body.items.exists(i, i == previous.status)":       true,
		`response.body.note == "previous"`:                          false,
		"response.status == 200 // previous":                        false,
		"vars.previous == 1":                                        false,
		"response.body.items.exists(previous, previous > 1)":        false,
		"response.body.items.all(x, x > 0) && previous.status == 1": true,
	} {
		got, err := UsesPrevious(src)
		if err != nil {
			t.Errorf("%q: %v", src, err)
			continue
		}
		if got != want {
			t.Errorf("UsesPrevious(%q) = %v, want %v", src, got, want)
		}
	}
}
