// Package web holds the HTML templates and static files. They are embedded
// into the Go binary at build time, so the server is a single file to deploy.
package web

import "embed"

//go:embed templates static
var FS embed.FS
