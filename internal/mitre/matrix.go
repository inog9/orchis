package mitre

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/db"
	"orchis-go/internal/httpx"
)

type Service struct {
	DB       *db.DB
	Importer *Importer
}

type Cell struct {
	ID                string     `json:"id"`
	Name              string     `json:"name"`
	Mapped            bool       `json:"mapped"`    // ada aksi platform yang melatih teknik ini
	Exercised         bool       `json:"exercised"` // aksi itu pernah berhasil dijalankan sungguhan (eksekusi nyata, bukan data demo)
	Simulated         bool       `json:"simulated"` // bukti sintetis tetap terpisah, dapat beririsan dengan mapped/exercised
	SimulatedRuns     int        `json:"simulated_runs"`
	SimulatedFindings int        `json:"simulated_findings"`
	Platforms         []string   `json:"platforms"`
	Runs              int        `json:"runs"`
	LastRunAt         *time.Time `json:"last_run_at"`
	Subs              []Cell     `json:"subs,omitempty"`
}

type Column struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Shortname  string `json:"shortname"`
	Techniques []Cell `json:"techniques"`
}

type Matrix struct {
	Tactics   []Column `json:"tactics"`
	Total     int      `json:"total"`     // teknik induk
	Mapped    int      `json:"mapped"`    // teknik (induk) yang bisa dilatih
	Exercised int      `json:"exercised"` // teknik (induk) yang pernah dilatih
	Simulated int      `json:"simulated"` // teknik induk dengan bukti sintetis, tidak menambah exercised
	Platforms []string `json:"platforms"` // semua platform yang muncul pada teknik aktif (untuk filter)
	Imported  bool     `json:"imported"`
	LastRun   *Run     `json:"last_run"`
}

func (s *Service) lastRun(ctx context.Context) (*Run, error) {
	var r Run
	err := s.DB.Pool.QueryRow(ctx, `SELECT id, status, source_version, techniques, tactics, subtechniques, deprecated, source_sha256, error, started_at, finished_at FROM mitre_import_runs ORDER BY started_at DESC LIMIT 1`).
		Scan(&r.ID, &r.Status, &r.Version, &r.Techniques, &r.Tactics, &r.Subs, &r.Deprecated, &r.SHA256, &r.Error, &r.StartedAt, &r.FinishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &r, err
}

// Matrix menyusun matriks per taktik. Sub-teknik yang berhasil dilatih menandai induknya sebagai dilatih juga.
func (s *Service) Matrix(ctx context.Context, p *auth.Principal) (*Matrix, error) {
	m := &Matrix{Tactics: []Column{}}
	last, err := s.lastRun(ctx)
	if err != nil {
		return nil, err
	}
	m.LastRun = last
	err = s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		stats, err := loadCoverage(ctx, tx, p.TenantID)
		if err != nil {
			return err
		}

		cols, err := tx.Query(ctx, `SELECT id, name, shortname FROM mitre_tactics ORDER BY sort_order, id`)
		if err != nil {
			return err
		}
		idx := map[string]int{}
		for cols.Next() {
			var c Column
			if err := cols.Scan(&c.ID, &c.Name, &c.Shortname); err != nil {
				cols.Close()
				return err
			}
			c.Techniques = []Cell{}
			idx[c.ID] = len(m.Tactics)
			m.Tactics = append(m.Tactics, c)
		}
		cols.Close()
		if err := cols.Err(); err != nil {
			return err
		}
		m.Imported = len(m.Tactics) > 0
		tr, err := tx.Query(ctx, `SELECT id, name, parent_id, tactic_ids, platforms FROM mitre_techniques WHERE NOT is_deprecated ORDER BY id`)
		if err != nil {
			return err
		}
		defer tr.Close()
		subs := map[string][]Cell{}
		allPlatforms := map[string]bool{}
		type parent struct {
			cell    Cell
			tactics []string
		}
		var parents []parent
		for tr.Next() {
			var id, name string
			var par *string
			var tactics, platforms []string
			if err := tr.Scan(&id, &name, &par, &tactics, &platforms); err != nil {
				return err
			}
			c := Cell{ID: id, Name: name, Platforms: platforms}
			applyCoverage(&c, stats[id])
			for _, pl := range platforms {
				allPlatforms[pl] = true
			}
			if par != nil {
				subs[*par] = append(subs[*par], c)
			} else {
				parents = append(parents, parent{c, tactics})
			}
		}
		if err := tr.Err(); err != nil {
			return err
		}
		for _, pr := range parents {
			c := pr.cell
			c.Subs = subs[c.ID]
			m.Total++
			if c.Mapped {
				m.Mapped++
			}
			if c.Exercised {
				m.Exercised++
			}
			if c.Simulated {
				m.Simulated++
			}
			for _, t := range pr.tactics {
				if i, ok := idx[t]; ok {
					m.Tactics[i].Techniques = append(m.Tactics[i].Techniques, c)
				}
			}
		}
		m.Platforms = make([]string, 0, len(allPlatforms))
		for pl := range allPlatforms {
			m.Platforms = append(m.Platforms, pl)
		}
		sort.Strings(m.Platforms)
		return nil
	})
	return m, err
}

// Mapping: satu aksi platform yang dipetakan ke teknik beserta alasannya.
type Mapping struct {
	Action string `json:"action"`
	Reason string `json:"reason"`
	Basis  string `json:"basis"`         // direct | estimate
	Via    string `json:"via,omitempty"` // id sub-teknik bila pemetaan ada di sub-teknik
}

type Detail struct {
	ID                string       `json:"id"`
	Name              string       `json:"name"`
	Description       string       `json:"description"`
	Platforms         []string     `json:"platforms"`
	Tactics           []string     `json:"tactics"`
	URL               string       `json:"url"`
	Actions           []string     `json:"actions"` // nama aksi platform yang melatih teknik ini
	Mappings          []Mapping    `json:"mappings"`
	Mitigations       []Mitigation `json:"mitigations"`
	Parent            *string      `json:"parent"`
	Deprecated        bool         `json:"deprecated"`
	Simulated         bool         `json:"simulated"`
	SimulatedRuns     int          `json:"simulated_runs"`
	SimulatedFindings int          `json:"simulated_findings"`
	Runs              int          `json:"runs"`
	LastRunAt         *time.Time   `json:"last_run_at"`
	Subs              []Cell       `json:"subs"`
}

var idRe = regexp.MustCompile(`^T[0-9]{4}(\.[0-9]{3})?$`)

func (s *Service) Technique(ctx context.Context, p *auth.Principal, id string) (*Detail, error) {
	if !idRe.MatchString(id) {
		return nil, httpx.NotFound("Technique")
	}
	d := &Detail{Actions: []string{}, Subs: []Cell{}, Mappings: []Mapping{}, Mitigations: []Mitigation{}}
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		var url *string
		var mits []byte
		err := tx.QueryRow(ctx, `SELECT t.id, t.name, COALESCE(t.description,''), t.platforms,
  COALESCE((SELECT array_agg(x.name ORDER BY x.sort_order) FROM mitre_tactics x WHERE x.id = ANY(t.tactic_ids)), '{}'),
  t.url, t.mitigations, t.parent_id, t.is_deprecated
FROM mitre_techniques t WHERE t.id=$1`, id).Scan(&d.ID, &d.Name, &d.Description, &d.Platforms, &d.Tactics, &url, &mits, &d.Parent, &d.Deprecated)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound("Technique")
		}
		if err != nil {
			return err
		}
		if err := json.Unmarshal(mits, &d.Mitigations); err != nil {
			return err
		}
		if url != nil {
			d.URL = *url
		} else {
			path := d.ID
			if i := indexDot(d.ID); i > 0 {
				path = d.ID[:i] + "/" + d.ID[i+1:]
			}
			d.URL = "https://attack.mitre.org/techniques/" + path + "/"
		}
		mrows, err := tx.Query(ctx, `SELECT c.name, ct.reason, ct.basis, CASE WHEN ct.technique_id=$2 THEN '' ELSE ct.technique_id END
FROM contract_techniques ct JOIN contracts c ON c.tenant_id=ct.tenant_id AND c.id=ct.contract_id
WHERE ct.tenant_id=$1 AND (ct.technique_id=$2 OR ct.technique_id IN (SELECT id FROM mitre_techniques WHERE parent_id=$2)) ORDER BY 1, 4`, p.TenantID, id)
		if err != nil {
			return err
		}
		for mrows.Next() {
			var mp Mapping
			if err := mrows.Scan(&mp.Action, &mp.Reason, &mp.Basis, &mp.Via); err != nil {
				mrows.Close()
				return err
			}
			d.Mappings = append(d.Mappings, mp)
		}
		mrows.Close()
		if err := mrows.Err(); err != nil {
			return err
		}
		stats, err := loadCoverage(ctx, tx, p.TenantID)
		if err != nil {
			return err
		}
		if st := stats[id]; st != nil {
			d.Runs, d.LastRunAt = st.Runs, st.LastRunAt
			d.SimulatedRuns, d.SimulatedFindings = st.SimulatedRuns, st.SimulatedFindings
			d.Simulated = st.SimulatedRuns > 0 || st.SimulatedFindings > 0
		}
		rows, err := tx.Query(ctx, `SELECT DISTINCT c.name FROM contract_techniques ct JOIN contracts c ON c.tenant_id=ct.tenant_id AND c.id=ct.contract_id
WHERE ct.tenant_id=$1 AND (ct.technique_id=$2 OR ct.technique_id IN (SELECT id FROM mitre_techniques WHERE parent_id=$2)) ORDER BY 1`, p.TenantID, id)
		if err != nil {
			return err
		}
		for rows.Next() {
			var n string
			if err := rows.Scan(&n); err != nil {
				rows.Close()
				return err
			}
			d.Actions = append(d.Actions, n)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		srows, err := tx.Query(ctx, `SELECT id, name, platforms FROM mitre_techniques WHERE parent_id=$1 AND NOT is_deprecated ORDER BY id`, id)
		if err != nil {
			return err
		}
		for srows.Next() {
			c := Cell{}
			if err := srows.Scan(&c.ID, &c.Name, &c.Platforms); err != nil {
				srows.Close()
				return err
			}
			applyCoverage(&c, stats[c.ID])
			d.Subs = append(d.Subs, c)
		}
		srows.Close()
		if err := srows.Err(); err != nil {
			return err
		}
		return nil
	})
	return d, err
}

func indexDot(s string) int {
	for i := range s {
		if s[i] == '.' {
			return i
		}
	}
	return -1
}

type Handlers struct{ Svc *Service }

func (h *Handlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/mitre/matrix", auth.Require(authz.ScenarioRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		m, err := h.Svc.Matrix(r.Context(), p)
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		httpx.JSON(w, 200, m)
	}))
	mux.HandleFunc("GET /api/v1/mitre/techniques/{id}", auth.Require(authz.ScenarioRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		d, err := h.Svc.Technique(r.Context(), p, r.PathValue("id"))
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		httpx.JSON(w, 200, d)
	}))
	mux.HandleFunc("POST /api/v1/mitre/import", auth.Require(authz.SettingsManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		if h.Svc.Importer == nil {
			httpx.Write(w, r, httpx.Conflict("MITRE import is not configured."))
			return
		}
		last, err := h.Svc.lastRun(r.Context())
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		if last != nil && last.Status == "running" && time.Since(last.StartedAt) < staleAfter {
			httpx.Write(w, r, httpx.Conflict("An import is already running."))
			return
		}
		// Berjalan di latar belakang: unduhan puluhan MB tidak boleh menahan permintaan HTTP.
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			if _, err := h.Svc.Importer.Import(ctx); err != nil {
				slog.Warn("impor MITRE manual gagal", "err", err)
			}
		}()
		w.WriteHeader(http.StatusAccepted)
	}))
}
