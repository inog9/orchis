// Package reporting membekukan data assessment sebelum merender ekspor tanpa akses jaringan.
package reporting

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"log/slog"
	"orchis-go/internal/audit"
	"orchis-go/internal/db"
	"orchis-go/internal/execution"
	"orchis-go/internal/ids"
	"orchis-go/internal/osquery"
	"os/exec"
	"sort"
	"strings"
	"time"
)

type Service struct {
	DB            *db.DB
	Store         execution.ObjectStore
	Script        string
	RendererURL   string
	RendererToken string
}

// FindingTechnique: teknik ATT&CK yang dikaitkan ke satu temuan, dibekukan bersama snapshot (nama ikut dibekukan,
// jadi tidak berubah bila ATT&CK diperbarui). Basis "estimate" berarti perkiraan, bukan bukti bahwa teknik itu terjadi.
type FindingTechnique struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Basis  string `json:"basis"`
	Reason string `json:"reason"`
}

// EstimatedTechniques memetakan temuan ke teknik secara konservatif. Hanya kerentanan yang dikaitkan (T1190,
// Exploit Public-Facing Application) dan hanya sebagai perkiraan: temuan port/layanan/teks tidak dipetakan,
// karena itu hasil pemindaian (discovery), bukan celah yang bisa dieksploitasi.
func EstimatedTechniques(f Finding) []FindingTechnique {
	if f.Type != "vulnerability" {
		return nil
	}
	return []FindingTechnique{{ID: "T1190", Name: "Exploit Public-Facing Application", Basis: "estimate",
		Reason: "A vulnerability found on a reachable service could be abused this way. This is an estimate from the finding type; it does not show that the vulnerability was exploited."}}
}

type Finding struct {
	MITRE    []FindingTechnique `json:"mitre,omitempty"`
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Value    string             `json:"value"`
	Title    string             `json:"title"`
	Severity string             `json:"severity"`
	Evidence map[string]any     `json:"evidence"`
}
type Method struct {
	MITRE     []string        `json:"mitre"`
	Action    string          `json:"action"`
	Version   int             `json:"version"`
	Params    json.RawMessage `json:"params"`
	Target    string          `json:"target"`
	Outcome   string          `json:"outcome"`
	Error     string          `json:"error"`
	StartedAt *time.Time      `json:"started_at"`
	EndedAt   *time.Time      `json:"ended_at"`
}

// EndpointSummary: ringkasan postur endpoint tingkat laporan (data dari DALAM mesin lewat agent + osquery, bukan scan).
type EndpointSummary struct {
	AssetsTotal    int            `json:"assets_total"`
	AssetsWithData int            `json:"assets_with_data"`
	BySeverity     map[string]int `json:"by_severity"`
}

type Asset struct {
	Discovered bool             `json:"discovered,omitempty"`
	Endpoint   *osquery.Posture `json:"endpoint,omitempty"`
	ID         uuid.UUID        `json:"id"`
	Name       string           `json:"name"`
	Status     string           `json:"status"`
	SkipReason string           `json:"skip_reason"`
	Findings   []Finding        `json:"findings"`
	Methods    []Method         `json:"methods"`
}
type Snapshot struct {
	SchemaVersion   string           `json:"schema_version,omitempty"`
	ScanID          *uuid.UUID       `json:"scan_id,omitempty"`
	EndpointSummary *EndpointSummary `json:"endpoint_summary,omitempty"`
	IsDemo          bool             `json:"is_demo,omitempty"`
	Label           string           `json:"label,omitempty"`
	Organization    string           `json:"organization"`
	AssessmentID    uuid.UUID        `json:"assessment_id"`
	Profile         string           `json:"profile"`
	CreatedAt       time.Time        `json:"created_at"`
	Status          string           `json:"status"`
	Risk            string           `json:"risk"`
	BySeverity      map[string]int   `json:"by_severity"`
	Assets          []Asset          `json:"assets"`
	Remediation     string           `json:"remediation"`
	Comparison      string           `json:"comparison"`
}
type Report struct {
	ID             uuid.UUID `json:"id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	AssessmentID   uuid.UUID `json:"assessment_id"`
	Version        int       `json:"version"`
	Status         string    `json:"status"`
	Error          string    `json:"error"`
	CreatedAt      time.Time `json:"created_at"`
	Snapshot       Snapshot  `json:"snapshot"`
}

// Freeze selalu dalam transaksi yang sama: satu laporan per assessment, termasuk seluruh aset yang dilewati.
func (s *Service) Freeze(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) error {
	rows, err := tx.Query(ctx, `SELECT a.id,a.organization_id,a.name,a.profile,a.status,a.created_at FROM assessments a WHERE a.tenant_id=$1 AND a.status<>'running' AND NOT a.is_scan AND NOT EXISTS(SELECT 1 FROM reports r WHERE r.tenant_id=a.tenant_id AND r.assessment_id=a.id) ORDER BY a.id LIMIT 10 FOR UPDATE OF a SKIP LOCKED`, tenant)
	if err != nil {
		return err
	}
	type pending struct {
		id, org uuid.UUID
		snap    Snapshot
	}
	list := []pending{}
	for rows.Next() {
		var p pending
		if err = rows.Scan(&p.id, &p.org, &p.snap.Organization, &p.snap.Profile, &p.snap.Status, &p.snap.CreatedAt); err != nil {
			rows.Close()
			return err
		}
		p.snap.AssessmentID = p.id
		p.snap.CreatedAt = p.snap.CreatedAt.UTC()
		list = append(list, p)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	for _, p := range list {
		snap := p.snap
		snap.BySeverity = map[string]int{"critical": 0, "high": 0, "medium": 0, "low": 0, "info": 0}
		snap.Assets = []Asset{}
		snap.Risk = "No data yet"
		snap.Remediation = "Review each finding before remediation. AI advice can be requested as a separate advisory supplement; it does not modify this assessment snapshot."
		snap.Comparison = "Comparison unavailable: no validated comparable assessment baseline."
		ar, e := tx.Query(ctx, `SELECT t.asset_id,t.asset_name,t.skip_reason,t.simulation_id,COALESCE(s.status,'skipped') FROM assessment_targets t LEFT JOIN simulations s ON s.tenant_id=t.tenant_id AND s.id=t.simulation_id WHERE t.tenant_id=$1 AND t.assessment_id=$2 ORDER BY t.asset_name,t.asset_id`, tenant, p.id)
		if e != nil {
			return e
		}
		type item struct {
			a   Asset
			sim *uuid.UUID
		}
		items := []item{}
		for ar.Next() {
			var x item
			if e = ar.Scan(&x.a.ID, &x.a.Name, &x.a.SkipReason, &x.sim, &x.a.Status); e != nil {
				ar.Close()
				return e
			}
			items = append(items, x)
		}
		ar.Close()
		if e = ar.Err(); e != nil {
			return e
		}
		for _, x := range items {
			a := x.a
			a.Name = Redact(a.Name, 200)
			a.SkipReason = Redact(a.SkipReason, 1000)
			a.Methods = []Method{}
			a.Findings = []Finding{}
			if x.sim != nil {
				mr, e := tx.Query(ctx, `SELECT c.key,e.contract_version,e.params,e.target_address,COALESCE(e.outcome,''),COALESCE(e.error,''),e.started_at,e.ended_at,ARRAY(SELECT ct.technique_id FROM contract_techniques ct WHERE ct.tenant_id=e.tenant_id AND ct.contract_id=e.contract_id ORDER BY ct.technique_id) FROM executions e JOIN contracts c ON c.tenant_id=e.tenant_id AND c.id=e.contract_id WHERE e.tenant_id=$1 AND e.simulation_id=$2 ORDER BY e.created_at,e.id`, tenant, *x.sim)
				if e != nil {
					return e
				}
				success := true
				for mr.Next() {
					var m Method
					if e = mr.Scan(&m.Action, &m.Version, &m.Params, &m.Target, &m.Outcome, &m.Error, &m.StartedAt, &m.EndedAt, &m.MITRE); e != nil {
						mr.Close()
						return e
					}
					m.Error = Redact(m.Error, 1000)
					m.Target = Redact(m.Target, 1000)
					if m.Outcome != "success" {
						success = false
					}
					a.Methods = append(a.Methods, m)
				}
				mr.Close()
				if e = mr.Err(); e != nil {
					return e
				}
				if !success {
					a.Status = "incomplete"
				}
				fr, e := tx.Query(ctx, `SELECT DISTINCT ON(f.id) f.id,COALESCE(o.finding_snapshot->>'type',f.type),COALESCE(o.finding_snapshot->>'value',f.value),COALESCE(o.finding_snapshot->>'title',f.title),COALESCE(o.finding_snapshot->>'severity',f.severity),o.details FROM finding_occurrences o JOIN findings f ON f.tenant_id=o.tenant_id AND f.id=o.finding_id WHERE o.tenant_id=$1 AND o.simulation_id=$2 ORDER BY f.id,o.seen_at DESC`, tenant, *x.sim)
				if e != nil {
					return e
				}
				for fr.Next() {
					var f Finding
					var details map[string]any
					if e = fr.Scan(&f.ID, &f.Type, &f.Value, &f.Title, &f.Severity, &details); e != nil {
						fr.Close()
						return e
					}
					f.Value = Redact(f.Value, 1000)
					f.Title = Redact(f.Title, 500)
					f.Evidence = SafeEvidence(details)
					f.MITRE = EstimatedTechniques(f)
					a.Findings = append(a.Findings, f)
					if f.Type == "vulnerability" {
						snap.BySeverity[f.Severity]++
					}
				}
				fr.Close()
				if e = fr.Err(); e != nil {
					return e
				}
			}
			ranks := map[string]int{"critical": 4, "high": 3, "medium": 2, "low": 1, "info": 0}
			sort.SliceStable(a.Findings, func(i, j int) bool {
				ri, rj := ranks[a.Findings[i].Severity], ranks[a.Findings[j].Severity]
				if ri != rj {
					return ri > rj
				}
				return a.Findings[i].ID < a.Findings[j].ID
			})
			if a.Endpoint, e = endpointPosture(ctx, tx, tenant, a); e != nil {
				return e
			}
			snap.Assets = append(snap.Assets, a)
		}
		snap.EndpointSummary = summarizeEndpoint(snap.Assets)
		snap.Risk = exposureRisk(snap)
		var previous []byte
		e = tx.QueryRow(ctx, `SELECT r.snapshot FROM reports r JOIN assessments a ON a.tenant_id=r.tenant_id AND a.id=r.assessment_id WHERE r.tenant_id=$1 AND r.organization_id=$2 AND a.profile=$3 AND a.created_at<$4 ORDER BY a.created_at DESC,a.id DESC LIMIT 1`, tenant, p.org, snap.Profile, snap.CreatedAt).Scan(&previous)
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if e == nil {
			var old Snapshot
			if e = json.Unmarshal(previous, &old); e != nil {
				return e
			}
			snap.Comparison = Compare(snap, old)
		}
		snap.Organization = Redact(snap.Organization, 200)
		data, e := json.Marshal(snap)
		if e != nil {
			return e
		}
		rid := ids.New()
		tag, e := tx.Exec(ctx, `INSERT INTO reports(id,tenant_id,organization_id,assessment_id,snapshot) VALUES($1,$2,$3,$4,$5) ON CONFLICT(tenant_id,assessment_id) DO NOTHING`, rid, tenant, p.org, p.id, data)
		if e != nil {
			return e
		}
		if tag.RowsAffected() > 0 {
			if e = audit.Write(ctx, tx, audit.Entry{Tenant: tenant, Action: "report.snapshot", ResourceType: "report", ResourceID: &rid, Detail: map[string]any{"assessment_id": p.id, "organization_id": p.org}}); e != nil {
				return e
			}
		}
	}
	return nil
}

// Tick memisahkan transaksi snapshot/klaim dari proses Chromium dan penyimpanan objek.
func (s *Service) Tick(ctx context.Context, tenant uuid.UUID) error {
	var id uuid.UUID
	var snapshot Snapshot
	var attempt int
	err := s.DB.InTenant(ctx, tenant, func(tx pgx.Tx) error {
		if e := s.Freeze(ctx, tx, tenant); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `UPDATE reports SET status='failed',error='The renderer repeatedly stopped before completion.',lease_until=NULL,updated_at=now() WHERE tenant_id=$1 AND status='rendering' AND lease_until<now() AND attempts>=3`, tenant); e != nil {
			return e
		}
		var data []byte
		e := tx.QueryRow(ctx, `UPDATE reports SET status='rendering',attempts=attempts+1,lease_until=now()+interval '2 minutes',updated_at=now() WHERE tenant_id=$1 AND id=(SELECT id FROM reports WHERE tenant_id=$1 AND attempts<3 AND (status='queued' OR (status='rendering' AND lease_until<now())) ORDER BY id LIMIT 1 FOR UPDATE SKIP LOCKED) RETURNING id,snapshot,attempts`, tenant).Scan(&id, &data, &attempt)
		if errors.Is(e, pgx.ErrNoRows) {
			return nil
		}
		if e != nil {
			return e
		}
		return json.Unmarshal(data, &snapshot)
	})
	if err != nil || id == uuid.Nil {
		return err
	}
	var pdf []byte
	if s.Store == nil {
		err = errors.New("Report storage is not configured.")
	} else {
		var html []byte
		html, err = HTML(snapshot)
		if err == nil {
			pdf, err = s.render(ctx, html)
		}
		if err == nil {
			err = s.Store.Put(ctx, tenant.String()+"/reports/"+id.String()+".pdf", pdf, "application/pdf")
		}
	}
	status, key, msg := "ready", tenant.String()+"/reports/"+id.String()+".pdf", ""
	if err != nil {
		status = "failed"
		key = ""
		msg = "PDF rendering or storage failed. Retry after checking the report worker."
		slog.Error("render laporan gagal", "report", id, "err", err)
	}
	return s.DB.InTenant(ctx, tenant, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `UPDATE reports SET status=$3,pdf_key=$4,error=$5,lease_until=NULL,updated_at=now() WHERE tenant_id=$1 AND id=$2 AND status='rendering' AND attempts=$6`, tenant, id, status, key, msg, attempt)
		return e
	})
}
func (s *Service) render(ctx context.Context, html []byte) ([]byte, error) {
	if s.RendererURL != "" {
		return s.renderRemote(ctx, html)
	}
	script := s.Script
	if script == "" {
		script = "deploy/report/render.mjs"
	}
	run, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(run, "node", script)
	cmd.Stdin = bytes.NewReader(html)
	var out, stderr cappedBuffer
	out.max = 32 << 20
	stderr.max = 8192
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if e := cmd.Run(); e != nil {
		return nil, fmt.Errorf("renderer: %w: %s", e, stderr.String())
	}
	if out.Len() < 5 || !strings.HasPrefix(out.String(), "%PDF-") {
		return nil, errors.New("renderer returned an invalid PDF")
	}
	return out.Bytes(), nil
}

type cappedBuffer struct {
	bytes.Buffer
	max int
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.max {
		return 0, errors.New("report output exceeds limit")
	}
	return b.Buffer.Write(p)
}
func (s *Service) Run(ctx context.Context, tenant uuid.UUID) {
	t := time.NewTicker(3 * time.Second)
	defer t.Stop()
	for {
		if err := s.Tick(ctx, tenant); err != nil && ctx.Err() == nil {
			slog.Error("report worker gagal", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// RenderPDF memakai halaman cetak yang sama dan guard egress renderer untuk suplemen AI.
func (s *Service) RenderPDF(ctx context.Context, html []byte) ([]byte, error) {
	return s.render(ctx, html)
}
