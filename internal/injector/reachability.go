package injector

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"orchis-go/internal/contract"
)

// checkWebEndpoint membedakan tidak ada temuan dari endpoint yang tidak menjawab.
// Tidak mengikuti redirect, proxy lingkungan, atau membaca/menyimpan isi respons.
func checkWebEndpoint(ctx context.Context, target string, guard contract.Guard) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	transport := &http.Transport{
		// Pemeriksaan keamanan tetap boleh mengamati layanan dengan sertifikat tidak valid.
		TLSClientConfig:        &tls.Config{InsecureSkipVerify: true},
		MaxResponseHeaderBytes: 64 << 10,
		DisableKeepAlives:      true,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil || len(ips) == 0 {
				return nil, errors.New("target name does not resolve")
			}
			// Semua hasil DNS diperiksa sebelum satu koneksi dibuat; dial memakai IP yang telah diperiksa.
			for _, ip := range ips {
				if err := guard.CheckHost(ip.IP.String()); err != nil {
					return nil, err
				}
			}
			var last error
			for _, ip := range ips {
				conn, err := (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
				if err == nil {
					return conn, nil
				}
				last = err
			}
			return nil, last
		},
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	candidates := []string{target}
	if !strings.Contains(target, "://") {
		host := target
		if net.ParseIP(host) != nil && strings.Contains(host, ":") {
			host = "[" + host + "]"
		}
		candidates = []string{"https://" + host, "http://" + host}
	}
	for _, candidate := range candidates {
		u, err := url.Parse(candidate)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || guard.CheckHost(u.Hostname()) != nil {
			continue
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, u.String(), nil)
		if err != nil {
			continue
		}
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			return nil
		} // 3xx/4xx/5xx tetap membuktikan layanan HTTP menjawab.
	}
	return errors.New("Web endpoint unreachable. Check the approved target address and the scan worker network; no clean result can be inferred.")
}
