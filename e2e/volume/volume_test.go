// Opt-in volume evidence against a disposable database and a uniquely owned ES index.
package volume_test

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"io"
	"net/http"
	"orchis-go/internal/auth"
	"orchis-go/internal/bootstrap"
	"orchis-go/internal/execution"
	"orchis-go/internal/home"
	"orchis-go/internal/reporting"
	"orchis-go/internal/search"
	"orchis-go/internal/testutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func id(kind string, n int) uuid.UUID {
	return uuid.UUID(md5.Sum([]byte(fmt.Sprintf("volume-%s-%d", kind, n))))
}

type timing struct {
	Min, Median, Max float64
	Samples          int
}

func TestVolume(t *testing.T) {
	if os.Getenv("ORCHIS_VOLUME_TEST") != "1" {
		t.Skip("Opt-in 50 organization / 5000 asset / 200000 finding disposable study")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	env := testutil.New(t)
	var databaseName string
	if err := env.Owner.QueryRow(ctx, "SELECT current_database()").Scan(&databaseName); err != nil {
		t.Fatal(err)
	}
	marker := os.Getenv("ORCHIS_VOLUME_MARKER")
	writeMarker := func(index string) {
		if marker != "" {
			raw, _ := json.Marshal(map[string]string{"database": databaseName, "es_index": index})
			if err := os.WriteFile(marker, raw, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	writeMarker("")
	if _, _, err := bootstrap.EnsureAdmin(ctx, env.App, env.TenantA, "local-volume@example.invalid", "local-volume-fixture-only"); err != nil {
		t.Fatal(err)
	}
	var user uuid.UUID
	var perms []string
	if err := env.Owner.QueryRow(ctx, "SELECT id FROM users WHERE tenant_id=$1 LIMIT 1", env.TenantA).Scan(&user); err != nil {
		t.Fatal(err)
	}
	if err := env.Owner.QueryRow(ctx, "SELECT permissions FROM roles WHERE tenant_id=$1 AND key='admin'", env.TenantA).Scan(&perms); err != nil {
		t.Fatal(err)
	}
	p := &auth.Principal{TenantID: env.TenantA, UserID: user, Permissions: perms}
	seed := time.Now()
	tx, err := env.Owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	queries := []string{
		`SET LOCAL statement_timeout='120s'`,
		`INSERT INTO organizations(id,tenant_id,name,slug) SELECT md5('volume-org-'||g)::uuid,$1,'LOCAL VOLUME SYNTHETIC '||g,'volume-'||g FROM generate_series(1,50)g`,
		`INSERT INTO assets(id,tenant_id,organization_id,name,category,platform) SELECT md5('volume-asset-'||g)::uuid,$1,md5('volume-org-'||(((g-1)/100)+1))::uuid,'LOCAL VOLUME ASSET '||g,'host','linux' FROM generate_series(1,5000)g`,
		`INSERT INTO findings(id,tenant_id,asset_id,type,value,value_hash,title,severity,first_seen_at,last_seen_at) SELECT md5('volume-finding-'||g)::uuid,$1,md5('volume-asset-'||(((g-1)/40)+1))::uuid,'vulnerability','volume-issue-'||g,decode(md5(g::text)||md5('x'||g),'hex'),'LOCAL VOLUME SYNTHETIC issue '||g,(ARRAY['info','low','medium','high','critical'])[((g-1)%5)+1],now()-make_interval(days=>(g%90)),now()-make_interval(days=>(g%30)) FROM generate_series(1,200000)g`,
	}
	for i, q := range queries {
		var err error
		if i == 0 {
			_, err = tx.Exec(ctx, q)
		} else {
			_, err = tx.Exec(ctx, q, env.TenantA)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO assessments(id,tenant_id,organization_id,name,profile,idempotency_key,request_hash,status,created_by,ended_at) SELECT md5('volume-assessment-'||g)::uuid,$1,md5('volume-org-'||g)::uuid,'LOCAL VOLUME SYNTHETIC','volume-synthetic','volume-'||g,'synthetic-only','finished',$2,now() FROM generate_series(1,50)g`, env.TenantA, user); err != nil {
		t.Fatal(err)
	}
	severities := []string{"info", "low", "medium", "high", "critical"}
	for org := 1; org <= 50; org++ {
		snap := reporting.Snapshot{Organization: fmt.Sprintf("LOCAL VOLUME SYNTHETIC %d", org), AssessmentID: id("assessment", org), Profile: "volume-synthetic", CreatedAt: time.Now(), Status: "synthetic", Risk: "Synthetic fixture", BySeverity: map[string]int{}, Remediation: "Synthetic volume fixture; no security conclusion."}
		for a := (org-1)*100 + 1; a <= org*100; a++ {
			asset := reporting.Asset{ID: id("asset", a), Name: fmt.Sprintf("LOCAL VOLUME ASSET %d", a), Status: "synthetic"}
			for f := (a-1)*40 + 1; f <= a*40; f++ {
				severity := severities[(f-1)%5]
				snap.BySeverity[severity]++
				asset.Findings = append(asset.Findings, reporting.Finding{ID: id("finding", f).String(), Type: "vulnerability", Value: fmt.Sprintf("volume-issue-%d", f), Title: fmt.Sprintf("LOCAL VOLUME SYNTHETIC issue %d", f), Severity: severity, Evidence: map[string]any{"synthetic": true}})
			}
			snap.Assets = append(snap.Assets, asset)
		}
		raw, _ := json.Marshal(snap)
		if _, err = tx.Exec(ctx, `INSERT INTO reports(id,tenant_id,organization_id,assessment_id,status,snapshot) VALUES($1,$2,$3,$4,'queued',$5)`, id("report", org), env.TenantA, id("org", org), id("assessment", org), raw); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	seedSeconds := time.Since(seed).Seconds()
	if _, err = env.Owner.Exec(ctx, "ANALYZE"); err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, table := range []string{"organizations", "assets", "findings", "reports"} {
		var n int
		if err = env.Owner.QueryRow(ctx, "SELECT count(*) FROM "+table+" WHERE tenant_id=$1", env.TenantA).Scan(&n); err != nil {
			t.Fatal(err)
		}
		counts[table] = n
	}
	if counts["organizations"] != 50 || counts["assets"] != 5000 || counts["findings"] != 200000 || counts["reports"] != 50 {
		t.Fatal(counts)
	}
	metrics := map[string]timing{}
	measure := func(name string, fn func() error) {
		t.Helper()
		samples := []float64{}
		for i := 0; i < 11; i++ {
			start := time.Now()
			if err := fn(); err != nil {
				t.Fatal(name, err)
			}
			if i > 0 {
				samples = append(samples, float64(time.Since(start).Microseconds())/1000)
			}
		}
		sort.Float64s(samples)
		metrics[name] = timing{samples[0], (samples[4] + samples[5]) / 2, samples[9], 10}
	}
	hs := &home.Service{DB: env.App}
	fs := &execution.Service{DB: env.App}
	rs := &reporting.Service{DB: env.App}
	measure("home_service_ms", func() error {
		s, e := hs.Summary(ctx, p)
		if e == nil && (s.Findings.Total != 200000 || s.Assets.Total != 5000) {
			return fmt.Errorf("incorrect Home counts")
		}
		return e
	})
	measure("findings_page_ms", func() error {
		page, e := fs.ListFindings(ctx, p, execution.FindingFilter{Limit: 25}, nil)
		if e == nil && len(page.Items) != 25 {
			return fmt.Errorf("incorrect findings page")
		}
		return e
	})
	measure("findings_severity_ms", func() error {
		_, e := fs.ListFindings(ctx, p, execution.FindingFilter{Limit: 25, Severity: "critical"}, nil)
		return e
	})
	measure("search_database_ms", func() error {
		page, e := fs.ListFindings(ctx, p, execution.FindingFilter{Limit: 25, Query: "issue 19999"}, nil)
		if e == nil && (page.Search != "database" || len(page.Items) == 0) {
			return fmt.Errorf("database search not exercised")
		}
		return e
	})
	measure("report_read_decode_ms", func() error {
		r, e := rs.Get(ctx, env.TenantA, id("report", 1))
		if e == nil && (len(r.Snapshot.Assets) != 100 || len(r.Snapshot.Assets[0].Findings) != 40) {
			return fmt.Errorf("incorrect volume report")
		}
		return e
	})
	plans := map[string]json.RawMessage{}
	err = env.App.InTenantRead(ctx, env.TenantA, func(tx pgx.Tx) error {
		for name, q := range map[string]string{
			"findings_order": "SELECT f.id,f.last_seen_at FROM findings f LEFT JOIN assets a ON a.tenant_id=f.tenant_id AND a.id=f.asset_id WHERE f.tenant_id=$1 AND (f.asset_id IS NULL OR a.deleted_at IS NULL) ORDER BY f.last_seen_at DESC,f.id DESC LIMIT 26",
			"home_aggregate": "SELECT f.type,f.severity,count(*) FROM findings f LEFT JOIN assets a ON a.tenant_id=f.tenant_id AND a.id=f.asset_id WHERE f.tenant_id=$1 AND (f.asset_id IS NULL OR a.deleted_at IS NULL) GROUP BY f.type,f.severity",
			"report_lookup":  "SELECT snapshot FROM reports WHERE tenant_id=$1 AND id='" + id("report", 1).String() + "'",
		} {
			var raw []byte
			if err := tx.QueryRow(ctx, "EXPLAIN(ANALYZE,BUFFERS,FORMAT JSON) "+q, env.TenantA).Scan(&raw); err != nil {
				return err
			}
			plans[name] = raw
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	es := search.New("http://127.0.0.1:19200")
	es.Index = "orchis-volume-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	writeMarker(es.Index)
	t.Cleanup(func() {
		c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := es.DeleteIndex(c); err != nil {
			t.Errorf("owned ES index cleanup failed: %v", err)
		}
	})
	if err = es.EnsureIndex(ctx); err != nil {
		t.Fatal(err)
	}
	indexed := time.Now()
	rows, err := env.Owner.Query(ctx, "SELECT id,asset_id,title,value,severity,last_seen_at FROM findings WHERE tenant_id=$1 ORDER BY id", env.TenantA)
	if err != nil {
		t.Fatal(err)
	}
	var batch bytes.Buffer
	documents := 0
	flush := func() error {
		req, err := http.NewRequestWithContext(ctx, "POST", es.URL+"/"+es.Index+"/_bulk", bytes.NewReader(batch.Bytes()))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/x-ndjson")
		res, err := es.HTTP.Do(req)
		if err != nil {
			return err
		}
		defer res.Body.Close()
		raw, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
		if err != nil {
			return err
		}
		var status struct {
			Errors bool `json:"errors"`
		}
		if json.Unmarshal(raw, &status) != nil || res.StatusCode >= 300 || status.Errors {
			return fmt.Errorf("bulk indexing failed")
		}
		batch.Reset()
		return nil
	}
	for rows.Next() {
		var fid, aid uuid.UUID
		var title, value, severity string
		var last time.Time
		if err = rows.Scan(&fid, &aid, &title, &value, &severity, &last); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		action, _ := json.Marshal(map[string]any{"index": map[string]string{"_id": fid.String()}})
		doc, _ := json.Marshal(search.FindingDoc{TenantID: env.TenantA, AssetID: &aid, AssetName: "LOCAL VOLUME SYNTHETIC", Type: "vulnerability", Severity: severity, Title: title, Value: value, LastSeenAt: last})
		batch.Write(action)
		batch.WriteByte('\n')
		batch.Write(doc)
		batch.WriteByte('\n')
		documents++
		if documents%1000 == 0 {
			if err = flush(); err != nil {
				rows.Close()
				t.Fatal(err)
			}
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		t.Fatal(err)
	}
	if batch.Len() > 0 {
		if err = flush(); err != nil {
			t.Fatal(err)
		}
	}
	if documents != 200000 {
		t.Fatal(documents)
	}
	if err = es.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	indexSeconds := time.Since(indexed).Seconds()
	measure("search_elasticsearch_ms", func() error {
		page, e := fs.ListFindings(ctx, p, execution.FindingFilter{Limit: 25, Query: "issue"}, es)
		if e == nil && (page.Search != "elasticsearch" || len(page.Items) != 25) {
			return fmt.Errorf("ES search not exercised")
		}
		return e
	})
	var size int64
	if err = env.Owner.QueryRow(ctx, "SELECT pg_database_size(current_database())").Scan(&size); err != nil {
		t.Fatal(err)
	}
	output := os.Getenv("ORCHIS_VOLUME_OUTPUT")
	if output == "" {
		output = "../../work/volume/result.json"
	}
	if err = os.MkdirAll(filepath.Dir(output), 0700); err != nil {
		t.Fatal(err)
	}
	result := map[string]any{"synthetic": true, "rows": counts, "seed_seconds": seedSeconds, "index_seconds": indexSeconds, "database_bytes": size, "timings": metrics, "plans": plans, "limits": "12 minute context, 120 second seed statements, fixed 200000 rows; no production scans, no SLA threshold", "cleanup": "test cleanup drops this database and deletes only its unique ES index"}
	raw, _ := json.MarshalIndent(result, "", "  ")
	if err = os.WriteFile(output, raw, 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("Volume measurements written to %s; database=%d bytes", output, size)
}
