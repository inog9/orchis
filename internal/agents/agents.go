// Package agents: pendaftaran agent (token sekali pakai), kredensial per agent, heartbeat, rilis, dan pencabutan.
// Agent hanya melakukan koneksi KELUAR ke server (polling); server tidak pernah menghubungi endpoint.
package agents

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/netip"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"orchis-go/internal/agentproto"
	"orchis-go/internal/alerts"
	"orchis-go/internal/audit"
	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/db"
	"orchis-go/internal/execution"
	"orchis-go/internal/httpx"
	"orchis-go/internal/ids"
)

const (
	EnrollTTL    = 60 * time.Minute
	OnlineWindow = 90 * time.Second
	secretPrefix = "oag_"
	tokenPrefix  = "oen_"
	PollSeconds  = 5
)

type Service struct {
	DB      *db.DB
	Tenant  uuid.UUID
	Release *Release
	// AfterEnd dipanggil dalam transaksi yang sama saat eksekusi agent berakhir karena pencabutan (agar chain maju).
	AfterEnd func(ctx context.Context, tx pgx.Tx, tenant, simulation uuid.UUID) error
	Now      func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func newSecret(prefix string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(b), nil
}

func fe(name, msg string) error { return httpx.Validation(httpx.FieldError{Name: name, Message: msg}) }

var (
	osSet   = map[string]bool{"windows": true, "linux": true, "macos": true}
	archSet = map[string]bool{"x86_64": true, "arm64": true}
	modeSet = map[string]bool{"service": true, "user": true}
)

// ---------- token pendaftaran ----------

type EnrollmentToken struct {
	ID        uuid.UUID `json:"id"`
	Token     string    `json:"token"` // hanya tampil sekali
	OS        string    `json:"os"`
	Arch      string    `json:"arch"`
	Mode      string    `json:"mode"`
	ExpiresAt time.Time `json:"expires_at"`
	Version   string    `json:"version"`   // versi agent yang akan terpasang
	SHA256    string    `json:"sha256"`    // untuk diverifikasi pemasang sebelum menjalankan biner
	FileName  string    `json:"file_name"` // nama berkas unduhan yang disarankan
}

type TokenInput struct {
	OrganizationID *uuid.UUID `json:"organization_id"`
	OS             string     `json:"os"`
	Arch           string     `json:"arch"`
	Mode           string     `json:"mode"`
}

func (s *Service) CreateEnrollmentToken(ctx context.Context, p *auth.Principal, in TokenInput, ip, reqID string) (*EnrollmentToken, error) {
	var errs []httpx.FieldError
	if !osSet[in.OS] {
		errs = append(errs, httpx.FieldError{Name: "os", Message: "Choose Windows, Linux or macOS."})
	}
	if !archSet[in.Arch] {
		errs = append(errs, httpx.FieldError{Name: "arch", Message: "Choose x86_64 or arm64."})
	}
	if !modeSet[in.Mode] {
		errs = append(errs, httpx.FieldError{Name: "mode", Message: "Choose service or user."})
	}
	if len(errs) > 0 {
		return nil, httpx.Validation(errs...)
	}
	m, err := s.Release.Manifest()
	if err != nil {
		return nil, httpx.Conflict("This server has no agent release to install. An administrator must publish one first.")
	}
	tgt, ok := m.Find(in.OS, in.Arch)
	if !ok {
		return nil, httpx.Conflict("There is no agent build for that platform yet.")
	}
	tok, err := newSecret(tokenPrefix)
	if err != nil {
		return nil, err
	}
	id := ids.New()
	exp := s.now().Add(EnrollTTL)
	err = s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		if in.OrganizationID != nil {
			if !authz.Has(p.Permissions, authz.OrganizationWrite) {
				return httpx.Forbidden(authz.OrganizationWrite)
			}
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM organizations WHERE tenant_id=$1 AND id=$2)`, p.TenantID, in.OrganizationID).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return fe("organization_id", "Organization does not exist.")
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO agent_enrollment_tokens (id, tenant_id, token_hash, os, arch, mode, created_by, expires_at,organization_id) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
			id, p.TenantID, auth.HashToken(tok), in.OS, in.Arch, in.Mode, p.UserID, exp, in.OrganizationID); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "agent.enrollment_token", ResourceType: "agent_enrollment_token", ResourceID: &id, IP: ip, RequestID: reqID,
			Detail: map[string]any{"os": in.OS, "arch": in.Arch, "mode": in.Mode}})
	})
	if err != nil {
		return nil, err
	}
	return &EnrollmentToken{ID: id, Token: tok, OS: in.OS, Arch: in.Arch, Mode: in.Mode, ExpiresAt: exp, Version: m.Version, SHA256: tgt.SHA256, FileName: tgt.File}, nil
}

type TokenStatus struct {
	Used    bool       `json:"used"`
	Expired bool       `json:"expired"`
	AgentID *uuid.UUID `json:"agent_id"`
	Agent   *Agent     `json:"agent"`
}

func (s *Service) EnrollmentTokenStatus(ctx context.Context, p *auth.Principal, id uuid.UUID) (*TokenStatus, error) {
	out := &TokenStatus{}
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		var used *time.Time
		var exp time.Time
		err := tx.QueryRow(ctx, `SELECT used_at, expires_at, agent_id FROM agent_enrollment_tokens WHERE tenant_id=$1 AND id=$2`, p.TenantID, id).Scan(&used, &exp, &out.AgentID)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound("Enrollment token")
		}
		if err != nil {
			return err
		}
		out.Used, out.Expired = used != nil, used == nil && exp.Before(s.now())
		if out.AgentID != nil {
			list, err := s.list(ctx, tx, p.TenantID, out.AgentID)
			if err != nil {
				return err
			}
			if len(list) == 1 {
				out.Agent = &list[0]
			}
		}
		return nil
	})
	return out, err
}

// ---------- pendaftaran (publik, dengan token) ----------

type EnrollInput struct {
	Name            string `json:"name"`
	Token           string `json:"token"`
	Hostname        string `json:"hostname"`
	OS              string `json:"os"`
	Arch            string `json:"arch"`
	Version         string `json:"version"`
	ProtocolVersion int    `json:"protocol_version"`
}

type Enrolled struct {
	AgentID     uuid.UUID `json:"agent_id"`
	Secret      string    `json:"secret"`
	Mode        string    `json:"mode"` // service | user (ditentukan token, bukan agent)
	PollSeconds int       `json:"poll_seconds"`
}

var hostRe = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9._-]{0,251}[A-Za-z0-9])?$`)

func errBadToken() error {
	return httpx.NewProblem(400, "Enrollment failed", "This enrollment token is invalid, already used, or expired. Create a new one in the console.")
}

// Enroll memakai token SEKALI (UPDATE bersyarat atomik), membuat/memakai ulang aset, lalu menerbitkan kredensial per agent.
func (s *Service) Enroll(ctx context.Context, in EnrollInput, ip string) (*Enrolled, error) {
	in.Hostname = strings.TrimSpace(in.Hostname)
	in.Name = strings.TrimSpace(in.Name)
	if in.Name != "" && !regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,39}$`).MatchString(in.Name) {
		return nil, fe("name", "Use 1 to 40 lowercase letters, digits, underscores or hyphens.")
	}
	switch {
	case !osSet[in.OS] || !archSet[in.Arch]:
		return nil, fe("os", "Unsupported platform.")
	case !agentproto.ValidVersion(in.Version):
		return nil, fe("version", "Invalid agent version.")
	case in.Hostname == "" || !hostRe.MatchString(in.Hostname):
		return nil, fe("hostname", "The host name is empty or not valid.")
	case in.ProtocolVersion != agentproto.ProtocolVersion:
		return nil, httpx.NewProblem(426, "Agent too old", "This agent speaks a different protocol version. Download the current agent from the console.")
	}
	secret, err := newSecret(secretPrefix)
	if err != nil {
		return nil, err
	}
	var out *Enrolled
	err = s.DB.InTenant(ctx, s.Tenant, func(tx pgx.Tx) error {
		var tokID, createdBy uuid.UUID
		var org *uuid.UUID
		var tos, tarch, mode string
		err := tx.QueryRow(ctx, `UPDATE agent_enrollment_tokens SET used_at=now()
WHERE tenant_id=$1 AND token_hash=$2 AND used_at IS NULL AND expires_at > $3 RETURNING id, os, arch, mode, created_by,organization_id`,
			s.Tenant, auth.HashToken(in.Token), s.now()).Scan(&tokID, &tos, &tarch, &mode, &createdBy, &org)
		if errors.Is(err, pgx.ErrNoRows) {
			return errBadToken()
		}
		if err != nil {
			return err
		}
		if tos != in.OS || tarch != in.Arch {
			// Transaksi dibatalkan: token TIDAK terpakai, admin dapat memasang di mesin yang benar.
			return httpx.NewProblem(400, "Enrollment failed", "This token was created for "+tos+" "+tarch+", but this agent runs on "+in.OS+" "+in.Arch+".")
		}
		// Pasang ulang di mesin yang sama memakai kembali asetnya (riwayat temuan tetap), selama belum ada agent aktif.
		var assetID uuid.UUID
		assetName := in.Hostname
		if in.Name != "" {
			assetName = in.Hostname + " / " + in.Name
		}
		if in.Name != "" && org != nil {
			var used bool
			if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agents WHERE tenant_id=$1 AND organization_id=$2 AND instance_name=$3 AND status<>'revoked')`, s.Tenant, org, in.Name).Scan(&used); e != nil {
				return e
			}
			if used {
				return httpx.Conflict("This agent instance name is already in use.")
			}
		}
		err = tx.QueryRow(ctx, `SELECT a.id FROM assets a WHERE a.tenant_id=$1 AND a.deleted_at IS NULL AND a.source='agent' AND a.name=$2 AND a.platform=$3 AND a.arch=$4 AND a.organization_id IS NOT DISTINCT FROM $6::uuid
AND NOT EXISTS (SELECT 1 FROM agents g WHERE g.tenant_id=a.tenant_id AND g.asset_id=a.id AND g.status<>'revoked' AND g.mode=$5)
ORDER BY a.created_at LIMIT 1`, s.Tenant, assetName, in.OS, in.Arch, mode, org).Scan(&assetID)
		if errors.Is(err, pgx.ErrNoRows) {
			assetID = ids.New()
			if _, err := tx.Exec(ctx, `INSERT INTO assets (id, tenant_id, category, name, platform, arch, source, created_by,organization_id) VALUES ($1,$2,'host',$3,$4,$5,'agent',$6,$7)`,
				assetID, s.Tenant, assetName, in.OS, in.Arch, createdBy, org); err != nil {
				return err
			}
			// Hostname hanya identitas agent; alamat scan harus ditetapkan admin pada aset.

		} else if err != nil {
			return err
		}
		agentID := ids.New()
		var ipAddr *netip.Addr
		if a, err := netip.ParseAddr(ip); err == nil {
			ipAddr = &a
		}
		if _, err := tx.Exec(ctx, `INSERT INTO agents (id, tenant_id, asset_id, hostname, os, arch, mode, version, protocol_version, secret_hash, last_heartbeat_at, last_ip,organization_id,instance_name)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,now(),$11,$12,NULLIF($13,''))`, agentID, s.Tenant, assetID, in.Hostname, in.OS, in.Arch, mode, in.Version, in.ProtocolVersion, auth.HashToken(secret), ipAddr, org, in.Name); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE agent_enrollment_tokens SET agent_id=$3 WHERE tenant_id=$1 AND id=$2`, s.Tenant, tokID, agentID); err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, audit.Entry{Tenant: s.Tenant, ActorUser: &createdBy, Action: "agent.enroll", ResourceType: "agent", ResourceID: &agentID, IP: ip,
			Detail: map[string]any{"hostname": in.Hostname, "os": in.OS, "arch": in.Arch, "mode": mode, "asset_id": assetID}}); err != nil {
			return err
		}
		out = &Enrolled{AgentID: agentID, Secret: secret, Mode: mode, PollSeconds: PollSeconds}
		return nil
	})
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" {
		return nil, httpx.Conflict("This agent instance name is already in use.")
	}
	return out, err
}

// ---------- autentikasi agent ----------

type Principal struct {
	AgentID       uuid.UUID
	AssetID       uuid.UUID
	IntegrationID uuid.UUID
	OS, Arch      string
	Version       string
}

func (p *Principal) Worker() *execution.Injector {
	id := p.AgentID
	return &execution.Injector{ID: p.IntegrationID, Key: "agent", AgentID: &id}
}

func (s *Service) Authenticate(ctx context.Context, secret string) (*Principal, error) {
	if !strings.HasPrefix(secret, secretPrefix) || len(secret) > 200 {
		return nil, httpx.Unauthorized()
	}
	var p Principal
	err := s.DB.InTenantRead(ctx, s.Tenant, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT g.id, g.asset_id, g.os, g.arch, g.version, i.id FROM agents g
JOIN integrations i ON i.tenant_id=g.tenant_id AND i.key='agent' AND i.enabled
JOIN assets a ON a.tenant_id=g.tenant_id AND a.id=g.asset_id AND a.deleted_at IS NULL
WHERE g.tenant_id=$1 AND g.secret_hash=$2 AND g.status<>'revoked' AND NOT g.is_demo`, s.Tenant, auth.HashToken(secret)).Scan(&p.AgentID, &p.AssetID, &p.OS, &p.Arch, &p.Version, &p.IntegrationID)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, httpx.Unauthorized()
	}
	return &p, err
}

type HeartbeatInput struct {
	Version         string `json:"version"`
	ProtocolVersion int    `json:"protocol_version"`
	LastUpdateError string `json:"last_update_error"`
}

type HeartbeatReply struct {
	LatestVersion string `json:"latest_version,omitempty"`
	PollSeconds   int    `json:"poll_seconds"`
}

// Heartbeat mencatat bahwa agent hidup dan memberi tahu bila ada rilis yang lebih baru untuk platformnya.
func (s *Service) Heartbeat(ctx context.Context, p *Principal, in HeartbeatInput, ip string) (*HeartbeatReply, error) {
	if !agentproto.ValidVersion(in.Version) {
		return nil, fe("version", "Invalid agent version.")
	}
	if in.ProtocolVersion != agentproto.ProtocolVersion {
		return nil, httpx.NewProblem(426, "Agent too old", "Protocol version mismatch.")
	}
	var ipAddr *netip.Addr
	if a, err := netip.ParseAddr(ip); err == nil {
		ipAddr = &a
	}
	err := s.DB.InTenant(ctx, s.Tenant, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE agents SET last_heartbeat_at=now(), last_ip=COALESCE($3, last_ip), version=$4, status='online', updated_at=now()
WHERE tenant_id=$1 AND id=$2 AND status<>'revoked'`, s.Tenant, p.AgentID, ipAddr, in.Version)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return httpx.Unauthorized()
		}
		// Integrasi "Agent" terlihat hidup selama ada agent yang berdetak.
		_, err = tx.Exec(ctx, `UPDATE integrations SET last_heartbeat_at=now(), status='running' WHERE tenant_id=$1 AND id=$2`, s.Tenant, p.IntegrationID)
		return err
	})
	if err != nil {
		return nil, err
	}
	reply := &HeartbeatReply{PollSeconds: PollSeconds}
	if m, err := s.Release.Manifest(); err == nil {
		if _, ok := m.Find(p.OS, p.Arch); ok && agentproto.Newer(m.Version, in.Version) {
			reply.LatestVersion = m.Version
		}
	}
	return reply, nil
}

// ---------- daftar dan pencabutan ----------

type Agent struct {
	OrganizationID  *uuid.UUID `json:"organization_id"`
	InstanceName    *string    `json:"instance_name"`
	ID              uuid.UUID  `json:"id"`
	AssetID         uuid.UUID  `json:"asset_id"`
	AssetName       string     `json:"asset_name"`
	Hostname        *string    `json:"hostname"`
	OS              string     `json:"os"`
	Arch            string     `json:"arch"`
	Mode            string     `json:"mode"`
	Version         string     `json:"version"`
	Status          string     `json:"status"` // online | offline | revoked (dihitung dari detak terakhir)
	LastHeartbeatAt *time.Time `json:"last_heartbeat_at"`
	EnrolledAt      time.Time  `json:"enrolled_at"`
	UpdateAvailable bool       `json:"update_available"`
}

func (s *Service) list(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, only *uuid.UUID) ([]Agent, error) {
	q := `SELECT g.id, g.asset_id, a.name, g.hostname, g.os, g.arch, g.mode, g.version,
  CASE WHEN g.status='revoked' THEN 'revoked' WHEN g.last_heartbeat_at > $2::timestamptz - interval '90 seconds' THEN 'online' ELSE 'offline' END,
  g.last_heartbeat_at, g.enrolled_at,g.organization_id,g.instance_name
FROM agents g JOIN assets a ON a.tenant_id=g.tenant_id AND a.id=g.asset_id WHERE g.tenant_id=$1`
	args := []any{tenant, s.now()}
	if only != nil {
		q += ` AND g.id=$3`
		args = append(args, *only)
	}
	rows, err := tx.Query(ctx, q+` ORDER BY (g.status='revoked'), a.name, g.enrolled_at LIMIT 500`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Agent{}
	m, _ := s.Release.Manifest()
	for rows.Next() {
		var a Agent
		if err := rows.Scan(&a.ID, &a.AssetID, &a.AssetName, &a.Hostname, &a.OS, &a.Arch, &a.Mode, &a.Version, &a.Status, &a.LastHeartbeatAt, &a.EnrolledAt, &a.OrganizationID, &a.InstanceName); err != nil {
			return nil, err
		}
		if m != nil && a.Status != "revoked" {
			if _, ok := m.Find(a.OS, a.Arch); ok {
				a.UpdateAvailable = agentproto.Newer(m.Version, a.Version)
			}
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Service) List(ctx context.Context, p *auth.Principal) ([]Agent, error) {
	var out []Agent
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		var err error
		out, err = s.list(ctx, tx, p.TenantID, nil)
		return err
	})
	return out, err
}

// Revoke mencabut kredensial agent selamanya (pemulihan = pendaftaran ulang dengan token baru) dan menghentikan eksekusinya.
func (s *Service) Revoke(ctx context.Context, p *auth.Principal, id uuid.UUID, ip, reqID string) error {
	return s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE agents SET status='revoked', revoked_at=now(), revoked_by=$3, updated_at=now() WHERE tenant_id=$1 AND id=$2 AND status<>'revoked'`, p.TenantID, id, p.UserID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agents WHERE tenant_id=$1 AND id=$2)`, p.TenantID, id).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return httpx.NotFound("Agent")
			}
			return httpx.Conflict("This agent is already revoked.")
		}
		rows, err := tx.Query(ctx, `UPDATE executions SET state='ended', outcome='canceled', ended_at=now(), lease_until=NULL, version=version+1, updated_at=now()
WHERE tenant_id=$1 AND agent_id=$2 AND state IN ('ready','running') RETURNING id, simulation_id`, p.TenantID, id)
		if err != nil {
			return err
		}
		type ended struct {
			id  uuid.UUID
			sim *uuid.UUID
		}
		var list []ended
		for rows.Next() {
			var e ended
			if err := rows.Scan(&e.id, &e.sim); err != nil {
				rows.Close()
				return err
			}
			list = append(list, e)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, e := range list {
			if _, err := tx.Exec(ctx, `INSERT INTO execution_logs (tenant_id, execution_id, level, message) VALUES ($1,$2,'warn','Canceled: the agent was revoked.')`, p.TenantID, e.id); err != nil {
				return err
			}
			if e.sim != nil && s.AfterEnd != nil {
				if err := s.AfterEnd(ctx, tx, p.TenantID, *e.sim); err != nil {
					return err
				}
			}
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "agent.revoke", ResourceType: "agent", ResourceID: &id, IP: ip, RequestID: reqID})
	})
}

// Sweep menandai agent yang senyap sebagai offline dan memberi alert (satu per episode senyap).
func (s *Service) Sweep(ctx context.Context) (int, error) {
	n := 0
	err := s.DB.InTenant(ctx, s.Tenant, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `UPDATE agents g SET status='offline', updated_at=now() FROM assets a
WHERE g.tenant_id=$1 AND NOT g.is_demo AND g.status='online' AND g.last_heartbeat_at < $2::timestamptz - interval '90 seconds' AND a.tenant_id=g.tenant_id AND a.id=g.asset_id
RETURNING g.id, a.name, extract(epoch from g.last_heartbeat_at)::bigint`, s.Tenant, s.now())
		if err != nil {
			return err
		}
		type off struct {
			id   uuid.UUID
			name string
			beat int64
		}
		var list []off
		for rows.Next() {
			var o off
			if err := rows.Scan(&o.id, &o.name, &o.beat); err != nil {
				rows.Close()
				return err
			}
			list = append(list, o)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, o := range list {
			id := o.id
			if err := alerts.Raise(ctx, tx, s.Tenant, alerts.New{Kind: "agent_offline", Severity: "warning", Title: "Agent on " + o.name + " went offline",
				Body: "It has not checked in for over a minute and a half.", ResourceType: "agent", ResourceID: &id, DedupeKey: "agent-offline:" + id.String() + ":" + itoa(o.beat)}); err != nil {
				return err
			}
			n++
		}
		return nil
	})
	return n, err
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

// RunSweeper menjalankan Sweep berkala.
func (s *Service) RunSweeper(ctx context.Context, every time.Duration, log func(error)) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := s.Sweep(ctx); err != nil && log != nil {
				log(err)
			}
		}
	}
}
