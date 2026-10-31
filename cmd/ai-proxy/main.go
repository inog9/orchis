// ai-proxy membatasi egress container AI pada dua endpoint TLS provider.
package main

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"
)

var allowed = map[string]bool{"api.openai.com:443": true, "api.deepseek.com:443": true}

func handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "CONNECT" || !allowed[r.Host] {
		http.Error(w, "Destination denied", 403)
		return
	}
	host, _, _ := net.SplitHostPort(r.Host)
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	ips, e := net.DefaultResolver.LookupIPAddr(ctx, host)
	if e != nil {
		http.Error(w, "DNS failed", 502)
		return
	}
	var upstream net.Conn
	for _, v := range ips {
		if !v.IP.IsGlobalUnicast() || v.IP.IsPrivate() || v.IP.IsLoopback() || v.IP.IsLinkLocalUnicast() {
			continue
		}
		upstream, e = (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(v.IP.String(), "443"))
		if e == nil {
			break
		}
	}
	if upstream == nil {
		http.Error(w, "Provider unavailable", 502)
		return
	}
	h, ok := w.(http.Hijacker)
	if !ok {
		upstream.Close()
		http.Error(w, "Tunnel unsupported", 500)
		return
	}
	down, buf, e := h.Hijack()
	if e != nil {
		upstream.Close()
		return
	}
	defer upstream.Close()
	defer down.Close()
	_ = upstream.SetDeadline(time.Now().Add(75 * time.Second))
	_ = down.SetDeadline(time.Now().Add(75 * time.Second))
	_, _ = buf.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
	if buf.Flush() != nil {
		return
	}
	done := make(chan struct{})
	go func() { _, _ = io.Copy(upstream, io.LimitReader(buf, 1<<20)); _ = upstream.Close(); close(done) }()
	_, _ = io.Copy(down, io.LimitReader(upstream, 1<<20))
	_ = down.Close()
	<-done
}
func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	s := &http.Server{Addr: ":8080", Handler: http.HandlerFunc(handler), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192}
	if err := s.ListenAndServe(); err != nil {
		slog.Error("AI proxy stopped")
		os.Exit(1)
	}
}
