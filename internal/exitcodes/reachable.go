// Package exitcodes reads the exit codes cmd/curlew can return out of the
// source, so that every document publishing an exit-code table is held to the
// binary rather than to a list someone maintains.
//
// The walk is scoped to return statements reachable from a named root. That
// scoping is the point, not an implementation detail: a walk that collected
// "integers appearing near the word exit" would sweep up any composite
// literal keyed by exit code — cmd/curlew's exitCodeSeverity map is one — and
// report a severity rank as a code the binary can return. A composite
// literal's keys and values are never return statements, so those are
// excluded structurally rather than by an exception list. That mattered
// concretely: when this package was written, exitCodeSeverity still carried a
// dead `6: 6, // feature gate` entry from the removed licensing system, and
// the structural exclusion is what let M26-002 delete it as provably
// unreachable rather than argue about it.
package exitcodes

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Code is one exit code and where the walk found it. Provenance is not
// decoration: "the binary can return 7" is unactionable; "perf.go:141, in
// perfCmdOut" is a place to go.
type Code struct {
	Value int
	File  string // path as opened, e.g. "perf.go" when dir is "."
	Line  int
	Fn    string // function whose return statement carries it
	Depth int    // 0 = the root function itself
}

var (
	// ErrRootNotFound means dir has no top-level function declaration named
	// root.
	ErrRootNotFound = errors.New("root function not found")
	// ErrNoSources means dir has no non-test .go files to parse.
	ErrNoSources = errors.New("no non-test Go sources")
)

// funcInfo is one parsed top-level function declaration: its AST body, the
// file it came from (for provenance), and its result arity. Arity matters
// for a multi-result function reached through a local identifier — resolving
// `code, _ := runCmdInner(...)` requires knowing that `code` is result 0 of
// a two-result function, so that runCmdInner's own return statements are read
// at index 0 rather than conflated with its second result.
type funcInfo struct {
	decl    *ast.FuncDecl
	file    string
	results int
}

// Reachable parses every non-test .go file in dir, roots a call graph at the
// function named root, follows calls appearing in return position only,
// resolving local identifiers through their assignments, and collects the
// integer literals appearing in return statements.
//
// dir is a parameter rather than a repo-root walk so that a second checkout
// under .claude/worktrees/ cannot inject or hide a code.
func Reachable(dir, root string) ([]Code, error) {
	fns, fset, err := parseDir(dir)
	if err != nil {
		if errors.Is(err, ErrNoSources) {
			return nil, fmt.Errorf("%s: %w", dir, err)
		}
		return nil, err
	}

	rootFn, ok := fns[root]
	if !ok {
		return nil, fmt.Errorf("%s: function %q: %w", dir, root, ErrRootNotFound)
	}

	w := &walker{fns: fns, fset: fset, visiting: map[string]bool{}}
	w.walk(rootFn, 0, 0)
	return w.codes, nil
}

// Set reduces codes to sorted, deduplicated integers.
func Set(codes []Code) []int {
	seen := map[int]bool{}
	for _, c := range codes {
		seen[c.Value] = true
	}
	out := make([]int, 0, len(seen))
	for v := range seen {
		out = append(out, v)
	}
	sort.Ints(out)
	return out
}

// Find returns the first Code with the given value, for failure messages.
func Find(codes []Code, value int) (Code, bool) {
	for _, c := range codes {
		if c.Value == value {
			return c, true
		}
	}
	return Code{}, false
}

// MaxDepth reports the deepest Depth in codes. A walk that collected nothing
// below its root resolved no calls at all, whatever its result looks like.
func MaxDepth(codes []Code) int {
	max := 0
	for _, c := range codes {
		if c.Depth > max {
			max = c.Depth
		}
	}
	return max
}

// parseDir parses every non-test .go file directly inside dir (not
// recursively — subpackages are not this package's business) and returns
// its top-level function declarations keyed by name, plus the FileSet their
// positions are recorded against.
func parseDir(dir string) (map[string]*funcInfo, *token.FileSet, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("reading %s: %w", dir, err)
	}

	fset := token.NewFileSet()
	fns := map[string]*funcInfo{}
	found := false

	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		found = true

		path := filepath.Join(dir, name)
		file, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return nil, nil, fmt.Errorf("parsing %s: %w", path, perr)
		}

		for _, decl := range file.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Recv != nil || fd.Body == nil {
				continue // methods and forward declarations carry no reachable literal returns of their own
			}
			fns[fd.Name.Name] = &funcInfo{
				decl:    fd,
				file:    path,
				results: resultCount(fd),
			}
		}
	}

	if !found {
		return nil, nil, ErrNoSources
	}
	return fns, fset, nil
}

// resultCount returns the number of values fd returns, counting a grouped
// field such as `(a, b int)` as two.
func resultCount(fd *ast.FuncDecl) int {
	if fd.Type.Results == nil {
		return 0
	}
	n := 0
	for _, field := range fd.Type.Results.List {
		if len(field.Names) == 0 {
			n++
		} else {
			n += len(field.Names)
		}
	}
	return n
}

// walker carries the state of one Reachable call: the function table it may
// recurse into, the FileSet for position lookups, a cycle guard, and the
// codes collected so far.
type walker struct {
	fns  map[string]*funcInfo
	fset *token.FileSet

	// visiting guards against infinite recursion when functions call each
	// other, directly or mutually. Keyed by function name and the result
	// slot being traced, since the same function can legitimately be
	// re-entered at a different slot from a different call site.
	visiting map[string]bool

	codes []Code
}

// walk visits every return statement belonging to fn itself — not to any
// function literal nested inside it — reading the expression at the given
// result slot from each one.
func (w *walker) walk(fn *funcInfo, slot, depth int) {
	key := fmt.Sprintf("%s#%d", fn.decl.Name.Name, slot)
	if w.visiting[key] {
		return
	}
	w.visiting[key] = true
	defer delete(w.visiting, key)

	for _, stmt := range ownStatements(fn.decl.Body) {
		if ret, ok := stmt.(*ast.ReturnStmt); ok {
			w.examineReturn(fn, ret, slot, depth)
		}
	}
}

// ownStatements flattens every statement in body's own local scope — nested
// blocks, if/for/switch/select bodies, case clauses — into one slice in
// source order. It stops at a function literal rather than descending into
// it: a closure's statements belong to the closure, not to the enclosing
// function, and a `return` inside one returns from the closure.
//
// Both callers (walk, looking for return statements, and findAssignment,
// looking for assignments) want the same scope; they differ only in which
// statement kind they keep, so the traversal is written once here and
// filtered by each caller.
func ownStatements(body *ast.BlockStmt) []ast.Stmt {
	var out []ast.Stmt
	var walk func(ast.Stmt)
	walk = func(stmt ast.Stmt) {
		if stmt == nil {
			return
		}
		out = append(out, stmt)
		switch s := stmt.(type) {
		case *ast.BlockStmt:
			for _, s2 := range s.List {
				walk(s2)
			}
		case *ast.IfStmt:
			walk(s.Init)
			walk(s.Body)
			walk(s.Else)
		case *ast.ForStmt:
			walk(s.Init)
			walk(s.Body)
		case *ast.RangeStmt:
			walk(s.Body)
		case *ast.SwitchStmt:
			walk(s.Init)
			walk(s.Body)
		case *ast.TypeSwitchStmt:
			walk(s.Init)
			walk(s.Body)
		case *ast.SelectStmt:
			walk(s.Body)
		case *ast.CaseClause:
			for _, s2 := range s.Body {
				walk(s2)
			}
		case *ast.CommClause:
			for _, s2 := range s.Body {
				walk(s2)
			}
		case *ast.LabeledStmt:
			walk(s.Stmt)
		default:
			// ReturnStmt, AssignStmt, ExprStmt, DeclStmt and everything else
			// carry no nested statements of the enclosing function's own
			// scope to descend into.
		}
	}
	walk(body)
	return out
}

// examineReturn reads the expression at slot out of a return statement.
//
// A return statement forwarding an entire multi-value call — `return f()`
// where fn has more than one result — supplies every result at once rather
// than one expression per result; that shape is detected and followed as a
// call rather than indexed like an ordinary N-expression return.
func (w *walker) examineReturn(fn *funcInfo, ret *ast.ReturnStmt, slot, depth int) {
	if slot >= len(ret.Results) {
		return
	}
	if len(ret.Results) == 1 && fn.results > 1 {
		if call, ok := ret.Results[0].(*ast.CallExpr); ok {
			w.followCall(call, slot, depth)
			return
		}
	}
	w.examineExpr(fn, ret.Results[slot], depth)
}

// examineExpr records expr if it is an integer literal, resolves it if it is
// a local identifier, or follows it if it is a call to a local function.
// Anything else — a binary expression, a selector, a type conversion — is
// not a literal exit code and is left unrecorded rather than guessed at.
func (w *walker) examineExpr(fn *funcInfo, expr ast.Expr, depth int) {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind == token.INT {
			w.record(fn, e, depth)
		}
	case *ast.Ident:
		w.resolveIdent(fn, e, depth)
	case *ast.CallExpr:
		w.followCall(e, 0, depth)
	case *ast.ParenExpr:
		w.examineExpr(fn, e.X, depth)
	}
}

// followCall recurses into the local function a call invokes, at the given
// result slot. Calls that are not a bare identifier — a selector such as
// pkg.Func or recv.Method, a call through a variable — cannot be resolved
// without cross-package or dynamic-dispatch information and are left alone.
func (w *walker) followCall(call *ast.CallExpr, slot, depth int) {
	id, ok := call.Fun.(*ast.Ident)
	if !ok {
		return
	}
	callee, ok := w.fns[id.Name]
	if !ok {
		return
	}
	w.walk(callee, slot, depth+1)
}

// resolveIdent finds the assignment that most recently — by source position —
// gave id its value within fn's body, and continues the walk from the
// right-hand side at the identifier's position in that assignment.
//
// Only simple local assignment (:= or =) is understood; a function parameter,
// a range variable or a struct field is left unresolved. Reachable under-
// collecting on code shaped differently from what it has been proven against
// is the safe failure direction: it is caught by the depth and size vacuity
// guards in the tests that call this package, not by inventing provenance
// here.
func (w *walker) resolveIdent(fn *funcInfo, id *ast.Ident, depth int) {
	assign, index, ok := findAssignment(fn.decl.Body, id.Name, id.Pos())
	if !ok {
		return
	}

	// A multi-value assignment from a single call: `code, _ := f()`. f's
	// result at `index` is what flows into id.
	if len(assign.Rhs) == 1 && len(assign.Lhs) > 1 {
		if call, ok := assign.Rhs[0].(*ast.CallExpr); ok {
			w.followCall(call, index, depth)
			return
		}
	}

	if index >= len(assign.Rhs) {
		return
	}
	w.examineExpr(fn, assign.Rhs[index], depth)
}

// findAssignment returns the assignment statement in body that assigns name
// and sits closest before pos in source order, along with name's index in
// that statement's left-hand side. Search is not scoped to the block
// containing pos — a plain top-to-bottom scan by position — which is
// sufficient for (and only claimed correct on) the straight-line branches
// cmd/curlew's exit-code plumbing actually uses: each branch assigns its own
// local immediately before returning it, so the nearest assignment before a
// given return, anywhere in the function, is that branch's own.
func findAssignment(body *ast.BlockStmt, name string, pos token.Pos) (assign *ast.AssignStmt, index int, found bool) {
	var bestPos token.Pos
	for _, stmt := range ownStatements(body) {
		as, ok := stmt.(*ast.AssignStmt)
		if !ok || as.Pos() >= pos {
			continue // only assignments strictly before the reference point count
		}
		for i, lhs := range as.Lhs {
			lid, ok := lhs.(*ast.Ident)
			if !ok || lid.Name != name {
				continue
			}
			if !found || as.Pos() > bestPos {
				assign, index, found = as, i, true
				bestPos = as.Pos()
			}
		}
	}
	return assign, index, found
}

// record appends an integer literal as a Code, attributing it to fn and the
// literal's own position.
func (w *walker) record(fn *funcInfo, lit *ast.BasicLit, depth int) {
	n, err := strconv.Atoi(lit.Value)
	if err != nil {
		return // not a plain decimal literal; none of the exit codes are written any other way
	}
	pos := w.fset.Position(lit.Pos())
	w.codes = append(w.codes, Code{
		Value: n,
		File:  fn.file,
		Line:  pos.Line,
		Fn:    fn.decl.Name.Name,
		Depth: depth,
	})
}
