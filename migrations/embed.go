package migrations

import "embed"

// Files встраивается в бинарник; SQL лежит в этой же папке.
//
//go:embed *.sql
var Files embed.FS
