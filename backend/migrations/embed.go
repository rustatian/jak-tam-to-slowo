// Package migrations holds the SQL migrations, one statement per file, applied in name order.
package migrations

import "embed"

// FS holds the migration files.
//
//go:embed *.sql
var FS embed.FS
