package markdown

// BodyCapBytes is the maximum number of body bytes rendered in markdown.
// Bodies larger than this are truncated and a footer marker is emitted.
// The cap is applied AFTER redaction so secret rewrites can never be
// split across the truncation boundary.
const BodyCapBytes = 1 << 20 // 1 MiB

// truncateForMarkdown returns body unchanged when its length is at or below
// limit. Otherwise the first limit bytes are returned along with truncated=true
// and the original (pre-limit) length. Callers use original to render the
// truncation marker.
func truncateForMarkdown(body []byte, limit int) (out []byte, truncated bool, original int) {
	if len(body) <= limit {
		return body, false, len(body)
	}
	return body[:limit], true, len(body)
}
