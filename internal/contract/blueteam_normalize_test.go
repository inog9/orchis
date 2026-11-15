package contract

import "testing"

// Kontrak blueteam harus lolos Normalize (dipakai saat agent mengklaim tugas); sebelumnya jatuh ke "Unknown contract".
func TestNormalizeAcceptsBlueteamContracts(t *testing.T) {
	for _, key := range []string{KeyBlueteamConnections, KeyBlueteamAnalyze} {
		if Find(key) == nil {
			t.Fatalf("%s tidak terdaftar", key)
		}
		if _, err := Normalize(key, []byte(`{}`)); err != nil {
			t.Fatalf("%s: %v", key, err)
		}
	}
}
