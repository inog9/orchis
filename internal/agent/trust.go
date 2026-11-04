package agent

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"
)

// NewHTTPClient returns a client that also trusts the PEM CA bundle in caFile (for an installation whose HTTPS certificate is
// issued by its own CA). The CA is added to the system roots; an empty caFile gives the default client.
func NewHTTPClient(caFile string, timeout time.Duration) (*http.Client, error) {
	if caFile == "" {
		return &http.Client{Timeout: timeout}, nil
	}
	pem, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("cannot read CA file: %w", err)
	}
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(pem) {
		return nil, errors.New("CA file contains no certificate")
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	return &http.Client{Timeout: timeout, Transport: tr}, nil
}

var clientCache sync.Map // "caFile|timeout" -> *http.Client, so connections are reused

type failingTransport struct{ err error }

func (f failingTransport) RoundTrip(*http.Request) (*http.Response, error) { return nil, f.err }

// clientFor never falls back to the default roots when a configured CA cannot be loaded: it fails closed.
func clientFor(caFile string, timeout time.Duration) *http.Client {
	key := caFile + "|" + timeout.String()
	if c, ok := clientCache.Load(key); ok {
		return c.(*http.Client)
	}
	c, err := NewHTTPClient(caFile, timeout)
	if err != nil {
		c = &http.Client{Timeout: timeout, Transport: failingTransport{err}}
	}
	actual, _ := clientCache.LoadOrStore(key, c)
	return actual.(*http.Client)
}

func (a *Agent) caFile() string {
	if a.Cfg == nil {
		return ""
	}
	return a.Cfg.CAFile
}
