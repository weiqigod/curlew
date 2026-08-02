package cel

import (
	"strings"
	"unicode"
)

// rootIdents is the set of top-level activation identifiers whose references
// we track in failure messages.
var rootIdents = map[string]bool{
	"response": true,
	"previous": true,
	"vars":     true,
	"env":      true,
}

// CollectTopLevelRefs returns the formatted-source form of every top-level
// named reference in src. A "top-level named reference" is a term (operand
// to a binary comparison or boolean operator at depth 0) whose leftmost
// token is one of {response, previous, vars, env}.
//
// The result is in source order, deduplicated. Returns nil if no matching
// references are found.
//
// ev is accepted for interface consistency but is not used: ref collection
// works directly on the source text to avoid CEL macro-expansion complexity.
func CollectTopLevelRefs(src string, _ Evaluator) []string {
	terms := splitTopLevel(strings.TrimSpace(src))
	var refs []string
	seen := map[string]bool{}
	for _, term := range terms {
		t := strings.TrimSpace(term)
		if t == "" {
			continue
		}
		// Check if the leftmost identifier is an activation root.
		root := leftmostToken(t)
		if root == "" || !rootIdents[root] {
			continue
		}
		if !seen[t] {
			seen[t] = true
			refs = append(refs, t)
		}
	}
	return refs
}

// splitTopLevel splits src at top-level binary comparison and boolean
// operators (==, !=, <=, >=, <, >, &&, ||) — operators not enclosed within
// parentheses, brackets, or braces. Returns the operand terms without the
// operator tokens.
func splitTopLevel(src string) []string {
	var terms []string
	depth := 0 // paren/bracket/brace depth
	start := 0
	i := 0
	runes := []rune(src)
	n := len(runes)
	for i < n {
		ch := runes[i]
		switch ch {
		case '(', '[', '{':
			depth++
			i++
		case ')', ']', '}':
			if depth > 0 {
				depth--
			}
			i++
		case '"', '\'', '`':
			// Skip string literals.
			i = skipStringLiteral(runes, i)
		default:
			if depth > 0 {
				i++
				continue
			}
			// Check for two-character operators first.
			if i+1 < n {
				two := string(runes[i : i+2])
				if two == "==" || two == "!=" || two == "<=" || two == ">=" || two == "&&" || two == "||" {
					term := string(runes[start:i])
					terms = append(terms, term)
					i += 2
					start = i
					continue
				}
			}
			// Single-character comparison operators (only < and >).
			if (ch == '<' || ch == '>') && (i+1 >= n || (runes[i+1] != '<' && runes[i+1] != '>')) {
				term := string(runes[start:i])
				terms = append(terms, term)
				i++
				start = i
				continue
			}
			i++
		}
	}
	// Append any trailing term.
	if start < n {
		terms = append(terms, string(runes[start:]))
	}
	return terms
}

// skipStringLiteral advances past a quoted string literal starting at
// position i. Handles both single and double quotes with backslash escaping.
// Returns the index after the closing quote.
func skipStringLiteral(runes []rune, i int) int {
	quote := runes[i]
	i++
	for i < len(runes) {
		ch := runes[i]
		if ch == '\\' {
			i += 2
			continue
		}
		i++
		if ch == quote {
			break
		}
	}
	return i
}

// leftmostToken returns the first identifier token in s (stopping at any
// non-ident rune), or "" if s is empty or starts with a non-ident character.
func leftmostToken(s string) string {
	s = strings.TrimSpace(s)
	// Consume unary not prefix (! or NOT).
	if strings.HasPrefix(s, "!") {
		s = strings.TrimSpace(s[1:])
	}
	// Read identifier: starts with a letter or underscore.
	if s == "" || (!unicode.IsLetter(rune(s[0])) && s[0] != '_') {
		return ""
	}
	end := 0
	for end < len(s) && (unicode.IsLetter(rune(s[end])) || unicode.IsDigit(rune(s[end])) || s[end] == '_') {
		end++
	}
	return s[:end]
}
