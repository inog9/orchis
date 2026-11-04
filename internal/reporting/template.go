package reporting

import (
	"embed"
	"encoding/base64"
	"strings"
)

//go:embed report.html
var reportSource string

//go:embed fonts/*.woff2
var reportFonts embed.FS

// Font dipatok di repository dan disisipkan sebagai data; renderer tetap tanpa egress.
// Hanya base64 dari aset statis yang disubstitusi sebelum html/template mem-parsing.
func reportTemplateSource() string {
	source := reportSource
	for _, font := range []struct{ token, file string }{
		{"__GE_FONT__", "fonts/geologica-latin.woff2"},
		{"__IBM_FONT__", "fonts/ibm-plex-sans-latin.woff2"},
	} {
		raw, err := reportFonts.ReadFile(font.file)
		if err != nil {
			panic(err)
		}
		source = strings.ReplaceAll(source, font.token, base64.StdEncoding.EncodeToString(raw))
	}
	return source
}
