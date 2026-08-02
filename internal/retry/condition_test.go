package retry

import (
	"strings"
	"testing"
)

func TestParseStatusRange(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantMin int
		wantMax int
		wantErr bool
	}{
		{"valid range 500-599", "500-599", 500, 599, false},
		{"valid range 400-499", "400-499", 400, 499, false},
		{"single code as range 503-503", "503-503", 503, 503, false},
		{"invalid format no dash", "500", 0, 0, true},
		{"invalid format too many dashes", "500-599-600", 0, 0, true},
		{"invalid min not number", "abc-599", 0, 0, true},
		{"invalid max not number", "500-xyz", 0, 0, true},
		{"min greater than max", "599-500", 0, 0, true},
		{"negative values", "-1-100", 0, 0, true},
		{"empty string", "", 0, 0, true},
		{"shorthand 5xx", "5xx", 500, 599, false},
		{"shorthand 4xx", "4xx", 400, 499, false},
		{"shorthand 1xx", "1xx", 100, 199, false},
		{"shorthand uppercase 5XX", "5XX", 500, 599, false},
		{"shorthand mixed case 4xX", "4xX", 400, 499, false},
		{"shorthand 0xx invalid", "0xx", 0, 0, true},
		{"shorthand bare xx invalid", "xx", 0, 0, true},
		{"shorthand single x invalid", "5x", 0, 0, true},
		{"shorthand two digits invalid", "50x", 0, 0, true},
		{"shorthand too many x invalid", "5xxx", 0, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMin, gotMax, err := ParseStatusRange(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseStatusRange(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if gotMin != tt.wantMin {
					t.Errorf("ParseStatusRange(%q) min = %d, want %d", tt.input, gotMin, tt.wantMin)
				}
				if gotMax != tt.wantMax {
					t.Errorf("ParseStatusRange(%q) max = %d, want %d", tt.input, gotMax, tt.wantMax)
				}
			}
		})
	}
}

func TestStatusInRanges(t *testing.T) {
	tests := []struct {
		name   string
		code   int
		ranges []string
		want   bool
	}{
		{"503 in 500-599", 503, []string{"500-599"}, true},
		{"500 in 500-599 (boundary)", 500, []string{"500-599"}, true},
		{"599 in 500-599 (boundary)", 599, []string{"500-599"}, true},
		{"499 not in 500-599", 499, []string{"500-599"}, false},
		{"600 not in 500-599", 600, []string{"500-599"}, false},
		{"429 in 400-499", 429, []string{"400-499"}, true},
		{"503 in multiple ranges", 503, []string{"400-499", "500-599"}, true},
		{"200 not in any range", 200, []string{"400-499", "500-599"}, false},
		{"empty ranges", 503, nil, false},
		{"invalid range ignored", 503, []string{"invalid"}, false},
		{"503 in 5xx shorthand", 503, []string{"5xx"}, true},
		{"500 in 5xx shorthand (boundary)", 500, []string{"5xx"}, true},
		{"599 in 5xx shorthand (boundary)", 599, []string{"5xx"}, true},
		{"499 not in 5xx shorthand", 499, []string{"5xx"}, false},
		{"600 not in 5xx shorthand", 600, []string{"5xx"}, false},
		{"429 in 4xx shorthand", 429, []string{"4xx"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StatusInRanges(tt.code, tt.ranges)
			if got != tt.want {
				t.Errorf("StatusInRanges(%d, %v) = %v, want %v", tt.code, tt.ranges, got, tt.want)
			}
		})
	}
}

func TestShouldRetry(t *testing.T) {
	tests := []struct {
		name         string
		method       string
		statusCode   int
		isNetworkErr bool
		isTimeout    bool
		retryOn      *RetryOnConfig
		doNotRetryOn *DoNotRetryOnConfig
		wantRetry    bool
		wantWarning  string
	}{
		// Status code exact match
		{
			"status 503 in retry_on.status_codes", "GET", 503, false, false,
			&RetryOnConfig{StatusCodes: []int{503}, Methods: []string{"GET"}}, nil, true, "",
		},
		{
			"status 400 not in retry_on.status_codes", "GET", 400, false, false,
			&RetryOnConfig{StatusCodes: []int{503}, Methods: []string{"GET"}}, nil, false, "",
		},

		// Status range match
		{
			"status 503 in range 500-599", "GET", 503, false, false,
			&RetryOnConfig{StatusRanges: []string{"500-599"}, Methods: []string{"GET"}}, nil, true, "",
		},
		{
			"status 499 not in range 500-599", "GET", 499, false, false,
			&RetryOnConfig{StatusRanges: []string{"500-599"}, Methods: []string{"GET"}}, nil, false, "",
		},

		// Exclusion wins
		{
			"501 excluded despite 500-599 range", "GET", 501, false, false,
			&RetryOnConfig{StatusRanges: []string{"500-599"}, Methods: []string{"GET"}},
			&DoNotRetryOnConfig{StatusCodes: []int{501}}, false, "",
		},
		{
			"501 excluded via do_not_retry_on.status_ranges", "GET", 501, false, false,
			&RetryOnConfig{StatusRanges: []string{"500-599"}, Methods: []string{"GET"}},
			&DoNotRetryOnConfig{StatusRanges: []string{"501-501"}}, false, "",
		},

		// Method restrictions
		{
			"POST not in default methods", "POST", 503, false, false,
			&RetryOnConfig{StatusCodes: []int{503}, Methods: []string{"GET", "HEAD"}}, nil, false, "",
		},
		{
			"POST allowed when in methods list", "POST", 503, false, false,
			&RetryOnConfig{StatusCodes: []int{503}, Methods: []string{"GET", "POST"}}, nil, true, "non-idempotent",
		},
		{
			"PATCH allowed with warning", "PATCH", 503, false, false,
			&RetryOnConfig{StatusCodes: []int{503}, Methods: []string{"GET", "PATCH"}}, nil, true, "non-idempotent",
		},
		{
			"GET no warning", "GET", 503, false, false,
			&RetryOnConfig{StatusCodes: []int{503}, Methods: []string{"GET"}}, nil, true, "",
		},

		// Method in do_not_retry_on.methods
		{
			"method excluded by do_not_retry_on", "GET", 503, false, false,
			&RetryOnConfig{StatusCodes: []int{503}, Methods: []string{"GET"}},
			&DoNotRetryOnConfig{Methods: []string{"GET"}}, false, "",
		},

		// Network errors
		{
			"network error with network_errors true", "GET", 0, true, false,
			&RetryOnConfig{NetworkErrors: BoolPtr(true), Methods: []string{"GET"}}, nil, true, "",
		},
		{
			"network error with network_errors false", "GET", 0, true, false,
			&RetryOnConfig{NetworkErrors: BoolPtr(false), Methods: []string{"GET"}}, nil, false, "",
		},
		{
			"network error with network_errors nil (default false)", "GET", 0, true, false,
			&RetryOnConfig{Methods: []string{"GET"}}, nil, false, "",
		},

		// Timeouts
		{
			"timeout with timeouts true", "GET", 0, false, true,
			&RetryOnConfig{Timeouts: BoolPtr(true), Methods: []string{"GET"}}, nil, true, "",
		},
		{
			"timeout with timeouts false", "GET", 0, false, true,
			&RetryOnConfig{Timeouts: BoolPtr(false), Methods: []string{"GET"}}, nil, false, "",
		},

		// Combined condition logic
		{
			"status matches AND method allowed AND NOT excluded", "GET", 503, false, false,
			&RetryOnConfig{StatusRanges: []string{"500-599"}, Methods: []string{"GET"}},
			&DoNotRetryOnConfig{StatusCodes: []int{501}}, true, "",
		},
		{
			"PATCH not in default methods no retry despite status match", "PATCH", 503, false, false,
			&RetryOnConfig{StatusCodes: []int{503}, Methods: []string{"GET", "HEAD"}}, nil, false, "",
		},

		// Nil configs use defaults
		{
			"nil retryOn uses hardcoded defaults GET 503", "GET", 503, false, false,
			nil, nil, true, "",
		},
		{
			"nil retryOn POST not retried", "POST", 503, false, false,
			nil, nil, false, "",
		},
		{
			"nil retryOn GET 400 not retried", "GET", 400, false, false,
			nil, nil, false, "",
		},
		{
			"nil retryOn GET network error retried", "GET", 0, true, false,
			nil, nil, true, "",
		},
		{
			"nil retryOn GET timeout retried", "GET", 0, false, true,
			nil, nil, true, "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := ShouldRetryInput{
				Method:       tt.method,
				StatusCode:   tt.statusCode,
				IsNetworkErr: tt.isNetworkErr,
				IsTimeout:    tt.isTimeout,
			}
			result := ShouldRetry(input, tt.retryOn, tt.doNotRetryOn)
			if result.Retry != tt.wantRetry {
				t.Errorf("ShouldRetry() Retry = %v, want %v", result.Retry, tt.wantRetry)
			}
			if tt.wantWarning != "" {
				if result.Warning == "" {
					t.Errorf("ShouldRetry() Warning is empty, want containing %q", tt.wantWarning)
				} else if !strings.Contains(result.Warning, tt.wantWarning) {
					t.Errorf("ShouldRetry() Warning = %q, want containing %q", result.Warning, tt.wantWarning)
				}
			} else {
				if result.Warning != "" {
					t.Errorf("ShouldRetry() Warning = %q, want empty", result.Warning)
				}
			}
		})
	}
}
