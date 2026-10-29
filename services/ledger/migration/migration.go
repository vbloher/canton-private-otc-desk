package migration

import "embed"

//go:embed postgres/*.sql
var FS embed.FS
