package schedule_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/peterlindqvist/apitest/internal/worker/schedule"
)

func TestTypes_NextRunResponse_RoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		rawJSON string
		want    schedule.NextRunResponse
	}{
		{
			name:    "minimal payload",
			rawJSON: `{"run_id":"run_abc","schedule_id":"sched_xyz","collection_ref":"file:./testdata/api.yaml","env_vars":{},"claim_token":"00000000-0000-0000-0000-000000000001","deadline":"2026-05-11T10:00:00Z"}`,
			want: schedule.NextRunResponse{
				RunID:         "run_abc",
				ScheduleID:    "sched_xyz",
				CollectionRef: "file:./testdata/api.yaml",
				EnvVars:       map[string]string{},
				ClaimToken:    "00000000-0000-0000-0000-000000000001",
				Deadline:      time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC),
			},
		},
		{
			name:    "envVars populated",
			rawJSON: `{"run_id":"r","schedule_id":"s","collection_ref":"file:x","env_vars":{"BASE_URL":"https://api.example.com"},"claim_token":"00000000-0000-0000-0000-000000000002","deadline":"2026-05-11T10:00:00Z"}`,
			want: schedule.NextRunResponse{
				RunID:         "r",
				ScheduleID:    "s",
				CollectionRef: "file:x",
				EnvVars:       map[string]string{"BASE_URL": "https://api.example.com"},
				ClaimToken:    "00000000-0000-0000-0000-000000000002",
				Deadline:      time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Unmarshal
			var got schedule.NextRunResponse
			if err := json.Unmarshal([]byte(tc.rawJSON), &got); err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}
			if got.RunID != tc.want.RunID {
				t.Errorf("RunID = %q; want %q", got.RunID, tc.want.RunID)
			}
			if got.ScheduleID != tc.want.ScheduleID {
				t.Errorf("ScheduleID = %q; want %q", got.ScheduleID, tc.want.ScheduleID)
			}
			if got.CollectionRef != tc.want.CollectionRef {
				t.Errorf("CollectionRef = %q; want %q", got.CollectionRef, tc.want.CollectionRef)
			}
			if got.ClaimToken != tc.want.ClaimToken {
				t.Errorf("ClaimToken = %q; want %q", got.ClaimToken, tc.want.ClaimToken)
			}
			if !got.Deadline.Equal(tc.want.Deadline) {
				t.Errorf("Deadline = %v; want %v", got.Deadline, tc.want.Deadline)
			}
			for k, v := range tc.want.EnvVars {
				if got.EnvVars[k] != v {
					t.Errorf("EnvVars[%q] = %q; want %q", k, got.EnvVars[k], v)
				}
			}

			// Marshal round-trip: unmarshal back what we just marshalled
			data, err := json.Marshal(got)
			if err != nil {
				t.Fatalf("Marshal error: %v", err)
			}
			var got2 schedule.NextRunResponse
			if err := json.Unmarshal(data, &got2); err != nil {
				t.Fatalf("second Unmarshal error: %v", err)
			}
			if got2.RunID != tc.want.RunID {
				t.Errorf("round-trip RunID = %q; want %q", got2.RunID, tc.want.RunID)
			}
		})
	}
}

func TestTypes_ResultRequest_RoundTrip(t *testing.T) {
	req := schedule.ResultRequest{
		ClaimToken:     "tok-1",
		CollectionName: "My API",
		RunAt:          time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC),
		DurationMs:     1234,
		PassCount:      3,
		FailCount:      1,
		SkippedCount:   0,
		TriggeredBy:    "schedule",
		Items: []schedule.ResultItem{
			{Name: "GET /health", Status: "passed", DurationMs: 100},
			{Name: "POST /users", Status: "failed", DurationMs: 50, Message: "status 500 != 201"},
		},
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got schedule.ResultRequest
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.ClaimToken != req.ClaimToken {
		t.Errorf("ClaimToken = %q; want %q", got.ClaimToken, req.ClaimToken)
	}
	if got.PassCount != req.PassCount {
		t.Errorf("PassCount = %d; want %d", got.PassCount, req.PassCount)
	}
	if len(got.Items) != 2 {
		t.Errorf("Items len = %d; want 2", len(got.Items))
	}
	if got.Items[1].Message != "status 500 != 201" {
		t.Errorf("Items[1].Message = %q; want %q", got.Items[1].Message, "status 500 != 201")
	}
}

func TestSentinelErrors_Distinct(t *testing.T) {
	errs := []error{
		schedule.ErrNoRunAvailable,
		schedule.ErrClaimReaped,
		schedule.ErrAlreadyCompleted,
		schedule.ErrUnsupportedCollectionRef,
		schedule.ErrPostExhausted,
	}
	for i, a := range errs {
		for j, b := range errs {
			if i == j {
				continue
			}
			if a == b {
				t.Errorf("errors[%d] == errors[%d]: %v", i, j, a)
			}
		}
	}
}
