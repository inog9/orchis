package execution

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"net/http"
	"orchis-go/internal/audit"
	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/contract"
	"orchis-go/internal/httpx"
	"orchis-go/internal/ids"
)

type Preset struct {
	ID              uuid.UUID       `json:"id"`
	Name            string          `json:"name"`
	ContractKey     string          `json:"contract_key"`
	ContractVersion int             `json:"contract_version"`
	Params          json.RawMessage `json:"params"`
	Version         int             `json:"version"`
}

func SeedPresets(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) error {
	for _, p := range []struct{ name, key, params string }{{"Quick exposure check", contract.KeyNmapTCP, `{"profile":"top100","service_detection":true}`}, {"Full TCP + services", contract.KeyNmapTCP, `{"profile":"custom","ports":"1-65535","service_detection":true}`}, {"Web CVEs", contract.KeyNucleiVS, `{"severity":["medium","high","critical"],"tags":["cve"],"template_profile":"cves"}`}} {
		params, e := contract.Normalize(p.key, json.RawMessage(p.params))
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO action_presets(id,tenant_id,name,contract_key,contract_version,params) VALUES($1,$2,$3,$4,1,$5) ON CONFLICT(tenant_id,name) DO NOTHING`, ids.New(), tenant, p.name, p.key, []byte(params)); e != nil {
			return e
		}
	}
	return nil
}
func (h *Handlers) registerPresets(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/action-presets", auth.Require(authz.AtomicRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		items := []Preset{}
		e := h.Svc.DB.InTenantRead(r.Context(), p.TenantID, func(tx pgx.Tx) error {
			rows, e := tx.Query(r.Context(), `SELECT id,name,contract_key,contract_version,params,version FROM action_presets WHERE tenant_id=$1 ORDER BY name LIMIT 200`, p.TenantID)
			if e != nil {
				return e
			}
			defer rows.Close()
			for rows.Next() {
				var v Preset
				if e = rows.Scan(&v.ID, &v.Name, &v.ContractKey, &v.ContractVersion, &v.Params, &v.Version); e != nil {
					return e
				}
				items = append(items, v)
			}
			return rows.Err()
		})
		if e != nil {
			httpx.Write(w, r, e)
			return
		}
		httpx.JSON(w, 200, map[string]any{"items": items})
	}))
	m.HandleFunc("POST /api/v1/action-presets", auth.Require(authz.AtomicWrite, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		var in struct {
			Name   string          `json:"name"`
			Key    string          `json:"contract_key"`
			Params json.RawMessage `json:"params"`
		}
		if e := httpx.Decode(w, r, &in); e != nil {
			httpx.Write(w, r, e)
			return
		}
		name, e := validateName(in.Name)
		if e != nil {
			httpx.Write(w, r, e)
			return
		}
		def := contract.Find(in.Key)
		if def == nil {
			httpx.Write(w, r, httpx.BadRequest("Unknown action."))
			return
		}
		params, e := contract.Normalize(in.Key, in.Params)
		if e != nil {
			httpx.Write(w, r, paramProblem(e))
			return
		}
		v := Preset{ID: ids.New(), Name: name, ContractKey: in.Key, ContractVersion: def.Version, Params: params, Version: 1}
		e = h.Svc.DB.InTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
			_, e := tx.Exec(r.Context(), `INSERT INTO action_presets(id,tenant_id,name,contract_key,contract_version,params) VALUES($1,$2,$3,$4,$5,$6)`, v.ID, p.TenantID, v.Name, v.ContractKey, v.ContractVersion, []byte(v.Params))
			if e != nil {
				var pe *pgconn.PgError
				if errors.As(e, &pe) && pe.Code == "23505" {
					return httpx.Conflict("A preset with this name already exists.")
				}
				return e
			}
			return audit.Write(r.Context(), tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "action_preset.create", ResourceType: "action_preset", ResourceID: &v.ID, IP: auth.ClientIP(r), RequestID: httpx.RequestIDFrom(r.Context())})
		})
		if e != nil {
			httpx.Write(w, r, e)
			return
		}
		httpx.JSON(w, 201, v)
	}))
}
