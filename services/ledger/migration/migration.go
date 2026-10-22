// Package migration embeds the ledger service's schema, applied on start.
package migration

import "embed"

//go:embed postgres/*.sql
var FS embed.FS
