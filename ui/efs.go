package ui

import (
	"embed"
)

// Files holds the HTML templates and static assets. They're embedded into the
// compiled binary at build time, so the server is a single file to deploy.
//
//go:embed "html" "static"
var Files embed.FS
