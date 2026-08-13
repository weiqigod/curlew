package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	gws "github.com/gorilla/websocket"

	"github.com/weiqigod/curlew/internal/assertion"
	"github.com/weiqigod/curlew/internal/parser"
	"github.com/weiqigod/curlew/internal/requtil"
	"github.com/weiqigod/curlew/internal/variable"
)

// Defaults used when the corresponding step field is unset.
const (
	defaultExpectTimeoutMs = 5000
	defaultCloseCode       = 1000
	closeWriteTimeout      = 5 * time.Second
)

// StepResult records the outcome of running a single WebSocketStep.
type StepResult struct {
	Action     string
	Passed     bool
	Err        error
	Assertions *assertion.Results // populated for expect steps
	Extracted  map[string]string  // populated for expect steps (variable name -> value)
	Duration   time.Duration
}

// Result is the aggregate outcome of running all WebSocket steps for a single
// request. Passed is true only if every executed step succeeded. Err holds the
// first error from a failed step (connection, timeout, send, close, or a
// synthesised "step N (action): assertions failed" error when a step failed
// only because its assertions did not match).
type Result struct {
	Steps    []StepResult
	Duration time.Duration
	Passed   bool
	Err      error
	Warnings []string // non-fatal warnings (e.g. buffer size exceeded)
}

// Execute dials the URL described by req, runs each WebSocket step
// sequentially, and returns the aggregate Result. Variables extracted inside
// expect steps are written to scope immediately so subsequent steps (and
// subsequent collection requests) can interpolate them.
//
// When dialer is nil, DefaultDialer is used. The returned Result is always
// non-nil; on dial failure Passed is false and Err wraps ErrDialFailed.
func Execute(ctx context.Context, req *parser.Request, scope *variable.Scope, dialer Dialer) *Result {
	if dialer == nil {
		dialer = DefaultDialer
	}
	start := time.Now()
	result := &Result{}

	if req == nil || req.WebSocket == nil {
		result.Err = fmt.Errorf("%w: missing websocket config", ErrDialFailed)
		result.Duration = time.Since(start)
		return result
	}
	if len(req.WebSocket.Steps) == 0 {
		result.Err = ErrNoSteps
		result.Duration = time.Since(start)
		return result
	}

	headers := http.Header{}
	for k, v := range req.Headers {
		headers.Set(k, v)
	}

	conn, dialResp, dialErr := dialer.Dial(ctx, req.URL, headers)
	if dialErr != nil {
		result.Err = describeDialFailure(dialResp, dialErr)
		result.Duration = time.Since(start)
		return result
	}
	defer func() {
		// Closing the connection is also what unblocks the pump's in-flight
		// read, so this must happen on every exit path.
		_ = conn.Close()
		result.Duration = time.Since(start)
	}()

	// The pump becomes the connection's sole reader for the rest of this
	// request. No step may call conn.ReadMessage directly: a second reader
	// would interleave mid-frame, and reads outside the pump reintroduce the
	// deadline problem the pump exists to remove (§11C.5).
	pump := startReadPump(conn)
	// A closure, not `defer pump.stop()`: the receiver would be bound now, and
	// a reconnect replaces pump — leaving the new one running.
	defer func() { pump.stop() }()

	// Start heartbeat goroutine (no-op when Heartbeat is nil or disabled).
	stopHeartbeat, heartbeatErr := startHeartbeat(ctx, conn, req.WebSocket.Heartbeat)
	defer stopHeartbeat()

	rstate := &reconnectState{cfg: req.WebSocket.Reconnect}

	buf := &messageBuffer{}
	for i, step := range req.WebSocket.Steps {
		if err := ctx.Err(); err != nil {
			result.Err = fmt.Errorf("step %d (%s): %w: %w", i+1, step.Action, ErrContextCanceled, err)
			return result
		}
		// Check for heartbeat failure before running the step.
		select {
		case hbErr, ok := <-heartbeatErr:
			if ok && hbErr != nil {
				result.Err = fmt.Errorf("step %d (%s): %w", i+1, step.Action, hbErr)
				return result
			}
		default:
		}
		sr := runStep(ctx, conn, pump, buf, step, scope)
		result.Steps = append(result.Steps, sr)
		if w := buf.drainWarning(); w != "" {
			result.Warnings = append(result.Warnings, w)
		}

		// Check for heartbeat failure that occurred during the step (e.g. while
		// runExpect was blocked on ReadMessage). A non-blocking receive catches
		// errors that the pre-step check missed.
		select {
		case hbErr, ok := <-heartbeatErr:
			if ok && hbErr != nil {
				result.Err = fmt.Errorf("step %d (%s): %w", i+1, step.Action, hbErr)
				return result
			}
		default:
		}

		// Reconnect branch: if the step failed due to a connection drop and
		// reconnect is enabled, attempt to redial and retry the step.
		for !sr.Passed && rstate.shouldReconnect(sr.Err) {
			if wErr := waitReconnect(ctx, rstate); wErr != nil {
				result.Err = fmt.Errorf("step %d (%s): %w", i+1, step.Action, wErr)
				return result
			}
			rstate.attempts++
			newConn, rErr := redial(ctx, dialer, req.URL, headers)
			if rErr != nil {
				// Dial itself failed; record as one attempt and let shouldReconnect
				// decide whether to keep trying on the next iteration.
				continue
			}
			_ = conn.Close()
			conn = newConn
			// Restart the pump on the new connection. Closing the old
			// connection above released the previous pump's blocked read.
			pump.stop()
			pump = startReadPump(conn)
			// Restart the heartbeat on the new connection; the old goroutine was
			// writing to a closed connection and may have already sent an error.
			stopHeartbeat()
			stopHeartbeat, heartbeatErr = startHeartbeat(ctx, conn, req.WebSocket.Heartbeat)
			// Reset buffer for the new connection.
			buf = &messageBuffer{}
			sr = runStep(ctx, conn, pump, buf, step, scope)
			// Remove the previous (failed) step result and replace with new.
			result.Steps = result.Steps[:len(result.Steps)-1]
			result.Steps = append(result.Steps, sr)
		}

		if !sr.Passed {
			if !rstate.shouldReconnect(sr.Err) && rstate.cfg != nil && rstate.cfg.Enabled && rstate.attempts > 0 {
				// Reconnect was attempted but ultimately failed.
				result.Err = fmt.Errorf("step %d (%s): %w after %d attempt(s)", i+1, step.Action, ErrReconnectExhausted, rstate.attempts)
			} else if sr.Err != nil {
				result.Err = fmt.Errorf("step %d (%s): %w", i+1, step.Action, sr.Err)
			} else {
				result.Err = fmt.Errorf("step %d (%s): assertions failed", i+1, step.Action)
			}
			return result
		}
		if step.Action == "close" {
			break
		}
	}

	result.Passed = true
	return result
}

func runStep(ctx context.Context, conn Conn, pump *readPump, buf *messageBuffer, step parser.WebSocketStep, scope *variable.Scope) StepResult {
	start := time.Now()
	var sr StepResult
	switch step.Action {
	case "send":
		sr = runSend(conn, step, scope)
	case "expect":
		sr = runExpect(ctx, pump, buf, step, scope)
	case "wait":
		sr = runWait(ctx, pump, buf, step)
	case "close":
		sr = runClose(conn, step, scope)
	default:
		sr = StepResult{Action: step.Action, Err: fmt.Errorf("%w: %q", ErrUnknownAction, step.Action)}
	}
	sr.Action = step.Action
	sr.Duration = time.Since(start)
	return sr
}

// runSend serialises the step payload and writes it to the connection.
// Per-step interpolation is applied so that variables extracted by earlier
// steps in the same WebSocket request (or by preceding requests) are
// resolved at send time.
func runSend(conn Conn, step parser.WebSocketStep, scope *variable.Scope) StepResult {
	var payload []byte
	switch {
	case step.MessageRawTemplate != "":
		data, err := renderTemplate(step, scope)
		if err != nil {
			return StepResult{Err: fmt.Errorf("%w: %w", ErrSendFailed, err)}
		}
		payload = data
	case step.MessageRaw != "":
		interp, err := scope.Interpolate(step.MessageRaw)
		if err != nil {
			return StepResult{Err: fmt.Errorf("%w: interpolate message_raw: %w", ErrSendFailed, err)}
		}
		payload = []byte(interp)
	case step.Message != nil:
		interp, err := scope.InterpolateBody(step.Message)
		if err != nil {
			return StepResult{Err: fmt.Errorf("%w: interpolate message: %w", ErrSendFailed, err)}
		}
		data, marshalErr := json.Marshal(interp)
		if marshalErr != nil {
			return StepResult{Err: fmt.Errorf("%w: marshal: %w", ErrSendFailed, marshalErr)}
		}
		payload = data
	default:
		return StepResult{Err: fmt.Errorf("%w: send requires message or message_raw", ErrSendFailed)}
	}
	if err := conn.WriteMessage(gws.TextMessage, payload); err != nil {
		return StepResult{Err: fmt.Errorf("%w: %w", ErrSendFailed, err)}
	}
	return StepResult{Passed: true}
}

// renderTemplate interpolates the MessageRawTemplate with step-local variables
// layered on top of scope using WithOverrides. Step variables do not leak into
// the parent scope.
func renderTemplate(step parser.WebSocketStep, scope *variable.Scope) ([]byte, error) {
	// Interpolate the step variable values themselves in case they reference
	// parent-scope variables.
	interpolatedVars := make(map[string]string, len(step.StepVariables))
	for k, v := range step.StepVariables {
		interp, err := scope.Interpolate(v)
		if err != nil {
			return nil, fmt.Errorf("interpolate variables[%q]: %w", k, err)
		}
		interpolatedVars[k] = interp
	}
	child, err := scope.WithOverrides(interpolatedVars)
	if err != nil {
		return nil, fmt.Errorf("build child scope: %w", err)
	}
	out, err := child.Interpolate(step.MessageRawTemplate)
	if err != nil {
		return nil, fmt.Errorf("interpolate message_template: %w", err)
	}
	return []byte(out), nil
}

// runExpect waits up to TimeoutMs for a message whose payload satisfies the
// step's assertions. It first checks the in-memory buffer for pre-arrived
// frames, then falls through to reading from the connection. Unmatched frames
// from the wire are pushed onto buf for consumption by later steps.
func runExpect(ctx context.Context, pump *readPump, buf *messageBuffer, step parser.WebSocketStep, scope *variable.Scope) StepResult {
	timeoutMs := step.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = defaultExpectTimeoutMs
	}
	deadline := time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)

	candidates, candErr := buildExpectCandidates(scope, step)
	if candErr != nil {
		return StepResult{Err: fmt.Errorf("%w: %w", ErrExpectAssertionVars, candErr)}
	}
	matchFn := func(data []byte) (*assertion.Results, bool) { return evalCandidates(candidates, data) }

	if ctx.Err() != nil {
		return StepResult{Err: fmt.Errorf("%w: %w", ErrContextCanceled, ctx.Err())}
	}
	if !time.Now().Before(deadline) {
		return StepResult{Err: fmt.Errorf("%w after %dms", ErrExpectTimeout, timeoutMs)}
	}

	count := step.Count
	if count <= 0 {
		count = 1
	}

	collected := 0
	var collectedVars []map[string]string
	var lastAssertions *assertion.Results

	// Predicate for buffer search: check if data matches any candidate.
	bufMatchFn := func(data []byte) bool {
		_, ok := matchFn(data)
		return ok
	}

	// Check buffer first before setting the read deadline.
	for collected < count {
		_, data := buf.takeMatch(bufMatchFn)
		if data == nil {
			break // no more buffered matches; fall through to wire reads
		}
		ar, _ := matchFn(data)
		lastAssertions = ar
		extracted, extErr := extractForMatch(step.Extract, data)
		if extErr != nil {
			return StepResult{Assertions: ar, Err: fmt.Errorf("%w: %w", ErrExtractFailed, extErr)}
		}
		collectedVars = append(collectedVars, extracted)
		collected++
	}

	if collected == count {
		return buildCountResult(step, collectedVars, count, lastAssertions, scope)
	}

	// Frames come from the pump, the connection's sole reader, and the timeout
	// is enforced here by a timer rather than by a read deadline. A deadline
	// would end the read by failing it, and a failed gorilla read is permanent —
	// it would poison every later step on the same connection (see readPump).
	timeout := time.NewTimer(time.Until(deadline))
	defer timeout.Stop()

	expired := func() StepResult {
		sr := StepResult{
			Err: fmt.Errorf("%w after %dms: collected %d/%d", ErrExpectTimeout, timeoutMs, collected, count),
		}
		if lastAssertions != nil {
			sr.Assertions = lastAssertions
		}
		return sr
	}

	for collected < count {
		var frame pumpFrame
		select {
		case <-ctx.Done():
			return StepResult{Err: fmt.Errorf("%w: %w", ErrContextCanceled, ctx.Err())}
		case <-timeout.C:
			return expired()
		case f, ok := <-pump.frames:
			if !ok {
				// The pump closed without an error frame: the connection is
				// gone and no further message can arrive.
				return expired()
			}
			frame = f
		}

		if frame.err != nil {
			if isTimeoutErr(frame.err) {
				return expired()
			}
			return StepResult{Err: fmt.Errorf("%w: read: %w", ErrExpectTimeout, frame.err)}
		}
		if len(frame.data) == 0 {
			continue
		}
		ar, ok := matchFn(frame.data)
		if !ok {
			// Stray frame: remember diff and buffer it for later steps.
			lastAssertions = ar
			buf.push(frame.data)
			continue
		}
		lastAssertions = ar
		extracted, extErr := extractForMatch(step.Extract, frame.data)
		if extErr != nil {
			return StepResult{Assertions: ar, Err: fmt.Errorf("%w: %w", ErrExtractFailed, extErr)}
		}
		collectedVars = append(collectedVars, extracted)
		collected++
	}

	return buildCountResult(step, collectedVars, count, lastAssertions, scope)
}

// buildExpectCandidates returns the list of assertion input sets to try for a
// given expect step. For a plain "message:" step there is exactly one set; for
// "any_of:" there is one set per alternative.
//
// Takes the scope because expected values interpolate like every other value in
// a collection: a step asserting against a token extracted by an earlier step
// would otherwise compare against the literal "{{token}}".
func buildExpectCandidates(scope *variable.Scope, step parser.WebSocketStep) ([][]assertion.BodyInput, error) {
	if len(step.AnyOf) > 0 {
		out := make([][]assertion.BodyInput, 0, len(step.AnyOf))
		for i, alt := range step.AnyOf {
			inputs, err := requtil.ToBodyInputs(scope, alt.Items)
			if err != nil {
				return nil, fmt.Errorf("any_of[%d]: %w", i, err)
			}
			out = append(out, inputs)
		}
		return out, nil
	}
	inputs, err := requtil.ToBodyInputs(scope, step.ExpectAssertions.Items)
	if err != nil {
		return nil, err
	}
	return [][]assertion.BodyInput{inputs}, nil
}

// evalCandidates runs each candidate assertion set against data in order.
// Returns the first passing result. If none pass, returns the last failing
// result with ok=false.
func evalCandidates(candidates [][]assertion.BodyInput, data []byte) (*assertion.Results, bool) {
	var last *assertion.Results
	for _, inputs := range candidates {
		results := assertion.CheckBody(inputs, data)
		ar := &assertion.Results{Items: results, Passed: allPassed(results)}
		if ar.Passed {
			return ar, true
		}
		last = ar
	}
	return last, false
}

// buildCountResult aggregates per-message extracted variables and returns a
// final StepResult. For count==1 the extracted map is returned flat; for
// count>1 each variable is encoded as a JSON array of per-message values.
func buildCountResult(step parser.WebSocketStep, collectedVars []map[string]string, count int, ar *assertion.Results, scope *variable.Scope) StepResult {
	aggregated := aggregateExtraction(step.Extract, collectedVars, count)
	for k, v := range aggregated {
		scope.Set(k, v)
	}
	return StepResult{Assertions: ar, Extracted: aggregated, Passed: true}
}

// extractForMatch evaluates extract expressions against a single frame body
// without writing to the scope. The caller aggregates and writes to scope
// after all N frames are collected.
func extractForMatch(extract map[string]string, body []byte) (map[string]string, error) {
	if len(extract) == 0 {
		return nil, nil
	}
	res, err := variable.Extract(variable.ExtractionInput{Extractions: extract, Body: body})
	if err != nil {
		return nil, err
	}
	return res.Variables, nil
}

// aggregateExtraction folds per-message extracted values into the final
// per-variable string. For count==1 the behaviour is identical to the single-
// message extract. For count>1 each variable becomes a JSON-encoded string
// array.
func aggregateExtraction(extract map[string]string, perMsg []map[string]string, count int) map[string]string {
	if len(extract) == 0 {
		return nil
	}
	if count == 1 {
		if len(perMsg) == 0 {
			return map[string]string{}
		}
		return perMsg[0]
	}
	out := make(map[string]string, len(extract))
	for name := range extract {
		arr := make([]string, 0, len(perMsg))
		for _, m := range perMsg {
			arr = append(arr, m[name])
		}
		data, marshalErr := json.Marshal(arr)
		if marshalErr != nil {
			return nil
		}
		out[name] = string(data)
	}
	return out
}

// runWait holds the connection for the step's duration — by READING it, not by
// sleeping on a timer.
//
// gorilla dispatches control frames from inside ReadMessage: the pong handler
// registered by the heartbeat runs only while something is reading. A wait that
// slept left the socket unread, so an incoming pong was never dispatched, the
// liveness flag stayed false, and the next tick declared a peer that had
// answered every ping dead. That inverted the feature — a heartbeat exists to
// hold an IDLE connection open, and this failed precisely when idle (§11C.5).
//
// Reading here rather than from a background goroutine keeps the single-reader
// invariant: two concurrent ReadMessage calls on one connection interleave
// mid-frame and corrupt the stream. Data frames that arrive during the wait go
// to the buffer, where a later expect step can still match them, so nothing is
// dropped by the change.
func runWait(ctx context.Context, pump *readPump, buf *messageBuffer, step parser.WebSocketStep) StepResult {
	if step.DurationMs <= 0 {
		return StepResult{Passed: true}
	}
	timer := time.NewTimer(time.Duration(step.DurationMs) * time.Millisecond)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return StepResult{Err: fmt.Errorf("%w: %w", ErrContextCanceled, ctx.Err())}
		case <-timer.C:
			// The wait elapsed with the pump blocked in ReadMessage throughout,
			// which is what let incoming pongs be dispatched.
			return StepResult{Passed: true}
		case frame, ok := <-pump.frames:
			if !ok {
				return StepResult{Err: fmt.Errorf("%w: connection closed", ErrWaitFailed)}
			}
			if frame.err != nil {
				// An orderly goodbye is not a fault. The hold ended early
				// because the peer said it was done, and anything it sent
				// first is in the buffer for the next step to match.
				if gws.IsCloseError(frame.err, gws.CloseNormalClosure, gws.CloseGoingAway) {
					return StepResult{Passed: true}
				}
				// Anything else is a real read failure: a broken connection or
				// an abnormal close. Reporting it lets the reconnect logic see
				// it, where a sleeping wait returned success on a dead
				// connection.
				return StepResult{Err: fmt.Errorf("%w: %w", ErrWaitFailed, frame.err)}
			}
			if len(frame.data) > 0 {
				// Buffered on this goroutine, never from the pump, because
				// messageBuffer is not safe for concurrent use. A message that
				// arrives during a wait is therefore still available to a later
				// expect step rather than being dropped.
				buf.push(frame.data)
			}
		}
	}
}

// runClose writes a close control frame. The reason field is interpolated at
// send time so that variables extracted by earlier steps resolve correctly.
func runClose(conn Conn, step parser.WebSocketStep, scope *variable.Scope) StepResult {
	code := step.Code
	if code == 0 {
		code = defaultCloseCode
	}
	reason := step.Reason
	if reason != "" {
		interp, err := scope.Interpolate(reason)
		if err != nil {
			return StepResult{Err: fmt.Errorf("%w: interpolate reason: %w", ErrCloseFailed, err)}
		}
		reason = interp
	}
	payload := gws.FormatCloseMessage(code, reason)
	if err := conn.WriteControl(gws.CloseMessage, payload, time.Now().Add(closeWriteTimeout)); err != nil {
		return StepResult{Err: fmt.Errorf("%w: %w", ErrCloseFailed, err)}
	}
	return StepResult{Passed: true}
}

func allPassed(results []assertion.Result) bool {
	for _, r := range results {
		if !r.Passed {
			return false
		}
	}
	return true
}

// isTimeoutErr reports whether err is a net.Error with Timeout() == true.
// This matches gorilla/websocket's read-deadline errors and our fake's
// timeoutError used in unit tests.
func isTimeoutErr(err error) bool {
	var ne interface{ Timeout() bool }
	if errors.As(err, &ne) {
		return ne.Timeout()
	}
	return false
}

// describeDialFailure builds the error for a failed WebSocket dial, including
// the refusing response when there is one.
//
// A server that declines an upgrade answers with an ordinary HTTP response, and
// that response is the only channel it has to explain itself: 426 with a
// supported-version header, 401 with an auth hint, 403, or a 500 carrying a
// gateway's error page. gorilla hands that response back alongside
// ErrBadHandshake — having already read up to 1024 bytes of the body into it —
// and curlew discarded it, so every refusal produced the single string
// "websocket dial failed: websocket: bad handshake" and the four cases were
// indistinguishable (§11C.4).
//
// The underlying error is still wrapped, so ErrDialFailed and gorilla's own
// sentinel both remain matchable.
func describeDialFailure(resp *http.Response, err error) error {
	if resp == nil {
		// No response: connection refused, DNS failure, a malformed URL. There
		// is nothing to report beyond the cause, and inventing a status here
		// would be worse than saying nothing.
		return fmt.Errorf("%w: %w", ErrDialFailed, err)
	}

	status := strings.TrimSpace(resp.Status)
	if status == "" {
		status = strconv.Itoa(resp.StatusCode)
	}
	detail := "server refused the upgrade with " + status
	if body := readDialFailureBody(resp); body != "" {
		detail += "; body: " + body
	}
	return fmt.Errorf("%w: %s (%w)", ErrDialFailed, detail, err)
}

// dialFailureBodyLimit caps how much of a refusing response is quoted. gorilla
// has already limited its read to 1024 bytes; this keeps a full HTML error page
// from burying the status it is attached to.
const dialFailureBodyLimit = 512

// readDialFailureBody returns the refusing response's body as a single line,
// truncated for display. An unreadable or empty body yields "", which keeps the
// message from announcing a body that is not there.
func readDialFailureBody(resp *http.Response) string {
	if resp.Body == nil {
		return ""
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, dialFailureBodyLimit+1))
	_ = resp.Body.Close()
	if err != nil && len(raw) == 0 {
		return ""
	}
	// An error page is often multi-line; a one-line message stays greppable in
	// CI output.
	body := strings.Join(strings.Fields(string(raw)), " ")
	if len(body) > dialFailureBodyLimit {
		body = body[:dialFailureBodyLimit] + "…"
	}
	return body
}
