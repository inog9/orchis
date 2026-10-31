// Package pentestharnes contains the skills shipped with the initializer.
package pentestharnes

import (
	"embed"
	"io/fs"
)

//go:embed pentest pentest-* gog-gmail totp-cli
var bundled embed.FS

// Skills returns the embedded skills and their supporting references.
func Skills() fs.FS { return bundled }
