#!/bin/sh
set -eu
# The npm name currently resolves to a security-holder placeholder. Build the
# actual upstream CLI from an immutable revision and its checked-in lockfile.
revision=c1a56e347684a37b6189bc3f12dfa9220084e11c
archive="$(mktemp)"
trap 'rm -f "$archive"' EXIT
curl -fL --retry 3 "https://codeload.github.com/cybershaykh/npxconfuse/tar.gz/$revision" -o "$archive"
echo "aa229718bb893bcb89a8b596dce220321ee2e932e602c817801b189fa507df13  $archive" | sha256sum -c -
mkdir -p /opt/npxconfuse
tar xzf "$archive" -C /opt/npxconfuse --strip-components=1
cd /opt/npxconfuse
npm ci --omit=dev --ignore-scripts --no-audit --no-fund
chmod +x bin/cli.js
ln -s /opt/npxconfuse/bin/cli.js /usr/local/bin/npxconfuse
./bin/cli.js --help >/dev/null
