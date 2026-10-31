package main

import (
	"net/http/httptest"
	"testing"
)

func TestDenyOtherEgress(t *testing.T) {
	for _, target := range []string{"evil.test:443", "169.254.169.254:80", "api.openai.com:80", "api.openai.com.evil.test:443", "127.0.0.1:443"} {
		r := httptest.NewRequest("CONNECT", "http://proxy", nil)
		r.Host = target
		w := httptest.NewRecorder()
		handler(w, r)
		if w.Code != 403 {
			t.Fatal("egress allowed", target)
		}
	}
	r := httptest.NewRequest("GET", "https://api.openai.com/", nil)
	w := httptest.NewRecorder()
	handler(w, r)
	if w.Code != 403 {
		t.Fatal("plain HTTP forwarding allowed")
	}
}
