// Package home: ringkasan untuk halaman Home. Hanya membaca; setiap bagian hanya diisi bila pemanggil berhak membacanya.
package home

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/db"
	"orchis-go/internal/httpx"
)

type Service struct{ DB *db.DB }

type AssetStats struct {
	Total          int `json:"total"`
	Authorized     int `json:"authorized"`
	InternetFacing int `json:"internet_facing"`
}

type FindingStats struct {
	Total            int            `json:"total"`
	Vulnerabilities  int            `json:"vulnerabilities"`
	OpenPorts        int            `json:"open_ports"`
	VulnerableAssets int            `json:"vulnerable_assets"`
	BySeverity       map[string]int `json:"by_severity"` // hanya type=vulnerability
	New7d            int            `json:"new_7d"`
}

type AtomicStats struct {
	Tests    int `json:"tests"`
	Active   int `json:"active"`
	Runs7d   int `json:"runs_7d"`
	Failed7d int `json:"failed_7d"`
}

type ChainStats struct {
	Scenarios   int `json:"scenarios"`
	Simulations int `json:"simulations"`
	Running     int `json:"running"`
}

type SimRow struct {
	ID        uuid.UUID  `json:"id"`
	Code      string     `json:"code"`
	Name      string     `json:"name"`
	Status    string     `json:"status"`
	StartedAt *time.Time `json:"started_at"`
	CreatedAt time.Time  `json:"created_at"`
}

type RunRow struct {
	ExecutionID  uuid.UUID `json:"execution_id"`
	TestID       uuid.UUID `json:"test_id"`
	TestName     string    `json:"test_name"`
	ContractName string    `json:"contract_name"`
	Target       *string   `json:"target"`
	State        string    `json:"state"`
	Outcome      *string   `json:"outcome"`
	CreatedAt    time.Time `json:"created_at"`
	FindingCount int       `json:"finding_count"`
}

type ExposedAsset struct {
	AssetID   uuid.UUID `json:"asset_id"`
	Name      string    `json:"name"`
	Critical  int       `json:"critical"`
	High      int       `json:"high"`
	Medium    int       `json:"medium"`
	Low       int       `json:"low"`
	OpenPorts int       `json:"open_ports"`
}

type InjectorRow struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Status string `json:"status"` // online | offline | disabled
}

type Summary struct {
	Organizations []OrganizationOverview `json:"organizations"`
	Assets        *AssetStats            `json:"assets"`
	Findings      *FindingStats          `json:"findings"`
	Atomic        *AtomicStats           `json:"atomic"`
	Chains        *ChainStats            `json:"chains"`
	LatestSims    []SimRow               `json:"latest_simulations"`
	LatestRuns    []RunRow               `json:"latest_runs"`
	MostExposed   []ExposedAsset         `json:"most_exposed"`
	Injectors     []InjectorRow          `json:"injectors"`
	GeneratedAt   time.Time              `json:"generated_at"`
}

func (s *Service) Summary(ctx context.Context, p *auth.Principal) (*Summary, error) {
	out := &Summary{LatestSims: []SimRow{}, LatestRuns: []RunRow{}, MostExposed: []ExposedAsset{}, Injectors: []InjectorRow{}, GeneratedAt: time.Now().UTC()}
	t := p.TenantID
	err := s.DB.InTenantRead(ctx, t, func(tx pgx.Tx) error {
		if authz.Has(p.Permissions, authz.OrganizationRead) && authz.Has(p.Permissions, authz.AssetRead) && authz.Has(p.Permissions, authz.FindingRead) && authz.Has(p.Permissions, authz.AssessmentRead) && authz.Has(p.Permissions, authz.ReportRead) {
			var e error
			out.Organizations, e = organizationOverview(ctx, tx, t)
			if e != nil {
				return e
			}
		}

		if authz.Has(p.Permissions, authz.AssetRead) {
			a := &AssetStats{}
			if err := tx.QueryRow(ctx, `
SELECT count(*), count(*) FILTER (WHERE testing_authorized), count(*) FILTER (WHERE is_internet_facing)
FROM assets WHERE tenant_id=$1 AND deleted_at IS NULL`, t).Scan(&a.Total, &a.Authorized, &a.InternetFacing); err != nil {
				return err
			}
			out.Assets = a
		}
		if authz.Has(p.Permissions, authz.FindingRead) {
			f := &FindingStats{BySeverity: map[string]int{"critical": 0, "high": 0, "medium": 0, "low": 0, "info": 0}}
			// Finding milik aset yang sudah dihapus tidak dihitung (selaras dengan halaman Findings).
			rows, err := tx.Query(ctx, `
SELECT f.type, f.severity, count(*), count(*) FILTER (WHERE f.first_seen_at > now() - interval '7 days')
FROM findings f LEFT JOIN assets a ON a.tenant_id=f.tenant_id AND a.id=f.asset_id
WHERE f.tenant_id=$1 AND (f.asset_id IS NULL OR a.deleted_at IS NULL) GROUP BY f.type, f.severity`, t)
			if err != nil {
				return err
			}
			for rows.Next() {
				var typ, sev string
				var n, recent int
				if err := rows.Scan(&typ, &sev, &n, &recent); err != nil {
					rows.Close()
					return err
				}
				f.Total += n
				f.New7d += recent
				switch typ {
				case "vulnerability":
					f.Vulnerabilities += n
					f.BySeverity[sev] += n // sebaran severity hanya untuk kerentanan, bukan port terbuka
				case "open_port":
					f.OpenPorts += n
				}
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return err
			}
			if err := tx.QueryRow(ctx, `
SELECT count(DISTINCT f.asset_id) FROM findings f JOIN assets a ON a.tenant_id=f.tenant_id AND a.id=f.asset_id AND a.deleted_at IS NULL
WHERE f.tenant_id=$1 AND f.type='vulnerability'`, t).Scan(&f.VulnerableAssets); err != nil {
				return err
			}
			out.Findings = f

			er, err := tx.Query(ctx, `
SELECT a.id, a.name,
  count(*) FILTER (WHERE f.type='vulnerability' AND f.severity='critical'),
  count(*) FILTER (WHERE f.type='vulnerability' AND f.severity='high'),
  count(*) FILTER (WHERE f.type='vulnerability' AND f.severity='medium'),
  count(*) FILTER (WHERE f.type='vulnerability' AND f.severity IN ('low','info')),
  count(*) FILTER (WHERE f.type='open_port')
FROM findings f JOIN assets a ON a.tenant_id=f.tenant_id AND a.id=f.asset_id AND a.deleted_at IS NULL
WHERE f.tenant_id=$1
GROUP BY a.id, a.name
ORDER BY 3 DESC, 4 DESC, 5 DESC, 6 DESC, 7 DESC, a.name LIMIT 5`, t)
			if err != nil {
				return err
			}
			for er.Next() {
				var e ExposedAsset
				if err := er.Scan(&e.AssetID, &e.Name, &e.Critical, &e.High, &e.Medium, &e.Low, &e.OpenPorts); err != nil {
					er.Close()
					return err
				}
				out.MostExposed = append(out.MostExposed, e)
			}
			er.Close()
			if err := er.Err(); err != nil {
				return err
			}
		}
		if authz.Has(p.Permissions, authz.ScenarioRead) || authz.Has(p.Permissions, authz.SimulationRead) {
			c := &ChainStats{}
			if authz.Has(p.Permissions, authz.ScenarioRead) {
				if err := tx.QueryRow(ctx, `SELECT count(*) FROM scenarios WHERE tenant_id=$1 AND deleted_at IS NULL`, t).Scan(&c.Scenarios); err != nil {
					return err
				}
			}
			if authz.Has(p.Permissions, authz.SimulationRead) {
				if err := tx.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE status IN ('running','paused')) FROM simulations WHERE tenant_id=$1`, t).Scan(&c.Simulations, &c.Running); err != nil {
					return err
				}
				sr, err := tx.Query(ctx, `SELECT id, number, name, status, started_at, created_at FROM simulations WHERE tenant_id=$1 ORDER BY created_at DESC, id LIMIT 5`, t)
				if err != nil {
					return err
				}
				for sr.Next() {
					var r SimRow
					var n int64
					if err := sr.Scan(&r.ID, &n, &r.Name, &r.Status, &r.StartedAt, &r.CreatedAt); err != nil {
						sr.Close()
						return err
					}
					r.Code = fmt.Sprintf("OP-%04d", n)
					out.LatestSims = append(out.LatestSims, r)
				}
				sr.Close()
				if err := sr.Err(); err != nil {
					return err
				}
			}
			out.Chains = c
		}
		if authz.Has(p.Permissions, authz.AtomicRead) {
			a := &AtomicStats{}
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM atomic_tests WHERE tenant_id=$1 AND deleted_at IS NULL`, t).Scan(&a.Tests); err != nil {
				return err
			}
			if err := tx.QueryRow(ctx, `
SELECT count(*) FILTER (WHERE state IN ('ready','running')),
  count(*) FILTER (WHERE created_at > now() - interval '7 days'),
  count(*) FILTER (WHERE created_at > now() - interval '7 days' AND outcome IN ('error','timeout'))
FROM executions WHERE tenant_id=$1 AND atomic_test_id IS NOT NULL`, t).Scan(&a.Active, &a.Runs7d, &a.Failed7d); err != nil {
				return err
			}
			out.Atomic = a
			rr, err := tx.Query(ctx, `
SELECT e.id, e.atomic_test_id, at.name, c.name, a.name, e.state, e.outcome, e.created_at,
  (SELECT count(*) FROM finding_occurrences o WHERE o.tenant_id=e.tenant_id AND o.execution_id=e.id)
FROM executions e
JOIN atomic_tests at ON at.tenant_id=e.tenant_id AND at.id=e.atomic_test_id AND at.deleted_at IS NULL
JOIN contracts c ON c.tenant_id=e.tenant_id AND c.id=e.contract_id
LEFT JOIN assets a ON a.tenant_id=e.tenant_id AND a.id=e.target_asset_id
WHERE e.tenant_id=$1 ORDER BY e.created_at DESC LIMIT 5`, t)
			if err != nil {
				return err
			}
			for rr.Next() {
				var r RunRow
				if err := rr.Scan(&r.ExecutionID, &r.TestID, &r.TestName, &r.ContractName, &r.Target, &r.State, &r.Outcome, &r.CreatedAt, &r.FindingCount); err != nil {
					rr.Close()
					return err
				}
				out.LatestRuns = append(out.LatestRuns, r)
			}
			rr.Close()
			if err := rr.Err(); err != nil {
				return err
			}
			ir, err := tx.Query(ctx, `SELECT key, name, enabled, last_heartbeat_at FROM integrations WHERE tenant_id=$1 AND kind='injector' ORDER BY name`, t)
			if err != nil {
				return err
			}
			for ir.Next() {
				var r InjectorRow
				var enabled bool
				var hb *time.Time
				if err := ir.Scan(&r.Key, &r.Name, &enabled, &hb); err != nil {
					ir.Close()
					return err
				}
				r.Status = "offline"
				if !enabled {
					r.Status = "disabled"
				} else if hb != nil && time.Since(*hb) < 60*time.Second {
					r.Status = "online"
				}
				out.Injectors = append(out.Injectors, r)
			}
			ir.Close()
			return ir.Err()
		}
		return nil
	})
	return out, err
}

type Handlers struct{ Svc *Service }

func (h *Handlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/home", auth.Require("", func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		s, err := h.Svc.Summary(r.Context(), p)
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, s)
	}))
}
