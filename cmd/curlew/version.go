package main

import (
	"regexp"
	"runtime/debug"
	"strings"
)

// defaultVersion is the compile-time placeholder. It is the single source of
// truth for "this build was not stamped with a version": `version` is
// initialised from it, so `version != defaultVersion` is exactly "the linker
// injected something", and editing the default cannot desynchronise the two.
const defaultVersion = "0.1.0-dev"

// version is the build's reported version before build-info fallback. It is
// deliberately a var, not a const: release builds overwrite it at link time
// (.goreleaser.yaml) and -ldflags -X is silently ignored for constants — the
// build would succeed and every released binary would claim to be the
// development version. Initialising it from a named constant keeps -X working
// (the linker accepts "a constant string expression") while leaving one
// literal in the tree.
//
// Read resolvedVersion, not this. TestVersion_only_version_go_reads_the_raw_symbol
// enforces that.
var version = defaultVersion

// resolvedVersion is the string every surface reports. Computed once, eagerly:
// a package-level initializer runs after the linker has already patched
// `version`, so it observes an injected value.
var resolvedVersion = resolveVersion(version, buildInfoVersion)

// releaseVersionRE matches a version we are willing to report. Build metadata
// is excluded by construction — there is no '+' in the pattern — so "+dirty"
// and "+incompatible" are rejected without needing a case each.
var releaseVersionRE = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$`)

// pseudoVersionRE matches Go's pseudo-version tail: a 14-digit timestamp
// followed by a 12-hex-digit commit hash, immediately preceded by either "-"
// or ".". Go defines three canonical forms (https://go.dev/ref/mod#pseudo-versions):
//
//	vX.0.0-yyyymmddhhmmss-abcdefabcdef          (no known base version)
//	vX.Y.(Z+1)-0.yyyymmddhhmmss-abcdefabcdef    (base is a release vX.Y.Z)
//	vX.Y.Z-pre.0.yyyymmddhhmmss-abcdefabcdef    (base is a pre-release vX.Y.Z-pre)
//
// The timestamp run is preceded by "-" in the first form and by "0." (i.e. a
// literal ".") in the other two — matching either character catches all
// three without special-casing which one produced it. Go 1.24+ stamps one of
// these into every VCS build that is not exactly at a tag, and its numeric
// prefix names a patch release that was never cut.
var pseudoVersionRE = regexp.MustCompile(`[.-][0-9]{14}-[0-9a-f]{12}$`)

// buildInfoVersion is the only impure part: the main module's version as
// recorded in the running binary. Populated by `go install <module>@<version>`
// and by VCS stamping; "(devel)" when neither applies.
func buildInfoVersion() (string, bool) {
	info, ok := debug.ReadBuildInfo()
	if !ok || info == nil {
		return "", false
	}
	return info.Main.Version, true
}

// resolveVersion picks the version to report. It never panics and never
// returns "": it runs during package initialisation, where a panic would be
// unrecoverable and would break every command.
func resolveVersion(injected string, readBuildInfo func() (string, bool)) string {
	if injected != defaultVersion && injected != "" {
		return injected // -X is authoritative, reported verbatim
	}
	v, ok := readBuildInfo()
	if !ok || !usableBuildVersion(v) {
		return defaultVersion
	}
	return trimVersionPrefix(v)
}

// usableBuildVersion reports whether v is a real release version rather than
// "(devel)", empty, a pseudo-version, or a version carrying build metadata.
func usableBuildVersion(v string) bool {
	return releaseVersionRE.MatchString(v) && !pseudoVersionRE.MatchString(v)
}

// trimVersionPrefix strips exactly one leading "v", matching goreleaser's
// {{ .Version }}. Applied to the build-info value only — the injected value is
// whatever the release pipeline asked for.
func trimVersionPrefix(v string) string {
	return strings.TrimPrefix(v, "v")
}
