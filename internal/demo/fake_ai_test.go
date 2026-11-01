package demo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"orchis-go/internal/ai"
	"orchis-go/internal/contract"
)

func TestLocalFakeAIUsesProviderWireFormatsAndValidatesOutput(t *testing.T) {
	p, close, err := newLocalFakeAI()
	if err != nil {
		t.Fatal(err)
	}
	defer close()
	if !strings.HasPrefix(p.TestURL, "http://127.0.0.1:") {
		t.Fatal("not an owned loopback listener")
	}
	in := ai.Input{Assets: []ai.Target{{ID: "asset_1", Findings: []ai.Observation{{ID: "finding_1", Type: "open_port", Severity: "info", CVEs: []string{}}}}}, Catalog: []contract.Def{*contract.Find("nmap.tcp_connect")}}
	for _, item := range []struct {
		kind, provider string
		actions        int
	}{{"planning", "deepseek", 1}, {"remediation", "openai", 0}} {
		out, usage, err := localFakeOutput(context.Background(), p, ai.Claim{Job: ai.Job{Kind: item.kind, Provider: item.provider, Model: "demo-unit", Input: in, Instructions: "Synthetic fixture"}, APIKey: fakeAPIKey, MaxOutputTokens: 512})
		if err != nil {
			t.Fatal(err)
		}
		if len(out.Actions) != item.actions || len(out.Remediations) != 1 || usage != 32 || !strings.Contains(out.Summary, "DEMO / SYNTHETIC") {
			t.Fatalf("wrong fake output: %+v", out)
		}
	}
}

func TestLocalFakeAIRejectsOtherOriginsBeforeDial(t *testing.T) {
	p, close, err := newLocalFakeAI()
	if err != nil {
		t.Fatal(err)
	}
	defer close()
	var calls atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(200) }))
	defer other.Close()
	// All adversarial destinations remain local owned fixtures, even if this
	// boundary is mutated. Never test a bypass by contacting a real provider.
	for _, scheme := range []string{"http", "https"} {
		req, err := http.NewRequest("POST", strings.Replace(other.URL, "http://", scheme+"://", 1), nil)
		if err != nil {
			t.Fatal(err)
		}
		if res, err := p.Client.Transport.RoundTrip(req); err == nil {
			res.Body.Close()
			t.Fatal("unowned demo provider origin accepted")
		}
	}
	if calls.Load() != 0 {
		t.Fatal("unowned fixture was contacted")
	}
}
