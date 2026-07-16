package schemas

import "embed"

// V0 contains the schemas enforced by the production compiler.
//
//go:embed v0/*.schema.json
var V0 embed.FS
