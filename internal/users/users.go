// Package users: akun, peran, link reset buatan admin (tanpa SMTP), profil, dan token API.
package users

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"orchis-go/internal/alerts"
	"orchis-go/internal/audit"
	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/db"
	"orchis-go/internal/httpx"
	"orchis-go/internal/ids"
)

const (
	ResetTTL     = 30 * time.Minute
	resetPrefix  = "orl_"
	historyDepth = 5
)

type Service struct {
	DB     *db.DB
	Tenant uuid.UUID // tenant tunggal MVP; dipakai oleh endpoint publik (reset, lupa password)
	Now    func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func newToken(prefix string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(b), nil
}

func fieldErr(name, msg string) error {
	return httpx.Validation(httpx.FieldError{Name: name, Message: msg})
}

// ---------- daftar ----------

type UserRow struct {
	ID                 uuid.UUID  `json:"id"`
	Email              string     `json:"email"`
	DisplayName        string     `json:"display_name"`
	RoleKey            string     `json:"role"`
	RoleName           string     `json:"role_name"`
	Status             string     `json:"status"`
	LastLoginAt        *time.Time `json:"last_login_at"`
	CreatedAt          time.Time  `json:"created_at"`
	Version            int        `json:"version"`
	RoleReviewRequired bool       `json:"role_review_required"`
	PreviousRoleKey    *string    `json:"previous_role_key"`
	HasPassword        bool       `json:"has_password"` // false = belum pernah memakai link reset pertama
}

func (s *Service) List(ctx context.Context, p *auth.Principal) ([]UserRow, error) {
	out := []UserRow{}
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
SELECT u.id, u.email::text, u.display_name, r.key, r.name, u.status, u.last_login_at, u.created_at, u.version, u.password_hash IS NOT NULL, u.role_review_required, u.previous_role_key
FROM users u JOIN roles r ON r.tenant_id=u.tenant_id AND r.id=u.role_id
WHERE u.tenant_id=$1 AND u.deleted_at IS NULL AND u.kind='human' ORDER BY u.created_at, u.id`, p.TenantID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var u UserRow
			if err := rows.Scan(&u.ID, &u.Email, &u.DisplayName, &u.RoleKey, &u.RoleName, &u.Status, &u.LastLoginAt, &u.CreatedAt, &u.Version, &u.HasPassword, &u.RoleReviewRequired, &u.PreviousRoleKey); err != nil {
				return err
			}
			out = append(out, u)
		}
		return rows.Err()
	})
	return out, err
}

type RoleRow struct {
	Key         string   `json:"key"`
	Name        string   `json:"name"`
	Description *string  `json:"description"`
	Permissions []string `json:"permissions"`
	IsSystem    bool     `json:"is_system"`
	Users       int      `json:"users"`
}

func (s *Service) Roles(ctx context.Context, p *auth.Principal) ([]RoleRow, error) {
	out := []RoleRow{}
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
SELECT r.key, r.name, r.description, r.permissions, r.is_system,
  (SELECT count(*) FROM users u WHERE u.tenant_id=r.tenant_id AND u.role_id=r.id AND u.deleted_at IS NULL)
FROM roles r WHERE r.tenant_id=$1 ORDER BY array_position(ARRAY['admin','analyst','viewer'], r.key), r.name`, p.TenantID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r RoleRow
			if err := rows.Scan(&r.Key, &r.Name, &r.Description, &r.Permissions, &r.IsSystem, &r.Users); err != nil {
				return err
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	return out, err
}

// ---------- buat, ubah ----------

type ResetLink struct {
	Token     string    `json:"token"` // hanya tampil sekali
	ExpiresAt time.Time `json:"expires_at"`
}

type CreateInput struct {
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

type Created struct {
	User      UserRow   `json:"user"`
	ResetLink ResetLink `json:"reset_link"`
}

func validEmail(e string) bool {
	a, err := mail.ParseAddress(e)
	return err == nil && a.Address == e && len(e) <= 254 && !strings.ContainsAny(e, " <>")
}

// Create membuat akun TANPA password dan sebuah link reset sekali pakai; admin menyerahkan link itu kepada pemilik akun.
func (s *Service) Create(ctx context.Context, p *auth.Principal, in CreateInput, ip, reqID string) (*Created, error) {
	in.Email = strings.TrimSpace(in.Email)
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	var fe []httpx.FieldError
	if !validEmail(in.Email) {
		fe = append(fe, httpx.FieldError{Name: "email", Message: "Enter a valid email address."})
	}
	if in.DisplayName == "" || len([]rune(in.DisplayName)) > 120 {
		fe = append(fe, httpx.FieldError{Name: "display_name", Message: "Enter a name of 1 to 120 characters."})
	}
	if len(fe) > 0 {
		return nil, httpx.Validation(fe...)
	}
	uid := ids.New()
	var out Created
	err := s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		var roleID uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT id FROM roles WHERE tenant_id=$1 AND key=$2`, p.TenantID, in.Role).Scan(&roleID); errors.Is(err, pgx.ErrNoRows) {
			return fieldErr("role", "Choose admin, analyst or viewer.")
		} else if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO users (id, tenant_id, email, display_name, role_id) VALUES ($1,$2,$3,$4,$5)`, uid, p.TenantID, in.Email, in.DisplayName, roleID); err != nil {
			var pg *pgconn.PgError
			if errors.As(err, &pg) && pg.Code == "23505" {
				return fieldErr("email", "An account with this email already exists.")
			}
			return err
		}
		link, err := s.newLink(ctx, tx, p, uid)
		if err != nil {
			return err
		}
		out.ResetLink = *link
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "user.create", ResourceType: "user", ResourceID: &uid, IP: ip, RequestID: reqID,
			Detail: map[string]any{"email": in.Email, "role": in.Role}})
	})
	if err != nil {
		return nil, err
	}
	rows, err := s.List(ctx, p)
	if err != nil {
		return nil, err
	}
	for _, u := range rows {
		if u.ID == uid {
			out.User = u
		}
	}
	return &out, nil
}

type PatchInput struct {
	AcceptRolePermissions bool    `json:"accept_role_permissions"`
	Version               int     `json:"version"`
	DisplayName           *string `json:"display_name"`
	Role                  *string `json:"role"`
	Status                *string `json:"status"`
}

func (s *Service) Patch(ctx context.Context, p *auth.Principal, id uuid.UUID, in PatchInput, ip, reqID string) (*UserRow, error) {
	if in.Version < 1 {
		return nil, fieldErr("version", "Send the version you loaded.")
	}
	err := s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		// Kunci SEMUA admin aktif lebih dulu, berurutan menurut id. Tanpa ini dua admin yang saling menurunkan secara serentak
		// sama-sama melihat "masih ada admin lain" dan platform berakhir tanpa admin; dengan urutan tetap tidak ada deadlock.
		if _, err := tx.Exec(ctx, `SELECT u.id FROM users u JOIN roles r ON r.tenant_id=u.tenant_id AND r.id=u.role_id
WHERE u.tenant_id=$1 AND u.deleted_at IS NULL AND u.status='active' AND r.key='admin' ORDER BY u.id FOR UPDATE OF u`, p.TenantID); err != nil {
			return err
		}
		var cur int
		var curRole, curStatus string
		var reviewRequired bool
		err := tx.QueryRow(ctx, `SELECT u.version, r.key, u.status, u.role_review_required FROM users u JOIN roles r ON r.tenant_id=u.tenant_id AND r.id=u.role_id
WHERE u.tenant_id=$1 AND u.id=$2 AND u.deleted_at IS NULL AND u.kind='human' FOR UPDATE OF u`, p.TenantID, id).Scan(&cur, &curRole, &curStatus, &reviewRequired)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound("User")
		}
		if err != nil {
			return err
		}
		if cur != in.Version {
			return httpx.Conflict("This account was changed by someone else. Reload and try again.")
		}
		newRole, newStatus := curRole, curStatus
		if in.Role != nil {
			newRole = *in.Role
		}
		if in.Status != nil {
			newStatus = *in.Status
		}
		if newStatus != "active" && newStatus != "disabled" {
			return fieldErr("status", "Choose active or disabled.")
		}
		if reviewRequired && newStatus == "active" && !in.AcceptRolePermissions {
			return fieldErr("accept_role_permissions", "Review the selected role permissions and explicitly accept them before enabling this migrated account.")
		}
		if id == p.UserID && (newRole != curRole || newStatus != curStatus) {
			return fieldErr("role", "You can’t change your own role or status. Ask another administrator.")
		}
		// Selalu harus ada minimal satu admin aktif.
		if curRole == "admin" && curStatus == "active" && (newRole != "admin" || newStatus != "active") {
			var others int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM users u JOIN roles r ON r.tenant_id=u.tenant_id AND r.id=u.role_id
WHERE u.tenant_id=$1 AND u.id<>$2 AND u.deleted_at IS NULL AND u.status='active' AND r.key='admin'`, p.TenantID, id).Scan(&others); err != nil {
				return err
			}
			if others == 0 {
				return fieldErr("role", "This is the last active administrator. Make someone else an administrator first.")
			}
		}
		name := ""
		if in.DisplayName != nil {
			name = strings.TrimSpace(*in.DisplayName)
			if name == "" || len([]rune(name)) > 120 {
				return fieldErr("display_name", "Enter a name of 1 to 120 characters.")
			}
		}
		var roleID uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT id FROM roles WHERE tenant_id=$1 AND key=$2`, p.TenantID, newRole).Scan(&roleID); errors.Is(err, pgx.ErrNoRows) {
			return fieldErr("role", "Choose admin, analyst or viewer.")
		} else if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE users SET display_name=COALESCE(NULLIF($3,''), display_name), role_id=$4, status=$5, role_review_required=CASE WHEN $6 THEN false ELSE role_review_required END, previous_role_key=CASE WHEN $6 THEN NULL ELSE previous_role_key END, version=version+1, updated_at=now() WHERE tenant_id=$1 AND id=$2`,
			p.TenantID, id, name, roleID, newStatus, reviewRequired && in.AcceptRolePermissions); err != nil {
			return err
		}
		if newStatus == "disabled" && curStatus != "disabled" {
			// akun dinonaktifkan: semua sesi berakhir saat itu juga
			if _, err := tx.Exec(ctx, `UPDATE sessions SET revoked_at=now() WHERE tenant_id=$1 AND user_id=$2 AND revoked_at IS NULL`, p.TenantID, id); err != nil {
				return err
			}
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "user.update", ResourceType: "user", ResourceID: &id, IP: ip, RequestID: reqID,
			Detail: map[string]any{"role": newRole, "status": newStatus, "role_review_accepted": reviewRequired && in.AcceptRolePermissions}})
	})
	if err != nil {
		return nil, err
	}
	rows, err := s.List(ctx, p)
	if err != nil {
		return nil, err
	}
	for _, u := range rows {
		if u.ID == id {
			return &u, nil
		}
	}
	return nil, httpx.NotFound("User")
}

// ---------- link reset ----------

func (s *Service) newLink(ctx context.Context, tx pgx.Tx, p *auth.Principal, user uuid.UUID) (*ResetLink, error) {
	tok, err := newToken(resetPrefix)
	if err != nil {
		return nil, err
	}
	// Hanya satu link aktif per akun: link lama yang belum dipakai dihapus.
	if _, err := tx.Exec(ctx, `DELETE FROM password_reset_links WHERE tenant_id=$1 AND user_id=$2 AND used_at IS NULL`, p.TenantID, user); err != nil {
		return nil, err
	}
	exp := s.now().Add(ResetTTL)
	if _, err := tx.Exec(ctx, `INSERT INTO password_reset_links (id, tenant_id, user_id, created_by, token_hash, expires_at) VALUES ($1,$2,$3,$4,$5,$6)`,
		ids.New(), p.TenantID, user, p.UserID, auth.HashToken(tok), exp); err != nil {
		return nil, err
	}
	return &ResetLink{Token: tok, ExpiresAt: exp}, nil
}

func (s *Service) CreateResetLink(ctx context.Context, p *auth.Principal, id uuid.UUID, ip, reqID string) (*ResetLink, error) {
	var out *ResetLink
	err := s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		var status string
		err := tx.QueryRow(ctx, `SELECT status FROM users WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL AND kind='human'`, p.TenantID, id).Scan(&status)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound("User")
		}
		if err != nil {
			return err
		}
		if status != "active" {
			return httpx.Conflict("This account is disabled. Enable it before creating a reset link.")
		}
		out, err = s.newLink(ctx, tx, p, id)
		if err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "user.reset_link", ResourceType: "user", ResourceID: &id, IP: ip, RequestID: reqID})
	})
	return out, err
}

func errBadLink() error {
	return httpx.NewProblem(400, "Link not valid", "This reset link is invalid, already used, or has expired. Ask your administrator for a new one.")
}

// CheckLink memberi tahu form apakah link masih berlaku (tanpa mengungkap apa pun tentang akun).
func (s *Service) CheckLink(ctx context.Context, token string) error {
	return s.DB.InTenantRead(ctx, s.Tenant, func(tx pgx.Tx) error {
		var ok bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM password_reset_links WHERE tenant_id=$1 AND token_hash=$2 AND NOT is_demo AND used_at IS NULL AND expires_at > $3)`,
			s.Tenant, auth.HashToken(token), s.now()).Scan(&ok); err != nil {
			return err
		}
		if !ok {
			return errBadLink()
		}
		return nil
	})
}

// UseLink mengganti password lewat link sekali pakai. Semua sesi akun berakhir dan penguncian login dibersihkan.
func (s *Service) UseLink(ctx context.Context, token, password, ip string) error {
	if msg := auth.CheckPolicy(password); msg != "" {
		return fieldErr("password", msg)
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	return s.DB.InTenant(ctx, s.Tenant, func(tx pgx.Tx) error {
		var linkID, uid uuid.UUID
		var email string
		err := tx.QueryRow(ctx, `
SELECT l.id, l.user_id, u.email::text FROM password_reset_links l JOIN users u ON u.tenant_id=l.tenant_id AND u.id=l.user_id
WHERE l.tenant_id=$1 AND l.token_hash=$2 AND NOT l.is_demo AND NOT u.is_demo AND l.used_at IS NULL AND l.expires_at > $3 AND u.status='active' AND u.deleted_at IS NULL
FOR UPDATE OF l`, s.Tenant, auth.HashToken(token), s.now()).Scan(&linkID, &uid, &email)
		if errors.Is(err, pgx.ErrNoRows) {
			return errBadLink()
		}
		if err != nil {
			return err
		}
		if err := s.setPassword(ctx, tx, s.Tenant, uid, password, hash); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE password_reset_links SET used_at=now() WHERE id=$1`, linkID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE sessions SET revoked_at=now() WHERE tenant_id=$1 AND user_id=$2 AND revoked_at IS NULL`, s.Tenant, uid); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM login_throttle WHERE tenant_id=$1 AND email=$2`, s.Tenant, email); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: s.Tenant, ActorUser: &uid, Action: "user.password_reset", ResourceType: "user", ResourceID: &uid, IP: ip})
	})
}

// setPassword menyimpan hash baru dan menolak password yang sama dengan 5 sebelumnya (termasuk yang sekarang).
func (s *Service) setPassword(ctx context.Context, tx pgx.Tx, tenant, uid uuid.UUID, plain, hash string) error {
	var current *string
	if err := tx.QueryRow(ctx, `SELECT password_hash FROM users WHERE tenant_id=$1 AND id=$2`, tenant, uid).Scan(&current); err != nil {
		return err
	}
	prev := []string{}
	if current != nil {
		prev = append(prev, *current)
	}
	rows, err := tx.Query(ctx, `SELECT password_hash FROM password_history WHERE tenant_id=$1 AND user_id=$2 ORDER BY created_at DESC LIMIT $3`, tenant, uid, historyDepth)
	if err != nil {
		return err
	}
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			rows.Close()
			return err
		}
		prev = append(prev, h)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, h := range prev {
		if auth.VerifyPassword(plain, h) {
			return fieldErr("password", "Choose a password you haven’t used in your last five.")
		}
	}
	if current != nil {
		if _, err := tx.Exec(ctx, `INSERT INTO password_history (id, tenant_id, user_id, password_hash) VALUES ($1,$2,$3,$4)`, ids.New(), tenant, uid, *current); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM password_history WHERE tenant_id=$1 AND user_id=$2 AND id NOT IN
(SELECT id FROM password_history WHERE tenant_id=$1 AND user_id=$2 ORDER BY created_at DESC LIMIT $3)`, tenant, uid, historyDepth); err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `UPDATE users SET password_hash=$3, version=version+1, updated_at=now() WHERE tenant_id=$1 AND id=$2`, tenant, uid, hash)
	return err
}

// Forgot: respons sama untuk email yang ada maupun tidak. Bila akunnya ada, admin mendapat alert (satu per akun per jam).
func (s *Service) Forgot(ctx context.Context, email string) error {
	email = strings.TrimSpace(email)
	if !validEmail(email) {
		return nil
	}
	return s.DB.InTenant(ctx, s.Tenant, func(tx pgx.Tx) error {
		var uid uuid.UUID
		var name string
		err := tx.QueryRow(ctx, `SELECT id, display_name FROM users WHERE tenant_id=$1 AND email=$2 AND status='active' AND deleted_at IS NULL AND kind='human'`, s.Tenant, email).Scan(&uid, &name)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		hour := s.now().UTC().Format("2006010215")
		return alerts.Raise(ctx, tx, s.Tenant, alerts.New{Kind: "password_reset_requested", Severity: "warning", Title: name + " asked for a password reset",
			Body: "Create a reset link for " + email + " in Settings → Users and send it to them.", ResourceType: "user", ResourceID: &uid,
			Audience: authz.UserManage, DedupeKey: "reset-req:" + uid.String() + ":" + hour})
	})
}

// ---------- profil ----------

func (s *Service) UpdateProfile(ctx context.Context, p *auth.Principal, name string) error {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 120 {
		return fieldErr("display_name", "Enter a name of 1 to 120 characters.")
	}
	return s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE users SET display_name=$3, version=version+1, updated_at=now() WHERE tenant_id=$1 AND id=$2`, p.TenantID, p.UserID, name)
		return err
	})
}

func (s *Service) ChangePassword(ctx context.Context, p *auth.Principal, current, next, ip string) error {
	if msg := auth.CheckPolicy(next); msg != "" {
		return fieldErr("new_password", msg)
	}
	hash, err := auth.HashPassword(next)
	if err != nil {
		return err
	}
	return s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		var cur *string
		if err := tx.QueryRow(ctx, `SELECT password_hash FROM users WHERE tenant_id=$1 AND id=$2`, p.TenantID, p.UserID).Scan(&cur); err != nil {
			return err
		}
		if cur == nil || !auth.VerifyPassword(current, *cur) {
			return fieldErr("current_password", "That isn’t your current password.")
		}
		if err := s.setPassword(ctx, tx, p.TenantID, p.UserID, next, hash); err != nil {
			var pr *httpx.Problem
			if errors.As(err, &pr) && len(pr.Fields) > 0 {
				return fieldErr("new_password", pr.Fields[0].Message)
			}
			return err
		}
		// sesi lain berakhir; sesi yang sedang dipakai tetap hidup
		if _, err := tx.Exec(ctx, `UPDATE sessions SET revoked_at=now() WHERE tenant_id=$1 AND user_id=$2 AND revoked_at IS NULL AND id<>$3`, p.TenantID, p.UserID, p.SessionID); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "user.password_change", ResourceType: "user", ResourceID: &p.UserID, IP: ip})
	})
}

// ---------- token API ----------

type Token struct {
	IsDemo     bool       `json:"is_demo"`
	RevokedAt  *time.Time `json:"revoked_at"`
	ID         uuid.UUID  `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	ExpiresAt  *time.Time `json:"expires_at"`
}

func (s *Service) Tokens(ctx context.Context, p *auth.Principal) ([]Token, error) {
	out := []Token{}
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id, name, token_prefix, created_at, last_used_at, expires_at, is_demo, revoked_at FROM api_tokens
WHERE tenant_id=$1 AND user_id=$2 AND (is_demo OR (revoked_at IS NULL AND (expires_at IS NULL OR expires_at > now()))) ORDER BY created_at DESC`, p.TenantID, p.UserID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var t Token
			if err := rows.Scan(&t.ID, &t.Name, &t.Prefix, &t.CreatedAt, &t.LastUsedAt, &t.ExpiresAt, &t.IsDemo, &t.RevokedAt); err != nil {
				return err
			}
			out = append(out, t)
		}
		return rows.Err()
	})
	return out, err
}

type NewToken struct {
	Token
	Secret string `json:"secret"` // hanya tampil sekali
}

func (s *Service) CreateToken(ctx context.Context, p *auth.Principal, name string, days int, ip, reqID string) (*NewToken, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 80 {
		return nil, fieldErr("name", "Enter a name of 1 to 80 characters.")
	}
	if days == 0 {
		days = 90
	}
	if days < 1 || days > 365 {
		return nil, fieldErr("expires_in_days", "Choose between 1 and 365 days.")
	}
	secret, err := newToken(auth.APITokenPrefix)
	if err != nil {
		return nil, err
	}
	id := ids.New()
	exp := s.now().Add(time.Duration(days) * 24 * time.Hour)
	out := &NewToken{Secret: secret}
	err = s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM api_tokens WHERE tenant_id=$1 AND user_id=$2 AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > now())`, p.TenantID, p.UserID).Scan(&n); err != nil {
			return err
		}
		if n >= 10 {
			return fieldErr("name", "You already have 10 active tokens. Revoke one first.")
		}
		if _, err := tx.Exec(ctx, `INSERT INTO api_tokens (id, tenant_id, user_id, name, token_prefix, token_hash, expires_at) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			id, p.TenantID, p.UserID, name, secret[:8], auth.HashToken(secret), exp); err != nil {
			return err
		}
		out.Token = Token{ID: id, Name: name, Prefix: secret[:8], CreatedAt: s.now(), ExpiresAt: &exp}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "api_token.create", ResourceType: "api_token", ResourceID: &id, IP: ip, RequestID: reqID, Detail: map[string]any{"name": name}})
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) RevokeToken(ctx context.Context, p *auth.Principal, id uuid.UUID, ip, reqID string) error {
	return s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE api_tokens SET revoked_at=now() WHERE tenant_id=$1 AND id=$2 AND user_id=$3 AND revoked_at IS NULL`, p.TenantID, id, p.UserID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return httpx.NotFound("Token")
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "api_token.revoke", ResourceType: "api_token", ResourceID: &id, IP: ip, RequestID: reqID})
	})
}
