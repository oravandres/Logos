package migrations

import "embed"

// FS embeds all SQL migration files for use with golang-migrate.
//
//go:embed *.sql
var FS embed.FS
