package recommendations

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"orchis-go/internal/db"
	"orchis-go/internal/httpx"
	"orchis-go/internal/ids"
	"orchis-go/internal/reporting"
)

type Service struct {
	DB     *db.DB
	Tenant uuid.UUID
}
type Job struct {
	ID        uuid.UUID  `json:"id"`
	Status    string     `json:"status"`
	Model     string     `json:"model"`
	Error     string     `json:"error,omitempty"`
	Input     Input      `json:"input"`
	Output    *Output    `json:"output,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	StartedAt *time.Time `json:"started_at,omitempty"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
}
type Claim struct {
	ID    uuid.UUID `json:"id"`
	Token string    `json:"token"`
	Input Input     `json:"input"`
	Model string    `json:"model"`
}
type Completion struct {
	Token  string          `json:"token"`
	Output json.RawMessage `json:"output"`
	Failed bool            `json:"failed"`
}

// Enqueue runs in the report-freezing transaction. A restart cannot enqueue twice.
func Enqueue(ctx context.Context, tx pgx.Tx, tenant, scan uuid.UUID) error {
	var raw []byte
	if err := tx.QueryRow(ctx, `SELECT r.snapshot FROM reports r JOIN assessments a ON a.tenant_id=r.tenant_id AND a.id=r.assessment_id WHERE r.tenant_id=$1 AND r.assessment_id=$2 AND NOT a.is_demo AND NOT r.is_demo`, tenant, scan).Scan(&raw); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	var snap reporting.Snapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		return err
	}
	in := Prepare(snap)
	status := "queued"
	if (snap.Status != "finished" && snap.Status != "incomplete") || len(in.Findings) == 0 {
		status = "not_needed"
	}
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO scan_recommendations(id,tenant_id,scan_id,report_id,status,input,model) SELECT $1,$2,$3,r.id,$4,$5,$6 FROM reports r WHERE r.tenant_id=$2 AND r.assessment_id=$3 ON CONFLICT(tenant_id,scan_id) DO NOTHING`, ids.New(), tenant, scan, status, b, Model)
	return err
}
func hash(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }

func (s *Service) Get(ctx context.Context, tenant, scan uuid.UUID) (*Job, error) {
	var job *Job
	err := s.DB.InTenantRead(ctx, tenant, func(tx pgx.Tx) error {
		var j Job
		var in, out []byte
		err := tx.QueryRow(ctx, `SELECT q.id,q.status,q.model,q.error,q.input,q.output,q.created_at,q.started_at,q.ended_at FROM scan_recommendations q JOIN assessments a ON a.tenant_id=q.tenant_id AND a.id=q.scan_id WHERE q.tenant_id=$1 AND q.scan_id=$2 AND a.results_expired_at IS NULL AND a.report_expired_at IS NULL`, tenant, scan).Scan(&j.ID, &j.Status, &j.Model, &j.Error, &in, &out, &j.CreatedAt, &j.StartedAt, &j.EndedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if err = json.Unmarshal(in, &j.Input); err != nil {
			return err
		}
		if len(out) > 0 {
			if err = json.Unmarshal(out, &j.Output); err != nil {
				return err
			}
		}
		job = &j
		return nil
	})
	return job, err
}
func (s *Service) Claim(ctx context.Context) (*Claim, error) {
	var claim *Claim
	err := s.DB.InTenant(ctx, s.Tenant, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "recommendations:"+s.Tenant.String()); err != nil {
			return err
		}
		// No automatic replay after a model call whose outcome is unknown.
		if _, err := tx.Exec(ctx, `UPDATE scan_recommendations SET status='failed',error='The OpenCode review was interrupted. Scan evidence is still available.',claim_hash=NULL,ended_at=now() WHERE tenant_id=$1 AND status='running' AND lease_until<now()`, s.Tenant); err != nil {
			return err
		}
		var id uuid.UUID
		var input []byte
		err := tx.QueryRow(ctx, `SELECT q.id,q.input FROM scan_recommendations q JOIN assessments a ON a.tenant_id=q.tenant_id AND a.id=q.scan_id JOIN reports r ON r.tenant_id=q.tenant_id AND r.id=q.report_id WHERE q.tenant_id=$1 AND q.status='queued' AND NOT a.is_demo AND NOT r.is_demo AND a.results_expired_at IS NULL AND a.report_expired_at IS NULL AND NOT EXISTS(SELECT 1 FROM scan_recommendations active WHERE active.tenant_id=$1 AND active.status='running') ORDER BY q.created_at,q.id LIMIT 1 FOR UPDATE OF q SKIP LOCKED`, s.Tenant).Scan(&id, &input)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		b := make([]byte, 32)
		if _, err = rand.Read(b); err != nil {
			return err
		}
		token := hex.EncodeToString(b)
		var in Input
		if err = json.Unmarshal(input, &in); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE scan_recommendations SET status='running',claim_hash=$3,lease_until=now()+interval '8 minutes',started_at=now() WHERE tenant_id=$1 AND id=$2`, s.Tenant, id, hash(token)); err != nil {
			return err
		}
		claim = &Claim{ID: id, Token: token, Input: in, Model: Model}
		return nil
	})
	return claim, err
}
func (s *Service) Complete(ctx context.Context, id uuid.UUID, c Completion) error {
	return s.DB.InTenant(ctx, s.Tenant, func(tx pgx.Tx) error {
		var input []byte
		err := tx.QueryRow(ctx, `SELECT q.input FROM scan_recommendations q JOIN assessments a ON a.tenant_id=q.tenant_id AND a.id=q.scan_id WHERE q.tenant_id=$1 AND q.id=$2 AND q.status='running' AND q.claim_hash=$3 AND q.lease_until>now() AND NOT a.is_demo AND a.results_expired_at IS NULL AND a.report_expired_at IS NULL FOR UPDATE OF q`, s.Tenant, id, hash(c.Token)).Scan(&input)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.Conflict("This recommendation job is no longer active.")
		}
		if err != nil {
			return err
		}
		var in Input
		if err = json.Unmarshal(input, &in); err != nil {
			return err
		}
		status, message := "ready", ""
		var output []byte
		if c.Failed {
			status, message = "failed", "OpenCode could not complete this review. Scan evidence is still available."
		} else {
			out, e := Validate(c.Output, in)
			if e != nil {
				status, message = "failed", "OpenCode returned an incomplete or invalid review. Scan evidence is still available."
			} else {
				output, err = json.Marshal(out)
				if err != nil {
					return err
				}
			}
		}
		_, err = tx.Exec(ctx, `UPDATE scan_recommendations SET status=$3,output=$4,error=$5,claim_hash=NULL,lease_until=NULL,ended_at=now() WHERE tenant_id=$1 AND id=$2`, s.Tenant, id, status, output, message)
		return err
	})
}
