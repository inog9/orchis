// Package migrations menyematkan berkas migrasi SQL ke dalam biner.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
