// Package migrations embeds the SQLite migration files, applied in
// filename order.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
