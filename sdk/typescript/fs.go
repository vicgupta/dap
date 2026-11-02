// Package typescript embeds the plugin SDK so scaffolds do not need the repo checkout.
package typescript

import "embed"

// Files is the @dap/plugin-sdk package, including its compiled dist.
//
//go:embed package.json tsconfig.json README.md src dist
var Files embed.FS
