package endpoint

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"orchis-go/internal/httpx"
)

func validateSettingVersions(keys []string, expected map[string]int64) error {
	if len(expected) != len(keys) {
		return httpx.Validation(httpx.FieldError{Name: "versions", Message: "Reload the collection choices before changing them."})
	}
	for _, key := range keys {
		if version, known := expected[key]; !known || version < 0 {
			return httpx.Validation(httpx.FieldError{Name: "versions", Message: "Provide the current version of every affected query."})
		}
	}
	return nil
}

// Organisasi harus dikunci pemanggil sebelum pemeriksaan dan penulisan pilihan.
// Tidak ada baris berarti versi0; konflik satu query membatalkan seluruh perubahan pack.
func checkSettingVersions(ctx context.Context, tx pgx.Tx, tenant, org uuid.UUID, keys []string, expected map[string]int64) error {
	for _, key := range keys {
		var current int64
		err := tx.QueryRow(ctx, `SELECT version FROM osquery_org_settings WHERE tenant_id=$1 AND organization_id=$2 AND query_key=$3`, tenant, org, key).Scan(&current)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if current != expected[key] {
			return httpx.Conflict("Collection choices changed; reload before saving.")
		}
	}
	return nil
}
