// Package fonts embeds the Inter typeface (SIL Open Font License, see OFL.txt)
// so exported PDFs print every character a name or job may contain.
package fonts

import _ "embed"

// Regular is Inter Regular (TTF).
//
//go:embed Inter-Regular.ttf
var Regular []byte

// Bold is Inter Bold (TTF).
//
//go:embed Inter-Bold.ttf
var Bold []byte
