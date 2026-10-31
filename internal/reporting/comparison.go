package reporting

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Hanya pasangan aset dengan cakupan yang sama dan SEMUA aksi sukses boleh menyimpulkan temuan teratasi.
func Compare(current, previous Snapshot) string {
	if current.Profile != previous.Profile {
		return "Comparison unavailable: assessment profiles differ."
	}
	old := map[string]Asset{}
	for _, a := range previous.Assets {
		old[a.ID.String()] = a
	}
	fresh, resolved, persistent, compared := 0, 0, 0, 0
	eligible := func(a Asset) bool {
		if a.Status != "finished" || a.SkipReason != "" || len(a.Methods) == 0 {
			return false
		}
		for _, m := range a.Methods {
			if m.Outcome != "success" {
				return false
			}
		}
		return true
	}
	for _, a := range current.Assets {
		b, ok := old[a.ID.String()]
		if !ok || !eligible(a) || !eligible(b) || len(a.Methods) != len(b.Methods) {
			continue
		}
		same := true
		for i, m := range a.Methods {
			n := b.Methods[i]
			var mp, np any
			_ = json.Unmarshal(m.Params, &mp)
			_ = json.Unmarshal(n.Params, &np)
			mb, _ := json.Marshal(mp)
			nb, _ := json.Marshal(np)
			if m.Action != n.Action || m.Version != n.Version || m.Target != n.Target || !bytes.Equal(mb, nb) {
				same = false
			}
		}
		if !same {
			continue
		}
		compared++
		seen := map[string]bool{}
		for _, f := range b.Findings {
			seen[f.ID] = true
		}
		for _, f := range a.Findings {
			if seen[f.ID] {
				persistent++
				delete(seen, f.ID)
			} else {
				fresh++
			}
		}
		resolved += len(seen)
	}
	if compared == 0 {
		return "Comparison unavailable: no assets have complete, comparable scan coverage."
	}
	return fmt.Sprintf("Compared %d fully tested assets: %d new, %d persistent, %d resolved findings. Skipped, failed and differently scoped assets are excluded.", compared, fresh, persistent, resolved)
}
