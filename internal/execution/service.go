package execution

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"orchis-go/internal/audit"
	"orchis-go/internal/auth"
	"orchis-go/internal/contract"
	"orchis-go/internal/db"
	"orchis-go/internal/httpx"
	"orchis-go/internal/ids"
)

const (
	maxNameLength    = 200
	onlineWindow     = 60 * time.Second // injector dianggap online bila heartbeat lebih baru dari ini
	TopicDispatch    = "exec.dispatch"
	TopicIndexFinder = "index.finding"
)

type Service struct {
	DB    *db.DB
	Guard contract.Guard
}

// SeedCatalog menyemai integrasi injector bawaan dan kontraknya. Aman dijalankan berulang.
func SeedCatalog(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) error {
	integ := []struct{ kind, key, name, desc string }{
		{"injector", "nmap", "Nmap", "Port and service discovery."},
		{"injector", "nuclei", "Nuclei", "Template-based vulnerability scanning."},
		{"executor", "agent", "Agent", "Runs read-only checks on the endpoint itself."},
	}
	for _, i := range integ {
		if _, err := tx.Exec(ctx, `
INSERT INTO integrations (id, tenant_id, kind, key, name, description, is_builtin)
VALUES ($1,$2,$3,$4,$5,$6,true)
ON CONFLICT (tenant_id, key) DO UPDATE SET name = EXCLUDED.name, description = EXCLUDED.description`,
			ids.New(), tenant, i.kind, i.key, i.name, i.desc); err != nil {
			return err
		}
	}
	for _, d := range contract.Defs {
		schema, _ := json.Marshal(d.Schema)
		if _, err := tx.Exec(ctx, `
INSERT INTO contracts (id, tenant_id, integration_id, key, version, name, description, kind, input_schema, output_types, default_timeout_seconds, needs_agent)
VALUES ($1,$2,(SELECT id FROM integrations WHERE tenant_id=$2 AND key=$3),$4,$5,$6,$7,$8,$9,$10,$11,$12)
ON CONFLICT (tenant_id, key, version) DO UPDATE SET name=EXCLUDED.name, description=EXCLUDED.description,
  input_schema=EXCLUDED.input_schema, output_types=EXCLUDED.output_types, default_timeout_seconds=EXCLUDED.default_timeout_seconds, needs_agent=EXCLUDED.needs_agent, updated_at=now()`,
			ids.New(), tenant, d.Integration, d.Key, d.Version, d.Name, d.Description, d.Kind, schema, d.OutputTypes, d.TimeoutSeconds, d.NeedsAgent); err != nil {
			return err
		}
	}
	return SeedPresets(ctx, tx, tenant)
}

func (s *Service) Contracts(ctx context.Context, p *auth.Principal) ([]ContractView, error) {
	out := []ContractView{}
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
SELECT c.key, c.name, COALESCE(c.description,''), c.kind, c.output_types, c.default_timeout_seconds, i.key, i.enabled, i.last_heartbeat_at, c.needs_agent,c.version,c.input_schema
FROM contracts c JOIN integrations i ON i.tenant_id=c.tenant_id AND i.id=c.integration_id
WHERE c.tenant_id=$1 AND c.is_active AND c.version=(SELECT max(c2.version) FROM contracts c2 WHERE c2.tenant_id=c.tenant_id AND c2.key=c.key AND c2.is_active) ORDER BY c.name`, p.TenantID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var v ContractView
			var enabled bool
			if err := rows.Scan(&v.Key, &v.Name, &v.Description, &v.Kind, &v.OutputTypes, &v.TimeoutSeconds, &v.Integration, &enabled, &v.LastHeartbeatAt, &v.NeedsAgent, &v.Version, &v.Schema); err != nil {
				return err
			}
			if def := contract.FindVersion(v.Key, v.Version); def != nil {
				v.Category = def.Category
			}
			v.IntegrationStatus = integrationStatus(enabled, v.LastHeartbeatAt)
			out = append(out, v)
		}
		return rows.Err()
	})
	return out, err
}

func integrationStatus(enabled bool, hb *time.Time) string {
	switch {
	case !enabled:
		return "disabled"
	case hb != nil && time.Since(*hb) < onlineWindow:
		return "online"
	}
	return "offline"
}

type TestInput struct {
	Name          string          `json:"name"`
	ContractKey   string          `json:"contract_key"`
	Params        json.RawMessage `json:"params"`
	TargetAssetID *uuid.UUID      `json:"target_asset_id"`
}

func paramProblem(err error) error {
	var pe *contract.ParamError
	if errors.As(err, &pe) {
		return httpx.Validation(httpx.FieldError{Name: pe.Field, Message: pe.Message})
	}
	return err
}

func validateName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > maxNameLength {
		return "", httpx.Validation(httpx.FieldError{Name: "name", Message: "Enter a name of 1 to 200 characters."})
	}
	return name, nil
}

const testCols = `t.id, t.name, c.key, c.name, t.params, t.target_asset_id, a.name, t.created_at, t.updated_at, t.version`
const testFrom = `FROM atomic_tests t
JOIN contracts c ON c.tenant_id=t.tenant_id AND c.id=t.contract_id
LEFT JOIN assets a ON a.tenant_id=t.tenant_id AND a.id=t.target_asset_id`

func scanTest(row pgx.Row) (AtomicTest, error) {
	var t AtomicTest
	err := row.Scan(&t.ID, &t.Name, &t.ContractKey, &t.ContractName, &t.Params, &t.TargetAssetID, &t.TargetAssetName, &t.CreatedAt, &t.UpdatedAt, &t.Version)
	return t, err
}

func (s *Service) CreateTest(ctx context.Context, p *auth.Principal, in TestInput, ip, reqID string) (*AtomicTest, error) {
	name, err := validateName(in.Name)
	if err != nil {
		return nil, err
	}
	def := contract.Find(in.ContractKey)
	if def == nil {
		return nil, httpx.Validation(httpx.FieldError{Name: "contract_key", Message: "Unknown contract."})
	}
	params, err := contract.Normalize(def.Key, in.Params)
	if err != nil {
		return nil, paramProblem(err)
	}
	id := ids.New()
	var out *AtomicTest
	err = s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		if err := checkTargetExists(ctx, tx, p.TenantID, in.TargetAssetID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO atomic_tests (id, tenant_id, name, contract_id, contract_version, params, target_asset_id, created_by)
VALUES ($1,$2,$3,(SELECT id FROM contracts WHERE tenant_id=$2 AND key=$4 AND version=$5),$5,$6,$7,$8)`,
			id, p.TenantID, name, def.Key, def.Version, params, in.TargetAssetID, p.UserID); err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "atomic_test.create",
			ResourceType: "atomic_test", ResourceID: &id, IP: ip, RequestID: reqID,
			Detail: map[string]any{"name": name, "contract": def.Key}}); err != nil {
			return err
		}
		t, err := scanTest(tx.QueryRow(ctx, `SELECT `+testCols+` `+testFrom+` WHERE t.tenant_id=$1 AND t.id=$2`, p.TenantID, id))
		out = &t
		return err
	})
	return out, err
}

func checkTargetExists(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, asset *uuid.UUID) error {
	if asset == nil {
		return nil
	}
	var ok bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM assets WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL)`, tenant, *asset).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return httpx.Validation(httpx.FieldError{Name: "target_asset_id", Message: "That asset does not exist."})
	}
	return nil
}

const summaryCols = `e.is_demo,e.id, e.state, e.outcome, e.error, e.created_at, e.started_at, e.ended_at,
 (SELECT count(*) FROM finding_occurrences o WHERE o.tenant_id=e.tenant_id AND o.execution_id=e.id)`

func scanSummary(row pgx.Row) (ExecutionSummary, error) {
	var e ExecutionSummary
	err := row.Scan(&e.IsDemo, &e.ID, &e.State, &e.Outcome, &e.Error, &e.CreatedAt, &e.StartedAt, &e.EndedAt, &e.FindingCount)
	return e, err
}

func (s *Service) ListTests(ctx context.Context, p *auth.Principal) ([]AtomicTest, error) {
	out := []AtomicTest{}
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+testCols+` `+testFrom+` WHERE t.tenant_id=$1 AND t.deleted_at IS NULL ORDER BY t.created_at DESC, t.id LIMIT 200`, p.TenantID)
		if err != nil {
			return err
		}
		for rows.Next() {
			t, err := scanTest(rows)
			if err != nil {
				rows.Close()
				return err
			}
			out = append(out, t)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for i := range out {
			e, err := scanSummary(tx.QueryRow(ctx, `SELECT `+summaryCols+` FROM executions e WHERE e.tenant_id=$1 AND e.atomic_test_id=$2 ORDER BY e.created_at DESC LIMIT 1`, p.TenantID, out[i].ID))
			if err == nil {
				out[i].LastExecution = &e
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		}
		return nil
	})
	return out, err
}

func (s *Service) GetTest(ctx context.Context, p *auth.Principal, id uuid.UUID) (*AtomicTest, error) {
	var out *AtomicTest
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		t, err := scanTest(tx.QueryRow(ctx, `SELECT `+testCols+` `+testFrom+` WHERE t.tenant_id=$1 AND t.id=$2 AND t.deleted_at IS NULL`, p.TenantID, id))
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound("Atomic test")
		}
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT `+summaryCols+` FROM executions e WHERE e.tenant_id=$1 AND e.atomic_test_id=$2 ORDER BY e.created_at DESC LIMIT 20`, p.TenantID, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		t.Executions = []ExecutionSummary{}
		for rows.Next() {
			e, err := scanSummary(rows)
			if err != nil {
				return err
			}
			t.Executions = append(t.Executions, e)
		}
		if len(t.Executions) > 0 {
			t.LastExecution = &t.Executions[0]
		}
		out = &t
		return rows.Err()
	})
	return out, err
}

type TestPatch struct {
	Version       int             `json:"version"`
	Name          *string         `json:"name"`
	Params        json.RawMessage `json:"params"`
	TargetAssetID *uuid.UUID      `json:"target_asset_id"`
}

func (s *Service) PatchTest(ctx context.Context, p *auth.Principal, id uuid.UUID, in TestPatch, ip, reqID string) (*AtomicTest, error) {
	if in.Version < 1 {
		return nil, httpx.Validation(httpx.FieldError{Name: "version", Message: "Send the version you loaded."})
	}
	var out *AtomicTest
	err := s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		cur, err := scanTest(tx.QueryRow(ctx, `SELECT `+testCols+` `+testFrom+` WHERE t.tenant_id=$1 AND t.id=$2 AND t.deleted_at IS NULL FOR UPDATE OF t`, p.TenantID, id))
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound("Atomic test")
		}
		if err != nil {
			return err
		}
		if cur.Version != in.Version {
			return httpx.Conflict("This test was changed by someone else. Reload and try again.")
		}
		name, params, target := cur.Name, json.RawMessage(cur.Params), cur.TargetAssetID
		if in.Name != nil {
			if name, err = validateName(*in.Name); err != nil {
				return err
			}
		}
		if len(in.Params) > 0 {
			if params, err = contract.Normalize(cur.ContractKey, in.Params); err != nil {
				return paramProblem(err)
			}
		}
		if in.TargetAssetID != nil {
			if err := checkTargetExists(ctx, tx, p.TenantID, in.TargetAssetID); err != nil {
				return err
			}
			target = in.TargetAssetID
		}
		if _, err := tx.Exec(ctx, `UPDATE atomic_tests SET name=$3, params=$4, target_asset_id=$5, version=version+1, updated_at=now() WHERE tenant_id=$1 AND id=$2`,
			p.TenantID, id, name, params, target); err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "atomic_test.update",
			ResourceType: "atomic_test", ResourceID: &id, IP: ip, RequestID: reqID}); err != nil {
			return err
		}
		t, err := scanTest(tx.QueryRow(ctx, `SELECT `+testCols+` `+testFrom+` WHERE t.tenant_id=$1 AND t.id=$2`, p.TenantID, id))
		out = &t
		return err
	})
	return out, err
}

func (s *Service) DeleteTest(ctx context.Context, p *auth.Principal, id uuid.UUID, ip, reqID string) error {
	return s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE atomic_tests SET deleted_at=now(), version=version+1 WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL`, p.TenantID, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return httpx.NotFound("Atomic test")
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "atomic_test.delete",
			ResourceType: "atomic_test", ResourceID: &id, IP: ip, RequestID: reqID})
	})
}

// loadAddresses mengambil alamat sebuah aset untuk penjaga scope.
func loadAddresses(ctx context.Context, tx pgx.Tx, tenant, asset uuid.UUID) ([]contract.Address, error) {
	rows, err := tx.Query(ctx, `SELECT kind, value, is_primary FROM asset_addresses WHERE tenant_id=$1 AND asset_id=$2 ORDER BY is_primary DESC, created_at`, tenant, asset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []contract.Address
	for rows.Next() {
		var a contract.Address
		if err := rows.Scan(&a.Kind, &a.Value, &a.IsPrimary); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Run membuat satu eksekusi dan pesan dispatch-nya dalam SATU transaksi (pola outbox).
// Penjaga scope: aset harus ada, belum dihapus, dan ditandai boleh diuji; alamatnya harus lolos Guard.
func (s *Service) Run(ctx context.Context, p *auth.Principal, testID uuid.UUID, ip, reqID string) (*ExecutionDetail, error) {
	execID := ids.New()
	err := s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error { return s.QueueTest(ctx, tx, p, testID, execID, ip, reqID) })
	if err != nil {
		return nil, err
	}
	return s.GetExecution(ctx, p, execID)
}

// QueueTest memakai guard Run yang sama dalam transaksi pemanggil; dipakai persetujuan rencana AI atomik.
func (s *Service) QueueTest(ctx context.Context, tx pgx.Tx, p *auth.Principal, testID, execID uuid.UUID, ip, reqID string) error {
	var (
		ckey         string
		contractID   uuid.UUID
		contractVer  int
		timeoutSec   int
		params       json.RawMessage
		target       *uuid.UUID
		integID      uuid.UUID
		integKey     string
		integEnabled bool
	)
	err := tx.QueryRow(ctx, `
SELECT c.key, c.id, c.version, c.default_timeout_seconds, t.params, t.target_asset_id, i.id, i.key, i.enabled
FROM atomic_tests t
JOIN contracts c ON c.tenant_id=t.tenant_id AND c.id=t.contract_id
JOIN integrations i ON i.tenant_id=c.tenant_id AND i.id=c.integration_id
WHERE t.tenant_id=$1 AND t.id=$2 AND t.deleted_at IS NULL FOR UPDATE OF t`, p.TenantID, testID).
		Scan(&ckey, &contractID, &contractVer, &timeoutSec, &params, &target, &integID, &integKey, &integEnabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return httpx.NotFound("Atomic test")
	}
	if err != nil {
		return err
	}
	if target == nil {
		return httpx.Validation(httpx.FieldError{Name: "target_asset_id", Message: "Choose a target asset first."})
	}
	if !integEnabled {
		return httpx.Conflict("The " + integKey + " injector is disabled.")
	}
	var authorized bool
	err = tx.QueryRow(ctx, `SELECT testing_authorized FROM assets WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL`, p.TenantID, *target).Scan(&authorized)
	if errors.Is(err, pgx.ErrNoRows) {
		return httpx.Validation(httpx.FieldError{Name: "target_asset_id", Message: "The target asset no longer exists."})
	}
	if err != nil {
		return err
	}
	if !authorized {
		return httpx.Validation(httpx.FieldError{Name: "target_asset_id", Message: "This asset is not authorized for testing. An administrator must authorize it first."})
	}
	def := contract.FindVersion(ckey, contractVer)
	execKind, addr := "injector", ""
	var agentID *uuid.UUID
	if def != nil && def.NeedsAgent {
		// Aksi agent berjalan di endpoint: butuh agent yang terpasang dan hidup, bukan alamat jaringan.
		var aid uuid.UUID
		var live bool
		err := tx.QueryRow(ctx, `SELECT id, COALESCE(last_heartbeat_at > now() - interval '90 seconds', false) FROM agents
WHERE tenant_id=$1 AND asset_id=$2 AND NOT is_demo AND status<>'revoked' ORDER BY last_heartbeat_at DESC NULLS LAST LIMIT 1`, p.TenantID, *target).Scan(&aid, &live)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.Validation(httpx.FieldError{Name: "target_asset_id", Message: "This asset has no agent. Install one from Assets → Agents first."})
		}
		if err != nil {
			return err
		}
		if !live {
			return httpx.Conflict("The agent on this asset is offline. Start it and try again.")
		}
		execKind, addr, agentID = "agent", "local", &aid
	} else {
		addrs, err := loadAddresses(ctx, tx, p.TenantID, *target)
		if err != nil {
			return err
		}
		if addr, err = s.Guard.Target(ckey, addrs); err != nil {
			return httpx.Validation(httpx.FieldError{Name: "target_asset_id", Message: "Target refused: " + err.Error() + "."})
		}
	}
	var busy bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM executions WHERE tenant_id=$1 AND atomic_test_id=$2 AND state IN ('ready','running'))`, p.TenantID, testID).Scan(&busy); err != nil {
		return err
	}
	if busy {
		return httpx.Conflict("This test is already queued or running.")
	}
	sum := sha256.Sum256([]byte(testID.String() + "|" + string(params) + "|" + addr + "|" + execID.String()))
	if _, err := tx.Exec(ctx, `
INSERT INTO executions (id, tenant_id, atomic_test_id, contract_id, contract_version, target_asset_id, target_address,
  executor_kind, integration_id, params, input_hash, agent_id)
VALUES ($1,$2,$3,$4,$5,$6,$7,$11,$8,$9,$10,$12)`,
		execID, p.TenantID, testID, contractID, contractVer, *target, addr, integID, params, sum[:], execKind, agentID); err != nil {
		return err
	}
	if execKind == "agent" {
		// Agent mengambil tugasnya sendiri (polling); tidak ada pesan antrean.
		if err := logLine(ctx, tx, p.TenantID, execID, "info", "Waiting for the agent to pick this up."); err != nil {
			return err
		}
	} else {
		if err := Enqueue(ctx, tx, p.TenantID, TopicDispatch, "exec."+integKey, "execution", execID,
			map[string]any{"execution_id": execID, "tenant_id": p.TenantID, "integration": integKey}); err != nil {
			return err
		}
		if err := logLine(ctx, tx, p.TenantID, execID, "info", "Queued for the "+integKey+" injector."); err != nil {
			return err
		}
	}
	return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "atomic_test.run",
		ResourceType: "execution", ResourceID: &execID, IP: ip, RequestID: reqID,
		Detail: map[string]any{"atomic_test_id": testID, "contract": ckey, "target": addr, "target_asset_id": *target}})
}

// Enqueue menulis satu pesan outbox dalam transaksi pemanggil. routingKey disimpan di header.
func Enqueue(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, topic, routingKey, aggType string, aggID uuid.UUID, payload any) error {
	return EnqueueAt(ctx, tx, tenant, topic, routingKey, aggType, aggID, payload, time.Now())
}

// EnqueueAt seperti Enqueue, tetapi relay baru menerbitkannya pada waktu availableAt (dipakai untuk jeda antar langkah chain).
func EnqueueAt(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, topic, routingKey, aggType string, aggID uuid.UUID, payload any, availableAt time.Time) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	hdr, _ := json.Marshal(map[string]string{"routing_key": routingKey})
	_, err = tx.Exec(ctx, `INSERT INTO outbox (message_id, tenant_id, topic, aggregate_type, aggregate_id, payload, headers, available_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, ids.New(), tenant, topic, aggType, aggID, body, hdr, availableAt)
	return err
}

func logLine(ctx context.Context, tx pgx.Tx, tenant, exec uuid.UUID, level, msg string) error {
	if len(msg) > 2000 {
		msg = msg[:2000]
	}
	_, err := tx.Exec(ctx, `INSERT INTO execution_logs (tenant_id, execution_id, level, message) VALUES ($1,$2,$3,$4)`, tenant, exec, level, msg)
	return err
}

// Cancel menghentikan eksekusi yang masih ready/running. Injector mengetahuinya dari heartbeat berikutnya.
func (s *Service) Cancel(ctx context.Context, p *auth.Principal, id uuid.UUID, ip, reqID string) (*ExecutionDetail, error) {
	err := s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
UPDATE executions SET state='ended', outcome='canceled', ended_at=now(), lease_until=NULL, version=version+1, updated_at=now()
WHERE tenant_id=$1 AND id=$2 AND state IN ('ready','running')`, p.TenantID, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM executions WHERE tenant_id=$1 AND id=$2)`, p.TenantID, id).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return httpx.NotFound("Execution")
			}
			return httpx.Conflict("This execution has already finished.")
		}
		if err := logLine(ctx, tx, p.TenantID, id, "warn", "Canceled by "+p.Email+"."); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "execution.cancel",
			ResourceType: "execution", ResourceID: &id, IP: ip, RequestID: reqID})
	})
	if err != nil {
		return nil, err
	}
	return s.GetExecution(ctx, p, id)
}

func (s *Service) GetExecution(ctx context.Context, p *auth.Principal, id uuid.UUID) (*ExecutionDetail, error) {
	var out *ExecutionDetail
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		var d ExecutionDetail
		err := tx.QueryRow(ctx, `
SELECT `+summaryCols+`, e.atomic_test_id, c.key, e.target_asset_id, e.target_address, e.params, e.exit_code, e.raw_output_key
FROM executions e JOIN contracts c ON c.tenant_id=e.tenant_id AND c.id=e.contract_id
WHERE e.tenant_id=$1 AND e.id=$2`, p.TenantID, id).
			Scan(&d.IsDemo, &d.ID, &d.State, &d.Outcome, &d.Error, &d.CreatedAt, &d.StartedAt, &d.EndedAt, &d.FindingCount,
				&d.AtomicTestID, &d.ContractKey, &d.TargetAssetID, &d.TargetAddress, &d.Params, &d.ExitCode, &d.RawOutputKey)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound("Execution")
		}
		if err != nil {
			return err
		}
		d.Logs = []LogLine{}
		rows, err := tx.Query(ctx, `SELECT at, level, message FROM execution_logs WHERE tenant_id=$1 AND execution_id=$2 ORDER BY id LIMIT 500`, p.TenantID, id)
		if err != nil {
			return err
		}
		for rows.Next() {
			var l LogLine
			if err := rows.Scan(&l.At, &l.Level, &l.Message); err != nil {
				rows.Close()
				return err
			}
			d.Logs = append(d.Logs, l)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		d.Findings, err = queryFindings(ctx, tx, p.TenantID, `JOIN finding_occurrences o ON o.tenant_id=f.tenant_id AND o.finding_id=f.id AND o.execution_id=$2 WHERE f.tenant_id=$1`, []any{p.TenantID, id}, "ORDER BY "+severityRank+" DESC, f.value LIMIT 1000")
		out = &d
		return err
	})
	return out, err
}
