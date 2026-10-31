package home

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type OrganizationOverview struct {
	ID               uuid.UUID  `json:"id"`
	Name             string     `json:"name"`
	Assets           int        `json:"assets"`
	Authorized       int        `json:"authorized"`
	Risk             string     `json:"risk"`
	Comparison       string     `json:"comparison"`
	LatestAssessment *uuid.UUID `json:"latest_assessment"`
	LatestStatus     string     `json:"latest_status"`
}

// Satu kueri agregat, tanpa kueri tambahan per organisasi; risiko berasal dari snapshot assessment terakhir.
func organizationOverview(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) ([]OrganizationOverview, error) {
	out := []OrganizationOverview{}
	rows, err := tx.Query(ctx, `WITH asset_counts AS(SELECT organization_id,count(*) AS total,count(*) FILTER(WHERE testing_authorized) AS authorized FROM assets WHERE tenant_id=$1 AND deleted_at IS NULL GROUP BY organization_id)
 SELECT o.id,o.name,COALESCE(c.total,0),COALESCE(c.authorized,0),COALESCE(r.snapshot->>'risk','No data yet'),COALESCE(r.snapshot->>'comparison','No assessment baseline yet.'),a.id,COALESCE(a.status,'not_assessed')
 FROM organizations o LEFT JOIN asset_counts c ON c.organization_id=o.id
 LEFT JOIN LATERAL(SELECT id,status FROM assessments WHERE tenant_id=o.tenant_id AND organization_id=o.id ORDER BY created_at DESC,id DESC LIMIT 1)a ON true
 LEFT JOIN LATERAL(SELECT snapshot FROM reports WHERE tenant_id=o.tenant_id AND organization_id=o.id ORDER BY created_at DESC,id DESC LIMIT 1)r ON true
 WHERE o.tenant_id=$1 ORDER BY o.updated_at DESC,o.id DESC LIMIT 12`, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var o OrganizationOverview
		if err = rows.Scan(&o.ID, &o.Name, &o.Assets, &o.Authorized, &o.Risk, &o.Comparison, &o.LatestAssessment, &o.LatestStatus); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
