package retry

import (
	"testing"
)

func TestBuiltinDefaults(t *testing.T) {
	d := BuiltinDefaults()

	if d.Enabled == nil || *d.Enabled != false {
		t.Error("BuiltinDefaults().Enabled should be false")
	}
	if d.MaxAttempts == nil || *d.MaxAttempts != DefaultMaxAttempts {
		t.Errorf("BuiltinDefaults().MaxAttempts = %v, want %d", d.MaxAttempts, DefaultMaxAttempts)
	}
	if d.InitialDelayMs == nil || *d.InitialDelayMs != DefaultInitialDelayMs {
		t.Errorf("BuiltinDefaults().InitialDelayMs = %v, want %d", d.InitialDelayMs, DefaultInitialDelayMs)
	}
	if d.BackoffStrategy == nil || *d.BackoffStrategy != "exponential" {
		t.Errorf("BuiltinDefaults().BackoffStrategy = %v, want exponential", d.BackoffStrategy)
	}
	if d.MaxDelayMs == nil || *d.MaxDelayMs != 30000 {
		t.Errorf("BuiltinDefaults().MaxDelayMs = %v, want 30000", d.MaxDelayMs)
	}
	if d.Jitter == nil || *d.Jitter != false {
		t.Error("BuiltinDefaults().Jitter should be false")
	}
	if d.RespectRetryAfter == nil || *d.RespectRetryAfter != true {
		t.Error("BuiltinDefaults().RespectRetryAfter should be true")
	}
}

func TestMergeConfigs(t *testing.T) {
	tests := []struct {
		name    string
		base    FullConfig
		overlay FullConfig
		check   func(t *testing.T, result FullConfig)
	}{
		{
			name:    "scalar override - enabled replaces",
			base:    FullConfig{Enabled: BoolPtr(false)},
			overlay: FullConfig{Enabled: BoolPtr(true)},
			check: func(t *testing.T, result FullConfig) {
				t.Helper()
				if result.Enabled == nil || *result.Enabled != true {
					t.Errorf("Enabled = %v, want true", result.Enabled)
				}
			},
		},
		{
			name:    "scalar override - max_attempts replaces",
			base:    FullConfig{MaxAttempts: IntPtr(3)},
			overlay: FullConfig{MaxAttempts: IntPtr(10)},
			check: func(t *testing.T, result FullConfig) {
				t.Helper()
				if result.MaxAttempts == nil || *result.MaxAttempts != 10 {
					t.Errorf("MaxAttempts = %v, want 10", result.MaxAttempts)
				}
			},
		},
		{
			name:    "nil overlay field inherits base",
			base:    FullConfig{Enabled: BoolPtr(true), MaxAttempts: IntPtr(5)},
			overlay: FullConfig{MaxAttempts: IntPtr(10)},
			check: func(t *testing.T, result FullConfig) {
				t.Helper()
				if result.Enabled == nil || *result.Enabled != true {
					t.Error("Enabled should inherit true from base")
				}
				if result.MaxAttempts == nil || *result.MaxAttempts != 10 {
					t.Errorf("MaxAttempts = %v, want 10", result.MaxAttempts)
				}
			},
		},
		{
			name:    "array field replaces entirely - status_codes",
			base:    FullConfig{RetryOn: &RetryOnConfig{StatusCodes: []int{429, 503}}},
			overlay: FullConfig{RetryOn: &RetryOnConfig{StatusCodes: []int{500, 502}}},
			check: func(t *testing.T, result FullConfig) {
				t.Helper()
				if result.RetryOn == nil {
					t.Fatal("RetryOn is nil")
				}
				want := []int{500, 502}
				if len(result.RetryOn.StatusCodes) != len(want) {
					t.Fatalf("StatusCodes = %v, want %v", result.RetryOn.StatusCodes, want)
				}
				for i, v := range want {
					if result.RetryOn.StatusCodes[i] != v {
						t.Errorf("StatusCodes[%d] = %d, want %d", i, result.RetryOn.StatusCodes[i], v)
					}
				}
			},
		},
		{
			name:    "array field replaces entirely - methods",
			base:    FullConfig{RetryOn: &RetryOnConfig{Methods: []string{"GET", "HEAD"}}},
			overlay: FullConfig{RetryOn: &RetryOnConfig{Methods: []string{"GET"}}},
			check: func(t *testing.T, result FullConfig) {
				t.Helper()
				if len(result.RetryOn.Methods) != 1 || result.RetryOn.Methods[0] != "GET" {
					t.Errorf("Methods = %v, want [GET]", result.RetryOn.Methods)
				}
			},
		},
		{
			name: "object deep merge - retry_on sub-fields",
			base: FullConfig{RetryOn: &RetryOnConfig{
				StatusCodes:   []int{429},
				NetworkErrors: BoolPtr(true),
			}},
			overlay: FullConfig{RetryOn: &RetryOnConfig{
				StatusCodes: []int{500, 502},
			}},
			check: func(t *testing.T, result FullConfig) {
				t.Helper()
				if result.RetryOn == nil {
					t.Fatal("RetryOn is nil")
				}
				// StatusCodes replaced
				if len(result.RetryOn.StatusCodes) != 2 {
					t.Fatalf("StatusCodes = %v, want [500 502]", result.RetryOn.StatusCodes)
				}
				// NetworkErrors inherited
				if result.RetryOn.NetworkErrors == nil || *result.RetryOn.NetworkErrors != true {
					t.Error("NetworkErrors should inherit true from base")
				}
			},
		},
		{
			name: "object deep merge - inherits unset sub-fields",
			base: FullConfig{RetryOn: &RetryOnConfig{
				Timeouts:      BoolPtr(true),
				NetworkErrors: BoolPtr(false),
			}},
			overlay: FullConfig{RetryOn: &RetryOnConfig{
				Timeouts: BoolPtr(false),
			}},
			check: func(t *testing.T, result FullConfig) {
				t.Helper()
				if result.RetryOn.Timeouts == nil || *result.RetryOn.Timeouts != false {
					t.Error("Timeouts should be false from overlay")
				}
				if result.RetryOn.NetworkErrors == nil || *result.RetryOn.NetworkErrors != false {
					t.Error("NetworkErrors should inherit false from base")
				}
			},
		},
		{
			name:    "nil overlay retains base completely",
			base:    FullConfig{Enabled: BoolPtr(true), MaxAttempts: IntPtr(5)},
			overlay: FullConfig{},
			check: func(t *testing.T, result FullConfig) {
				t.Helper()
				if result.Enabled == nil || *result.Enabled != true {
					t.Error("Enabled should be true from base")
				}
				if result.MaxAttempts == nil || *result.MaxAttempts != 5 {
					t.Errorf("MaxAttempts = %v, want 5", result.MaxAttempts)
				}
			},
		},
		{
			name:    "both nil results in zero value",
			base:    FullConfig{},
			overlay: FullConfig{},
			check: func(t *testing.T, result FullConfig) {
				t.Helper()
				if result.Enabled != nil {
					t.Error("Enabled should be nil")
				}
				if result.MaxAttempts != nil {
					t.Error("MaxAttempts should be nil")
				}
			},
		},
		{
			name: "do_not_retry_on deep merge",
			base: FullConfig{DoNotRetryOn: &DoNotRetryOnConfig{
				StatusCodes: []int{401, 403},
				Methods:     []string{"DELETE"},
			}},
			overlay: FullConfig{DoNotRetryOn: &DoNotRetryOnConfig{
				StatusCodes: []int{404},
			}},
			check: func(t *testing.T, result FullConfig) {
				t.Helper()
				if result.DoNotRetryOn == nil {
					t.Fatal("DoNotRetryOn is nil")
				}
				if len(result.DoNotRetryOn.StatusCodes) != 1 || result.DoNotRetryOn.StatusCodes[0] != 404 {
					t.Errorf("StatusCodes = %v, want [404]", result.DoNotRetryOn.StatusCodes)
				}
				if len(result.DoNotRetryOn.Methods) != 1 || result.DoNotRetryOn.Methods[0] != "DELETE" {
					t.Errorf("Methods should inherit [DELETE], got %v", result.DoNotRetryOn.Methods)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MergeConfigs(tt.base, tt.overlay)
			tt.check(t, result)
		})
	}
}

func TestMergeAll(t *testing.T) {
	tests := []struct {
		name    string
		configs []FullConfig
		check   func(t *testing.T, result FullConfig)
	}{
		{
			name:    "empty input returns zero FullConfig",
			configs: nil,
			check: func(t *testing.T, result FullConfig) {
				t.Helper()
				if result.Enabled != nil {
					t.Error("Enabled should be nil")
				}
			},
		},
		{
			name:    "single config returned as-is",
			configs: []FullConfig{{Enabled: BoolPtr(true), MaxAttempts: IntPtr(5)}},
			check: func(t *testing.T, result FullConfig) {
				t.Helper()
				if result.Enabled == nil || *result.Enabled != true {
					t.Error("Enabled should be true")
				}
				if result.MaxAttempts == nil || *result.MaxAttempts != 5 {
					t.Errorf("MaxAttempts = %v, want 5", result.MaxAttempts)
				}
			},
		},
		{
			name: "three levels merged left to right",
			configs: []FullConfig{
				{Enabled: BoolPtr(false), MaxAttempts: IntPtr(3), InitialDelayMs: IntPtr(1000)},
				{Enabled: BoolPtr(true), MaxAttempts: IntPtr(5)},
				{MaxAttempts: IntPtr(10)},
			},
			check: func(t *testing.T, result FullConfig) {
				t.Helper()
				if *result.Enabled != true {
					t.Error("Enabled should be true (from second)")
				}
				if *result.MaxAttempts != 10 {
					t.Errorf("MaxAttempts = %d, want 10 (from third)", *result.MaxAttempts)
				}
				if *result.InitialDelayMs != 1000 {
					t.Errorf("InitialDelayMs = %d, want 1000 (from first)", *result.InitialDelayMs)
				}
			},
		},
		{
			name: "global false + collection true = enabled (Behavior 1)",
			configs: []FullConfig{
				{Enabled: BoolPtr(false)},
				{Enabled: BoolPtr(true)},
			},
			check: func(t *testing.T, result FullConfig) {
				t.Helper()
				if *result.Enabled != true {
					t.Error("collection true should override global false")
				}
			},
		},
		{
			name: "collection max 5 + request max 10 = 10 (Behavior 2)",
			configs: []FullConfig{
				{MaxAttempts: IntPtr(5)},
				{MaxAttempts: IntPtr(10)},
			},
			check: func(t *testing.T, result FullConfig) {
				t.Helper()
				if *result.MaxAttempts != 10 {
					t.Errorf("MaxAttempts = %d, want 10", *result.MaxAttempts)
				}
			},
		},
		{
			name: "global status_codes replaced by collection (Behavior 5)",
			configs: []FullConfig{
				{RetryOn: &RetryOnConfig{StatusCodes: []int{429, 503}}},
				{RetryOn: &RetryOnConfig{StatusCodes: []int{500, 502}}},
			},
			check: func(t *testing.T, result FullConfig) {
				t.Helper()
				want := []int{500, 502}
				if len(result.RetryOn.StatusCodes) != len(want) {
					t.Fatalf("StatusCodes = %v, want %v", result.RetryOn.StatusCodes, want)
				}
				for i, v := range want {
					if result.RetryOn.StatusCodes[i] != v {
						t.Errorf("StatusCodes[%d] = %d, want %d", i, result.RetryOn.StatusCodes[i], v)
					}
				}
			},
		},
		{
			name: "global network_errors inherited when collection omits (Behavior 6)",
			configs: []FullConfig{
				{RetryOn: &RetryOnConfig{NetworkErrors: BoolPtr(true), StatusCodes: []int{429}}},
				{RetryOn: &RetryOnConfig{StatusCodes: []int{500}}},
			},
			check: func(t *testing.T, result FullConfig) {
				t.Helper()
				if result.RetryOn.NetworkErrors == nil || *result.RetryOn.NetworkErrors != true {
					t.Error("NetworkErrors should inherit true from global")
				}
				if len(result.RetryOn.StatusCodes) != 1 || result.RetryOn.StatusCodes[0] != 500 {
					t.Errorf("StatusCodes = %v, want [500]", result.RetryOn.StatusCodes)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MergeAll(tt.configs...)
			tt.check(t, result)
		})
	}
}

func TestFullConfig_Resolve(t *testing.T) {
	t.Run("fully populated", func(t *testing.T) {
		fc := FullConfig{
			Enabled:        BoolPtr(true),
			MaxAttempts:    IntPtr(5),
			InitialDelayMs: IntPtr(2000),
		}
		got := fc.Resolve()
		assertConfigScalars(t, got, true, 5, 2000, "", 0, false, 0)
		if got.RetryOn != nil {
			t.Error("RetryOn should be nil")
		}
		if got.DoNotRetryOn != nil {
			t.Error("DoNotRetryOn should be nil")
		}
	})

	t.Run("nil fields use Config zero values", func(t *testing.T) {
		got := FullConfig{}.Resolve()
		assertConfigScalars(t, got, false, 0, 0, "", 0, false, 0)
	})

	t.Run("partial fields", func(t *testing.T) {
		got := FullConfig{Enabled: BoolPtr(true)}.Resolve()
		if !got.Enabled {
			t.Error("Enabled should be true")
		}
	})

	t.Run("backoff fields transferred", func(t *testing.T) {
		fc := FullConfig{
			Enabled:         BoolPtr(true),
			MaxAttempts:     IntPtr(5),
			InitialDelayMs:  IntPtr(2000),
			BackoffStrategy: StringPtr("linear"),
			MaxDelayMs:      IntPtr(10000),
			Jitter:          BoolPtr(true),
			JitterFactor:    Float64Ptr(0.2),
		}
		got := fc.Resolve()
		assertConfigScalars(t, got, true, 5, 2000, "linear", 10000, true, 0.2)
	})

	t.Run("retryOn transferred", func(t *testing.T) {
		fc := FullConfig{
			RetryOn: &RetryOnConfig{StatusCodes: []int{503}, Methods: []string{"GET"}},
		}
		got := fc.Resolve()
		if got.RetryOn == nil {
			t.Fatal("RetryOn should not be nil")
		}
		if len(got.RetryOn.StatusCodes) != 1 || got.RetryOn.StatusCodes[0] != 503 {
			t.Errorf("RetryOn.StatusCodes = %v, want [503]", got.RetryOn.StatusCodes)
		}
		if len(got.RetryOn.Methods) != 1 || got.RetryOn.Methods[0] != "GET" {
			t.Errorf("RetryOn.Methods = %v, want [GET]", got.RetryOn.Methods)
		}
		// Ensure it's a copy, not the same pointer.
		if got.RetryOn == fc.RetryOn {
			t.Error("RetryOn should be a copy, not the same pointer")
		}
	})

	t.Run("doNotRetryOn transferred", func(t *testing.T) {
		fc := FullConfig{
			DoNotRetryOn: &DoNotRetryOnConfig{StatusCodes: []int{501}},
		}
		got := fc.Resolve()
		if got.DoNotRetryOn == nil {
			t.Fatal("DoNotRetryOn should not be nil")
		}
		if len(got.DoNotRetryOn.StatusCodes) != 1 || got.DoNotRetryOn.StatusCodes[0] != 501 {
			t.Errorf("DoNotRetryOn.StatusCodes = %v, want [501]", got.DoNotRetryOn.StatusCodes)
		}
		if got.DoNotRetryOn == fc.DoNotRetryOn {
			t.Error("DoNotRetryOn should be a copy, not the same pointer")
		}
	})

	t.Run("nil stays nil", func(t *testing.T) {
		got := FullConfig{}.Resolve()
		if got.RetryOn != nil {
			t.Error("RetryOn should be nil")
		}
		if got.DoNotRetryOn != nil {
			t.Error("DoNotRetryOn should be nil")
		}
	})
}

// assertConfigScalars verifies the scalar fields of Config.
func assertConfigScalars(t *testing.T, got Config, enabled bool, maxAttempts, initialDelayMs int, strategy string, maxDelayMs int, jitter bool, jitterFactor float64) {
	t.Helper()
	if got.Enabled != enabled {
		t.Errorf("Enabled = %v, want %v", got.Enabled, enabled)
	}
	if got.MaxAttempts != maxAttempts {
		t.Errorf("MaxAttempts = %d, want %d", got.MaxAttempts, maxAttempts)
	}
	if got.InitialDelayMs != initialDelayMs {
		t.Errorf("InitialDelayMs = %d, want %d", got.InitialDelayMs, initialDelayMs)
	}
	if got.BackoffStrategy != strategy {
		t.Errorf("BackoffStrategy = %q, want %q", got.BackoffStrategy, strategy)
	}
	if got.MaxDelayMs != maxDelayMs {
		t.Errorf("MaxDelayMs = %d, want %d", got.MaxDelayMs, maxDelayMs)
	}
	if got.Jitter != jitter {
		t.Errorf("Jitter = %v, want %v", got.Jitter, jitter)
	}
	if got.JitterFactor != jitterFactor {
		t.Errorf("JitterFactor = %v, want %v", got.JitterFactor, jitterFactor)
	}
}
