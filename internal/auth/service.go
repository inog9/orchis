package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/netip"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"orchis-go/internal/audit"
	"orchis-go/internal/db"
	"orchis-go/internal/httpx"
	"orchis-go/internal/ids"
)

const (
	MaxFailures   = 5
	LockDuration  = 15 * time.Minute
	IdleTTL       = 30 * time.Minute
	AbsoluteTTL   = 12 * time.Hour
	RememberTTL   = 30 * 24 * time.Hour
	touchInterval = time.Minute
)

type Principal struct {
	TenantID    uuid.UUID
	UserID      uuid.UUID
	SessionID   uuid.UUID
	Email       string
	DisplayName string
	RoleKey     string
	Permissions []string
	UILanguage  string
}

type Service struct {
	DB  *db.DB
	Now func() time.Time
}

func NewService(d *db.DB) *Service { return &Service{DB: d, Now: time.Now} }

// dummyHash dipakai agar waktu respons untuk email tak dikenal setara dengan email yang dikenal.
var dummyHash, _ = HashPassword("tidak-ada-akun-dengan-password-ini")

type LoginResult struct {
	Token     string
	Principal *Principal
	Remember  bool
}

func errInvalid() error {
	return httpx.NewProblem(401, "Sign-in failed", "Email or password is incorrect.")
}

func errLocked(until, now time.Time) error {
	p := httpx.NewProblem(429, "Account temporarily locked", "Too many sign-in attempts. Try again later.")
	p.RetryAfter = int(until.Sub(now).Seconds()) + 1
	return p
}

// Login never persists guessed email for an unknown account. Unknown attempts
// share an IP bucket; known accounts retain their account lock and generic errors.
func (s *Service) Login(ctx context.Context, tenant uuid.UUID, email, password, ip, userAgent string, remember bool) (*LoginResult, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || password == "" {
		return nil, httpx.Validation(httpx.FieldError{Name: "email", Message: "Email and password are required."})
	}
	now := s.Now()
	var result *LoginResult
	var outErr error
	err := s.DB.InTenant(ctx, tenant, func(tx pgx.Tx) error {

		var uid uuid.UUID
		var hash *string
		var displayName, roleKey, language string
		var perms []string
		var status string
		uerr := tx.QueryRow(ctx, `
SELECT u.id, u.password_hash, u.display_name, u.status, r.key, r.permissions, COALESCE(u.ui_language,'')
FROM users u JOIN roles r ON r.tenant_id = u.tenant_id AND r.id = u.role_id
WHERE u.tenant_id=$1 AND u.email=$2 AND u.deleted_at IS NULL AND u.kind='human' AND NOT u.is_demo`, tenant, email).Scan(&uid, &hash, &displayName, &status, &roleKey, &perms, &language)
		found := uerr == nil
		if uerr != nil && !errors.Is(uerr, pgx.ErrNoRows) {
			return uerr
		}

		bucket := email
		var actor *uuid.UUID
		if found {
			actor = &uid
		} else {
			addr, err := netip.ParseAddr(ip)
			bucket = "unknown:unavailable"
			if err == nil {
				bucket = "unknown:" + addr.Unmap().String()
			}
		}
		// Serialize first failures too: SELECT FOR UPDATE cannot lock a missing row.
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, tenant.String()+"/login/"+bucket); err != nil {
			return err
		}
		record := func(action, reason string, session *uuid.UUID) error {
			return audit.Write(ctx, tx, audit.Entry{Tenant: tenant, ActorUser: actor, Action: action, ResourceType: "session", ResourceID: session, IP: ip, RequestID: httpx.RequestIDFrom(ctx), Detail: map[string]any{"reason": reason}})
		}
		var failed int
		var firstFailed, lockedUntil *time.Time
		err := tx.QueryRow(ctx, `SELECT failed_attempts, first_failed_at, locked_until FROM login_throttle
WHERE tenant_id=$1 AND email=$2 FOR UPDATE`, tenant, bucket).Scan(&failed, &firstFailed, &lockedUntil)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if lockedUntil != nil && lockedUntil.After(now) {
			outErr = errLocked(*lockedUntil, now)
			return record("auth.login_failed", "locked", nil)
		}

		good := false
		if found && hash != nil && status == "active" {
			good = VerifyPassword(password, *hash)
		} else {
			VerifyPassword(password, dummyHash) // meratakan waktu
		}

		if !good {
			if firstFailed == nil || now.Sub(*firstFailed) > LockDuration {
				failed, firstFailed = 0, &now
			}
			failed++
			var lock *time.Time
			if failed >= MaxFailures {
				l := now.Add(LockDuration)
				lock = &l
			}
			addr, _ := netip.ParseAddr(ip)
			var ipp *netip.Addr
			if addr.IsValid() {
				ipp = &addr
			}
			if _, err := tx.Exec(ctx, `
INSERT INTO login_throttle (tenant_id, email, failed_attempts, first_failed_at, locked_until, last_ip, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT (tenant_id, email) DO UPDATE SET failed_attempts=EXCLUDED.failed_attempts,
  first_failed_at=EXCLUDED.first_failed_at, locked_until=EXCLUDED.locked_until, last_ip=EXCLUDED.last_ip, updated_at=EXCLUDED.updated_at`,
				tenant, bucket, failed, firstFailed, lock, ipp, now); err != nil {
				return err
			}
			if lock != nil {
				outErr = errLocked(*lock, now)
			} else {
				outErr = errInvalid()
			}
			return record("auth.login_failed", "invalid_credentials", nil) // commit count and audit together
		}

		if _, err := tx.Exec(ctx, `DELETE FROM login_throttle WHERE tenant_id=$1 AND email=$2`, tenant, email); err != nil {
			return err
		}
		token, sid, err := s.createSession(ctx, tx, tenant, uid, ip, userAgent, remember, now)
		if err != nil {
			return err
		}
		var ipp *netip.Addr
		if a, err := netip.ParseAddr(ip); err == nil {
			ipp = &a
		}
		if _, err := tx.Exec(ctx, `UPDATE users SET last_login_at=$3, last_login_ip=$4 WHERE tenant_id=$1 AND id=$2`, tenant, uid, now, ipp); err != nil {
			return err
		}
		result = &LoginResult{Token: token, Remember: remember, Principal: &Principal{
			TenantID: tenant, UserID: uid, SessionID: sid, Email: email, DisplayName: displayName, RoleKey: roleKey, Permissions: perms, UILanguage: language}}
		return record("auth.login", "authenticated", &sid)
	})
	if err != nil {
		return nil, err
	}
	if outErr != nil {
		return nil, outErr
	}
	return result, nil
}

func (s *Service) createSession(ctx context.Context, tx pgx.Tx, tenant, uid uuid.UUID, ip, ua string, remember bool, now time.Time) (string, uuid.UUID, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", uuid.Nil, err
	}
	// Token memuat ID tenant (bukan rahasia) agar RLS bisa diterapkan sebelum sesi dicari.
	token := tenant.String() + "." + base64.RawURLEncoding.EncodeToString(raw)
	abs := AbsoluteTTL
	if remember {
		abs = RememberTTL
	}
	sid := ids.New()
	var ipp *netip.Addr
	if a, err := netip.ParseAddr(ip); err == nil {
		ipp = &a
	}
	_, err := tx.Exec(ctx, `
INSERT INTO sessions (id, tenant_id, user_id, token_hash, remember_me, ip, user_agent, created_at, last_seen_at, idle_expires_at, absolute_expires_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$8,$9,$10)`,
		sid, tenant, uid, hashToken(token), remember, ipp, truncate(ua, 300), now, now.Add(IdleTTL), now.Add(abs))
	return token, sid, err
}

func hashToken(t string) []byte { h := sha256.Sum256([]byte(t)); return h[:] }

// HashToken: hash SHA-256 untuk token acak berentropi tinggi (token API, link reset).
func HashToken(t string) []byte { return hashToken(t) }

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// Authenticate mengubah token sesi menjadi Principal, atau mengembalikan 401.
func (s *Service) Authenticate(ctx context.Context, token string) (*Principal, error) {
	tenantPart, _, ok := strings.Cut(token, ".")
	tenant, err := uuid.Parse(tenantPart)
	if !ok || err != nil {
		return nil, httpx.Unauthorized()
	}
	now := s.Now()
	var p Principal
	var lastSeen time.Time
	err = s.DB.InTenantRead(ctx, tenant, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
SELECT s.id, s.last_seen_at, u.id, u.email::text, u.display_name, r.key, r.permissions, COALESCE(u.ui_language,'')
FROM sessions s
JOIN users u ON u.tenant_id = s.tenant_id AND u.id = s.user_id
JOIN roles r ON r.tenant_id = u.tenant_id AND r.id = u.role_id
WHERE s.tenant_id=$1 AND s.token_hash=$2 AND s.revoked_at IS NULL
  AND s.idle_expires_at > $3 AND s.absolute_expires_at > $3
  AND u.status='active' AND u.deleted_at IS NULL AND NOT u.is_demo`, tenant, hashToken(token), now).
			Scan(&p.SessionID, &lastSeen, &p.UserID, &p.Email, &p.DisplayName, &p.RoleKey, &p.Permissions, &p.UILanguage)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, httpx.Unauthorized()
	}
	if err != nil {
		return nil, err
	}
	p.TenantID = tenant
	if now.Sub(lastSeen) > touchInterval { // perpanjang batas idle, tapi tidak menulis di setiap permintaan
		_ = s.DB.InTenant(ctx, tenant, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `UPDATE sessions SET last_seen_at=$3, idle_expires_at=$4 WHERE tenant_id=$1 AND id=$2`,
				tenant, p.SessionID, now, now.Add(IdleTTL))
			return err
		})
	}
	return &p, nil
}

func (s *Service) Logout(ctx context.Context, p *Principal, clientIP ...string) error {
	return s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE sessions SET revoked_at=now() WHERE tenant_id=$1 AND id=$2`, p.TenantID, p.SessionID)
		if err != nil {
			return err
		}
		ip := ""
		if len(clientIP) > 0 {
			ip = clientIP[0]
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "auth.logout", ResourceType: "session", IP: ip, RequestID: httpx.RequestIDFrom(ctx)})
	})
}

// APITokenPrefix membedakan token API dari token sesi.
const APITokenPrefix = "oat_"

// AuthenticateToken mengubah token API menjadi Principal dengan izin peran pemiliknya saat ini (bukan saat token dibuat),
// sehingga mengubah peran atau menonaktifkan akun langsung berlaku pada token.
func (s *Service) AuthenticateToken(ctx context.Context, tenant uuid.UUID, token string) (*Principal, error) {
	if !strings.HasPrefix(token, APITokenPrefix) || len(token) > 200 {
		return nil, httpx.Unauthorized()
	}
	now := s.Now()
	var p Principal
	var tokenID uuid.UUID
	var lastUsed *time.Time
	err := s.DB.InTenantRead(ctx, tenant, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
SELECT t.id, t.last_used_at, u.id, u.email::text, u.display_name, r.key, r.permissions, COALESCE(u.ui_language,'')
FROM api_tokens t
JOIN users u ON u.tenant_id=t.tenant_id AND u.id=t.user_id
JOIN roles r ON r.tenant_id=u.tenant_id AND r.id=u.role_id
WHERE t.tenant_id=$1 AND t.token_hash=$2 AND NOT t.is_demo AND t.revoked_at IS NULL AND (t.expires_at IS NULL OR t.expires_at > $3)
  AND u.status='active' AND u.deleted_at IS NULL AND NOT u.is_demo`, tenant, hashToken(token), now).
			Scan(&tokenID, &lastUsed, &p.UserID, &p.Email, &p.DisplayName, &p.RoleKey, &p.Permissions, &p.UILanguage)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, httpx.Unauthorized()
	}
	if err != nil {
		return nil, err
	}
	p.TenantID = tenant
	if lastUsed == nil || now.Sub(*lastUsed) > touchInterval {
		_ = s.DB.InTenant(ctx, tenant, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `UPDATE api_tokens SET last_used_at=$3 WHERE tenant_id=$1 AND id=$2`, tenant, tokenID, now)
			return err
		})
	}
	return &p, nil
}
