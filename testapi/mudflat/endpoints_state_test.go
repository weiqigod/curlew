package mudflat

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

// sessionURL builds a session-scoped URL with an id unique to the test, so tests
// running in parallel cannot contaminate one another.
//
// The id is filtered to the character set §7 allows rather than having a few
// known-bad characters replaced. Subtest names reach here too, and a subtest
// named "?times=abc" turned the path into a query string — the request then
// matched a different route and returned 301, which looks like a routing bug
// and is not one.
func sessionURL(t *testing.T, base, path string) string {
	t.Helper()

	var sid strings.Builder
	for _, r := range t.Name() {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			sid.WriteRune(r)
		default:
			sid.WriteRune('-')
		}
	}

	id := sid.String()
	if len(id) > 60 {
		id = id[:60]
	}
	if err := ValidateSessionID(id); err != nil {
		t.Fatalf("test helper produced an invalid session id %q: %v", id, err)
	}
	return fmt.Sprintf("%s/s/%s%s", base, id, path)
}

func TestFlaky_FailsNTimesThenSucceeds(t *testing.T) {
	base, client := startServer(t)
	url := sessionURL(t, base, "/flaky/fail-then-succeed?times=2&status=503")

	for attempt := 1; attempt <= 4; attempt++ {
		resp, body := get(t, client, url)
		switch {
		case attempt <= 2:
			if resp.StatusCode != http.StatusServiceUnavailable {
				t.Errorf("attempt %d: status = %d, want 503", attempt, resp.StatusCode)
			}
		default:
			if resp.StatusCode != http.StatusOK {
				t.Errorf("attempt %d: status = %d, want 200", attempt, resp.StatusCode)
			}
		}

		var payload struct {
			Attempt int `json:"attempt"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("attempt %d: body is not JSON: %v (%q)", attempt, err, body)
		}
		if payload.Attempt != attempt {
			t.Errorf("attempt %d: reported attempt = %d", attempt, payload.Attempt)
		}
	}
}

func TestFlaky_AttemptCountersAreScopedToASession(t *testing.T) {
	base, client := startServer(t)

	a := base + "/s/flaky-a/flaky/fail-then-succeed?times=1"
	b := base + "/s/flaky-b/flaky/fail-then-succeed?times=1"

	if resp, _ := get(t, client, a); resp.StatusCode == http.StatusOK {
		t.Fatal("session a: first attempt should fail")
	}
	// Session b must start its own sequence, not inherit a's progress.
	if resp, _ := get(t, client, b); resp.StatusCode == http.StatusOK {
		t.Error("session b saw session a's attempt counter (§6.2 isolation)")
	}
}

func TestFlaky_CountersAreKeyedByQuery(t *testing.T) {
	base, client := startServer(t)

	one := sessionURL(t, base, "/flaky/fail-then-succeed?times=1&tag=one")
	two := sessionURL(t, base, "/flaky/fail-then-succeed?times=1&tag=two")

	if resp, _ := get(t, client, one); resp.StatusCode == http.StatusOK {
		t.Fatal("tag=one: first attempt should fail")
	}
	if resp, _ := get(t, client, two); resp.StatusCode == http.StatusOK {
		t.Error("tag=two shared a counter with tag=one; counters are per (endpoint, query)")
	}
}

func TestFlaky_FailOnListedAttempts(t *testing.T) {
	base, client := startServer(t)
	url := sessionURL(t, base, "/flaky/fail-on?attempts=1,3")

	want := []int{500, 200, 500, 200}
	for i, wantStatus := range want {
		resp, _ := get(t, client, url)
		if resp.StatusCode != wantStatus {
			t.Errorf("attempt %d: status = %d, want %d", i+1, resp.StatusCode, wantStatus)
		}
	}
}

func TestFlaky_ResetsConnectionThenSucceeds(t *testing.T) {
	base, client := startServer(t)
	url := sessionURL(t, base, "/flaky/reset-then-succeed?times=1")

	if _, err := client.Get(url); err == nil {
		t.Error("first attempt returned a response; it should be a network error")
	}

	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("second attempt: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("second attempt: status = %d, want 200", resp.StatusCode)
	}
}

func TestRetryAfter_Seconds(t *testing.T) {
	base, client := startServer(t)

	resp, _ := get(t, client, base+"/retry-after/seconds?n=3")
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("status = %d, want 429", resp.StatusCode)
	}
	if got := resp.Header.Get("Retry-After"); got != "3" {
		t.Errorf("Retry-After = %q, want 3", got)
	}
}

func TestRetryAfter_HTTPDate(t *testing.T) {
	base, client := startServer(t)

	resp, _ := get(t, client, base+"/retry-after/date?ms=2000")
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("status = %d, want 429", resp.StatusCode)
	}
	raw := resp.Header.Get("Retry-After")
	if _, err := http.ParseTime(raw); err != nil {
		t.Errorf("Retry-After = %q, not a parseable HTTP-date: %v", raw, err)
	}
}

func TestRetryAfter_HugeValueIsSentVerbatim(t *testing.T) {
	// The clamp is the client's job (CLI_SPECIFICATION §9.4 caps it at 30s).
	// The server's job is to send a value large enough that a missing clamp is
	// unmistakable rather than merely slow.
	base, client := startServer(t)

	resp, _ := get(t, client, base+"/retry-after/huge")
	if got := resp.Header.Get("Retry-After"); got != "3600" {
		t.Errorf("Retry-After = %q, want 3600", got)
	}
}

func TestRetryAfter_MalformedValue(t *testing.T) {
	base, client := startServer(t)

	resp, _ := get(t, client, base+"/retry-after/malformed")
	if got := resp.Header.Get("Retry-After"); got != "soon" {
		t.Errorf("Retry-After = %q, want the unparseable literal", got)
	}
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("status = %d, want 429", resp.StatusCode)
	}
}

func doJSON(t *testing.T, client *http.Client, method, url, body string) (*http.Response, []byte) {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	out, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp, out
}

// resourceView is the package's own wire shape (endpoints_resources.go); the
// tests decode into it so a field rename cannot pass here and fail on the wire.

func TestResources_FullLifecycle(t *testing.T) {
	base, client := startServer(t)
	collection := sessionURL(t, base, "/resources")

	resp, body := doJSON(t, client, http.MethodPost, collection, `{"name":"first"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST status = %d, want 201: %s", resp.StatusCode, body)
	}

	var created resourceView
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatalf("POST body is not JSON: %v (%q)", err, body)
	}
	if created.ID == "" {
		t.Fatal("created resource has no id; extraction and chaining depend on it")
	}
	if loc := resp.Header.Get("Location"); loc == "" {
		t.Error("POST did not return a Location header")
	}

	item := collection + "/" + created.ID
	resp, body = doJSON(t, client, http.MethodGet, item, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET status = %d, want 200: %s", resp.StatusCode, body)
	}

	resp, _ = doJSON(t, client, http.MethodPut, item, `{"name":"second"}`)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("PUT status = %d, want 200", resp.StatusCode)
	}

	resp, _ = doJSON(t, client, http.MethodDelete, item, "")
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("DELETE status = %d, want 204", resp.StatusCode)
	}

	resp, _ = doJSON(t, client, http.MethodGet, item, "")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET after DELETE status = %d, want 404", resp.StatusCode)
	}
}

func TestResources_IDsAreDeterministic(t *testing.T) {
	base, client := startServer(t)
	collection := base + "/s/det-ids/resources"

	var got []string
	for range 3 {
		_, body := doJSON(t, client, http.MethodPost, collection, `{}`)
		var r resourceView
		if err := json.Unmarshal(body, &r); err != nil {
			t.Fatalf("POST body: %v", err)
		}
		got = append(got, r.ID)
	}

	want := []string{"res_det-ids_1", "res_det-ids_2", "res_det-ids_3"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("id[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestResources_OffsetPagination(t *testing.T) {
	base, client := startServer(t)
	collection := sessionURL(t, base, "/resources")

	for i := range 7 {
		doJSON(t, client, http.MethodPost, collection, fmt.Sprintf(`{"n":%d}`, i))
	}

	resp, body := doJSON(t, client, http.MethodGet, collection+"?offset=2&limit=3", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var page struct {
		Items []resourceView `json:"items"`
		Total int            `json:"total"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatalf("page is not JSON: %v (%q)", err, body)
	}
	if page.Total != 7 {
		t.Errorf("total = %d, want 7", page.Total)
	}
	if len(page.Items) != 3 {
		t.Fatalf("items = %d, want 3", len(page.Items))
	}
	if page.Items[0].Seq != 3 {
		t.Errorf("first item seq = %d, want 3 (offset 2 of a 1-based sequence)", page.Items[0].Seq)
	}
}

func TestResources_CursorPaginationWalksTheWholeSet(t *testing.T) {
	base, client := startServer(t)
	collection := sessionURL(t, base, "/resources")

	for i := range 5 {
		doJSON(t, client, http.MethodPost, collection, fmt.Sprintf(`{"n":%d}`, i))
	}

	seen := map[string]bool{}
	url := collection + "?limit=2"
	for range 10 {
		resp, body := doJSON(t, client, http.MethodGet, url, "")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}

		var page struct {
			Items      []resourceView `json:"items"`
			NextCursor string         `json:"next_cursor"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			t.Fatalf("page is not JSON: %v", err)
		}
		for _, item := range page.Items {
			if seen[item.ID] {
				t.Errorf("item %s returned twice across pages", item.ID)
			}
			seen[item.ID] = true
		}

		if page.NextCursor == "" {
			if link := resp.Header.Get("Link"); link != "" && strings.Contains(link, `rel="next"`) {
				t.Error("no next_cursor but Link still advertises a next page")
			}
			break
		}
		if link := resp.Header.Get("Link"); !strings.Contains(link, `rel="next"`) {
			t.Errorf("Link = %q, want a rel=\"next\" entry while paging", link)
		}
		url = collection + "?limit=2&cursor=" + page.NextCursor
	}

	if len(seen) != 5 {
		t.Errorf("walked %d distinct items, want 5", len(seen))
	}
}

func TestETag_ConditionalRequests(t *testing.T) {
	base, client := startServer(t)
	url := sessionURL(t, base, "/etag")

	resp, _ := doJSON(t, client, http.MethodGet, url, "")
	etag := resp.Header.Get("ETag")
	if etag == "" {
		t.Fatal("no ETag on the initial response")
	}

	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("If-None-Match", etag)
	matched, err := client.Do(req)
	if err != nil {
		t.Fatalf("If-None-Match request: %v", err)
	}
	defer func() { _ = matched.Body.Close() }()
	if matched.StatusCode != http.StatusNotModified {
		t.Errorf("If-None-Match status = %d, want 304", matched.StatusCode)
	}

	req, _ = http.NewRequest(http.MethodPut, url, strings.NewReader(`{"v":2}`))
	req.Header.Set("If-Match", `"definitely-not-the-etag"`)
	mismatched, err := client.Do(req)
	if err != nil {
		t.Fatalf("If-Match request: %v", err)
	}
	defer func() { _ = mismatched.Body.Close() }()
	if mismatched.StatusCode != http.StatusPreconditionFailed {
		t.Errorf("If-Match status = %d, want 412", mismatched.StatusCode)
	}
}

func TestIdempotency_ReplaysTheFirstResponse(t *testing.T) {
	base, client := startServer(t)
	url := sessionURL(t, base, "/idempotency")

	send := func(key, body string) (*http.Response, []byte) {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Idempotency-Key", key)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		t.Cleanup(func() { _ = resp.Body.Close() })
		out, _ := io.ReadAll(resp.Body)
		return resp, out
	}

	first, firstBody := send("key-1", `{"v":1}`)
	if first.Header.Get("Idempotent-Replay") != "false" {
		t.Errorf("first call replay header = %q, want false", first.Header.Get("Idempotent-Replay"))
	}

	second, secondBody := send("key-1", `{"v":2}`)
	if second.Header.Get("Idempotent-Replay") != "true" {
		t.Errorf("second call replay header = %q, want true", second.Header.Get("Idempotent-Replay"))
	}
	if string(firstBody) != string(secondBody) {
		t.Errorf("replay body differs:\nfirst:  %s\nsecond: %s", firstBody, secondBody)
	}

	_, otherBody := send("key-2", `{"v":3}`)
	if string(otherBody) == string(firstBody) {
		t.Error("a different Idempotency-Key replayed the first result")
	}
}

func TestSessionReset_ClearsState(t *testing.T) {
	base, client := startServer(t)
	sessionRoot := base + "/s/reset-me"

	doJSON(t, client, http.MethodPost, sessionRoot+"/resources", `{}`)

	resp, _ := doJSON(t, client, http.MethodDelete, sessionRoot, "")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE session status = %d, want 204", resp.StatusCode)
	}

	_, body := doJSON(t, client, http.MethodGet, sessionRoot+"/resources", "")
	var page struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatalf("list body: %v", err)
	}
	if page.Total != 0 {
		t.Errorf("total after reset = %d, want 0", page.Total)
	}
}

func TestSession_InvalidIDIsRejected(t *testing.T) {
	base, client := startServer(t)

	resp, body := get(t, client, base+"/s/bad%20id/resources")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a malformed session id", resp.StatusCode)
	}
	if !strings.Contains(string(body), "session") {
		t.Errorf("error does not mention the session: %s", body)
	}
}

func TestEchoDelay_TakesAtLeastTheRequestedTime(t *testing.T) {
	base, client := startServer(t)

	start := time.Now()
	resp, _ := get(t, client, base+"/echo/delay/120")
	elapsed := time.Since(start)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if elapsed < 120*time.Millisecond {
		t.Errorf("elapsed = %v, want at least 120ms", elapsed)
	}
}

func TestCapabilities_ReportsPhaseAndAbsentFamilies(t *testing.T) {
	base, client := startServer(t)

	_, body := get(t, client, base+"/capabilities")

	var doc struct {
		Phase    int               `json:"phase"`
		Families []string          `json:"families"`
		Absent   map[string]string `json:"Absent"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("capabilities is not JSON: %v (%q)", err, body)
	}
	if doc.Phase != 1 {
		t.Errorf("phase = %d, want 1", doc.Phase)
	}
	if len(doc.Families) == 0 {
		t.Error("no families reported")
	}
	if len(doc.Absent) == 0 {
		t.Error("absent families not reported; a caller cannot tell a missing family from a broken one")
	}
}

func TestStatus_CodeIsNotAffectedByAnotherSession(t *testing.T) {
	// Guards the isolation invariant at the routing level: a stateless endpoint
	// reached under a session prefix must behave identically to the bare form.
	base, client := startServer(t)

	bare, _ := get(t, client, base+"/status/418")
	scoped, _ := get(t, client, base+"/s/whatever/status/418")

	if bare.StatusCode != scoped.StatusCode {
		t.Errorf("bare = %d, session-scoped = %d; they must agree",
			bare.StatusCode, scoped.StatusCode)
	}
	if bare.StatusCode != http.StatusTeapot {
		t.Errorf("status = %d, want 418", bare.StatusCode)
	}
}

func TestFlaky_RejectsMalformedParameters(t *testing.T) {
	base, client := startServer(t)

	for _, q := range []string{"?times=abc", "?times=-1", "?status=99"} {
		t.Run(q, func(t *testing.T) {
			resp, _ := get(t, client, sessionURL(t, base, "/flaky/fail-then-succeed")+q)
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", resp.StatusCode)
			}
		})
	}
}

func TestNDJSON_RejectsNegativeCount(t *testing.T) {
	base, client := startServer(t)

	resp, _ := get(t, client, base+"/ndjson/"+strconv.Itoa(-1))
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}
