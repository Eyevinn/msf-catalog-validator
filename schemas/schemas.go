// Package schemas embeds the CUE catalog schemas so they ship inside the
// compiled binary. Each supported catalog version has its own subdirectory
// (e.g. draft-01/) holding a single catalog.cue file.
package schemas

import _ "embed"

// Draft01 is the CUE source for the MSF/CMSF draft-01 catalog schema.
//
//go:embed draft-01/catalog.cue
var Draft01 string
