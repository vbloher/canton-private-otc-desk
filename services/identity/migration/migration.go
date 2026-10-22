// Package migration embeds the SQL migrations of the identity database.
package migration

import "embed"

//go:embed postgres/*.sql
var FS embed.FS
