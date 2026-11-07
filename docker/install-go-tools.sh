#!/bin/sh
# Run in the dedicated Go 1.27.3 builder. Versions are immutable module pins;
# normal Go checksum database verification remains enabled.
set -eu
export GOTOOLCHAIN=local
export GOBIN="${GOBIN:-/out/bin}"
mkdir -p "$GOBIN"

install_tool() {
  attempt=1
  while ! go install -trimpath -ldflags="$1" "$2"; do
    if [ "$attempt" -ge 3 ]; then
      return 1
    fi
    echo "Retrying pinned Go tool download/build: $2 (attempt $((attempt + 1))/3)" >&2
    attempt=$((attempt + 1))
    sleep 2
  done
}

CGO_ENABLED=0 install_tool '-s -w' github.com/ffuf/ffuf/v2@v2.3.0
# jsluice embeds tree-sitter's C grammar. A CGO-disabled binary cannot parse JS.
CGO_ENABLED=1 install_tool '-s -w' github.com/BishopFox/jsluice/cmd/jsluice@v0.0.0-20240110145140-0ddfab153e06
CGO_ENABLED=0 install_tool '-s -w' github.com/projectdiscovery/httpx/cmd/httpx@v1.12.0
CGO_ENABLED=0 install_tool '-s -w' github.com/lc/gau/v2/cmd/gau@v2.2.4
CGO_ENABLED=0 install_tool '-s -w' github.com/yitsushi/totp-cli@v1.9.2
CGO_ENABLED=0 install_tool '-s -w -X github.com/openclaw/gogcli/internal/cmd.version=v0.48.0' github.com/openclaw/gogcli/cmd/gog@v0.48.0
CGO_ENABLED=0 install_tool '-s -w' github.com/projectdiscovery/interactsh/cmd/interactsh-client@v1.4.1
