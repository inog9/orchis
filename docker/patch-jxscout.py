#!/usr/bin/env python3
"""Patch the pinned fork's downloader to use an explicit trusted proxy.

The original custom DialTLS bypasses normal proxy selection. Replace just that
transport, keeping scope checks, rate limits, and redirect handling upstream.
Fail closed if the pinned source no longer matches.
"""
from pathlib import Path
import sys

path = Path(sys.argv[1]) / "internal/core/asset-fetcher/service.go"
text = path.read_text()
start = text.index('\t\tTransport: &http.Transport{')
end = text.index('\n\t}\n\n\tvar rateLimiter', start)
assert 'InsecureSkipVerify: true' in text[start:end]
text = text[:start] + '''\t\tTransport: &http.Transport{
            Proxy: func(request *http.Request) (*url.URL, error) {
                // ProxyFromEnvironment always bypasses loopback, even when
                // NO_PROXY is empty. An explicit capture proxy must not.
                if value := os.Getenv("JXSCOUT_PROXY_URL"); value != "" {
                    proxy, err := url.Parse(value)
                    if err != nil || proxy.Host == "" || (proxy.Scheme != "http" && proxy.Scheme != "https") {
                        return nil, fmt.Errorf("invalid JXSCOUT_PROXY_URL")
                    }
                    return proxy, nil
                }
                return http.ProxyFromEnvironment(request)
            },
            TLSHandshakeTimeout: 30 * time.Second,
            TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
        },''' + text[end:]
text = text.replace('\n\t"net"', '')
text = text.replace('\n\t"net/http"', '\n\t"net/http"\n\t"net/url"\n\t"os"')
text = text.replace('\t// taken from https://github.com/sweetbbak/go-cloudflare-bypass\n\ttlsConfig := http.DefaultTransport.(*http.Transport).TLSClientConfig\n', '')
path.write_text(text)
