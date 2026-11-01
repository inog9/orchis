package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"orchis-go/internal/auth"
	"orchis-go/internal/endpoint"
	"orchis-go/internal/osquery"
	"testing"
	"time"
)

func TestOsqueryConsentMustMatchDocumentAndRecordsActorAndTime(t *testing.T) {
	f := newAgentFixture(t)
	_, asset := f.authorizedAgent(t, "consent")
	org := f.joinOrg(t, asset)
	for _, path := range []string{"/osquery/osquery.users", "/osquery-packs/it-compliance"} {
		for _, ref := range []osquery.ConsentRef{{}, {Version: "older", SHA256: osquery.Consent().SHA256}, {Version: osquery.ConsentVersion, SHA256: "bad"}} {
			r := f.a.do("PUT", "/api/v1/organizations/"+org+path, map[string]any{"enabled": true, "consent": ref}, f.admin)
			if r.Status != 422 {
				t.Fatalf("dokumen tidak cocok harus ditolak: %d %s", r.Status, r.Body)
			}
		}
	}
	if n := rowCount(t, f, `SELECT count(*) FROM osquery_org_settings WHERE organization_id=$1`, org); n != 0 {
		t.Fatal("persetujuan tidak sah membuat setting")
	}
	r := f.a.do("PUT", "/api/v1/organizations/"+org+"/osquery-packs/it-compliance", map[string]any{"enabled": true, "consent": osquery.Consent().ConsentRef}, f.admin)
	if r.Status != 204 {
		t.Fatalf("persetujuan: %d %s", r.Status, r.Body)
	}
	// Nama JSON mengikuti kontrak API, bukan nama field Go.
	var payload struct {
		Items []struct {
			Key     string     `json:"key"`
			Version string     `json:"consent_version"`
			Hash    string     `json:"consent_sha256"`
			Actor   *uuid.UUID `json:"consented_by"`
			At      *time.Time `json:"consented_at"`
		}
	}
	f.a.do("GET", "/api/v1/organizations/"+org+"/osquery", nil, f.admin).JSON(&payload)
	count := 0
	for _, item := range payload.Items {
		if item.At != nil {
			count++
			if item.Version != osquery.ConsentVersion || item.Hash != osquery.Consent().SHA256 || item.Actor == nil || item.At.After(time.Now().Add(time.Minute)) {
				t.Fatalf("bukti persetujuan tidak lengkap: %+v", item)
			}
		}
	}
	if count != len(osquery.FindPack("it-compliance").RunnableKeys()) {
		t.Fatalf("jumlah persetujuan: %d", count)
	}
	var auditDetail string
	if err := f.a.env.Owner.QueryRow(context.Background(), `SELECT detail::text FROM audit_log WHERE action='osquery.pack' AND resource_id=$1 ORDER BY at DESC LIMIT 1`, org).Scan(&auditDetail); err != nil {
		t.Fatal(err)
	}
	var detail map[string]any
	if err := json.Unmarshal([]byte(auditDetail), &detail); err != nil {
		t.Fatal(err)
	}
	if detail["consent"].(map[string]any)["version"] != osquery.ConsentVersion {
		t.Fatal("audit tidak merekam versi dokumen")
	}
}

func TestOsqueryOldApprovalNeedsRenewal(t *testing.T) {
	f := newAgentFixture(t)
	secret, asset := f.authorizedAgent(t, "old-consent")
	org := f.joinOrg(t, asset)
	if _, err := f.a.env.Owner.Exec(context.Background(), `INSERT INTO osquery_org_settings(tenant_id,organization_id,query_key,enabled) VALUES($1,$2,'osquery.users',true)`, f.a.env.TenantA, org); err != nil {
		t.Fatal(err)
	}
	ex := f.createRun(t, secret, asset, "osquery.users")
	if r := f.agentCall("POST", "executions/claim", nil, secret); r.Status != 204 {
		t.Fatalf("persetujuan lama diklaim: %d", r.Status)
	}
	if status, _ := f.datasetStatus(t, asset, "osquery.users"); status != "skipped" {
		t.Fatal("persetujuan lama harus dilewati")
	}
	if f.exec(t, ex).Outcome != "success" {
		t.Fatal("penolakan persetujuan lama harus tercatat sebagai dilewati")
	}
}

func TestOsqueryDeleteIsVersionedAuditedScopedAndInvalidatesInflightResults(t *testing.T) {
	f := newAgentFixture(t)
	secret, asset := f.approvedAgent(t, "delete-data", "osquery.users")
	ex := f.agentTask(t, secret, asset, "osquery.users")
	if r := f.agentCall("POST", "executions/"+ex+"/result", map[string]any{"exit_code": 0, "datasets": []osquery.Dataset{{Query: "osquery.users", Status: osquery.StatusOK, Rows: []map[string]string{{"username": "toor", "uid": "0", "shell": "/bin/sh", "groups": "root"}}}}}, secret); r.Status != 204 {
		t.Fatalf("hasil: %d %s", r.Status, r.Body)
	}
	findings := rowCount(t, f, `SELECT count(*) FROM findings WHERE asset_id=$1`, asset)
	if findings == 0 {
		t.Fatal("kontrol temuan belum terbentuk")
	}
	inFlight := f.agentTask(t, secret, asset, "osquery.users")
	var data struct{ Version int }
	f.a.do("GET", "/api/v1/assets/"+asset+"/endpoint-data", nil, f.admin).JSON(&data)
	path := fmt.Sprintf("/api/v1/assets/%s/endpoint-data?version=%d", asset, data.Version)
	if r := f.a.do("DELETE", path, nil, f.analyst); r.Status != 403 {
		t.Fatalf("analyst nekat menghapus: %d", r.Status)
	}
	if r := f.a.do("DELETE", path+"0", nil, f.admin); r.Status != 409 {
		t.Fatalf("versi salah: %d %s", r.Status, r.Body)
	}
	svc := &endpoint.Service{DB: f.a.env.App}
	if err := svc.DeleteData(context.Background(), &auth.Principal{TenantID: f.a.env.TenantB, UserID: uuid.New()}, uuid.MustParse(asset), data.Version, "", ""); err == nil {
		t.Fatal("tenant lain menghapus data")
	}
	if r := f.a.do("DELETE", path, nil, f.admin); r.Status != 204 {
		t.Fatalf("hapus: %d %s", r.Status, r.Body)
	}
	if n := rowCount(t, f, `SELECT count(*) FROM endpoint_datasets WHERE asset_id=$1`, asset); n != 0 {
		t.Fatal("dataset tidak terhapus")
	}
	if n := rowCount(t, f, `SELECT count(*) FROM findings WHERE asset_id=$1`, asset); n != findings {
		t.Fatal("penghapusan dataset menyentuh temuan")
	}
	if n := rowCount(t, f, `SELECT count(*) FROM audit_log WHERE action='osquery.data.delete' AND resource_id=$1`, asset); n != 1 {
		t.Fatal("audit hapus hilang")
	}
	if r := f.agentCall("POST", "executions/"+inFlight+"/result", okUsers(), secret); r.Status != 204 {
		t.Fatalf("hasil in-flight: %d %s", r.Status, r.Body)
	}
	if got := f.exec(t, inFlight); got.Outcome != "error" {
		t.Fatal("penghapusan tidak membatalkan data in-flight")
	}
	if n := rowCount(t, f, `SELECT count(*) FROM endpoint_datasets WHERE asset_id=$1`, asset); n != 0 {
		t.Fatal("hasil in-flight mengisi ulang data yang dihapus")
	}
}
