//go:build readme_install

// This file executes the command blocks under README.md's "## Install"
// heading. It is behind //go:build readme_install because it costs ~14s and
// needs the public internet, an authenticated gh, and git credentials for a
// private repository -- none of which the three routine `go test` passes in
// scripts/ci-local.sh should acquire (M25-003 D1, mirroring the
// release_artifacts precedent from M25-002). scripts/ci-local.sh names the
// test here as an unconditional step of its own, so the tag is not an
// opt-out: see D3, "missing network, gh, or credentials is a failure, never a
// skip."
package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// readmeReleasedVersionRE matches a real release version only: `curlew
// 0.1.0-dev` and `curlew 0.1.1-snapshot` must not satisfy it. Of the
// README's three install blocks, only the download block ever runs the
// binary it produces -- it ends `./curlew --version`. "Install from
// source" ends at `go install` and "clone and build" ends at `go build`;
// neither invokes the binary, so neither can print a version at all. That
// is a property of the commands the README documents, not of the
// repository's tag state -- it held before M25-004 and holds after it, and
// a future tag does not change it. Verified by running both other blocks in
// isolation: `go install` prints nothing on success, and `git clone && go
// build` prints only git's own "Cloning into..." line.
//
// So a match here can only ever come from the download block.
// TestReadme_install_commands_execute below attributes it there directly
// rather than relying on that being the only possibility: it requires the
// matching block's body to contain `gh release download`.
// TestReadme_documents_a_binary_download's "only the download block runs
// the built binary" subtest (readme_install_test.go) holds that assumption
// itself to the README, hermetically, so it is enforced rather than only
// asserted here.
var readmeReleasedVersionRE = regexp.MustCompile(`(?m)^curlew [0-9]+\.[0-9]+\.[0-9]+$`)

// TestReadme_install_commands_execute runs every fenced bash/sh block under
// README.md's "## Install" heading as a whole script, each in its own
// t.TempDir(). This is the test that makes M25-003's claim true: the README's
// install commands are executed by something, not merely read by a human who
// trusts they still work.
func TestReadme_install_commands_execute(t *testing.T) {
	repoRoot := readmeRepoRoot(t)
	doc := readmeReadFileOrFatal(t, filepath.Join(repoRoot, "README.md"))
	blocks := readmeSectionBlocks(doc, "Install")

	// Vacuity guard: a renamed or deleted "## Install" heading must fail this
	// test, not silently execute zero commands and report success.
	if len(blocks) == 0 {
		t.Fatal("no fenced code blocks under README.md '## Install' -- the extraction is broken, not the README")
	}
	// Floor guard, independent of the count above: measured 3 install paths
	// on this tree (download, source install, clone-and-build). Fewer means
	// an install path was silently deleted from the README.
	if len(blocks) < 3 {
		t.Fatalf("found %d command block(s) under '## Install'; measured 3 on this tree -- an install path was deleted or the extraction is broken", len(blocks))
	}

	// Bounded well above the ~14s measured wall time (M25-003 plan) so a
	// genuine hang -- e.g. a network stall on `go mod download` or `gh
	// release download` -- fails this test rather than hanging the gate,
	// mirroring release_artifacts_test.go's runReleaseSnapshot.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	executed := 0
	sawReleasedVersionFromDownloadBlock := false
	for _, b := range blocks {
		if b.lang != "bash" && b.lang != "sh" {
			t.Errorf("README.md:%d: fence language %q under '## Install' -- a block here is either an executable bash command or it does not belong in the Install section", b.line, b.lang)
			continue
		}

		// Each block runs as a whole script under `bash -euo pipefail`, not
		// line by line: the clone block's `cd curlew` must affect the
		// following `go build ./cmd/curlew` in the same block. The flags
		// live on the interpreter, not in the documented text, so the
		// README stays byte-for-byte what the reader sees while a failure
		// in any line still fails the test.
		dir := t.TempDir()
		script := filepath.Join(dir, "block.sh")
		if err := os.WriteFile(script, []byte(b.body), 0o600); err != nil {
			t.Fatalf("README.md:%d: write script: %v", b.line, err)
		}

		cmd := exec.CommandContext(ctx, "bash", "-euo", "pipefail", script)
		cmd.Dir = dir
		// GOBIN is redirected into this block's own temp dir so `go install`
		// cannot overwrite the developer's own ~/go/bin/curlew. GOMODCACHE is
		// deliberately left shared with the ambient environment -- that is
		// the difference between a warm few-second install and a cold
		// multi-minute one.
		cmd.Env = append(os.Environ(), "GOBIN="+filepath.Join(dir, "bin"))
		out, err := cmd.CombinedOutput()
		executed++
		if err != nil {
			t.Errorf("README.md:%d: install block failed: %v\n--- block ---\n%s\n--- output ---\n%s", b.line, err, b.body, out)
			continue
		}
		if readmeReleasedVersionRE.Match(out) {
			// M25-004: a released version must be attributed to the block
			// that actually produced it, not merely observed from some
			// block. Only the download block can ever reach this far with a
			// match (see readmeReleasedVersionRE) -- the other two blocks
			// never run the binary they install or build, at any tag -- so
			// the branch below is a tripwire against a future README edit,
			// not a condition today's tree can trigger.
			if !strings.Contains(b.body, "gh release download") {
				t.Errorf("README.md:%d: install block produced a released version but its body does not contain "+
					"\"gh release download\" -- want the released version attributed specifically to the download block\n--- block ---\n%s\n--- output ---\n%s",
					b.line, b.body, out)
				continue
			}
			sawReleasedVersionFromDownloadBlock = true
		}
	}

	if executed != len(blocks) {
		t.Errorf("executed %d of %d blocks under '## Install' -- see the fence-language error(s) above", executed, len(blocks))
	}
	// This is how the DoD item "a download-and-run path is documented and
	// works" is enforced without hand-naming which block is the download one
	// by line number: it is derived from what the blocks actually printed,
	// attributed to the block whose body contains `gh release download`. A
	// bare "did any block print a released version" check would give the
	// same answer today, since the download block is the only one that runs
	// a binary at all -- but it would give it because of what the other two
	// blocks' bodies happen not to contain, which is exactly the kind of
	// thing that should be checked, not assumed. Attribution costs one
	// strings.Contains and does not depend on that holding.
	if !sawReleasedVersionFromDownloadBlock {
		t.Error("no install block whose body contains \"gh release download\" produced a binary reporting a released version " +
			"(want `curlew X.Y.Z`, not X.Y.Z-dev or X.Y.Z-snapshot) -- the download-and-run path is gone")
	}
}
