// Package agents: pendaftaran agent (token sekali pakai), kredensial per agent, heartbeat, rilis, dan pencabutan.
// Agent hanya melakukan koneksi KELUAR ke server (polling); server tidak pernah menghubungi endpoint.
package agents

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/netip"
	"orchis-go/internal/scancontrol"
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
	"orchis-go/internal/osquery"
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
	AfterEnd   func(ctx context.Context, tx pgx.Tx, tenant, simulation uuid.UUID) error
	Now        func() time.Time
	CollectNow func(context.Context, *auth.Principal, uuid.UUID, string, string) ([]uuid.UUID, error)
	// AutoConsent is retained for compatibility and has no effect.
	AutoConsent bool
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
	InstallTrust
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
		if err := scancontrol.Lock(ctx, tx, p.TenantID); err != nil {
			return err
		}
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
	return &EnrollmentToken{ID: id, Token: tok, OS: in.OS, Arch: in.Arch, Mode: in.Mode, ExpiresAt: exp, Version: m.Version, SHA256: tgt.SHA256, FileName: tgt.File, InstallTrust: LoadInstallTrust(s.Release.Dir)}, nil
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
		if err := scancontrol.Lock(ctx, tx, s.Tenant); err != nil {
			return err
		}
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
	LastUpdateError string `json:"last_update_error"` // legacy input ignored, never persisted
	ControlProtocol int    `json:"control_protocol,omitempty"`
	OsqueryVersion  string `json:"osquery_version,omitempty"`
	DiagnosticCode  string `json:"diagnostic_code,omitempty"`
	// OSVersion adalah laporan agent tentang dirinya sendiri; data tidak tepercaya, dibersihkan sebelum disimpan.
	OSVersion string `json:"os_version,omitempty"`
}

var osVersionRe = regexp.MustCompile(`^[\p{L}\p{N} ._()+/-]{1,64}$`)

// CleanOSVersion menerima hanya string pendek bergaya nama OS ("macOS 10.15.7", "Ubuntu 24.04.1 LTS"); selain itu kosong.
func CleanOSVersion(s string) string {
	s = strings.TrimSpace(s)
	if !osVersionRe.MatchString(s) {
		return ""
	}
	return s
}

type HeartbeatReply struct {
	LatestVersion string                     `json:"latest_version,omitempty"`
	PollSeconds   int                        `json:"poll_seconds"`
	Paused        bool                       `json:"paused"`
	Command       *agentproto.ControlCommand `json:"command,omitempty"`
}

// Heartbeat mencatat bahwa agent hidup dan memberi tahu bila ada rilis yang lebih baru untuk platformnya.
func (s *Service) Heartbeat(ctx context.Context, p *Principal, in HeartbeatInput, ip string) (*HeartbeatReply, error) {
	if !agentproto.ValidVersion(in.Version) {
		return nil, fe("version", "Invalid agent version.")
	}
	if in.ProtocolVersion != agentproto.ProtocolVersion {
		return nil, httpx.NewProblem(426, "Agent too old", "Protocol version mismatch.")
	}
	if in.ControlProtocol < 0 || in.ControlProtocol > agentproto.ControlProtocol {
		return nil, fe("control_protocol", "Unsupported control protocol.")
	}
	if in.OsqueryVersion != "" && !agentproto.ValidVersion(in.OsqueryVersion) {
		return nil, fe("osquery_version", "Invalid osquery version.")
	}
	reply := &HeartbeatReply{PollSeconds: PollSeconds}
	var requested string
	var policy RolloutPolicy
	var ipAddr *netip.Addr
	if a, err := netip.ParseAddr(ip); err == nil {
		ipAddr = &a
	}
	err := s.DB.InTenant(ctx, s.Tenant, func(tx pgx.Tx) error {
		if err := scancontrol.Lock(ctx, tx, s.Tenant); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE agents SET last_heartbeat_at=$5, last_ip=COALESCE($3, last_ip), version=$4, status='online', updated_at=now(),offline_episode=offline_episode+CASE WHEN status='offline' THEN 1 ELSE 0 END,update_failure_episode=update_failure_episode+CASE WHEN diagnostic_code NOT IN('update_failed','release_key_missing') AND $8 IN('update_failed','release_key_missing') THEN 1 ELSE 0 END,control_protocol=$6,osquery_version=$7,diagnostic_at=CASE WHEN diagnostic_code<>$8 THEN $5 ELSE diagnostic_at END,diagnostic_code=$8
WHERE tenant_id=$1 AND id=$2 AND status<>'revoked'`, s.Tenant, p.AgentID, ipAddr, in.Version, s.now(), in.ControlProtocol, in.OsqueryVersion, agentproto.DiagnosticCode(in.DiagnosticCode))
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return httpx.Unauthorized()
		}
		if code := agentproto.DiagnosticCode(in.DiagnosticCode); code == "update_failed" || code == "release_key_missing" {
			var episode int64
			if err = tx.QueryRow(ctx, `SELECT update_failure_episode FROM agents WHERE tenant_id=$1 AND id=$2`, s.Tenant, p.AgentID).Scan(&episode); err != nil {
				return err
			}
			if err = alerts.Raise(ctx, tx, s.Tenant, alerts.New{Kind: "agent_update_failed", Severity: "warning", Title: "Agent update failed", ResourceType: "agent", ResourceID: &p.AgentID, Audience: authz.AgentRead, DedupeKey: "agent-update-failed:" + p.AgentID.String() + ":" + itoa(episode)}); err != nil {
				return err
			}
		}
		if v := CleanOSVersion(in.OSVersion); v != "" {
			// Kunci terpisah dari os_version isian manual: isian pengguna selalu menang, dan hanya ditulis bila nilainya berubah.
			if _, err = tx.Exec(ctx, `UPDATE assets SET attributes=attributes||jsonb_build_object('agent_os_version',$3::text)
WHERE tenant_id=$1 AND id=(SELECT asset_id FROM agents WHERE tenant_id=$1 AND id=$2) AND attributes->>'agent_os_version' IS DISTINCT FROM $3::text`, s.Tenant, p.AgentID, v); err != nil {
				return err
			}
		}
		if _, err = tx.Exec(ctx, `UPDATE assets SET platform=$3, arch=$4, updated_at=now()
WHERE tenant_id=$1 AND id=(SELECT asset_id FROM agents WHERE tenant_id=$1 AND id=$2) AND source='agent'
AND (platform IS DISTINCT FROM $3 OR arch IS DISTINCT FROM $4)`, s.Tenant, p.AgentID, p.OS, p.Arch); err != nil {
			return err
		}
		var command *uuid.UUID
		var state string
		if err = tx.QueryRow(ctx, `SELECT paused,requested_update_version,uninstall_id,uninstall_status FROM agents WHERE tenant_id=$1 AND id=$2`, s.Tenant, p.AgentID).Scan(&reply.Paused, &requested, &command, &state); err != nil {
			return err
		}
		if command != nil && (state == "queued" || state == "dispatched") && agentproto.CanUninstall(in.ControlProtocol) {
			reply.Command = &agentproto.ControlCommand{ID: command.String(), Kind: "uninstall", AgentID: p.AgentID.String()}
			if _, err = tx.Exec(ctx, `UPDATE agents SET uninstall_status='dispatched' WHERE tenant_id=$1 AND id=$2`, s.Tenant, p.AgentID); err != nil {
				return err
			}
		}
		// A collect command is a wake-up for a queued, already scoped execution.
		// Claim remains the acknowledgement and rechecks consent before any SQL runs.
		if reply.Command == nil && !reply.Paused && agentproto.CanCollect(in.ControlProtocol) {
			consent := osquery.Consent()
			rows, e := tx.Query(ctx, `SELECT req.execution_id,req.query_key FROM endpoint_collection_requests req
JOIN executions e ON e.tenant_id=req.tenant_id AND e.id=req.execution_id AND e.agent_id=req.agent_id AND e.target_asset_id=req.asset_id
JOIN agents g ON g.tenant_id=req.tenant_id AND g.id=req.agent_id AND g.asset_id=req.asset_id
JOIN assets a ON a.tenant_id=req.tenant_id AND a.id=req.asset_id AND a.organization_id=g.organization_id
JOIN osquery_org_settings s ON s.tenant_id=a.tenant_id AND s.organization_id=a.organization_id AND s.query_key=req.query_key
WHERE req.tenant_id=$1 AND req.agent_id=$2 AND e.state='ready' AND e.not_before<=now()
AND a.testing_authorized AND a.deleted_at IS NULL AND NOT a.is_demo AND NOT g.is_demo AND g.status<>'revoked'
AND s.enabled AND s.consent_version=$3 AND s.consent_sha256=$4 AND s.consented_by IS NOT NULL AND s.consented_at IS NOT NULL
ORDER BY req.created_at,req.execution_id LIMIT 100`, s.Tenant, p.AgentID, consent.Version, consent.SHA256)
			if e != nil {
				return e
			}
			command := &agentproto.ControlCommand{Kind: "collect", AgentID: p.AgentID.String()}
			for rows.Next() {
				var executionID uuid.UUID
				var key string
				if e = rows.Scan(&executionID, &key); e != nil {
					rows.Close()
					return e
				}
				if q := osquery.Find(key); q != nil && q.SupportsOS(p.OS) && osquery.CollectionSupported(p.OS, p.Arch) && len(command.Queries) < 8 {
					command.Queries = append(command.Queries, key)
					command.ExecutionIDs = append(command.ExecutionIDs, executionID.String())
				}
			}
			rows.Close()
			if e = rows.Err(); e != nil {
				return e
			}
			if len(command.Queries) > 0 {
				command.ID = command.ExecutionIDs[0]
				reply.Command = command
			}
		}
		policy, err = rollout(ctx, tx, s.Tenant)
		if err != nil {
			return err
		}
		// Integrasi "Agent" terlihat hidup selama ada agent yang berdetak.
		_, err = tx.Exec(ctx, `UPDATE integrations SET last_heartbeat_at=now(), status='running' WHERE tenant_id=$1 AND id=$2`, s.Tenant, p.IntegrationID)
		return err
	})
	if err != nil {
		return nil, err
	}
	if m, err := s.Release.Manifest(); err == nil && reply.Command == nil && (policy.Mode == "automatic" || requested == m.Version) {
		if _, ok := m.Find(p.OS, p.Arch); ok && agentproto.Newer(m.Version, in.Version) {
			reply.LatestVersion = m.Version
		}
	}
	return reply, nil
}

// ---------- daftar dan pencabutan ----------

type Agent struct {
	OrganizationID         *uuid.UUID `json:"organization_id"`
	InstanceName           *string    `json:"instance_name"`
	ID                     uuid.UUID  `json:"id"`
	AssetID                uuid.UUID  `json:"asset_id"`
	AssetName              string     `json:"asset_name"`
	Hostname               *string    `json:"hostname"`
	OS                     string     `json:"os"`
	Arch                   string     `json:"arch"`
	Mode                   string     `json:"mode"`
	Version                string     `json:"version"`
	Status                 string     `json:"status"` // online | offline | revoked (dihitung dari detak terakhir)
	LastHeartbeatAt        *time.Time `json:"last_heartbeat_at"`
	EnrolledAt             time.Time  `json:"enrolled_at"`
	UpdateAvailable        bool       `json:"update_available"`
	ControlVersion         int        `json:"control_version"`
	Paused                 bool       `json:"paused"`
	ControlProtocol        int        `json:"control_protocol"`
	OsqueryVersion         string     `json:"osquery_version"`
	DiagnosticCode         string     `json:"diagnostic_code"`
	DiagnosticAt           *time.Time `json:"diagnostic_at"`
	RequestedUpdateVersion string     `json:"requested_update_version"`
	UninstallStatus        string     `json:"uninstall_status"`
	UninstallRequestedAt   *time.Time `json:"uninstall_requested_at"`
	UninstallCompletedAt   *time.Time `json:"uninstall_completed_at"`
	UpdateStatus           string     `json:"update_status"`
}

func (s *Service) list(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, only *uuid.UUID) ([]Agent, error) {
	q := `SELECT g.id, g.asset_id, a.name, g.hostname, g.os, g.arch, g.mode, g.version,
  CASE WHEN g.status='revoked' THEN 'revoked' WHEN g.last_heartbeat_at > $2::timestamptz - interval '90 seconds' THEN 'online' ELSE 'offline' END,
  g.last_heartbeat_at, g.enrolled_at,g.organization_id,g.instance_name, g.control_version,g.paused,g.control_protocol,g.osquery_version,g.diagnostic_code,g.diagnostic_at,g.requested_update_version,g.uninstall_status,g.uninstall_requested_at,g.uninstall_completed_at
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
		if err := rows.Scan(&a.ID, &a.AssetID, &a.AssetName, &a.Hostname, &a.OS, &a.Arch, &a.Mode, &a.Version, &a.Status, &a.LastHeartbeatAt, &a.EnrolledAt, &a.OrganizationID, &a.InstanceName, &a.ControlVersion, &a.Paused, &a.ControlProtocol, &a.OsqueryVersion, &a.DiagnosticCode, &a.DiagnosticAt, &a.RequestedUpdateVersion, &a.UninstallStatus, &a.UninstallRequestedAt, &a.UninstallCompletedAt); err != nil {
			return nil, err
		}
		a.UpdateStatus = "unknown"
		if m != nil && a.Status != "revoked" {
			if _, ok := m.Find(a.OS, a.Arch); ok {
				a.UpdateAvailable = agentproto.Newer(m.Version, a.Version)
				a.UpdateStatus = "current"
				if a.UpdateAvailable {
					a.UpdateStatus = "behind"
				}
			}
		}
		if a.DiagnosticCode == "update_failed" || a.DiagnosticCode == "release_key_missing" {
			a.UpdateStatus = "failed"
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
		if err := scancontrol.Lock(ctx, tx, p.TenantID); err != nil {
			return err
		}
		if err := s.revokeTx(ctx, tx, p.TenantID, id, &p.UserID); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "agent.revoke", ResourceType: "agent", ResourceID: &id, IP: ip, RequestID: reqID})
	})
}
func (s *Service) revokeTx(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID, actor *uuid.UUID) error {
	tag, err := tx.Exec(ctx, `UPDATE agents SET status='revoked', revoked_at=now(), revoked_by=$3, updated_at=now() WHERE tenant_id=$1 AND id=$2 AND status<>'revoked'`, tenant, id, actor)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agents WHERE tenant_id=$1 AND id=$2)`, tenant, id).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return httpx.NotFound("Agent")
		}
		return httpx.Conflict("This agent is already revoked.")
	}
	if err := alerts.Raise(ctx, tx, tenant, alerts.New{Kind: "agent_revoked", Severity: "warning", Title: "Agent credentials revoked", ResourceType: "agent", ResourceID: &id, Audience: authz.AgentRead, DedupeKey: "agent-revoked:" + id.String()}); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `UPDATE executions SET state='ended', outcome='canceled', ended_at=now(), lease_until=NULL, version=version+1, updated_at=now()
WHERE tenant_id=$1 AND agent_id=$2 AND state IN ('ready','running') RETURNING id, simulation_id`, tenant, id)
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
		if _, err := tx.Exec(ctx, `INSERT INTO execution_logs (tenant_id, execution_id, level, message) VALUES ($1,$2,'warn','Canceled: the agent was revoked.')`, tenant, e.id); err != nil {
			return err
		}
		if e.sim != nil && s.AfterEnd != nil {
			if err := s.AfterEnd(ctx, tx, tenant, *e.sim); err != nil {
				return err
			}
		}
	}
	return nil
}

// Sweep retains the 90-second contact indicator, separately notifies after the configured threshold.
func (s *Service) Sweep(ctx context.Context) (int, error) {
	n := 0
	err := s.DB.InTenant(ctx, s.Tenant, func(tx pgx.Tx) error {
		if e := scancontrol.Lock(ctx, tx, s.Tenant); e != nil {
			return e
		}
		tag, e := tx.Exec(ctx, `UPDATE agents SET status='offline',updated_at=now() WHERE tenant_id=$1 AND NOT is_demo AND status='online' AND last_heartbeat_at<$2::timestamptz-interval '90 seconds'`, s.Tenant, s.now())
		if e != nil {
			return e
		}
		n = int(tag.RowsAffected())
		policy, e := alerts.CurrentPolicy(ctx, tx, s.Tenant)
		if e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT g.id,g.offline_episode FROM agents g JOIN assets a ON a.tenant_id=g.tenant_id AND a.id=g.asset_id AND a.deleted_at IS NULL WHERE g.tenant_id=$1 AND NOT g.is_demo AND g.status='offline' AND g.last_heartbeat_at<$2::timestamptz-make_interval(mins=>$3) ORDER BY g.id`, s.Tenant, s.now(), policy.OfflineMinutes)
		if e != nil {
			return e
		}
		type off struct {
			id      uuid.UUID
			episode int64
		}
		list := []off{}
		for rows.Next() {
			var o off
			if e = rows.Scan(&o.id, &o.episode); e != nil {
				rows.Close()
				return e
			}
			list = append(list, o)
		}
		rows.Close()
		if e = rows.Err(); e != nil {
			return e
		}
		for _, o := range list {
			if e = alerts.Raise(ctx, tx, s.Tenant, alerts.New{Kind: "agent_offline", Severity: "warning", Title: "Agent is offline", ResourceType: "agent", ResourceID: &o.id, Audience: authz.AgentRead, DedupeKey: "agent-offline:" + o.id.String() + ":" + itoa(o.episode)}); e != nil {
				return e
			}
		}
		return alerts.PendingApprovals(ctx, tx, s.Tenant)
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
