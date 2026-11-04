package server

import (
	_ "embed"
	"net/http"
)

// installScript adalah pemasang agent (assets/install.sh). Isinya statis dan tidak berisi rahasia; token, hash, dan kunci rilis
// diberikan konsol sebagai argumen sehingga tidak pernah bergantung pada server yang menyajikan skrip ini.
//
//go:embed assets/install.sh
var installScript []byte

func registerInstallScript(mux *http.ServeMux) {
	mux.HandleFunc("GET /install.sh", func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Type", "text/x-shellscript; charset=utf-8")
		h.Set("Cache-Control", "no-store")
		h.Set("X-Content-Type-Options", "nosniff")
		_, _ = w.Write(installScript)
	})
}
