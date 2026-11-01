// Package endpoint: API untuk data endpoint dari osquery: katalog "data yang dapat dipetakan", persetujuan per organisasi,
// data terakhir per aset, dan tombol "jalankan sekarang". Semua data dibaca dari hasil yang sudah disanitasi server.
package endpoint

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"orchis-go/internal/audit"
	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/db"
	"orchis-go/internal/execution"
	"orchis-go/internal/httpx"
	"orchis-go/internal/osquery"
)

type Service struct {
	DB   *db.DB
	Exec *execution.Service
}

type RuleMeta struct {
	Rule     string `json:"rule"`
	Type     string `json:"type"`
	Severity string `json:"severity"`
	Title    string `json:"title"`
	Explain  string `json:"explain"`
}

type QueryMeta struct {
	Key               string              `json:"key"`
	Pack              string              `json:"pack,omitempty"`
	Title             string              `json:"title"`
	Description       string              `json:"description"`
	Category          string              `json:"category"`
	Tables            []string            `json:"tables"`
	Columns           []string            `json:"columns"`
	OS                []string            `json:"os"`
	Sensitivity       string              `json:"sensitivity"`
	Load              string              `json:"load"`
	NeedsRoot         bool                `json:"needs_root"`
	DefaultEnabled    bool                `json:"default_enabled"`
	TimeoutSeconds    int                 `json:"timeout_seconds"`
	MaxRows           int                 `json:"max_rows"`
	Mitre             []osquery.MitreRef  `json:"mitre"`
	Attributes        []string            `json:"attributes"`
	Findings          []RuleMeta          `json:"findings"`
	Report            string              `json:"report_section"`
	Example           []map[string]string `json:"example"`
	ExampleFindings   []ExampleFinding    `json:"example_findings"`
	ExampleAttributes map[string]any      `json:"example_attributes"`
}

// ExampleFinding: temuan yang dihasilkan aturan beku dari data contoh (pratinjau, bukan data nyata siapa pun).
type ExampleFinding struct {
	Rule     string `json:"rule"`
	Type     string `json:"type"`
	Severity string `json:"severity"`
	Title    string `json:"title"`
}

// PackView: pack resmi beku apa adanya, termasuk query yang TIDAK dijalankan beserta alasannya.
type PackView struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	File        string              `json:"file"`
	SHA256      string              `json:"sha256"`
	Platform    string              `json:"platform"`
	TablesRead  []string            `json:"tables_read"` // gabungan tabel query yang dapat dijalankan: ditampilkan sebelum admin menyetujui
	Queries     []osquery.PackQuery `json:"queries"`
}

type Catalog struct {
	Consent        osquery.ConsentDocument `json:"consent"`
	OsqueryVersion string                  `json:"osquery_version"`
	License        string                  `json:"license"`
	ProvenOS       []string                `json:"proven_os"`
	Queries        []QueryMeta             `json:"queries"`
	Packs          []PackView              `json:"packs"`
}

func packView(p *osquery.Pack) PackView {
	v := PackView{Name: p.Name, Description: p.Description, File: p.File, SHA256: p.SHA256, Platform: p.Platform, Queries: p.Queries, TablesRead: packTables(p)}
	return v
}

func packTables(p *osquery.Pack) []string {
	set := map[string]bool{}
	for _, k := range p.RunnableKeys() {
		for _, t := range osquery.Find(k).TablesRead() {
			set[t] = true
		}
	}
	out := []string{}
	for t := range set {
		out = append(out, t)
	}
	sortStrings(out)
	return out
}

func meta(q *osquery.Query) QueryMeta {
	m := QueryMeta{Key: q.Key, Pack: q.Pack, Title: q.Title, Description: q.Description, Category: q.Category, Tables: q.TablesRead(), Columns: q.Columns, OS: q.OS,
		Sensitivity: q.Sensitivity, Load: q.Load, NeedsRoot: q.NeedsRoot, DefaultEnabled: q.DefaultEnabled(), TimeoutSeconds: q.TimeoutSeconds,
		MaxRows: q.MaxRows, Mitre: q.Mitre, Attributes: []string{}, Findings: []RuleMeta{}, Report: "Endpoint posture", Example: q.Example, ExampleFindings: []ExampleFinding{}}
	mapped := q.Map(&osquery.Dataset{Query: q.Key, Status: osquery.StatusOK, Rows: q.Example})
	m.ExampleAttributes = mapped.Attributes
	for _, f := range mapped.Findings {
		m.ExampleFindings = append(m.ExampleFindings, ExampleFinding{Rule: f.Rule, Type: f.Type, Severity: f.Severity, Title: f.Title})
	}
	for k := range q.Attributes.FirstRow {
		m.Attributes = append(m.Attributes, k)
	}
	for _, a := range q.Attributes.Aggregates {
		m.Attributes = append(m.Attributes, a.Key)
	}
	sortStrings(m.Attributes)
	for _, r := range q.Rules {
		m.Findings = append(m.Findings, RuleMeta{Rule: r.ID, Type: r.Finding.Type, Severity: r.Finding.Severity, Title: r.Finding.Title, Explain: r.Explain})
	}
	return m
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// CatalogView adalah katalog beku apa adanya. Linux adalah satu-satunya OS yang terbukti; OS lain hanya struktur.
func CatalogView() Catalog {
	c := Catalog{Consent: osquery.Consent(), OsqueryVersion: osquery.Version, License: osquery.License, ProvenOS: []string{"linux"}, Queries: []QueryMeta{}, Packs: []PackView{}}
	for i := range osquery.Packs {
		c.Packs = append(c.Packs, packView(&osquery.Packs[i]))
	}
	for i := range osquery.Queries {
		c.Queries = append(c.Queries, meta(&osquery.Queries[i]))
	}
	return c
}

type OrgSetting struct {
	ConsentVersion *string    `json:"consent_version"`
	ConsentSHA256  *string    `json:"consent_sha256"`
	ConsentedBy    *uuid.UUID `json:"consented_by"`
	ConsentedAt    *time.Time `json:"consented_at"`
	Key            string     `json:"key"`
	Enabled        bool       `json:"enabled"`
	Explicit       bool       `json:"explicit"` // true = keputusan admin; false = bawaan katalog
	UpdatedAt      *time.Time `json:"updated_at"`
}

func (s *Service) OrgSettings(ctx context.Context, p *auth.Principal, org uuid.UUID) ([]OrgSetting, error) {
	out := []OrgSetting{}
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM organizations WHERE tenant_id=$1 AND id=$2)`, p.TenantID, org).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return httpx.NotFound("Organization")
		}
		for i := range osquery.Queries {
			q := &osquery.Queries[i]
			en, ex, err := execution.OsqueryEnabled(ctx, tx, p.TenantID, &org, q)
			if err != nil {
				return err
			}
			st := OrgSetting{Key: q.Key, Enabled: en, Explicit: ex}
			if ex {
				if err := tx.QueryRow(ctx, `SELECT updated_at,consent_version,consent_sha256,consented_by,consented_at FROM osquery_org_settings WHERE tenant_id=$1 AND organization_id=$2 AND query_key=$3`, p.TenantID, org, q.Key).Scan(&st.UpdatedAt, &st.ConsentVersion, &st.ConsentSHA256, &st.ConsentedBy, &st.ConsentedAt); err != nil {
					return err
				}
			}
			out = append(out, st)
		}
		return nil
	})
	return out, err
}

func (s *Service) SetOrg(ctx context.Context, p *auth.Principal, org uuid.UUID, key string, enabled bool, consent osquery.ConsentRef, ip, reqID string) error {
	q := osquery.Find(key)
	if q == nil {
		return httpx.NotFound("Query")
	}
	if enabled && !consent.Valid() {
		return httpx.Validation(httpx.FieldError{Name: "consent", Message: "Review and accept the current endpoint collection document."})
	}
	return s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		var found uuid.UUID
		err := tx.QueryRow(ctx, `SELECT id FROM organizations WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, p.TenantID, org).Scan(&found)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound("Organization")
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO osquery_org_settings (tenant_id, organization_id, query_key, enabled, updated_by, updated_at,consent_version,consent_sha256,consented_by,consented_at) VALUES ($1,$2,$3,$4,$5::uuid,now(),CASE WHEN $4 THEN $6::text ELSE NULL END,CASE WHEN $4 THEN $7::text ELSE NULL END,CASE WHEN $4 THEN $5::uuid ELSE NULL END,CASE WHEN $4 THEN now() ELSE NULL END)
ON CONFLICT (tenant_id, organization_id, query_key) DO UPDATE SET enabled=EXCLUDED.enabled, updated_by=EXCLUDED.updated_by, updated_at=now(), version=osquery_org_settings.version+1,consent_version=CASE WHEN EXCLUDED.enabled THEN EXCLUDED.consent_version ELSE osquery_org_settings.consent_version END,consent_sha256=CASE WHEN EXCLUDED.enabled THEN EXCLUDED.consent_sha256 ELSE osquery_org_settings.consent_sha256 END,consented_by=CASE WHEN EXCLUDED.enabled THEN EXCLUDED.consented_by ELSE osquery_org_settings.consented_by END,consented_at=CASE WHEN EXCLUDED.enabled THEN EXCLUDED.consented_at ELSE osquery_org_settings.consented_at END`, p.TenantID, org, key, enabled, p.UserID, consent.Version, consent.SHA256); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "osquery.setting", ResourceType: "organization", ResourceID: &org, IP: ip, RequestID: reqID,
			Detail: map[string]any{"query": key, "enabled": enabled, "consent": consent, "tables_read": q.TablesRead(), "sensitivity": q.Sensitivity}})
	})
}

// SetOrgPack menyetujui/mencabut SELURUH query pack yang dapat dijalankan dalam satu transaksi. Audit memuat versi dan hash
// pack serta tabel yang dibaca, sehingga persetujuan dapat ditelusuri. Query yang dilewati tidak pernah ikut diaktifkan.
func (s *Service) SetOrgPack(ctx context.Context, p *auth.Principal, org uuid.UUID, pack string, enabled bool, consent osquery.ConsentRef, ip, reqID string) error {
	pk := osquery.FindPack(pack)
	if pk == nil {
		return httpx.NotFound("Pack")
	}
	keys := pk.RunnableKeys()
	if enabled && !consent.Valid() {
		return httpx.Validation(httpx.FieldError{Name: "consent", Message: "Review and accept the current endpoint collection document."})
	}
	return s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		var found uuid.UUID
		err := tx.QueryRow(ctx, `SELECT id FROM organizations WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, p.TenantID, org).Scan(&found)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound("Organization")
		}
		if err != nil {
			return err
		}
		for _, k := range keys {
			if _, err := tx.Exec(ctx, `INSERT INTO osquery_org_settings (tenant_id, organization_id, query_key, enabled, updated_by, updated_at,consent_version,consent_sha256,consented_by,consented_at) VALUES ($1,$2,$3,$4,$5::uuid,now(),CASE WHEN $4 THEN $6::text ELSE NULL END,CASE WHEN $4 THEN $7::text ELSE NULL END,CASE WHEN $4 THEN $5::uuid ELSE NULL END,CASE WHEN $4 THEN now() ELSE NULL END)
ON CONFLICT (tenant_id, organization_id, query_key) DO UPDATE SET enabled=EXCLUDED.enabled, updated_by=EXCLUDED.updated_by, updated_at=now(), version=osquery_org_settings.version+1,consent_version=CASE WHEN EXCLUDED.enabled THEN EXCLUDED.consent_version ELSE osquery_org_settings.consent_version END,consent_sha256=CASE WHEN EXCLUDED.enabled THEN EXCLUDED.consent_sha256 ELSE osquery_org_settings.consent_sha256 END,consented_by=CASE WHEN EXCLUDED.enabled THEN EXCLUDED.consented_by ELSE osquery_org_settings.consented_by END,consented_at=CASE WHEN EXCLUDED.enabled THEN EXCLUDED.consented_at ELSE osquery_org_settings.consented_at END`, p.TenantID, org, k, enabled, p.UserID, consent.Version, consent.SHA256); err != nil {
				return err
			}
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "osquery.pack", ResourceType: "organization", ResourceID: &org, IP: ip, RequestID: reqID,
			Detail: map[string]any{"pack": pack, "enabled": enabled, "consent": consent, "osquery_version": osquery.Version, "pack_sha256": pk.SHA256, "queries": keys, "tables_read": packTables(pk)}})
	})
}

type DatasetView struct {
	Key           string              `json:"key"`
	Title         string              `json:"title"`
	Status        string              `json:"status"` // ok | skipped | failed | never
	Reason        string              `json:"reason"`
	CollectedAt   *time.Time          `json:"collected_at"`
	LastAttemptAt *time.Time          `json:"last_attempt_at"`
	Version       string              `json:"osquery_version"`
	Attributes    map[string]any      `json:"attributes"`
	Rows          []map[string]string `json:"rows"`
	Truncated     bool                `json:"truncated"`
	Enabled       bool                `json:"enabled"`
	Synthetic     bool                `json:"synthetic"`
}

type FindingView struct {
	ID       uuid.UUID `json:"id"`
	Type     string    `json:"type"`
	Severity string    `json:"severity"`
	Title    string    `json:"title"`
	Value    string    `json:"value"`
	Query    string    `json:"query"`
}

type AssetData struct {
	Version        int           `json:"version"`
	ReadOnly       bool          `json:"read_only"`
	AssetID        uuid.UUID     `json:"asset_id"`
	OrganizationID *uuid.UUID    `json:"organization_id"`
	HasAgent       bool          `json:"has_agent"`
	Items          []DatasetView `json:"items"`
	Findings       []FindingView `json:"findings"`
}

const rowsShown = 200

func (s *Service) AssetData(ctx context.Context, p *auth.Principal, asset uuid.UUID) (*AssetData, error) {
	out := &AssetData{AssetID: asset, Items: []DatasetView{}, Findings: []FindingView{}}
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT version,is_demo,organization_id, EXISTS(SELECT 1 FROM agents ag WHERE ag.tenant_id=a.tenant_id AND ag.asset_id=a.id AND NOT ag.is_demo AND ag.status<>'revoked')
FROM assets a WHERE a.tenant_id=$1 AND a.id=$2 AND a.deleted_at IS NULL`, p.TenantID, asset).Scan(&out.Version, &out.ReadOnly, &out.OrganizationID, &out.HasAgent)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound("Asset")
		}
		if err != nil {
			return err
		}
		stored := map[string]DatasetView{}
		rows, err := tx.Query(ctx, `SELECT query_key, rows, attributes, truncated, collected_at, last_attempt_at, COALESCE(osquery_version,''), last_status, last_reason, is_demo
FROM endpoint_datasets WHERE tenant_id=$1 AND asset_id=$2`, p.TenantID, asset)
		if err != nil {
			return err
		}
		for rows.Next() {
			var v DatasetView
			var rj, aj []byte
			if err := rows.Scan(&v.Key, &rj, &aj, &v.Truncated, &v.CollectedAt, &v.LastAttemptAt, &v.Version, &v.Status, &v.Reason, &v.Synthetic); err != nil {
				rows.Close()
				return err
			}
			_ = json.Unmarshal(rj, &v.Rows)
			_ = json.Unmarshal(aj, &v.Attributes)
			if len(v.Rows) > rowsShown {
				v.Rows, v.Truncated = v.Rows[:rowsShown], true
			}
			stored[v.Key] = v
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for i := range osquery.Queries {
			q := &osquery.Queries[i]
			v, ok := stored[q.Key]
			if !ok {
				v = DatasetView{Status: "never", Rows: []map[string]string{}, Attributes: map[string]any{}}
			}
			if v.Rows == nil {
				v.Rows = []map[string]string{}
			}
			if v.Attributes == nil {
				v.Attributes = map[string]any{}
			}
			v.Key, v.Title = q.Key, q.Title
			en, _, err := execution.OsqueryEnabled(ctx, tx, p.TenantID, out.OrganizationID, q)
			if err != nil {
				return err
			}
			v.Enabled = en
			out.Items = append(out.Items, v)
		}
		fr, err := tx.Query(ctx, `SELECT id, type, severity, title, value, COALESCE(details->>'query','') FROM findings
WHERE tenant_id=$1 AND asset_id=$2 AND details->>'source'='osquery' ORDER BY severity DESC, title LIMIT 200`, p.TenantID, asset)
		if err != nil {
			return err
		}
		defer fr.Close()
		for fr.Next() {
			var f FindingView
			if err := fr.Scan(&f.ID, &f.Type, &f.Severity, &f.Title, &f.Value, &f.Query); err != nil {
				return err
			}
			out.Findings = append(out.Findings, f)
		}
		return fr.Err()
	})
	return out, err
}

// Refresh menjalankan satu query bernama pada aset (memakai atomic test yang ada atau membuat satu), lewat jalur yang sama
// dengan "Run" biasa: aset harus diotorisasi, agent hidup, dan persetujuan organisasi dicek saat agent mengambil tugas.
func (s *Service) Refresh(ctx context.Context, p *auth.Principal, asset uuid.UUID, key, ip, reqID string) (*execution.ExecutionDetail, error) {
	q := osquery.Find(key)
	if q == nil {
		return nil, httpx.NotFound("Query")
	}
	tests, err := s.Exec.ListTests(ctx, p)
	if err != nil {
		return nil, err
	}
	var testID uuid.UUID
	for _, t := range tests {
		if t.ContractKey == key && t.TargetAssetID != nil && *t.TargetAssetID == asset {
			testID = t.ID
			break
		}
	}
	if testID == uuid.Nil {
		t, err := s.Exec.CreateTest(ctx, p, execution.TestInput{Name: "Endpoint data: " + q.Title, ContractKey: key, TargetAssetID: &asset}, ip, reqID)
		if err != nil {
			return nil, err
		}
		testID = t.ID
	}
	return s.Exec.Run(ctx, p, testID, ip, reqID)
}

type Handlers struct{ Svc *Service }

func reply(w http.ResponseWriter, r *http.Request, status int, v any, err error) {
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	httpx.JSON(w, status, v)
}

func id(r *http.Request, name, what string) (uuid.UUID, error) {
	v, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		return v, httpx.NotFound(what)
	}
	return v, nil
}

func (h *Handlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/osquery/catalog", auth.Require(authz.AtomicRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		httpx.JSON(w, 200, CatalogView())
	}))
	mux.HandleFunc("GET /api/v1/organizations/{id}/osquery", auth.Require(authz.OrganizationRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		org, err := id(r, "id", "Organization")
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		items, err := h.Svc.OrgSettings(r.Context(), p, org)
		reply(w, r, 200, map[string]any{"items": items}, err)
	}))
	mux.HandleFunc("PUT /api/v1/organizations/{id}/osquery/{key}", auth.Require(authz.SettingsManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		org, err := id(r, "id", "Organization")
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		var in struct {
			Enabled *bool              `json:"enabled"`
			Consent osquery.ConsentRef `json:"consent"`
		}
		if err := httpx.Decode(w, r, &in); err != nil {
			httpx.Write(w, r, err)
			return
		}
		if in.Enabled == nil {
			httpx.Write(w, r, httpx.Validation(httpx.FieldError{Name: "enabled", Message: "Choose on or off."}))
			return
		}
		err = h.Svc.SetOrg(r.Context(), p, org, r.PathValue("key"), *in.Enabled, in.Consent, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("PUT /api/v1/organizations/{id}/osquery-packs/{pack}", auth.Require(authz.SettingsManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		org, err := id(r, "id", "Organization")
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		var in struct {
			Enabled *bool              `json:"enabled"`
			Consent osquery.ConsentRef `json:"consent"`
		}
		if err := httpx.Decode(w, r, &in); err != nil {
			httpx.Write(w, r, err)
			return
		}
		if in.Enabled == nil {
			httpx.Write(w, r, httpx.Validation(httpx.FieldError{Name: "enabled", Message: "Choose on or off."}))
			return
		}
		if err := h.Svc.SetOrgPack(r.Context(), p, org, r.PathValue("pack"), *in.Enabled, in.Consent, auth.ClientIP(r), httpx.RequestIDFrom(r.Context())); err != nil {
			httpx.Write(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("DELETE /api/v1/assets/{id}/endpoint-data", auth.Require(authz.AssetWrite, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		if !authz.Has(p.Permissions, authz.SettingsManage) {
			httpx.Write(w, r, httpx.Forbidden(authz.SettingsManage))
			return
		}
		asset, err := id(r, "id", "Asset")
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		version, err := strconv.Atoi(r.URL.Query().Get("version"))
		if err != nil || version < 1 {
			httpx.Write(w, r, httpx.Validation(httpx.FieldError{Name: "version", Message: "Reload the asset before deleting collected data."}))
			return
		}
		if err = h.Svc.DeleteData(r.Context(), p, asset, version, auth.ClientIP(r), httpx.RequestIDFrom(r.Context())); err != nil {
			httpx.Write(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("GET /api/v1/assets/{id}/endpoint-data", auth.Require(authz.AssetRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		asset, err := id(r, "id", "Asset")
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		d, err := h.Svc.AssetData(r.Context(), p, asset)
		reply(w, r, 200, d, err)
	}))
	mux.HandleFunc("POST /api/v1/assets/{id}/endpoint-data/refresh", auth.Require(authz.AtomicRun, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		asset, err := id(r, "id", "Asset")
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		var in struct {
			Query string `json:"query_key"`
		}
		if err := httpx.Decode(w, r, &in); err != nil {
			httpx.Write(w, r, err)
			return
		}
		d, err := h.Svc.Refresh(r.Context(), p, asset, in.Query, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
		reply(w, r, http.StatusAccepted, d, err)
	}))
}

// DeleteData menghapus dataset terbaru saja; snapshot dan temuan mempunyai siklus retensi sendiri.
func (s *Service) DeleteData(ctx context.Context, p *auth.Principal, asset uuid.UUID, version int, ip, reqID string) error {
	return s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		var current int
		var demo bool
		err := tx.QueryRow(ctx, `SELECT version,is_demo FROM assets WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL FOR UPDATE`, p.TenantID, asset).Scan(&current, &demo)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound("Asset")
		}
		if err != nil {
			return err
		}
		if demo {
			return httpx.Conflict("Demo data is read-only; use the demo purge action.")
		}
		if current != version {
			return httpx.Conflict("The asset changed; reload it before deleting collected data.")
		}
		if _, err = tx.Exec(ctx, `UPDATE assets SET version=version+1,updated_at=now() WHERE tenant_id=$1 AND id=$2`, p.TenantID, asset); err != nil {
			return err
		}
		deleted, err := tx.Exec(ctx, `DELETE FROM endpoint_datasets WHERE tenant_id=$1 AND asset_id=$2`, p.TenantID, asset)
		if err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "osquery.data.delete", ResourceType: "asset", ResourceID: &asset, IP: ip, RequestID: reqID, Detail: map[string]any{"datasets_deleted": deleted.RowsAffected(), "findings_and_reports_retained": true}})
	})
}
