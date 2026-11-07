// Package themes embeds the original upstream theme snapshot and its provenance.
package themes

import "embed"

// Source contains bundled themes, the source manifest and the upstream license.
//
//go:embed bundled
var Source embed.FS
