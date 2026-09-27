// Package migrations embeds the immutable, ordered database migrations.
package migrations

import "embed"

// FS includes SQL only, so binaries behave independently of the working directory.
//
//go:embed *.sql
var FS embed.FS

// LatestVersion is the schema required by API and worker.
const LatestVersion = 7
