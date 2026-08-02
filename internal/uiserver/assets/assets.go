// Package assets embeds the built curlew ui SPA. A committed placeholder
// index.html keeps `go build ./...` green without a frontend build; real
// builds (scripts/build-ui.sh) overwrite dist/ locally and in release
// pipelines. See UI_SPECIFICATION.md §3.3.
package assets

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// Dist returns the embedded dist/ tree as a root filesystem.
func Dist() fs.FS {
	sub, _ := fs.Sub(distFS, "dist")
	return sub
}
