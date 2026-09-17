package main

import (
	"debug/buildinfo"
	"encoding/json"
	"fmt"
	"go/ast"
	goparser "go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The version must be settable at link time. A release binary has to report
// the tag it was cut from, and a `const` cannot carry that: -ldflags -X is
// silently ignored for constants, so a release pipeline would produce
// binaries that all claim to be the development version without any build
// step failing to warn about it.

const testVersion = "9.9.9-ldflags-test"

// buildBinaryWithFlags is buildBinary (main_test.go) generalised to accept
// extra `go build` flags ahead of "-o". buildBinaryWithVersion below is
// expressed through it, and TestVersion_default_when_not_injected uses it
// directly to pass -buildvcs=false.
func buildBinaryWithFlags(t *testing.T, extraFlags ...string) string {
	t.Helper()
	binary := testExecutablePath(t.TempDir(), "curlew")
	args := append([]string{"build"}, extraFlags...)
	args = append(args, "-o", binary, ".")
	cmd := exec.Command("go", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build (flags=%v) failed: %v\n%s", extraFlags, err, out)
	}
	return binary
}

// buildBinaryWithVersion builds the real binary with the version injected,
// exactly as a release build would.
func buildBinaryWithVersion(t *testing.T, v string) string {
	t.Helper()
	return buildBinaryWithFlags(t, "-ldflags", "-X main.version="+v)
}

func TestVersion_is_injectable_at_link_time(t *testing.T) {
	binary := buildBinaryWithVersion(t, testVersion)

	stdout, _, code := runBinary(t, binary, "--version")
	if code != 0 {
		t.Fatalf("--version exited %d", code)
	}
	got := strings.TrimSpace(stdout)
	want := "curlew " + testVersion
	if got != want {
		t.Errorf("--version = %q, want %q — the linker flag did not take effect, so release builds would misreport their version",
			got, want)
	}
}

// runBinaryIn runs the binary with a working directory, which the shared
// runBinary helper does not support. `info` resolves a project relative to
// the process working directory, so it must run inside one.
func runBinaryIn(t *testing.T, dir, binary string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// The injected version must reach the surfaces that report it, not only
// `--version`. Both of these are read by humans deciding which build they are
// looking at, so a release binary reporting the development version here
// would be actively misleading.
func TestVersion_injection_reaches_help(t *testing.T) {
	binary := buildBinaryWithVersion(t, testVersion)

	stdout, _, code := runBinary(t, binary, "--help")
	if code != 0 {
		t.Fatalf("--help exited %d", code)
	}
	if !strings.Contains(stdout, "Version: "+testVersion) {
		t.Errorf("--help does not report the injected version %q\n---\n%s", testVersion, stdout)
	}
}

// `info --format json` carries the version as machine-readable provenance —
// the plain text form deliberately does not. This is the surface a script or
// results consumer reads to record which build produced an artifact, so an
// uninjected version here is wrong long after the run.
func TestVersion_injection_reaches_info_json(t *testing.T) {
	binary := buildBinaryWithVersion(t, testVersion)
	dir := t.TempDir()

	if _, stderr, code := runBinaryIn(t, dir, binary, "init"); code != 0 {
		t.Fatalf("init exited %d: %s", code, stderr)
	}

	stdout, stderr, code := runBinaryIn(t, dir, binary, "info", "--format", "json")
	if code != 0 {
		t.Fatalf("info --format json exited %d: %s", code, stderr)
	}

	var payload struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("info --format json is not valid JSON: %v\n---\n%s", err, stdout)
	}
	if payload.Version != testVersion {
		t.Errorf("info JSON version = %q, want %q", payload.Version, testVersion)
	}
}

// A build with VCS stamping suppressed reports "(devel)" from build info,
// which is not a version — the binary must fall back to the compile-time
// default and report it exactly. Exact equality, not a prefix check: the old
// assertion here (`strings.HasPrefix(got, "curlew ")`) was satisfied equally
// by the placeholder, by a real version, and by "curlew (devel)" — it tested
// only that something was printed, not what.
//
// -buildvcs=false rather than a plain build: a plain `go build` in this
// repository is stamped with whatever the ambient checkout yields — normally
// a pseudo-version (M25-004 plan measurement 1), but the tag itself during a
// release (measurement 5), when asserting the default would be wrong either
// way. -buildvcs=false is the one build mode with a deterministic build-info
// result ("(devel)") on any checkout, tagged or not.
func TestVersion_default_when_not_injected(t *testing.T) {
	binary := buildBinaryWithFlags(t, "-buildvcs=false")

	stdout, _, code := runBinary(t, binary, "--version")
	if code != 0 {
		t.Fatalf("--version exited %d", code)
	}
	got := strings.TrimSpace(stdout)
	want := "curlew " + defaultVersion
	if got != want {
		t.Errorf("--version = %q, want %q", got, want)
	}
}

// Whatever the ambient checkout stamped — tagged, untagged, dirty, or
// shallow — the binary's own report must equal what resolveVersion makes of
// that same stamp. This is the wiring proof: that --version actually reads
// this process's own debug.ReadBuildInfo() and routes it through
// resolveVersion, rather than e.g. a hardcoded value that happens to match
// in the common case. On its own this is a weaker proof than
// TestVersion_tagged_build_reports_the_tag (on an untagged checkout both
// sides reduce to the same default, so a hardcoded default would also pass
// here) — the two are complementary, not redundant.
func TestVersion_reported_matches_this_binarys_build_info(t *testing.T) {
	binary := buildBinary(t)

	info, err := buildinfo.ReadFile(binary)
	if err != nil {
		t.Fatalf("debug/buildinfo.ReadFile(%s): %v", binary, err)
	}
	want := "curlew " + resolveVersion(defaultVersion, func() (string, bool) { return info.Main.Version, true })

	stdout, _, code := runBinary(t, binary, "--version")
	if code != 0 {
		t.Fatalf("--version exited %d", code)
	}
	got := strings.TrimSpace(stdout)
	if got != want {
		t.Errorf("--version = %q, want %q (derived from this binary's own build info %q)", got, want, info.Main.Version)
	}
}

// --version, --help and `info --format json` are the three surfaces the task
// names explicitly; they must report one identical string for the same
// binary and the same run. Deliberately does not assume what that string is
// (tagged checkout, untagged, dirty — all report something different) —
// only that the three surfaces cannot disagree with each other.
func TestVersion_all_surfaces_agree(t *testing.T) {
	binary := buildBinary(t)
	dir := t.TempDir()

	if _, stderr, code := runBinaryIn(t, dir, binary, "init"); code != 0 {
		t.Fatalf("init exited %d: %s", code, stderr)
	}

	versionOut, _, code := runBinary(t, binary, "--version")
	if code != 0 {
		t.Fatalf("--version exited %d", code)
	}
	fromVersion := strings.TrimPrefix(strings.TrimSpace(versionOut), "curlew ")
	if fromVersion == "" {
		t.Fatalf("--version reported an empty version: %q", versionOut)
	}

	helpOut, _, code := runBinary(t, binary, "--help")
	if code != 0 {
		t.Fatalf("--help exited %d", code)
	}
	var fromHelp string
	var sawHelpLine bool
	for _, line := range strings.Split(helpOut, "\n") {
		if v, ok := strings.CutPrefix(line, "Version: "); ok {
			fromHelp, sawHelpLine = v, true
			break
		}
	}
	if !sawHelpLine {
		t.Fatalf("--help output has no \"Version: \" line:\n%s", helpOut)
	}

	infoOut, stderr, code := runBinaryIn(t, dir, binary, "info", "--format", "json")
	if code != 0 {
		t.Fatalf("info --format json exited %d: %s", code, stderr)
	}
	var payload struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal([]byte(infoOut), &payload); err != nil {
		t.Fatalf("info --format json is not valid JSON: %v\n---\n%s", err, infoOut)
	}

	if fromVersion != fromHelp || fromVersion != payload.Version {
		t.Errorf("surfaces disagree: --version=%q --help=%q info-json=%q", fromVersion, fromHelp, payload.Version)
	}
}

// TestVersion_falls_back_to_build_info is resolveVersion's contract: every
// row is either a real measurement (M25-004 plan D2) or a synthetic
// precedence/edge case. No I/O — buildVer/buildOK stand in for
// debug.ReadBuildInfo.
func TestVersion_falls_back_to_build_info(t *testing.T) {
	const injectedByRelease = "0.1.0" // goreleaser's {{ .Version }} form
	tests := []struct {
		name     string
		injected string
		buildVer string
		buildOK  bool
		want     string
	}{
		{"installed at a tag", defaultVersion, "v0.1.0", true, "0.1.0"},
		{"installed at a pre-release tag", defaultVersion, "v0.2.0-rc.1", true, "0.2.0-rc.1"},
		{"built at a local tag", defaultVersion, "v0.99.0", true, "0.99.0"},
		{"ldflag wins over build info", "9.9.9-ldflags-test", "v0.1.0", true, "9.9.9-ldflags-test"},
		{"ldflag wins even when build info is junk", "9.9.9-ldflags-test", "(devel)", true, "9.9.9-ldflags-test"},
		{"goreleaser form is passed through verbatim", injectedByRelease, "v0.1.0", true, injectedByRelease},
		{"devel is not a version", defaultVersion, "(devel)", true, defaultVersion},
		{"empty is not a version", defaultVersion, "", true, defaultVersion},
		{"absent build info", defaultVersion, "", false, defaultVersion},
		{"pseudo-version from a commit after a tag", defaultVersion, "v0.1.1-0.20260817171911-ee182919250e", true, defaultVersion},
		{"pseudo-version from an untagged repo", defaultVersion, "v0.0.0-20260817171911-ee182919250e", true, defaultVersion},
		// Go's third canonical pseudo-version form (base is a pre-release tag,
		// https://go.dev/ref/mod#pseudo-versions): not in the plan's measured
		// table, but the same "-0." infix bug that failed the "commit after a
		// tag" case above would also have let this one through.
		{"pseudo-version from a pre-release base", defaultVersion, "v0.2.0-rc.0.20260817171911-ee182919250e", true, defaultVersion},
		{"dirty pseudo-version", defaultVersion, "v0.1.1-0.20260817171911-ee182919250e+dirty", true, defaultVersion},
		{"dirty at an exact tag", defaultVersion, "v1.2.3+dirty", true, defaultVersion},
		{"incompatible major", defaultVersion, "v2.0.0+incompatible", true, defaultVersion},
		{"module path is not a version", defaultVersion, "github.com/weiqigod/curlew", true, defaultVersion},
		{"empty injection is treated as untouched", "", "v0.1.0", true, "0.1.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveVersion(tt.injected, func() (string, bool) { return tt.buildVer, tt.buildOK })
			if got != tt.want {
				t.Errorf("resolveVersion(%q, ->(%q,%v)) = %q, want %q", tt.injected, tt.buildVer, tt.buildOK, got, tt.want)
			}
		})
	}
}

// The var must still be initialised from the const, or `version != defaultVersion`
// silently becomes "always injected" and the fallback never fires.
func TestVersion_default_is_the_constant(t *testing.T) {
	if version != defaultVersion {
		t.Errorf("version = %q, want it initialised from defaultVersion %q", version, defaultVersion)
	}
}

// buildInfoVersion must read Main.Version, not Main.Path or the sum. A test
// binary is not VCS-stamped, so this is "(devel)" — measured on go1.25.5
// (both in an isolated scratch module and inside this module, git tag
// present). If a future toolchain stamps test binaries this fails loudly,
// which is correct: the design depends on the fact.
func TestVersion_build_info_reader_reads_this_binary(t *testing.T) {
	v, ok := buildInfoVersion()
	if !ok {
		t.Fatalf("buildInfoVersion() ok = false, want true — debug.ReadBuildInfo should always succeed for a binary built by `go test`")
	}
	if v != "(devel)" {
		t.Errorf("buildInfoVersion() = %q, want %q — a test binary is not VCS-stamped on go1.25.5; if a newer toolchain changes this, update the test to match reality rather than deleting it", v, "(devel)")
	}
}

// versionGoreleaserBuildsConfig is the subset of .goreleaser.yaml this test
// needs: builds[0].ldflags. Deliberately separate from readmeArchiveConfig
// (readme_install_test.go, archives[] only) and releaseConfig
// (release_artifacts_test.go, behind a different build tag) — each file
// parses only the section it cares about rather than sharing one config
// struct that would need every field every caller happens to need.
type versionGoreleaserBuildsConfig struct {
	Builds []struct {
		Ldflags []string `yaml:"ldflags"`
	} `yaml:"builds"`
}

// goreleaser's {{ .Version }} strips the leading v: the published v0.1.0
// archive was linked with -X main.version=0.1.0 while its own build info reads
// v0.1.0 (M25-004 plan measurement 8). {{ .Tag }} would keep the v, and that
// one-word swap is exactly what would make the two install paths disagree
// about the same release. This test pins the goreleaser half of that
// byte-identity claim offline (no network, no release); the fallback half is
// resolveVersion itself, exercised directly below.
func TestVersion_build_info_form_matches_goreleaser(t *testing.T) {
	repoRoot := readmeRepoRoot(t)
	raw, err := os.ReadFile(filepath.Join(repoRoot, ".goreleaser.yaml"))
	if err != nil {
		t.Fatalf("read .goreleaser.yaml: %v", err)
	}
	var cfg versionGoreleaserBuildsConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("parse .goreleaser.yaml: %v", err)
	}
	if len(cfg.Builds) == 0 {
		t.Fatalf(".goreleaser.yaml: no builds[] entries")
	}

	var ldflagsJoined string
	for _, f := range cfg.Builds[0].Ldflags {
		ldflagsJoined += f + " "
	}
	if !strings.Contains(ldflagsJoined, "-X main.version={{ .Version }}") {
		t.Errorf(".goreleaser.yaml builds[0].ldflags %q does not contain \"-X main.version={{ .Version }}\"", ldflagsJoined)
	}
	if strings.Contains(ldflagsJoined, "{{ .Tag }}") {
		t.Errorf(".goreleaser.yaml builds[0].ldflags %q uses {{ .Tag }}, which keeps the leading \"v\" — "+
			"the fallback strips it, so the two paths would disagree about the same release", ldflagsJoined)
	}

	// The fallback must reproduce {{ .Version }}'s form byte for byte: for the
	// same release, -X yields "0.1.0" (measurement 8) and the fallback must
	// yield the same from build info's "v0.1.0".
	got := resolveVersion(defaultVersion, func() (string, bool) { return "v0.1.0", true })
	if got != "0.1.0" {
		t.Errorf("resolveVersion(defaultVersion, ->\"v0.1.0\") = %q, want %q — must match goreleaser's {{ .Version }} form byte for byte", got, "0.1.0")
	}
}

// TestVersion_only_version_go_reads_the_raw_symbol guards against an eighth
// surface reading the pre-fallback `version` var instead of `resolvedVersion`.
// A new surface that read `version` would silently report the placeholder
// while every other surface reported the real version, and nothing else in
// the suite would catch it — `resolvedVersion == version == defaultVersion`
// in every test binary (buildInfoVersion is "(devel)"), so the two symbols
// are indistinguishable by value in-process.
//
// An AST walk, not a grep: "version" occurs as a substring in the flag
// "--version" and in the string literal "TAP version 13" (main.go), neither
// of which is an identifier. Modelled on TestNoOsStdoutAssignment above,
// which walks the same directory the same way for a different symbol.
func TestVersion_only_version_go_reads_the_raw_symbol(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	fset := token.NewFileSet()
	var offenders []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") {
			continue
		}
		// version.go is where `version` is declared and read exactly once,
		// by design (see its own doc comment). Test files are exempt: they
		// legitimately compare against the raw symbol (e.g.
		// TestVersion_default_is_the_constant above).
		if strings.HasSuffix(name, "_test.go") || name == "version.go" {
			continue
		}
		file, parseErr := goparser.ParseFile(fset, name, nil, goparser.SkipObjectResolution)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", name, parseErr)
		}

		var visit func(ast.Node) bool
		visit = func(n ast.Node) bool {
			// A SelectorExpr's Sel (the "Foo" in "pkg.Foo") is a field or
			// method name, not a reference to this package's `version` —
			// only its receiver (X) can collide. Manually walking X and
			// returning false stops the default walk from also visiting Sel.
			if sel, ok := n.(*ast.SelectorExpr); ok {
				ast.Inspect(sel.X, visit)
				return false
			}
			if ident, ok := n.(*ast.Ident); ok && ident.Name == "version" {
				pos := fset.Position(ident.Pos())
				offenders = append(offenders, fmt.Sprintf("%s:%d", pos.Filename, pos.Line))
			}
			return true
		}
		ast.Inspect(file, visit)
	}
	if len(offenders) > 0 {
		t.Fatalf("found read(s) of the raw `version` symbol outside version.go — use `resolvedVersion` instead:\n  %s",
			strings.Join(offenders, "\n  "))
	}
}

// copyWorkingTree copies every git-tracked file, plus every untracked file
// `git` would not ignore, from src into dst, preserving relative paths and
// file mode. Deliberately not `git clone`: a clone reflects HEAD rather than
// the working tree, so it would miss uncommitted work during TDD, and a
// clone of a shallow repository is itself shallow, where Go ignores tags
// entirely and stamps a pseudo-version instead (M25-004 plan measurement 6d).
// Symlinks are skipped — none are tracked in this repository today, and a
// dangling one would fail the build for a reason unrelated to versioning.
func copyWorkingTree(t *testing.T, src, dst string) {
	t.Helper()

	var rels []string
	for _, args := range [][]string{
		{"ls-files", "-z"},
		{"ls-files", "-z", "--others", "--exclude-standard"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = src
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("git %s: %v", strings.Join(args, " "), err)
		}
		for _, rel := range strings.Split(strings.TrimRight(string(out), "\x00"), "\x00") {
			if rel != "" {
				rels = append(rels, rel)
			}
		}
	}
	if len(rels) == 0 {
		t.Fatalf("git ls-files (tracked + untracked) returned no paths from %s — the fixture would build from an empty tree", src)
	}

	for _, rel := range rels {
		srcPath := filepath.Join(src, rel)
		info, err := os.Lstat(srcPath)
		if err != nil {
			t.Fatalf("lstat %s: %v", srcPath, err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		data, err := os.ReadFile(srcPath)
		if err != nil {
			t.Fatalf("read %s: %v", srcPath, err)
		}
		dstPath := filepath.Join(dst, rel)
		if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", dstPath, err)
		}
		if err := os.WriteFile(dstPath, data, info.Mode().Perm()); err != nil {
			t.Fatalf("write %s: %v", dstPath, err)
		}
	}
}

// runFixtureGit runs `git <args...>` in dir and fails the test with the
// command's combined output on error.
func runFixtureGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s (in %s): %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return string(out)
}

// buildTaggedFixture copies the live working tree into a fresh git
// repository, commits it, and tags HEAD with tag. It never touches this
// repository's own refs: `git worktree` is deliberately not used, because it
// writes into the developer's .git/worktrees/ and a tag created in a
// worktree is a tag in the real repository. All git identity, signing and
// hook configuration is set locally on the fixture repo (not via the
// developer's global config), so this works in a container with no git
// identity and does not hang on a global commit-signing or hook setup.
//
// Returns the fixture directory. Callers build from it (buildFixtureBinary)
// once for the clean leg, and again after dirtying the tree for the dirty
// leg — the copy and git setup above are the expensive part (measurement 11:
// ~3.3s combined) and are deliberately done only once per test.
func buildTaggedFixture(t *testing.T, tag string) string {
	t.Helper()

	repoRoot := readmeRepoRoot(t)
	fixtureDir := t.TempDir()

	copyWorkingTree(t, repoRoot, fixtureDir)

	// Guard: the copy actually carries the fix, not a stale or partial copy.
	if _, err := os.Stat(filepath.Join(fixtureDir, "cmd", "curlew", "version.go")); err != nil {
		t.Fatalf("copied fixture tree is missing cmd/curlew/version.go: %v", err)
	}

	runFixtureGit(t, fixtureDir, "init", "-q")
	runFixtureGit(t, fixtureDir, "config", "user.email", "fixture@curlew.test")
	runFixtureGit(t, fixtureDir, "config", "user.name", "curlew-fixture")
	runFixtureGit(t, fixtureDir, "config", "commit.gpgsign", "false")
	runFixtureGit(t, fixtureDir, "config", "tag.gpgsign", "false")
	runFixtureGit(t, fixtureDir, "config", "core.hooksPath", "/dev/null")
	runFixtureGit(t, fixtureDir, "add", "-A")
	runFixtureGit(t, fixtureDir, "commit", "-q", "--no-verify", "-m", "fixture commit")
	runFixtureGit(t, fixtureDir, "tag", tag)

	// Guard: the tag actually landed on HEAD. Without this, a failure below
	// cannot tell "the fixture's tag didn't take" from "our resolver is
	// broken".
	gotTag := strings.TrimSpace(runFixtureGit(t, fixtureDir, "describe", "--tags", "--exact-match", "HEAD"))
	if gotTag != tag {
		t.Fatalf("git describe --tags --exact-match HEAD = %q, want %q", gotTag, tag)
	}
	// Guard: the freshly tagged tree is clean before any leg dirties it.
	if status := runFixtureGit(t, fixtureDir, "status", "--porcelain"); status != "" {
		t.Fatalf("fixture tree is not clean immediately after tagging:\n%s", status)
	}

	return fixtureDir
}

// buildFixtureBinary builds ./cmd/curlew from fixtureDir into a location
// outside it (a build product inside the fixture tree would itself make the
// tree dirty, per M25-004 plan measurement 6c) and returns the binary's path
// plus the version debug/buildinfo.Read finds stamped into it.
//
// GOWORK=off and a cleared GOFLAGS keep the build from picking up an ambient
// workspace file or -mod=vendor from the environment this test runs in,
// neither of which the fixture tree itself carries.
func buildFixtureBinary(t *testing.T, fixtureDir string) (binPath, stampedVersion string) {
	t.Helper()

	binDir := t.TempDir()
	binPath = testExecutablePath(binDir, "curlew")

	cmd := exec.Command("go", "build", "-o", binPath, "./cmd/curlew")
	cmd.Dir = fixtureDir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build in fixture: %v\n%s", err, out)
	}

	// Guard: the toolchain actually stamped VCS info into the binary — a
	// failure here means the fixture's git setup is broken (e.g. Go decided
	// the checkout was shallow or unclean for reasons other than the ones
	// this test controls), not that resolveVersion is broken.
	info, err := buildinfo.ReadFile(binPath)
	if err != nil {
		t.Fatalf("debug/buildinfo.ReadFile(%s): %v", binPath, err)
	}
	return binPath, info.Main.Version
}

// TestVersion_tagged_build_reports_the_tag is the end-to-end proof: a real
// tag, real Go VCS stamping, real debug.ReadBuildInfo, real --version. Two
// legs, both required — accept-everything would pass the clean leg alone,
// accept-nothing would pass the dirty leg alone, and only checking both
// proves the boundary is in the right place.
//
// What this does NOT prove, stated plainly (M25-004 plan D7): behavior 2
// names `go install <module>@<tag>`. That exact path is unreachable today —
// the only tag in THIS repository is v0.1.0, whose source predates this fix,
// and creating a new tag here is out of bounds. Measurement 7 (plan)
// established that `go install @<tag>` populates Main.Version with a clean
// "vX.Y.Z" — the identical shape this fixture produces via VCS stamping
// instead of the module proxy. The fixture is a faithful proxy for that
// path, not a `go install` invocation, and this comment says so rather than
// letting the test name imply one ran.
//
// "v0.1.0 predates this fix" above is pinned as an executable check, not
// left as only a comment, by TestVersion_v0_1_0_predates_the_fallback below.
func TestVersion_tagged_build_reports_the_tag(t *testing.T) {
	// Major 0 or 1: go.mod declares an unsuffixed module path
	// (github.com/weiqigod/curlew), so a v2+ tag is silently ignored for
	// versioning and falls back to a pseudo-version instead (M25-004 plan
	// measurement 6b) — this fixture is about proving the tagged case, so it
	// must pick a tag Go will actually honour.
	const tag = "v0.99.0"

	fixtureDir := buildTaggedFixture(t, tag)

	t.Run("clean checkout at the tag reports the tag", func(t *testing.T) {
		binPath, stamped := buildFixtureBinary(t, fixtureDir)
		if stamped != tag {
			t.Fatalf("fixture binary build info Main.Version = %q, want %q — toolchain did not stamp the tag", stamped, tag)
		}

		stdout, _, code := runBinary(t, binPath, "--version")
		if code != 0 {
			t.Fatalf("--version exited %d", code)
		}
		got := strings.TrimSpace(stdout)
		want := "curlew " + strings.TrimPrefix(tag, "v")
		if got != want {
			t.Errorf("--version = %q, want %q", got, want)
		}
	})

	t.Run("dirty checkout at the tag reports the default", func(t *testing.T) {
		marker := filepath.Join(fixtureDir, "DIRTY_MARKER.txt")
		if err := os.WriteFile(marker, []byte("dirty\n"), 0o600); err != nil {
			t.Fatalf("dirty the fixture tree: %v", err)
		}
		if status := runFixtureGit(t, fixtureDir, "status", "--porcelain"); status == "" {
			t.Fatalf("fixture tree still reports clean after adding a marker file — dirty leg would not be testing anything")
		}

		binPath, stamped := buildFixtureBinary(t, fixtureDir)
		wantStamped := tag + "+dirty"
		if stamped != wantStamped {
			t.Fatalf("fixture binary build info Main.Version = %q, want %q — toolchain did not mark the tree dirty", stamped, wantStamped)
		}

		stdout, _, code := runBinary(t, binPath, "--version")
		if code != 0 {
			t.Fatalf("--version exited %d", code)
		}
		got := strings.TrimSpace(stdout)
		want := "curlew " + defaultVersion
		if got != want {
			t.Errorf("--version = %q, want %q — a dirty tag checkout must fall back to the default, not report a version with +dirty appended", got, want)
		}
	})
}

// TestVersion_v0_1_0_predates_the_fallback pins, as an executable check
// rather than only a comment, a fact two places in this repository state in
// prose: README.md's "Download a release binary" paragraph and the doc
// comment on TestVersion_tagged_build_reports_the_tag above both say v0.1.0
// -- the only tag this repository has ever published -- predates this file,
// so a source build or `go install` at v0.1.0 has no fallback to run and
// reports defaultVersion forever, regardless of how resolveVersion handles
// any tag cut after this one. Verified directly during the M25-004 improve
// phase that added this test: `git cat-file -e
// v0.1.0:cmd/curlew/version.go` fails because the path does not exist at
// that tag, and both `go install .../curlew@latest` and
// `go install .../curlew@v0.1.0` print `curlew 0.1.0-dev` from a clean
// GOBIN.
//
// This does not re-check that README.md's wording still matches the fact --
// only that the fact itself still holds. It exists so that if v0.1.0 ever
// came to carry this file (the tag recreated at a different commit -- not a
// thing this repository's workflow does, but not something git itself
// forbids either), both places making the claim would need updating, and
// this test would say so loudly here instead of the claim staying silently
// wrong in a document nothing else exercises.
func TestVersion_v0_1_0_predates_the_fallback(t *testing.T) {
	// Guard: the tag itself must resolve in this checkout before the absence
	// check below means anything -- otherwise a checkout that cannot see the
	// tag at all (e.g. a shallow clone with tags pruned) would look
	// identical to "version.go is absent at the tag" without actually
	// checking anything.
	if out, err := exec.Command("git", "rev-parse", "--verify", "-q", "v0.1.0^{commit}").CombinedOutput(); err != nil {
		t.Fatalf("git rev-parse v0.1.0 failed (%v) -- this checkout cannot resolve tag v0.1.0 (a shallow clone would look like this), so the check below would be meaningless\n%s", err, out)
	}

	if out, err := exec.Command("git", "cat-file", "-e", "v0.1.0:cmd/curlew/version.go").CombinedOutput(); err == nil {
		t.Fatalf("cmd/curlew/version.go exists at tag v0.1.0 -- README.md's \"Download a release binary\" paragraph and the doc comment on "+
			"TestVersion_tagged_build_reports_the_tag both say v0.1.0 predates the build-info fallback; that is no longer true and both need updating\n%s", out)
	}
}
