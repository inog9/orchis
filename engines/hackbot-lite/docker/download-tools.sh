#!/bin/sh
set -eu
arch="${1:-$(dpkg --print-architecture)}"
case "$arch" in
  arm64) interceptor_arch=arm64; gori_arch=arm64
    interceptor_sha=f9915fa051f0fb94f52e3c9ed1bc93a13592b9172fc669190ee8b81ca0829b8e
    gori_sha=3d3cd1f61680d7d805911d010d824d22cbdd58468b047b3baaed53b23bbc9445 ;;
  amd64) interceptor_arch=x64; gori_arch=x86_64
    interceptor_sha=5744c954366365d3dbf04affc4d428979b8632623bbd21b2b10b26fbf7a1386f
    gori_sha=4e6a84ee9bf3d454306816ecc0c0cee50bcedb41e9d82c045be0b8ee813148b0 ;;
  *) echo "Unsupported architecture: $arch" >&2; exit 1 ;;
esac
curl -fL --retry 3 "https://github.com/Hacker-Valley-Media/Interceptor/releases/download/v2.1.11/Interceptor-Browser-2.1.11-linux-${interceptor_arch}.tar.gz" -o /tmp/interceptor.tgz
echo "$interceptor_sha  /tmp/interceptor.tgz" | sha256sum -c -
mkdir -p /opt/interceptor
tar xzf /tmp/interceptor.tgz -C /opt/interceptor --strip-components=1
curl -fL --retry 3 "https://github.com/hahwul/gori/releases/download/v0.9.1/gori-v0.9.1-linux-${gori_arch}" -o /usr/local/bin/gori
echo "$gori_sha  /usr/local/bin/gori" | sha256sum -c -
chmod +x /usr/local/bin/gori
ln -s /opt/interceptor/dist/interceptor /usr/local/bin/interceptor
rm /tmp/interceptor.tgz
