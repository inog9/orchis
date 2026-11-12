package server_test

import (
	"context"
	"github.com/google/uuid"
	"orchis-go/internal/execution"
	"testing"
)

func TestRecommendationRoutesRequireReadAccessAndTheReviewWorker(t *testing.T) {
	f := newScanFixture(t, true)
	path := "/api/v1/scans/" + uuid.NewString() + "/recommendations"
	if r := f.a.do("GET", path, nil, ""); r.Status != 401 {
		t.Fatalf("anonymous advice read: %d", r.Status)
	}
	if r := f.a.do("GET", path, nil, f.admin); r.Status != 200 {
		t.Fatalf("permitted advice read: %d %s", r.Status, r.Body)
	}
	if r := f.inj("POST", "recommendations/claim", nil, f.token); r.Status != 403 {
		t.Fatalf("nmap claimed model work: %d", r.Status)
	}
	if r := f.a.do("POST", "/api/v1/injector/recommendations/claim", nil, f.admin); r.Status != 401 {
		t.Fatalf("cookie authenticated a worker: %d", r.Status)
	}
	token, err := execution.IssueToken(context.Background(), f.a.env.App, f.a.env.TenantA, "hackbot")
	if err != nil {
		t.Fatal(err)
	}
	if r := f.inj("POST", "recommendations/claim", nil, token); r.Status != 200 {
		t.Fatalf("review worker rejected: %d %s", r.Status, r.Body)
	}
}
