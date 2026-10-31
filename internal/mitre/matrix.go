package mitre

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
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
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Mapped    bool       `json:"mapped"`    // ada aksi platform yang melatih teknik ini
	Exercised bool       `json:"exercised"` // aksi itu pernah berhasil dijalankan
	Runs      int        `json:"runs"`
	LastRunAt *time.Time `json:"last_run_at"`
	Subs      []Cell     `json:"subs,omitempty"`
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
	Imported  bool     `json:"imported"`
	LastRun   *Run     `json:"last_run"`
}

func (s *Service) lastRun(ctx context.Context) (*Run, error) {
	var r Run
	err := s.DB.Pool.QueryRow(ctx, `SELECT id, status, source_version, techniques, tactics, error, started_at, finished_at FROM mitre_import_runs ORDER BY started_at DESC LIMIT 1`).
		Scan(&r.ID, &r.Status, &r.Version, &r.Techniques, &r.Tactics, &r.Error, &r.StartedAt, &r.FinishedAt)
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
		// teknik yang dipetakan ke aksi, dan jumlah/waktu eksekusi sukses per teknik
		type stat struct {
			mapped bool
			runs   int
			last   *time.Time
		}
		stats := map[string]*stat{}
		mr, err := tx.Query(ctx, `SELECT technique_id FROM contract_techniques WHERE tenant_id=$1`, p.TenantID)
		if err != nil {
			return err
		}
		for mr.Next() {
			var id string
			if err := mr.Scan(&id); err != nil {
				mr.Close()
				return err
			}
			stats[id] = &stat{mapped: true}
		}
		mr.Close()
		if err := mr.Err(); err != nil {
			return err
		}
		er, err := tx.Query(ctx, `
SELECT ct.technique_id, count(*), max(e.ended_at) FROM executions e
JOIN contract_techniques ct ON ct.tenant_id=e.tenant_id AND ct.contract_id=e.contract_id
WHERE e.tenant_id=$1 AND e.state='ended' AND e.outcome='success' GROUP BY 1`, p.TenantID)
		if err != nil {
			return err
		}
		for er.Next() {
			var id string
			var n int
			var at *time.Time
			if err := er.Scan(&id, &n, &at); err != nil {
				er.Close()
				return err
			}
			if stats[id] == nil {
				stats[id] = &stat{mapped: true}
			}
			stats[id].runs, stats[id].last = n, at
		}
		er.Close()
		if err := er.Err(); err != nil {
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
		tr, err := tx.Query(ctx, `SELECT id, name, parent_id, tactic_ids FROM mitre_techniques WHERE NOT is_deprecated ORDER BY id`)
		if err != nil {
			return err
		}
		defer tr.Close()
		subs := map[string][]Cell{}
		type parent struct {
			cell    Cell
			tactics []string
		}
		var parents []parent
		for tr.Next() {
			var id, name string
			var par *string
			var tactics []string
			if err := tr.Scan(&id, &name, &par, &tactics); err != nil {
				return err
			}
			c := Cell{ID: id, Name: name}
			if st := stats[id]; st != nil {
				c.Mapped, c.Runs, c.LastRunAt, c.Exercised = st.mapped, st.runs, st.last, st.runs > 0
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
			for _, sc := range c.Subs {
				c.Mapped = c.Mapped || sc.Mapped
				c.Exercised = c.Exercised || sc.Exercised
				c.Runs += sc.Runs
				if sc.LastRunAt != nil && (c.LastRunAt == nil || sc.LastRunAt.After(*c.LastRunAt)) {
					c.LastRunAt = sc.LastRunAt
				}
			}
			m.Total++
			if c.Mapped {
				m.Mapped++
			}
			if c.Exercised {
				m.Exercised++
			}
			for _, t := range pr.tactics {
				if i, ok := idx[t]; ok {
					m.Tactics[i].Techniques = append(m.Tactics[i].Techniques, c)
				}
			}
		}
		return nil
	})
	return m, err
}

type Detail struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Platforms   []string   `json:"platforms"`
	Tactics     []string   `json:"tactics"`
	URL         string     `json:"url"`
	Actions     []string   `json:"actions"` // nama aksi platform yang melatih teknik ini
	Runs        int        `json:"runs"`
	LastRunAt   *time.Time `json:"last_run_at"`
	Subs        []Cell     `json:"subs"`
}

var idRe = regexp.MustCompile(`^T[0-9]{4}(\.[0-9]{3})?$`)

func (s *Service) Technique(ctx context.Context, p *auth.Principal, id string) (*Detail, error) {
	if !idRe.MatchString(id) {
		return nil, httpx.NotFound("Technique")
	}
	d := &Detail{Actions: []string{}, Subs: []Cell{}}
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT t.id, t.name, COALESCE(t.description,''), t.platforms,
  COALESCE((SELECT array_agg(x.name ORDER BY x.sort_order) FROM mitre_tactics x WHERE x.id = ANY(t.tactic_ids)), '{}')
FROM mitre_techniques t WHERE t.id=$1`, id).Scan(&d.ID, &d.Name, &d.Description, &d.Platforms, &d.Tactics)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound("Technique")
		}
		if err != nil {
			return err
		}
		path := d.ID
		if i := indexDot(d.ID); i > 0 {
			path = d.ID[:i] + "/" + d.ID[i+1:]
		}
		d.URL = "https://attack.mitre.org/techniques/" + path + "/"
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
		return tx.QueryRow(ctx, `
SELECT count(*), max(e.ended_at) FROM executions e JOIN contract_techniques ct ON ct.tenant_id=e.tenant_id AND ct.contract_id=e.contract_id
WHERE e.tenant_id=$1 AND e.state='ended' AND e.outcome='success' AND (ct.technique_id=$2 OR ct.technique_id IN (SELECT id FROM mitre_techniques WHERE parent_id=$2))`, p.TenantID, id).Scan(&d.Runs, &d.LastRunAt)
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
