// Package pentestharnes contains the orchestrator and skills shipped with the initializer.
package pentestharnes

import (
	"embed"
	"io/fs"
)

//go:embed AGENTS.md skills
var bundled embed.FS

// Skills returns the embedded orchestrator and skills/ tree with supporting references.
func Skills() fs.FS { return bundled }

//go:embed pi-runtime/*.mjs pi-runtime/*.ts pi-runtime/*.md
var runtime embed.FS

// Runtime returns the Pi extension and its controller modules.
func Runtime() fs.FS {
	sub, err := fs.Sub(runtime, "pi-runtime")
	if err != nil {
		panic(err)
	}
	return sub
}
