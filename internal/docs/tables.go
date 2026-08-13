// Package docs reads the markdown tables in docs/ so that tests can execute
// what those tables claim.
//
// The documents make three kinds of statement about the binary, and each has
// now been wrong at least once. Examples are caught by parsing them
// (internal/parser). Tables are not: no parser will ever reject a markdown
// table, so a table can promise behaviour the binary does not have and nothing
// notices — which is exactly what happened to the GraphQL error-handling
// matrix, where the documents described four outcomes against three modes and
// the binary ignored the mode for half of them.
//
// This package exists so a test can read a table and run it, rather than
// restating it in Go and letting the copy drift.
package docs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Dir is the documentation directory, relative to a package under internal/.
const Dir = "../../docs"

// Table finds the first markdown table in the named document whose header row
// contains every cell in headerCells, and returns that header and the rows
// beneath it.
//
// Cells are trimmed and stripped of backticks, so a header written as
// `fail` matches "fail".
func Table(doc string, headerCells ...string) (header []string, rows [][]string, err error) {
	path := filepath.Join(Dir, doc)
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		return nil, nil, fmt.Errorf("reading %s: %w", path, readErr)
	}

	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), "|") {
			continue
		}
		cells := SplitRow(line)
		if !containsAll(cells, headerCells) {
			continue
		}
		// The line after the header is the |---|---| separator; rows run until
		// the table ends.
		for _, r := range lines[i+2:] {
			if !strings.HasPrefix(strings.TrimSpace(r), "|") {
				break
			}
			rows = append(rows, SplitRow(r))
		}
		return cells, rows, nil
	}
	return nil, nil, fmt.Errorf("no table in %s with header cells %v", path, headerCells)
}

// Data is one table: where it is, its header, and its rows.
type Data struct {
	Ref    TableRef
	Header []string
	Rows   [][]string
}

// AllTables returns every table in the document whose header contains
// headerCells, rather than only the first.
//
// Some claims are made by a family of tables rather than by one: the manual
// lists dynamic functions across twelve tables split by category. A test that
// reads only the first is a test that stops noticing the moment someone adds a
// thirteenth, so the reader takes the whole family and the inventory credits
// every table it returns.
func AllTables(doc string, headerCells ...string) ([]Data, error) {
	refs, err := Inventory(doc)
	if err != nil {
		return nil, err
	}
	var out []Data
	for _, r := range refs {
		if !containsAll(r.Header, headerCells) {
			continue
		}
		rows, rowErr := rowsAt(doc, r.Line)
		if rowErr != nil {
			return nil, rowErr
		}
		out = append(out, Data{Ref: r, Header: r.Header, Rows: rows})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no table in %s with header cells %v", doc, headerCells)
	}
	return out, nil
}

// TableUnder is Table scoped to one part of the document: it returns the first
// matching table whose heading or introducing prose line contains where.
//
// Several tables often share a header — the manual lists `Flag | Meaning` for
// `curlew exec`, `curlew ui` and performance testing — and Table would always
// hand back the first of them. Naming the section is how a test says which one
// it means without hard-coding a line number that the next edit invalidates.
func TableUnder(doc, where string, headerCells ...string) (header []string, rows [][]string, err error) {
	refs, err := Inventory(doc)
	if err != nil {
		return nil, nil, err
	}
	for _, r := range refs {
		if !r.Under(where) || !containsAll(r.Header, headerCells) {
			continue
		}
		rows, err = rowsAt(doc, r.Line)
		if err != nil {
			return nil, nil, err
		}
		return r.Header, rows, nil
	}
	return nil, nil, fmt.Errorf("no table in %s under %q with header cells %v", doc, where, headerCells)
}

// rowsAt reads the body rows of the table whose header is at the given 1-based
// line.
func rowsAt(doc string, headerLine int) ([][]string, error) {
	data, err := os.ReadFile(filepath.Join(Dir, doc))
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", doc, err)
	}
	lines := strings.Split(string(data), "\n")
	var rows [][]string
	// headerLine is 1-based; the line after the header is the |---| separator.
	for _, r := range lines[headerLine+1:] {
		if !strings.HasPrefix(strings.TrimSpace(r), "|") {
			break
		}
		rows = append(rows, SplitRow(r))
	}
	return rows, nil
}

// SplitRow splits one markdown table row into trimmed, backtick-stripped cells.
func SplitRow(line string) []string {
	trimmed := strings.Trim(strings.TrimSpace(line), "|")
	parts := strings.Split(trimmed, "|")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, strings.Trim(strings.TrimSpace(p), "`"))
	}
	return out
}

// Column returns the index of the named header cell, or -1. Looking columns up
// by name rather than by position means reordering a table cannot silently
// change what a test asserts.
func Column(header []string, name string) int {
	for i, h := range header {
		if h == name {
			return i
		}
	}
	return -1
}

func containsAll(cells, want []string) bool {
	for _, w := range want {
		found := false
		for _, c := range cells {
			if c == w {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
