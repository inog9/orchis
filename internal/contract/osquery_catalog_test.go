package contract

import (
	"testing"

	"orchis-go/internal/osquery"
)

// Setiap query katalog osquery harus punya kontrak: tanpanya server menganggapnya aksi jaringan dan menolak aset agent.
func TestEveryOsqueryCatalogQueryHasAnAgentContract(t *testing.T) {
	for _, q := range osquery.Queries {
		d := Find(q.Key)
		if d == nil {
			t.Errorf("tidak ada kontrak untuk %s", q.Key)
			continue
		}
		if !d.NeedsAgent {
			t.Errorf("%s harus berjalan di agent", q.Key)
		}
	}
}
