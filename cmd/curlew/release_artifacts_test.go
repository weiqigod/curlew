//go:build release_artifacts

// This file drives a real `goreleaser release --snapshot --clean` and holds
// every produced archive to what .goreleaser.yaml claims and to a hardcoded
// floor Apache-2.0 requires independent of the config. It is behind
// //go:build release_artifacts because a full run costs ~28s (M25-002 plan
// measurement 1) — running that inside all three routine `go test` passes in
// scripts/ci-local.sh (plain, -race, -coverprofile) would triple the cost for
// no additional coverage, since none of what this file drives is concurrent
// code this package owns. scripts/ci-local.sh names the tests here as an
// unconditional step of its own (M25-002 D1), so the tag is not an opt-out.
package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// Sentinel errors for the well-known failure modes (CLAUDE.md error
// conventions).
var (
	// ErrGoreleaserNotFound means neither $(go env GOPATH)/bin nor PATH had
	// an executable named goreleaser.
	ErrGoreleaserNotFound = errors.New("goreleaser not found")
	// ErrNoArchives means the release produced zero *.tar.gz/*.zip files.
	ErrNoArchives = errors.New("no release archives produced")
	// ErrEmptyArchive means an archive opened cleanly but held zero entries —
	// exactly what a failed release's stub archives look like to
	// archive/tar and archive/zip (M25-002 plan Mutation A: a failed release
	// leaves 32-byte .tar.gz / 22-byte .zip stubs that read as zero entries
	// with no error).
	ErrEmptyArchive = errors.New("archive contains no entries")
)

// releaseTargets is the hardcoded floor: the six os/arch combinations
// .goreleaser.yaml must build, independent of what the config currently
// says. Mutation B (M25-002 plan) proved a config-derived expectation alone
// is a tautology: deleting a target from builds[].goos shrinks the
// config-derived list in lockstep with what gets archived, so that check
// would agree with itself and pass. The floor is what makes this a real
// assertion.
var releaseTargets = []struct{ goos, goarch string }{
	{"linux", "amd64"},
	{"linux", "arm64"},
	{"darwin", "amd64"},
	{"darwin", "arm64"},
	{"windows", "amd64"},
	{"windows", "arm64"},
}

// releaseRequiredFileCases is the hardcoded floor for archive contents, one
// entry per required file. LICENSE and NOTICE are not housekeeping:
// Apache-2.0 section 4(a) requires the License to accompany any
// distribution and 4(d) requires the NOTICE to travel with it.
var releaseRequiredFileCases = []struct{ caseName, path, why string }{
	{"LICENSE — Apache-2.0 section 4(a)", "LICENSE", "Apache-2.0 section 4(a) requires the License to accompany any distribution"},
	{"NOTICE — Apache-2.0 section 4(d)", "NOTICE", "Apache-2.0 section 4(d) requires the NOTICE to travel with the distribution"},
	{"README.md", "README.md", "declared in .goreleaser.yaml archives[0].files"},
	{"CHANGELOG.md", "CHANGELOG.md", "declared in .goreleaser.yaml archives[0].files"},
	{"docs/MANUAL.md", "docs/MANUAL.md", "declared in .goreleaser.yaml archives[0].files"},
	{"docs/CLI_SPECIFICATION.md", "docs/CLI_SPECIFICATION.md", "declared in .goreleaser.yaml archives[0].files"},
}

// releaseSemverRE is the shape a release version must have. Note
// "0.1.0-dev" matches this regex too — the shape check alone never rejects
// the unreleased default; TestRelease_version_matches_the_tag additionally
// compares against the `version` symbol directly.
var releaseSemverRE = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$`)

// releaseArchiveNameRE parses curlew_<version>_<goos>_<goarch>.(tar.gz|zip).
// Version may itself contain dots and hyphens (semver pre-release
// identifiers do), which is why it is (.+) rather than a stricter class —
// goos/goarch anchor the match from the right.
var releaseArchiveNameRE = regexp.MustCompile(`^curlew_(.+)_([a-z0-9]+)_([a-z0-9]+)\.(tar\.gz|zip)$`)

// releaseChecksumLineRE is checksums.txt's line format, as produced by
// goreleaser: <64 lowercase hex><two spaces><basename> (M25-002 plan
// measurement 11).
var releaseChecksumLineRE = regexp.MustCompile(`^([0-9a-f]{64})  (.+)$`)

// releaseVersionFlag is the literal, case-sensitive ldflags token this test
// looks for. Matched exactly — never via strings.Contains or a
// case-insensitive test — because -X main.Version= (wrong case) is silently
// ignored by the linker (no such symbol exists) while buildinfo still
// faithfully records the flag text it was given. A loose match would report
// success on a binary that still carries the cmd/curlew/main.go default.
const releaseVersionFlag = "-X main.version="

// releaseArchive is one produced release archive, fully read into memory.
// Measured at 573ms for all six (M25-002 plan measurement 9), so holding
// them in memory is cheaper than the bookkeeping of streaming them. Entries
// are read fully into memory too — nothing in this file extracts to the
// working directory, where a root-level curlew entry would collide with the
// dogfood gate's own binary (scripts/ci-local.sh:169). The one exception is
// the single native-target binary, deliberately written to t.TempDir() by
// TestRelease_version_matches_the_tag.
type releaseArchive struct {
	Path       string // dist/curlew_0.0.1-snapshot_linux_amd64.tar.gz
	Name       string // basename
	Version    string // parsed from Name
	GOOS       string // parsed from Name
	GOARCH     string // parsed from Name
	Zip        bool   // .zip rather than .tar.gz
	Raw        []byte // the archive file itself, for checksum re-hashing
	ModTime    time.Time
	Entries    map[string][]byte
	EntryModes map[string]os.FileMode
}

// releaseSnapshotResult is the cached result of one `goreleaser release
// --snapshot --clean`. Cached because all three named tests in this file
// need it and it costs ~28s.
type releaseSnapshotResult struct {
	Archives  []releaseArchive
	Checksums map[string]string // basename -> lowercase hex sha256, as listed
	StartedAt time.Time         // captured before invoking goreleaser; for archive-freshness checks
	Err       error
}

var (
	releaseOnce   sync.Once
	releaseResult releaseSnapshotResult
)

// releaseSnapshot drives goreleaser exactly once per test binary and returns
// the cached, parsed result. It never skips: a missing goreleaser is a
// failure, because a gate that reports clear while quietly omitting a step
// is worse than no gate at all (internal/backlog/backlog.go's package
// comment makes the same point about this codebase's own reconciliation
// check).
//
// The once-function (runReleaseSnapshot) must never call t.Fatal or anything
// that calls runtime.Goexit: sync.Once.Do marks itself complete via a
// deferred call even when the function body Goexits partway through, so a
// t.Fatal inside it would leave releaseResult at its zero value while the
// Once is permanently marked done — the *next* caller would then read a
// zero-valued result (nil Err, zero Archives) and every range-over-archives
// assertion would pass vacuously. Every failure inside runReleaseSnapshot is
// instead stored in releaseSnapshotResult.Err and surfaced by the caller's
// own t.Fatalf, outside Once.Do. As a second guard, a nil Err with zero
// Archives is also rejected below, so a zero value can never be mistaken for
// a successful release even if a future change to runReleaseSnapshot finds
// another way to return early.
func releaseSnapshot(t *testing.T) *releaseSnapshotResult {
	t.Helper()
	releaseOnce.Do(func() {
		releaseResult = runReleaseSnapshot()
	})
	if releaseResult.Err != nil {
		t.Fatalf("release snapshot: %v", releaseResult.Err)
	}
	if len(releaseResult.Archives) == 0 {
		t.Fatalf("release snapshot: no error but zero archives — refusing to treat this as success")
	}
	return &releaseResult
}

// runReleaseSnapshot performs the real work. It has no *testing.T on
// purpose — see releaseSnapshot's doc comment on why the once-function must
// never Goexit.
func runReleaseSnapshot() releaseSnapshotResult {
	start := time.Now()

	grPath, err := findGoreleaser()
	if err != nil {
		return releaseSnapshotResult{Err: err}
	}

	repoRoot, err := releaseFindRepoRoot()
	if err != nil {
		return releaseSnapshotResult{Err: err}
	}

	// Bounded well above the measured 28s wall time (M25-002 plan
	// measurement 1) so a genuine hang — e.g. a network stall in the
	// `go mod download` before-hook — fails this test rather than hanging
	// the whole gate.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, grPath, "release", "--snapshot", "--clean")
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return releaseSnapshotResult{Err: fmt.Errorf("%s release --snapshot --clean: %w\n%s", grPath, err, out)}
	}

	distDir := filepath.Join(repoRoot, "dist")
	archivePaths, err := releaseFindArchivePaths(distDir)
	if err != nil {
		return releaseSnapshotResult{Err: err}
	}
	if len(archivePaths) == 0 {
		return releaseSnapshotResult{Err: fmt.Errorf("%w: no *.tar.gz or *.zip under %s", ErrNoArchives, distDir)}
	}

	archives := make([]releaseArchive, 0, len(archivePaths))
	for _, p := range archivePaths {
		a, err := readReleaseArchive(p)
		if err != nil {
			return releaseSnapshotResult{Err: fmt.Errorf("read archive %s: %w", p, err)}
		}
		archives = append(archives, a)
	}

	checksums, err := loadChecksums(filepath.Join(distDir, "checksums.txt"))
	if err != nil {
		return releaseSnapshotResult{Err: err}
	}

	return releaseSnapshotResult{
		Archives:  archives,
		Checksums: checksums,
		StartedAt: start,
	}
}

// releaseFindRepoRoot resolves the repository root from cmd/curlew/, the
// package directory this file's tests run in — the same technique
// ci_local_test.go already uses for TestCiLocalDownIdempotent. There is no
// --dist flag on goreleaser (M25-002 plan measurement 13), so the release
// must run with the repo root as its working directory for
// .goreleaser.yaml's unset `dist:` key to resolve to the repo-root dist/.
func releaseFindRepoRoot() (string, error) {
	root, err := filepath.Abs("../..")
	if err != nil {
		return "", fmt.Errorf("resolve repo root: %w", err)
	}
	return root, nil
}

// findGoreleaser mirrors goreleaser_cmd in scripts/ci-local.sh — probing
// $(go env GOPATH)/bin before PATH — so the test and the gate cannot resolve
// different binaries.
func findGoreleaser() (string, error) {
	if gopath, err := exec.Command("go", "env", "GOPATH").Output(); err == nil {
		candidate := filepath.Join(strings.TrimSpace(string(gopath)), "bin", "goreleaser")
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return candidate, nil
		}
	}
	if p, err := exec.LookPath("goreleaser"); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("%w (checked $(go env GOPATH)/bin and PATH); install with: go install github.com/goreleaser/goreleaser/v2@latest", ErrGoreleaserNotFound)
}

// releaseFindArchivePaths globs dist/ for the two archive formats this
// project produces, sorted for deterministic iteration order.
func releaseFindArchivePaths(distDir string) ([]string, error) {
	var paths []string
	for _, pattern := range []string{"*.tar.gz", "*.zip"} {
		matches, err := filepath.Glob(filepath.Join(distDir, pattern))
		if err != nil {
			return nil, fmt.Errorf("glob %s in %s: %w", pattern, distDir, err)
		}
		paths = append(paths, matches...)
	}
	sort.Strings(paths)
	return paths, nil
}

// readReleaseArchive reads one .tar.gz or .zip entirely into memory.
func readReleaseArchive(path string) (releaseArchive, error) {
	info, err := os.Stat(path)
	if err != nil {
		return releaseArchive{}, fmt.Errorf("stat: %w", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return releaseArchive{}, fmt.Errorf("read: %w", err)
	}

	base := filepath.Base(path)
	ver, goos, goarch, isZip, err := parseArchiveName(base)
	if err != nil {
		return releaseArchive{}, err
	}

	var entries map[string][]byte
	var modes map[string]os.FileMode
	if isZip {
		entries, modes, err = readZipEntries(raw)
	} else {
		entries, modes, err = readTarGzEntries(raw)
	}
	if err != nil {
		return releaseArchive{}, err
	}
	if len(entries) == 0 {
		return releaseArchive{}, fmt.Errorf("%w: %s", ErrEmptyArchive, base)
	}

	return releaseArchive{
		Path:       path,
		Name:       base,
		Version:    ver,
		GOOS:       goos,
		GOARCH:     goarch,
		Zip:        isZip,
		Raw:        raw,
		ModTime:    info.ModTime(),
		Entries:    entries,
		EntryModes: modes,
	}, nil
}

// parseArchiveName splits curlew_<version>_<goos>_<goarch>.(tar.gz|zip).
func parseArchiveName(base string) (version, goos, goarch string, isZip bool, err error) {
	m := releaseArchiveNameRE.FindStringSubmatch(base)
	if m == nil {
		return "", "", "", false, fmt.Errorf("archive name %q does not match curlew_<version>_<goos>_<goarch>.(tar.gz|zip)", base)
	}
	return m[1], m[2], m[3], m[4] == "zip", nil
}

// readTarGzEntries reads every regular file in a gzipped tar into memory,
// keyed by its archive-relative name, alongside its recorded file mode.
func readTarGzEntries(raw []byte) (_ map[string][]byte, _ map[string]os.FileMode, err error) {
	gz, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, nil, fmt.Errorf("gzip: %w", err)
	}
	defer func() {
		if cerr := gz.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("gzip close: %w", cerr)
		}
	}()

	tr := tar.NewReader(gz)
	entries := make(map[string][]byte)
	modes := make(map[string]os.FileMode)
	for {
		hdr, hdrErr := tr.Next()
		if errors.Is(hdrErr, io.EOF) {
			break
		}
		if hdrErr != nil {
			return nil, nil, fmt.Errorf("tar: %w", hdrErr)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		content, readErr := io.ReadAll(tr)
		if readErr != nil {
			return nil, nil, fmt.Errorf("tar read %s: %w", hdr.Name, readErr)
		}
		entries[hdr.Name] = content
		modes[hdr.Name] = os.FileMode(hdr.Mode) & os.ModePerm
	}
	return entries, modes, nil
}

// readZipEntries is readTarGzEntries's zip equivalent, for the windows
// archives.
func readZipEntries(raw []byte) (map[string][]byte, map[string]os.FileMode, error) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, nil, fmt.Errorf("zip: %w", err)
	}

	entries := make(map[string][]byte)
	modes := make(map[string]os.FileMode)
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		content, err := readZipEntry(f)
		if err != nil {
			return nil, nil, err
		}
		entries[f.Name] = content
		modes[f.Name] = f.Mode() & os.ModePerm
	}
	return entries, modes, nil
}

// readZipEntry reads and closes a single zip entry, reporting whichever of
// the read or the close failed first.
func readZipEntry(f *zip.File) (_ []byte, err error) {
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("zip open %s: %w", f.Name, err)
	}
	defer func() {
		if cerr := rc.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("zip close %s: %w", f.Name, cerr)
		}
	}()

	content, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("zip read %s: %w", f.Name, err)
	}
	return content, nil
}

// loadChecksums parses checksums.txt. Every non-blank line must match
// releaseChecksumLineRE; an unparseable line is a hard error, never a
// skipped continue, because a manifest with a corrupt line silently
// tolerated is exactly the kind of gap TestRelease_checksums_match_the_archives
// exists to close.
func loadChecksums(path string) (map[string]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	checksums := make(map[string]string)
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		m := releaseChecksumLineRE.FindStringSubmatch(line)
		if m == nil {
			return nil, fmt.Errorf("%s: line does not match <64 lowercase hex><two spaces><name>: %q", path, line)
		}
		checksums[m[2]] = m[1]
	}
	return checksums, nil
}

// releaseConfig is the subset of .goreleaser.yaml this test holds the
// artifacts to.
type releaseConfig struct {
	Builds []struct {
		GOOS   []string `yaml:"goos"`
		GOARCH []string `yaml:"goarch"`
		Ignore []any    `yaml:"ignore"`
	} `yaml:"builds"`
	Archives []struct {
		Files []string `yaml:"files"`
	} `yaml:"archives"`
}

// loadReleaseConfig parses .goreleaser.yaml and fails loudly on the shapes
// that would make the rest of this test's expectations wrong: no builds[]
// entry, or an empty archives[0].files.
func loadReleaseConfig(path string) (releaseConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return releaseConfig{}, fmt.Errorf("read %s: %w", path, err)
	}
	var cfg releaseConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return releaseConfig{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if len(cfg.Builds) == 0 {
		return releaseConfig{}, fmt.Errorf("%s: no builds[] entries", path)
	}
	if len(cfg.Archives) == 0 {
		return releaseConfig{}, fmt.Errorf("%s: no archives[] entries", path)
	}
	if len(cfg.Archives[0].Files) == 0 {
		return releaseConfig{}, fmt.Errorf("%s: archives[0].files is empty", path)
	}
	return cfg, nil
}

// targets is the cartesian product goos × goarch, guarded against an empty
// product and against an `ignore:` key (absent today) that would make the
// cartesian product the wrong expectation.
func (c releaseConfig) targets() ([]string, error) {
	b := c.Builds[0]
	if len(b.Ignore) > 0 {
		return nil, errors.New("builds[0].ignore is non-empty; the goos×goarch cartesian product is no longer the right expectation")
	}
	if len(b.GOOS) == 0 {
		return nil, errors.New("builds[0].goos is empty")
	}
	if len(b.GOARCH) == 0 {
		return nil, errors.New("builds[0].goarch is empty")
	}
	out := make([]string, 0, len(b.GOOS)*len(b.GOARCH))
	for _, goos := range b.GOOS {
		for _, goarch := range b.GOARCH {
			out = append(out, goos+"/"+goarch)
		}
	}
	return out, nil
}

// injectedVersion pulls <V> out of a buildinfo -ldflags value containing the
// literal, case-sensitive token releaseVersionFlag ("-X main.version=").
// Returns ok=false when the flag is absent — exactly the regression
// .goreleaser.yaml:43-46 warns about, where a `version` that regresses from
// var to const would make the linker silently drop the flag.
func injectedVersion(ldflags string) (string, bool) {
	idx := strings.Index(ldflags, releaseVersionFlag)
	if idx < 0 {
		return "", false
	}
	rest := ldflags[idx+len(releaseVersionFlag):]
	if end := strings.IndexAny(rest, " \t"); end >= 0 {
		rest = rest[:end]
	}
	if rest == "" {
		return "", false
	}
	return rest, true
}

// archiveBuildSettings reads every build setting out of a cross-compiled
// binary held in memory — GOOS, GOARCH, CGO_ENABLED, -ldflags, etc.
// debug/buildinfo works on all six targets without needing to write
// anything to disk (M25-002 plan measurement 8).
func archiveBuildSettings(bin []byte) (map[string]string, error) {
	info, err := buildinfo.Read(bytes.NewReader(bin))
	if err != nil {
		return nil, fmt.Errorf("buildinfo: %w", err)
	}
	settings := make(map[string]string, len(info.Settings))
	for _, s := range info.Settings {
		settings[s.Key] = s.Value
	}
	return settings, nil
}

// releaseELFMachineByGOARCH, releaseMachoCPUByGOARCH and
// releasePEMachineByGOARCH are the expected machine-field values measured
// for this project's two supported architectures (M25-002 plan's
// binary-format table).
var (
	releaseELFMachineByGOARCH = map[string]elf.Machine{
		"amd64": elf.EM_X86_64,
		"arm64": elf.EM_AARCH64,
	}
	releaseMachoCPUByGOARCH = map[string]macho.Cpu{
		"amd64": macho.CpuAmd64,
		"arm64": macho.CpuArm64,
	}
	releasePEMachineByGOARCH = map[string]uint16{
		"amd64": 0x8664, // IMAGE_FILE_MACHINE_AMD64
		"arm64": 0xaa64, // IMAGE_FILE_MACHINE_ARM64
	}
)

// assertArchiveMachine parses bin with the debug package selected by
// wantGOOS and checks the machine field against wantGOARCH. Choosing the
// parser by *expected* GOOS means a windows archive holding an ELF binary
// fails on the parse itself, not on a magic-byte mismatch buried in a
// generic error.
func assertArchiveMachine(t *testing.T, name string, bin []byte, wantGOOS, wantGOARCH string) {
	t.Helper()
	r := bytes.NewReader(bin)
	switch wantGOOS {
	case "linux":
		f, err := elf.NewFile(r)
		if err != nil {
			t.Fatalf("%s: not a valid ELF binary: %v", name, err)
		}
		// elf.NewFile (unlike elf.Open) attaches no closer to an in-memory
		// reader, so Close is always nil here; discarded explicitly rather
		// than omitted, so this stays correct if that ever changes.
		defer func() { _ = f.Close() }()
		wantMachine, ok := releaseELFMachineByGOARCH[wantGOARCH]
		if !ok {
			t.Fatalf("%s: no expected ELF machine registered for GOARCH %q", name, wantGOARCH)
		}
		if f.Machine != wantMachine {
			t.Errorf("%s: ELF machine = %v, want %v (GOARCH %s)", name, f.Machine, wantMachine, wantGOARCH)
		}
		if f.Class != elf.ELFCLASS64 {
			t.Errorf("%s: ELF class = %v, want ELFCLASS64", name, f.Class)
		}
		// "Genuinely static" (.goreleaser.yaml's own header comment) means no
		// dynamic linker is required. That is what the absence of .interp
		// asserts. It is deliberately not a Type == ET_EXEC check: a
		// statically linked PIE binary (CGO_ENABLED=0, -buildmode=pie) is
		// legitimately ET_DYN with no .interp section, and a Type check
		// would then fail on a harmless future build-mode change while the
		// binary remained exactly as static as before.
		if sec := f.Section(".interp"); sec != nil {
			t.Errorf("%s: has a .interp section — requires a dynamic linker despite CGO_ENABLED=0", name)
		}
	case "darwin":
		f, err := macho.NewFile(r)
		if err != nil {
			t.Fatalf("%s: not a valid Mach-O binary: %v", name, err)
		}
		defer func() { _ = f.Close() }()
		wantMachine, ok := releaseMachoCPUByGOARCH[wantGOARCH]
		if !ok {
			t.Fatalf("%s: no expected Mach-O CPU registered for GOARCH %q", name, wantGOARCH)
		}
		if f.Cpu != wantMachine {
			t.Errorf("%s: Mach-O CPU = %v, want %v (GOARCH %s)", name, f.Cpu, wantMachine, wantGOARCH)
		}
		if f.Type != macho.TypeExec {
			t.Errorf("%s: Mach-O type = %v, want TypeExec", name, f.Type)
		}
	case "windows":
		f, err := pe.NewFile(r)
		if err != nil {
			t.Fatalf("%s: not a valid PE binary: %v", name, err)
		}
		defer func() { _ = f.Close() }()
		wantMachine, ok := releasePEMachineByGOARCH[wantGOARCH]
		if !ok {
			t.Fatalf("%s: no expected PE machine registered for GOARCH %q", name, wantGOARCH)
		}
		if f.Machine != wantMachine {
			t.Errorf("%s: PE machine = 0x%x, want 0x%x (GOARCH %s)", name, f.Machine, wantMachine, wantGOARCH)
		}
	default:
		t.Fatalf("%s: no format parser registered for GOOS %q", name, wantGOOS)
	}
}

// findArchiveFor returns the archive matching goos/goarch, or nil.
func findArchiveFor(archives []releaseArchive, goos, goarch string) *releaseArchive {
	for i := range archives {
		if archives[i].GOOS == goos && archives[i].GOARCH == goarch {
			return &archives[i]
		}
	}
	return nil
}

func archiveNames(archives []releaseArchive) []string {
	names := make([]string, len(archives))
	for i, a := range archives {
		names[i] = a.Name
	}
	return names
}

// TestRelease_every_archive_is_complete drives a snapshot release and holds
// every produced archive to .goreleaser.yaml AND to the floor Apache-2.0
// requires. Dropping `- NOTICE` from the config produces six valid archives
// with exit 0 (measured, M25-002 plan Mutation B); only the floor catches
// it.
func TestRelease_every_archive_is_complete(t *testing.T) {
	rel := releaseSnapshot(t)

	repoRoot, err := releaseFindRepoRoot()
	if err != nil {
		t.Fatalf("%v", err)
	}
	cfg, err := loadReleaseConfig(filepath.Join(repoRoot, ".goreleaser.yaml"))
	if err != nil {
		t.Fatalf("load .goreleaser.yaml: %v", err)
	}
	configTargets, err := cfg.targets()
	if err != nil {
		t.Fatalf(".goreleaser.yaml targets: %v", err)
	}
	configFiles := cfg.Archives[0].Files

	// --- vacuity guards, before any per-archive assertion ---
	wantCount := len(releaseTargets)
	if len(rel.Archives) != wantCount {
		t.Fatalf("got %d archives, want exactly %d: %v", len(rel.Archives), wantCount, archiveNames(rel.Archives))
	}
	for _, a := range rel.Archives {
		if len(a.Entries) == 0 {
			// readReleaseArchive already rejects this, so this branch would
			// mean the invariant it enforces was somehow bypassed. Kept as
			// an explicit belt-and-braces check: this line is the single
			// most load-bearing guard in this file.
			t.Fatalf("%s: %v", a.Name, ErrEmptyArchive)
		}
	}

	archiveTargetSet := make(map[string]bool, len(rel.Archives))
	for _, a := range rel.Archives {
		archiveTargetSet[a.GOOS+"/"+a.GOARCH] = true
	}

	// Config-derived leg: every target .goreleaser.yaml declares must have
	// actually been archived.
	for _, want := range configTargets {
		if !archiveTargetSet[want] {
			t.Errorf("%s: declared in .goreleaser.yaml builds[].goos/goarch but missing from the archives produced", want)
		}
	}
	// Hardcoded-floor leg: independent of what the config currently says.
	// Mutation B: deleting a target from the config would shrink the
	// config-derived expectation above in lockstep with the archives, so
	// that check alone would agree with itself and pass. Only this floor
	// catches it.
	for _, want := range releaseTargets {
		key := want.goos + "/" + want.goarch
		if !archiveTargetSet[key] {
			t.Errorf("%s: required by the M25-002 floor but missing from the archives produced, regardless of what .goreleaser.yaml currently declares", key)
		}
	}

	tests := []struct {
		name    string
		goos    string
		goarch  string
		wantExt string
		wantBin string
	}{
		{"linux/amd64 is a tar.gz holding curlew", "linux", "amd64", ".tar.gz", "curlew"},
		{"linux/arm64 is a tar.gz holding curlew", "linux", "arm64", ".tar.gz", "curlew"},
		{"darwin/amd64 is a tar.gz holding curlew", "darwin", "amd64", ".tar.gz", "curlew"},
		{"darwin/arm64 is a tar.gz holding curlew", "darwin", "arm64", ".tar.gz", "curlew"},
		{"windows/amd64 is a zip holding curlew.exe", "windows", "amd64", ".zip", "curlew.exe"},
		{"windows/arm64 is a zip holding curlew.exe", "windows", "arm64", ".zip", "curlew.exe"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := findArchiveFor(rel.Archives, tc.goos, tc.goarch)
			if a == nil {
				t.Fatalf("no archive found for %s/%s", tc.goos, tc.goarch)
			}

			if !strings.HasSuffix(a.Name, tc.wantExt) {
				t.Errorf("archive name %q does not end in %q", a.Name, tc.wantExt)
			}
			if a.Zip != (tc.wantExt == ".zip") {
				t.Errorf("archive %q Zip=%v, want extension %q", a.Name, a.Zip, tc.wantExt)
			}

			bin, ok := a.Entries[tc.wantBin]
			switch {
			case !ok:
				t.Fatalf("%s: missing binary entry %q", a.Name, tc.wantBin)
			case len(bin) == 0:
				t.Fatalf("%s: binary entry %q is empty", a.Name, tc.wantBin)
			}
			if mode, ok := a.EntryModes[tc.wantBin]; ok {
				if mode.Perm() != 0o755 {
					t.Errorf("%s: binary entry %q has mode %04o, want 0755 — unrunnable after a plain extract", a.Name, tc.wantBin, mode.Perm())
				}
			}

			// Config-derived leg: everything .goreleaser.yaml's
			// archives[0].files declares must actually be inside this
			// archive.
			for _, f := range configFiles {
				if _, ok := a.Entries[f]; !ok {
					t.Errorf("%s: %s declared in .goreleaser.yaml archives[0].files but missing from the archive", a.Name, f)
				}
			}
			// Hardcoded-floor leg, one named subtest per file so a failure
			// names exactly which compliance requirement broke.
			for _, rf := range releaseRequiredFileCases {
				t.Run(rf.caseName, func(t *testing.T) {
					content, ok := a.Entries[rf.path]
					if !ok {
						t.Errorf("%s: missing %s — required regardless of current .goreleaser.yaml content (%s)", a.Name, rf.path, rf.why)
						return
					}
					if len(content) == 0 {
						t.Errorf("%s: %s is present but empty", a.Name, rf.path)
					}
				})
			}

			settings, err := archiveBuildSettings(bin)
			if err != nil {
				t.Fatalf("%s: buildinfo: %v", a.Name, err)
			}
			if got := settings["GOOS"]; got != tc.goos {
				t.Errorf("%s: buildinfo GOOS = %q, want %q", a.Name, got, tc.goos)
			}
			if got := settings["GOARCH"]; got != tc.goarch {
				t.Errorf("%s: buildinfo GOARCH = %q, want %q", a.Name, got, tc.goarch)
			}
			if got := settings["CGO_ENABLED"]; got != "0" {
				t.Errorf("%s: buildinfo CGO_ENABLED = %q, want \"0\" — the static-binary claim in .goreleaser.yaml's header comment depends on this", a.Name, got)
			}

			assertArchiveMachine(t, a.Name, bin, tc.goos, tc.goarch)

			// Archive freshness: the archive FILE's own filesystem mtime —
			// never an entry's mtime, which mod_timestamp pins to the commit
			// timestamp for reproducibility and would not reflect when this
			// run produced it — must not predate this test's own release.
			// Guards against a stale dist/ somehow surviving --clean and
			// being misread as this run's output; D2's exit-code-first check
			// remains the primary guard.
			if a.ModTime.Before(rel.StartedAt.Add(-2 * time.Second)) {
				t.Errorf("%s: archive file mtime %s predates this test's release start %s — stale dist/?", a.Name, a.ModTime, rel.StartedAt)
			}
		})
	}
}

// TestRelease_version_matches_the_tag proves -X main.version reached every
// artifact. Two legs: buildinfo covers all six targets but is blind to a
// `version` var-to-const regression (the linker would ignore the flag while
// buildinfo still records it); executing the one natively-runnable binary
// covers that, for one target. Neither leg subsumes the other.
func TestRelease_version_matches_the_tag(t *testing.T) {
	t.Run("injectedVersion matches the flag exactly, case-sensitively", func(t *testing.T) {
		tests := []struct {
			name        string
			ldflags     string
			wantVersion string
			wantOK      bool
		}{
			{"happy path", "-s -w -X main.version=0.0.1-snapshot", "0.0.1-snapshot", true},
			{"flag absent entirely", "-s -w", "", false},
			// The linker silently drops -X for a symbol that does not exist
			// (main.Version, capital V) — buildinfo still faithfully records
			// the flag text it was given. A case-insensitive or substring
			// match would wrongly report this as injected.
			{"wrong case is not a match", "-s -w -X main.Version=0.0.1-snapshot", "", false},
			{"empty value is not a match", "-s -w -X main.version=", "", false},
			{"value followed by more flags", "-X main.version=1.2.3 -s -w", "1.2.3", true},
			{"flag appears after an unrelated -X", "-s -w -X main.foo=bar -X main.version=9.9.9", "9.9.9", true},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				got, ok := injectedVersion(tc.ldflags)
				if ok != tc.wantOK || got != tc.wantVersion {
					t.Errorf("injectedVersion(%q) = (%q, %v), want (%q, %v)", tc.ldflags, got, ok, tc.wantVersion, tc.wantOK)
				}
			})
		}
	})

	rel := releaseSnapshot(t)

	// 1. All six archive filenames agree on one version.
	releaseVersion := rel.Archives[0].Version
	for _, a := range rel.Archives {
		if a.Version != releaseVersion {
			t.Errorf("%s: archive version %q does not match %s's %q", a.Name, a.Version, rel.Archives[0].Name, releaseVersion)
		}
	}
	if releaseVersion == "" {
		t.Fatalf("empty version parsed from archive filenames")
	}
	if !releaseSemverRE.MatchString(releaseVersion) {
		t.Errorf("release version %q is not semver-shaped (X.Y.Z or X.Y.Z-pre)", releaseVersion)
	}
	// The default is referenced by symbol, not by literal, so this
	// assertion cannot drift if cmd/curlew/main.go's default is ever
	// changed. `version` is that package-level var; this test binary is
	// built without -ldflags, so it holds the plain source default here.
	if releaseVersion == version {
		t.Errorf("release version %q equals the unreleased default — -X main.version never reached these archives", releaseVersion)
	}
	// Mode-specific: this test always invokes --snapshot, so the version
	// always ends in this suffix by construction of
	// snapshot.version_template. A real tagged build would not carry it;
	// that is out of scope here, since this test only ever runs --snapshot.
	if !strings.HasSuffix(releaseVersion, "-snapshot") {
		t.Errorf("release version %q does not end in \"-snapshot\" (snapshot.version_template)", releaseVersion)
	}

	// 2. checksums.txt filenames carry the same version.
	if len(rel.Checksums) == 0 {
		t.Fatalf("checksums.txt produced no entries")
	}
	for name := range rel.Checksums {
		if !strings.Contains(name, "_"+releaseVersion+"_") {
			t.Errorf("checksums.txt entry %q does not carry version %q", name, releaseVersion)
		}
	}

	// 3. Every binary's buildinfo -ldflags carries the same version.
	nativeCount := 0
	var nativeArchive *releaseArchive
	var nativeBin []byte
	for i := range rel.Archives {
		a := &rel.Archives[i]
		binName := "curlew"
		if a.Zip {
			binName = "curlew.exe"
		}
		bin, ok := a.Entries[binName]
		if !ok {
			t.Errorf("%s: no %s entry to read buildinfo from", a.Name, binName)
			continue
		}
		settings, err := archiveBuildSettings(bin)
		if err != nil {
			t.Errorf("%s: buildinfo: %v", a.Name, err)
			continue
		}
		got, ok := injectedVersion(settings["-ldflags"])
		if !ok {
			t.Errorf("%s: -ldflags %q does not contain %q — -X main.version was not passed to this build", a.Name, settings["-ldflags"], releaseVersionFlag)
			continue
		}
		if got != releaseVersion {
			t.Errorf("%s: buildinfo -X main.version=%q, want %q", a.Name, got, releaseVersion)
		}

		if a.GOOS == runtime.GOOS && a.GOARCH == runtime.GOARCH {
			nativeCount++
			nativeArchive = a
			nativeBin = bin
		}
	}

	// 4. Exactly one archive matches this host, and it is actually
	// executed — never a silent skip. Rosetta makes darwin_amd64 also
	// runnable on darwin_arm64 (measured), but that is deliberately not
	// used here: a check that passes only when a translation layer happens
	// to be installed is a conditional pass, not a guarantee about the
	// artifact. A host outside the release matrix is a t.Fatal naming the
	// host, not a skip that would leave this leg silently uncovered.
	if nativeCount != 1 {
		t.Fatalf("expected exactly 1 archive matching this host's runtime.GOOS/GOARCH (%s/%s), found %d",
			runtime.GOOS, runtime.GOARCH, nativeCount)
	}

	dir := t.TempDir()
	binName := "curlew"
	if nativeArchive.Zip {
		binName = "curlew.exe"
	}
	binPath := filepath.Join(dir, binName)
	if err := os.WriteFile(binPath, nativeBin, 0o755); err != nil {
		t.Fatalf("write native binary: %v", err)
	}
	stdout, _, code := runBinary(t, binPath, "--version")
	if code != 0 {
		t.Fatalf("%s --version exited %d", nativeArchive.Name, code)
	}
	want := "curlew " + releaseVersion
	got := strings.TrimSpace(stdout)
	if got != want {
		t.Errorf("%s --version = %q, want %q — buildinfo alone is blind to `version` regressing from var to const, since the linker still records the flag it silently ignored; executing the binary is what catches that", nativeArchive.Name, got, want)
	}
}

// TestRelease_checksums_match_the_archives re-hashes rather than trusting
// the file, in both directions so neither can be vacuous.
func TestRelease_checksums_match_the_archives(t *testing.T) {
	rel := releaseSnapshot(t)
	wantCount := len(releaseTargets)

	// Cardinality first, and independently of either set-difference check
	// below: "every archive has a line" and "every line has an archive" are
	// jointly vacuous when both sets are empty — a release that produced
	// zero archives and no checksums.txt would pass both with nothing to
	// range over.
	t.Run("both sides have the expected cardinality", func(t *testing.T) {
		if len(rel.Archives) != wantCount {
			t.Errorf("got %d archives on disk, want %d", len(rel.Archives), wantCount)
		}
		if len(rel.Checksums) != wantCount {
			t.Errorf("got %d entries in checksums.txt, want %d", len(rel.Checksums), wantCount)
		}
	})

	onDisk := make(map[string]releaseArchive, len(rel.Archives))
	for _, a := range rel.Archives {
		onDisk[a.Name] = a
	}

	t.Run("every archive appears in checksums.txt", func(t *testing.T) {
		for name := range onDisk {
			if _, ok := rel.Checksums[name]; !ok {
				t.Errorf("%s exists in dist/ but has no line in checksums.txt", name)
			}
		}
	})

	t.Run("every checksums.txt line names an archive that exists", func(t *testing.T) {
		for name := range rel.Checksums {
			if _, ok := onDisk[name]; !ok {
				t.Errorf("checksums.txt names %q, which does not exist on disk", name)
			}
		}
	})

	t.Run("each recomputed sha256 equals the listed digest", func(t *testing.T) {
		for name, a := range onDisk {
			want, ok := rel.Checksums[name]
			if !ok {
				continue // already reported above
			}
			sum := sha256.Sum256(a.Raw)
			got := hex.EncodeToString(sum[:])
			if got != want {
				t.Errorf("%s: sha256 = %s, want %s (from checksums.txt)", name, got, want)
			}
		}
	})

	t.Run("checksums.txt does not list itself", func(t *testing.T) {
		if _, ok := rel.Checksums["checksums.txt"]; ok {
			t.Errorf("checksums.txt lists itself as one of its own entries")
		}
	})
}
