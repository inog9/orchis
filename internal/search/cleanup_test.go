package search

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFindingCleanupRequiresCompleteTenantScopedDeletion(t *testing.T) {
	tenant, id := uuid.New(), uuid.New()
	for _, tc := range []struct {
		name string
		code int
		body string
		ok   bool
	}{
		{"complete", 200, `{"total":1,"deleted":1,"timed_out":false,"version_conflicts":0,"failures":[]}`, true},
		{"absent", 200, `{"total":0,"deleted":0,"timed_out":false,"failures":[]}`, true},
		{"missing-index", 404, `{"error":{"type":"index_not_found_exception"}}`, true},
		{"wrong-endpoint", 404, `{"error":{"type":"not_found"}}`, false},
		{"empty-proxy-response", 200, `{}`, false},
		{"partial", 200, `{"total":2,"deleted":1}`, false},
		{"conflict", 200, `{"total":1,"deleted":0,"version_conflicts":1}`, false},
		{"timeout", 200, `{"total":1,"deleted":1,"timed_out":true}`, false},
		{"failed-shard", 200, `{"total":1,"deleted":1,"failures":[{"reason":"local fixture"}]}`, false},
		{"unavailable", 503, `{}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || !strings.HasPrefix(r.URL.Path, "/fixture/_delete_by_query") {
					t.Error("Unexpected deletion endpoint")
				}
				var body struct {
					Query struct {
						Bool struct {
							Filter []map[string]json.RawMessage `json:"filter"`
						} `json:"bool"`
					} `json:"query"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Query.Bool.Filter) != 2 {
					t.Error("Missing scope filters")
				}
				var term map[string]string
				json.Unmarshal(body.Query.Bool.Filter[0]["term"], &term)
				if term["tenant_id"] != tenant.String() {
					t.Error("Tenant filter omitted")
				}
				var ids struct {
					Values []uuid.UUID `json:"values"`
				}
				json.Unmarshal(body.Query.Bool.Filter[1]["ids"], &ids)
				if len(ids.Values) != 1 || ids.Values[0] != id {
					t.Error("Owned ID filter omitted")
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.code)
				w.Write([]byte(tc.body))
			}))
			defer server.Close()
			client := New(server.URL)
			client.Index = "fixture"
			err := client.RemoveFindings(context.Background(), tenant, []uuid.UUID{id})
			if (err == nil) != tc.ok {
				t.Fatalf("Deletion result accepted=%v err=%v", tc.ok, err)
			}
		})
	}
}
