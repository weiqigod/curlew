package docs

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// ProseRef is one checkable claim stated in a sentence, rather than in a
// table row.
//
// Tables are the tractable documentation surface (see inventory.go) and are
// now exhausted: 77 tables, 72 executed, 5 marked, 0 owed. Sentences are the
// larger, unswept one -- the cross-format correlation-id claim that broke
// silently (defect 8, MANUAL.md:2315) sat directly beside a table that had
// already been executed. It survived because the promise it broke was a
// sentence, and no register keyed on sentences existed yet.
type ProseRef struct {
	Doc      string // "MANUAL.md"
	Line     int    // 1-based line of the block the claim came from
	Heading  string // nearest preceding heading, "" before the first one
	Text     string // the sentence, whitespace-normalised
	Shape    string // "modal", "same-as", "written-appears", "given-curlew"
	Referent string // "code", "flag", "env", "exit", "file", "cmd"
	Exempt   string // reason from a preceding marker; "" when none
}

// String renders a reference the way a test failure should print it:
// clickable, with enough context to find the sentence without re-running the
// extractor.
func (r ProseRef) String() string {
	return fmt.Sprintf("%s:%d %s | %s", r.Doc, r.Line, r.Heading, r.Text)
}

// maxKeyText bounds how much of a claim's text becomes part of its key, the
// same trade-off docs/table-execution-baseline.txt makes for header cells: a
// key long enough to read, short enough to fit a register line.
const maxKeyText = 120

// Key identifies a claim by what it says rather than where it sits, so
// editing the prose around a claim does not silently retire its debt.
//
// Deliberate trade-off: rewording a claim changes its key, which retires the
// old debt line and re-registers the new text as new debt -- failing the
// build until the register is updated by hand. That is correct. A reworded
// claim is a new promise and deserves a fresh read, the same way a hash key
// was rejected for docs/table-execution-baseline.txt: every line on that
// register is a line someone can read and pay, and a key nobody can read is
// not payable.
func (r ProseRef) Key() string {
	text := r.Text
	if len(text) > maxKeyText {
		text = text[:maxKeyText]
	}
	return r.Doc + "\t" + r.Heading + "\t" + text
}

// ProseMarker exempts every claim in the block that follows it -- a
// paragraph or a list item, not just the next sentence.
//
//	<!-- doc-check: prose-not-executable a worked example, not a claim -->
//
// A marker is authored against a paragraph a human is looking at, and a
// paragraph routinely holds two or three extracted claims; a sentence-scoped
// marker would need stacking and would read as noise. The total is capped
// (see the inventory test) so the check cannot be hollowed out a claim at a
// time -- the cap counts markers, not the claims each one exempts.
const ProseMarker = "<!-- doc-check: prose-not-executable"

// ignoreNamesMarker is the region cmd/curlew/doc_prose_test.go already
// governs: a section that names removed things ON PURPOSE. The prose
// extractor reads it as an exclusion -- CLI_SPECIFICATION.md's "Deliberately
// Absent Surfaces" appendix leaks a claim about *removed* surfaces
// otherwise, and a section that exists to name absent things cannot hold an
// executable claim -- but never writes to that machinery.
const ignoreNamesMarker = "<!-- doc-check: ignore-names -->"

// ExtractProse pulls checkable claims out of markdown source.
//
// It takes the text rather than a filename so the shrink-only guards in
// prose_test.go can be proven against synthetic documents, in all three
// directions (new debt, stale debt, zero claims), the same way AuditProse
// does.
//
// A sentence becomes a claim only when it matches a checkable shape AND
// names a concrete referent -- a flag, an environment variable, an exit
// code, a filename, a `curlew ...` command, or any backticked code span.
// That conjunction is the precision lever: a narrow extractor that catches
// real claims beats a broad one whose register nobody empties.
func ExtractProse(doc, src string) []ProseRef {
	var out []ProseRef
	for _, b := range blockify(src) {
		for _, sentence := range splitSentences(b.text) {
			shape := matchShape(sentence)
			if shape == "" {
				continue
			}
			if declineSentence(sentence) {
				continue
			}
			referent, ok := matchReferent(sentence)
			if !ok {
				continue
			}
			out = append(out, ProseRef{
				Doc:      doc,
				Line:     b.startLine,
				Heading:  b.heading,
				Text:     sentence,
				Shape:    shape,
				Referent: referent,
				Exempt:   b.exempt,
			})
		}
	}
	return out
}

// ProseInventory is ExtractProse over a document in Dir.
func ProseInventory(doc string) ([]ProseRef, error) {
	src, err := ReadDoc(doc)
	if err != nil {
		return nil, err
	}
	return ExtractProse(doc, src), nil
}

// proseBlock is one paragraph or list item, reflowed to a single line, after
// fenced code, tables, headings, blockquotes and HTML comments have been
// filtered out.
type proseBlock struct {
	startLine int
	heading   string
	text      string
	exempt    string
}

// listMarkerRe recognises the line a list item starts on. Each match begins
// a new block even without a blank line before it -- consecutive bullets are
// still separate claims, not one merged paragraph.
var listMarkerRe = regexp.MustCompile(`^([-*+]|\d+\.)\s+`)

// blockify splits markdown source into the prose blocks worth extracting
// claims from. It is a line-classifying state machine over fence, table,
// heading and marker state shared across cases, which is why it reads as one
// function rather than several.
func blockify(src string) []proseBlock {
	var (
		blocks        []proseBlock
		heading       string
		pendingExempt string
		ignoring      bool
		inFence       bool
		cur           []string
		curStart      int
		curExempt     string
	)

	flush := func() {
		if len(cur) == 0 {
			return
		}
		text := strings.Join(strings.Fields(strings.Join(cur, " ")), " ")
		if !ignoring && text != "" {
			blocks = append(blocks, proseBlock{startLine: curStart, heading: heading, text: text, exempt: curExempt})
		}
		cur = nil
		curExempt = ""
	}

	for i, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		lineNum := i + 1

		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			flush()
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}

		switch {
		case strings.HasPrefix(trimmed, "#"):
			flush()
			heading = strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
			// A heading always closes an ignore-names region: the marker
			// text in cmd/curlew/doc_prose_test.go documents the same rule
			// ("until the next markdown heading").
			ignoring = false
			pendingExempt = ""
		case strings.Contains(trimmed, ignoreNamesMarker):
			flush()
			ignoring = true
		case strings.HasPrefix(trimmed, ProseMarker):
			flush()
			pendingExempt = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, ProseMarker), "-->"))
		case strings.HasPrefix(trimmed, "<!--"):
			flush()
		case strings.HasPrefix(trimmed, ">"):
			flush()
		case strings.HasPrefix(trimmed, "|"):
			flush()
		case trimmed == "":
			flush()
		case listMarkerRe.MatchString(trimmed):
			flush()
			curStart = lineNum
			curExempt = pendingExempt
			pendingExempt = ""
			cur = append(cur, listMarkerRe.ReplaceAllString(trimmed, ""))
		default:
			if len(cur) == 0 {
				curStart = lineNum
				curExempt = pendingExempt
				pendingExempt = ""
			}
			cur = append(cur, trimmed)
		}
	}
	flush()
	return blocks
}

// closingMarkupRunes are markdown emphasis/quote characters that can sit
// between a sentence-ending mark and the whitespace that follows it --
// "...the same UUID.** If you..." ends its sentence after the "**", not
// before it.
const closingMarkupRunes = "*_'\""

// splitSentences splits a reflowed block into sentences on '.', '!', '?' and
// ';', each guarded against firing on an abbreviation, a bare initial, a
// mid-number decimal, or punctuation inside an open backtick span.
//
// Splitting on ';' is deliberate: MANUAL.md:2315 states two independent
// promises separated by a semicolon, and each is separately checkable.
func splitSentences(text string) []string {
	var sentences []string
	start := 0
	backtickOpen := false
	n := len(text)

	for i := 0; i < n; i++ {
		c := text[i]
		if c == '`' {
			backtickOpen = !backtickOpen
			continue
		}
		if c != '.' && c != '!' && c != '?' && c != ';' {
			continue
		}
		if backtickOpen {
			continue // punctuation inside an inline code span
		}

		j := i + 1
		for j < n && strings.ContainsRune(closingMarkupRunes, rune(text[j])) {
			j++
		}
		if j < n && !unicode.IsSpace(rune(text[j])) {
			continue // not followed by whitespace or end of text: mid-word, mid-number
		}
		if c == '.' && isAbbreviation(text[:i+1]) {
			continue
		}
		if c == '.' && isSingleLetterToken(text[:i]) {
			continue
		}

		if sentence := strings.TrimSpace(text[start:j]); sentence != "" {
			sentences = append(sentences, sentence)
		}
		start = j
	}
	if tail := strings.TrimSpace(text[start:]); tail != "" {
		sentences = append(sentences, tail)
	}
	return sentences
}

// abbreviationSuffixes are Latin/English abbreviations whose trailing period
// must not be read as a sentence boundary.
var abbreviationSuffixes = []string{"e.g.", "i.e.", "etc.", "cf.", "vs."}

// isAbbreviation reports whether upToDot -- the text up to and including the
// candidate period -- ends in a known abbreviation.
func isAbbreviation(upToDot string) bool {
	lower := strings.ToLower(upToDot)
	for _, ab := range abbreviationSuffixes {
		if strings.HasSuffix(lower, ab) {
			return true
		}
	}
	return false
}

// isSingleLetterToken reports whether the character immediately before a
// candidate period is a letter forming a one-character token of its own --
// an enumeration marker ("Step A.") or an initial ("J. Smith") rather than
// the end of a word.
func isSingleLetterToken(upToButNotIncludingDot string) bool {
	if upToButNotIncludingDot == "" {
		return false
	}
	runes := []rune(upToButNotIncludingDot)
	last := runes[len(runes)-1]
	if !unicode.IsLetter(last) {
		return false
	}
	if len(runes) == 1 {
		return true
	}
	before := runes[len(runes)-2]
	return unicode.IsSpace(before) || before == '(' || before == '['
}

// Shapes are the checkable sentence structures measured against the two CLI
// documents. Each sentence is classified by the first shape it matches, in
// this priority order.
const (
	shapeModal             = "modal"
	shapeSameAs            = "same-as"
	shapeWrittenAppears    = "written-appears"
	shapeGivenCurlew       = "given-curlew"
	referentCode           = "code"
	referentFlag           = "flag"
	referentEnv            = "env"
	referentExit           = "exit"
	referentFile           = "file"
	referentCommand        = "cmd"
	imperativeAdviceAlways = "always"
	imperativeAdviceNever  = "never"
)

var (
	modalRe = regexp.MustCompile(`(?i)\b(always|never|every|must|cannot|no longer)\b`)

	sameAsRe = regexp.MustCompile(`(?i)\b(the same|identical to|matches the|link a|links a|corresponds to)\b`)

	writtenAppearsRe = regexp.MustCompile(
		`(?i)\b(is|are)\s+(written|emitted|logged|recorded)\s+(to|in)\b|\bappears in\b`)

	givenCurlewVerbs = `exits|writes|emits|prints|reads|returns|resolves|sends|generates|produces|` +
		`redacts|records|validates|parses|reports|measures|treats|disables|enables|rejects|accepts|` +
		`binds|mints|applies|loads|stores|keeps|uses|runs|makes|needs|requires|supports`
	givenCurlewRe = regexp.MustCompile(`(?i)\bcurlew\s+(` + givenCurlewVerbs + `)\b`)
)

// matchShape returns the shape a sentence matches, or "" when it states
// nothing checkable in a recognised form.
func matchShape(s string) string {
	switch {
	case modalRe.MatchString(s):
		return shapeModal
	case sameAsRe.MatchString(s):
		return shapeSameAs
	case writtenAppearsRe.MatchString(s):
		return shapeWrittenAppears
	case givenCurlewRe.MatchString(s):
		return shapeGivenCurlew
	}
	return ""
}

var (
	// crossReferenceRe matches a sentence that points a reader elsewhere
	// rather than making a claim itself.
	crossReferenceRe = regexp.MustCompile(`(?i)\bsee \[`)
	// ifYouRe and reachForRe are the narrow second-person framings that mark
	// advice rather than a claim. A blanket \byou\b filter was tried and
	// rejected: it discarded "You cannot set both path: and request: on the
	// same item", a genuine, parser-enforced prohibition.
	ifYouRe    = regexp.MustCompile(`(?i)^if you\b`)
	reachForRe = regexp.MustCompile(`(?i)\breach for\b`)
	shouldRe   = regexp.MustCompile(`(?i)\bshould\b`)
)

// declineSentence reports whether a sentence that matched a shape should
// still be declined: imperative guidance addressed to the reader, or a
// cross-reference to another document. Nothing in the binary can be asked to
// demonstrate either.
func declineSentence(s string) bool {
	if startsWithImperative(s) {
		return true
	}
	return crossReferenceRe.MatchString(s) || ifYouRe.MatchString(s) ||
		reachForRe.MatchString(s) || shouldRe.MatchString(s)
}

// startsWithImperative reports whether a sentence opens with "Always" or
// "Never" used as a bare imperative ("Always source HMAC keys...") rather
// than as a modal describing the binary's behaviour ("Colour is never
// emitted..."). The subject-first form has a word before always/never; the
// imperative form does not.
func startsWithImperative(s string) bool {
	trimmed := strings.TrimLeft(s, closingMarkupRunes+"`")
	lower := strings.ToLower(trimmed)
	return strings.HasPrefix(lower, imperativeAdviceAlways+" ") || strings.HasPrefix(lower, imperativeAdviceNever+" ")
}

var (
	backtickSpanRe = regexp.MustCompile("`[^`]+`")
	flagRe         = regexp.MustCompile(`--[a-z][a-z0-9-]*`)
	envVarRe       = regexp.MustCompile(`CURLEW_[A-Z0-9_]+`)
	exitCodeRe     = regexp.MustCompile(`(?i)\bexit code\b|\bexit\s+\d+\b`)
	commandRe      = regexp.MustCompile(`\bcurlew\s+[a-z][a-z-]*\b`)
	filenameRe     = regexp.MustCompile(`\b[\w.-]+\.(md|yaml|yml|json|txt|go)\b`)
)

// matchReferent reports whether a sentence names something concrete enough
// to check, and what kind. Any backticked code span counts first: the
// documents backtick nearly every flag, filename and command they name, so
// it is the referent that covers the most ground.
func matchReferent(s string) (string, bool) {
	switch {
	case backtickSpanRe.MatchString(s):
		return referentCode, true
	case flagRe.MatchString(s):
		return referentFlag, true
	case envVarRe.MatchString(s):
		return referentEnv, true
	case exitCodeRe.MatchString(s):
		return referentExit, true
	case commandRe.MatchString(s):
		return referentCommand, true
	case filenameRe.MatchString(s):
		return referentFile, true
	}
	return "", false
}
