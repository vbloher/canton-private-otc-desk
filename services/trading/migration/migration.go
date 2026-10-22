// Package migration embeds the trading schema; app.Run applies it on start.
package migration

import "embed"

//go:embed postgres/*.sql
var FS embed.FS
