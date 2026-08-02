package variable

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	apierrors "github.com/peterlindqvist/apitest/internal/errors"
)

// hmacHexLower computes HMAC-SHA256 over msg under key and returns the
// canonical lowercase 64-char hex (matches sha256sum(1) and every webhook
// signature spec we target).
func hmacHexLower(msg, key []byte) string {
	h := hmac.New(sha256.New, key)
	h.Write(msg)
	return fmt.Sprintf("%x", h.Sum(nil))
}

// stripeWebhookSig builds the Stripe-Signature header value
//
//	t=<timestamp>,v1=<hex>
//
// where <hex> is HMAC-SHA256("<timestamp>.<body>", secret) per
// https://stripe.com/docs/webhooks/signatures (signed_payload construction).
// timestamp is the Unix-seconds integer string.
func stripeWebhookSig(body, secret, timestamp string) string {
	signedPayload := timestamp + "." + body
	mac := hmacHexLower([]byte(signedPayload), []byte(secret))
	return "t=" + timestamp + ",v1=" + mac
}

// githubWebhookSig builds the X-Hub-Signature-256 header value
//
//	sha256=<hex>
//
// where <hex> is HMAC-SHA256(body, secret) per
// https://docs.github.com/en/webhooks/using-webhooks/validating-webhook-deliveries.
// The legacy x-hub-signature (SHA-1) form is intentionally not produced.
func githubWebhookSig(body, secret string) string {
	mac := hmacHexLower([]byte(body), []byte(secret))
	return "sha256=" + mac
}

// slackWebhookSig builds the X-Slack-Signature header value
//
//	v0=<hex>
//
// where <hex> is HMAC-SHA256("v0:<timestamp>:<body>", secret) per
// https://api.slack.com/authentication/verifying-requests-from-slack
// (the version + colon + timestamp + colon + body construction).
func slackWebhookSig(body, secret, timestamp string) string {
	basestring := "v0:" + timestamp + ":" + body
	mac := hmacHexLower([]byte(basestring), []byte(secret))
	return "v0=" + mac
}

// decodeJWTSegment splits token on '.', base64-url-decodes the segment at
// segIdx (0 = header, 1 = claims), JSON-roundtrips the bytes via the
// standard encoding/json marshaller (alphabetical key sort), and returns
// the canonical compact JSON string. funcName is the user-facing function
// name ("jwtDecodeHeader" or "jwtDecodeClaims") used to shape the Hint
// field of any structured error.
//
// On failure returns a *apierrors.Structured with Category=CategoryInput
// and one of three stable codes:
//
//   - DYNFN_JWT_DECODE_BAD_FORMAT  — token does not split into exactly 3
//     dot-separated segments (zero, one, two, or four+).
//   - DYNFN_JWT_DECODE_BAD_BASE64  — target segment is not valid
//     base64-url (RFC 4648 §5, no padding).
//   - DYNFN_JWT_DECODE_BAD_JSON    — base64 decode succeeded but the
//     bytes are not valid JSON.
//
// The Message field carries the offending input truncated to 32 chars
// (with "..." marker when truncated) so long tokens neither flood logs
// nor leak in full. Signature segment (segments[2]) is intentionally
// NOT examined — this is decode-only, not verification. See MANUAL.md
// §3.7 for the security caveat.
func decodeJWTSegment(funcName, token string, segIdx int) (string, error) {
	snippet := token
	if len(snippet) > 32 {
		snippet = snippet[:32] + "..."
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", &apierrors.Structured{
			Category: apierrors.CategoryInput,
			Code:     "DYNFN_JWT_DECODE_BAD_FORMAT",
			Message:  fmt.Sprintf("expected 3 dot-separated segments, got %d; input prefix=%q", len(parts), snippet),
			Hint:     fmt.Sprintf("Pass a JWT (three '.'-separated base64-url segments) to $%s.", funcName),
		}
	}
	decoded, err := base64.RawURLEncoding.DecodeString(parts[segIdx])
	if err != nil {
		return "", &apierrors.Structured{
			Category: apierrors.CategoryInput,
			Code:     "DYNFN_JWT_DECODE_BAD_BASE64",
			Message:  fmt.Sprintf("segment %d is not valid base64-url: %v; input prefix=%q", segIdx, err, snippet),
			Hint:     "JWT segments must use the URL-safe base64 alphabet (RFC 4648 §5) without '=' padding.",
			Inner:    err,
		}
	}
	var v interface{}
	if err := json.Unmarshal(decoded, &v); err != nil {
		return "", &apierrors.Structured{
			Category: apierrors.CategoryInput,
			Code:     "DYNFN_JWT_DECODE_BAD_JSON",
			Message:  fmt.Sprintf("segment %d is not valid JSON: %v; input prefix=%q", segIdx, err, snippet),
			Hint:     "Each JWT segment must base64-url-decode to a JSON object.",
			Inner:    err,
		}
	}
	out, err := json.Marshal(v)
	if err != nil {
		// json.Marshal of an interface{} populated by json.Unmarshal cannot
		// fail in practice — interface{} only holds JSON-representable values.
		// Report defensively rather than panic.
		return "", &apierrors.Structured{
			Category: apierrors.CategoryInput,
			Code:     "DYNFN_JWT_DECODE_BAD_JSON",
			Message:  fmt.Sprintf("segment %d failed JSON re-marshal: %v; input prefix=%q", segIdx, err, snippet),
			Hint:     "Each JWT segment must base64-url-decode to a JSON object.",
			Inner:    err,
		}
	}
	return string(out), nil
}
