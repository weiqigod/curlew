package discovery

import "strings"

// matchPattern reports whether path matches pattern.
//
// Supported syntax (path uses forward slashes):
//   - *    matches any sequence of non-separator characters
//   - ?    matches any single non-separator character
//   - [...] character class as accepted by filepath.Match
//   - **   matches zero or more path segments (including separators)
func matchPattern(pattern, path string) bool {
	// Delegate to the recursive helper.
	return matchSegments(pattern, path)
}

// matchSegments is the core recursive matcher.
// Both pattern and path use forward slashes.
func matchSegments(pattern, path string) bool {
	for len(pattern) > 0 {
		// Find next double-star.
		if strings.HasPrefix(pattern, "**/") {
			rest := pattern[3:] // after **/ e.g. "*.yaml"
			// ** matches zero or more segments: try consuming each prefix of path.
			// Try matching rest against every suffix of path (including the whole path).
			if matchSegments(rest, path) {
				return true
			}
			// Consume one path segment and recurse.
			slash := strings.Index(path, "/")
			if slash == -1 {
				return false
			}
			path = path[slash+1:]
			// keep pattern as-is (** can match multiple segments)
			continue
		}
		if pattern == "**" {
			// ** alone matches everything remaining.
			return true
		}

		// No ** at front — process one segment at a time.
		patSeg, patRest := splitFirst(pattern, '/')
		pathSeg, pathRest := splitFirst(path, '/')

		if pathSeg == "" && patSeg == "" {
			return pathRest == "" && patRest == ""
		}
		if pathSeg == "" {
			return false
		}

		if !matchGlob(patSeg, pathSeg) {
			return false
		}

		pattern = patRest
		path = pathRest
	}

	// Pattern exhausted — path must also be exhausted.
	return path == ""
}

// splitFirst splits s on the first occurrence of sep.
// Returns (before, after); after is "" if sep not found (and before == s).
func splitFirst(s string, sep byte) (string, string) {
	i := strings.IndexByte(s, sep)
	if i == -1 {
		return s, ""
	}
	return s[:i], s[i+1:]
}

// matchGlob matches a single path segment against a single pattern segment
// (no path separators in either). Supports *, ?, and [...] via manual
// implementation matching filepath.Match semantics.
func matchGlob(pattern, name string) bool {
	for len(pattern) > 0 {
		switch pattern[0] {
		case '*':
			// Consume all consecutive '*' (but since we handle ** above, here
			// each * is a single-segment wildcard).
			for len(pattern) > 0 && pattern[0] == '*' {
				pattern = pattern[1:]
			}
			if len(pattern) == 0 {
				// Trailing * matches everything remaining in name.
				return true
			}
			// Try matching pattern against each suffix of name.
			for i := 0; i <= len(name); i++ {
				if matchGlob(pattern, name[i:]) {
					return true
				}
			}
			return false
		case '?':
			if len(name) == 0 {
				return false
			}
			pattern = pattern[1:]
			name = name[1:]
		case '[':
			// Character class: find matching ']'.
			end := strings.IndexByte(pattern[1:], ']')
			if end == -1 {
				// Malformed class — treat '[' as literal.
				if len(name) == 0 || name[0] != pattern[0] {
					return false
				}
				pattern = pattern[1:]
				name = name[1:]
				continue
			}
			class := pattern[1 : end+1]
			pattern = pattern[end+2:]
			if len(name) == 0 {
				return false
			}
			if !matchClass(class, name[0]) {
				return false
			}
			name = name[1:]
		default:
			if len(name) == 0 || name[0] != pattern[0] {
				return false
			}
			pattern = pattern[1:]
			name = name[1:]
		}
	}
	return name == ""
}

// matchClass reports whether c matches the character class expr (contents between '[' and ']').
// Supports negated classes with '^' or '!' as first character, and '-' for ranges.
func matchClass(expr string, c byte) bool {
	negate := false
	if len(expr) > 0 && (expr[0] == '^' || expr[0] == '!') {
		negate = true
		expr = expr[1:]
	}
	matched := false
	for len(expr) > 0 {
		if len(expr) >= 3 && expr[1] == '-' {
			// Range: lo-hi
			lo := expr[0]
			hi := expr[2]
			if c >= lo && c <= hi {
				matched = true
			}
			expr = expr[3:]
		} else {
			if expr[0] == c {
				matched = true
			}
			expr = expr[1:]
		}
	}
	if negate {
		return !matched
	}
	return matched
}
