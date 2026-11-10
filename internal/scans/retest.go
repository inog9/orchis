package scans

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"orchis-go/internal/auth"
	"orchis-go/internal/httpx"
)

// insideMethods adalah metode bukti yang berarti temuan datang dari sisi dalam (orchis-defense-agent);
// metode lain berarti sisi luar (pipeline Hackbot). Retest menjalankan ulang HANYA metode yang menemukan
// finding, pada aset yang sama. Hasil "resolved / still present" dibaca dari perbandingan laporan bawaan
// (aturan konsistensi pelaporan: cakupan parsial tidak pernah mengklaim resolved).
var insideMethods = map[string]bool{"osv-distro": true, "osv-exact": true, "nvd-cpe": true}

func (s *Service) now() time.Time {
	if s.Now == nil {
		return time.Now()
	}
	return s.Now()
}

// Retest membuat scan baru untuk satu aset dengan engine yang sama seperti asal finding.
// Key idempotensi bawaan mengikat satu detik sehingga klik ganda tidak menggandakan run.
func (s *Service) Retest(ctx context.Context, p *auth.Principal, finding uuid.UUID, key, ip, req string) (*Scan, error) {
	if key == "" {
		key = "retest-" + finding.String() + "-" + strconv.FormatInt(s.now().Unix(), 10)
	}
	var org uuid.UUID
	var asset *uuid.UUID
	var inside bool
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		e := tx.QueryRow(ctx, `SELECT COALESCE(f.organization_id,a.organization_id), f.asset_id FROM findings f LEFT JOIN assets a ON a.tenant_id=f.tenant_id AND a.id=f.asset_id WHERE f.tenant_id=$1 AND f.id=$2`, p.TenantID, finding).Scan(&org, &asset)
		if errors.Is(e, pgx.ErrNoRows) {
			return httpx.NotFound("Finding")
		}
		if e != nil {
			return e
		}
		if asset == nil || org == uuid.Nil {
			return invalid("finding_id", "This finding is not tied to an organisation asset, so there is nothing to retest.")
		}
		var method *string
		if e := tx.QueryRow(ctx, `SELECT o.details->>'method' FROM finding_occurrences o WHERE o.tenant_id=$1 AND o.finding_id=$2 AND o.details ? 'method' ORDER BY o.seen_at DESC LIMIT 1`, p.TenantID, finding).Scan(&method); e == nil {
			// metode terakhir dari occurrence
		} else if !errors.Is(e, pgx.ErrNoRows) {
			return e
		} else if e := tx.QueryRow(ctx, `SELECT f.details->>'method' FROM findings f WHERE f.tenant_id=$1 AND f.id=$2 AND f.details ? 'method'`, p.TenantID, finding).Scan(&method); e != nil && !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		inside = method != nil && insideMethods[*method]
		return nil
	})
	if err != nil {
		return nil, err
	}
	language := p.UILanguage
	if language == "" {
		language = "en"
	}
	in := Input{Language: language, OrganizationID: org, AssetIDs: []uuid.UUID{*asset}}
	if inside {
		return s.launchDefense(ctx, p, in, language, key, ip, req)
	}
	return s.Launch(ctx, p, in, key, ip, req)
}
