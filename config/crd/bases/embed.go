package bases

import "embed"

//go:embed *.yaml
var CRDs embed.FS
