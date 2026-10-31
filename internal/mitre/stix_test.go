package mitre

import (
	"os"
	"strings"
	"testing"
)

func load(t *testing.T) *Dataset {
	t.Helper()
	f, err := os.Open("testdata/bundle.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	ds, err := Parse(f)
	if err != nil {
		t.Fatal(err)
	}
	return ds
}

func TestParseKeepsLiveTechniquesAndOrdersTactics(t *testing.T) {
	ds := load(t)
	if ds.Version != "99.0" {
		t.Errorf("versi: %q", ds.Version)
	}
	var order []string
	for _, x := range ds.Tactics {
		order = append(order, x.Shortname)
	}
	if strings.Join(order, ",") != "reconnaissance,initial-access,execution,discovery" {
		t.Errorf("urutan taktik kanonik: %v", order)
	}
	byID := map[string]Technique{}
	for _, x := range ds.Techniques {
		byID[x.ID] = x
	}
	if _, ok := byID["T1000"]; ok {
		t.Error("teknik revoked tidak boleh diimpor")
	}
	if !byID["T1001"].Deprecated {
		t.Error("teknik deprecated harus ditandai")
	}
	if byID["T1595.002"].ParentID != "T1595" || byID["T1595"].ParentID != "" {
		t.Errorf("induk sub-teknik: %+v", byID["T1595.002"])
	}
	if got := strings.Join(byID["T1133"].TacticIDs, ","); got != "TA0001" {
		t.Errorf("fase tanpa taktik dikenal diabaikan, dapat %q", got)
	}
	if got := strings.Join(byID["T1046"].TacticIDs, ","); got != "TA0007" {
		t.Errorf("taktik T1046: %q", got)
	}
	// induk selalu mendahului anak (kunci asing saat insert berurutan)
	seen := map[string]bool{}
	for _, x := range ds.Techniques {
		if x.ParentID != "" && !seen[x.ParentID] {
			t.Errorf("%s muncul sebelum induknya %s", x.ID, x.ParentID)
		}
		seen[x.ID] = true
	}
}

func TestParseRejectsGarbageAndImplausibleBundles(t *testing.T) {
	for name, in := range map[string]string{
		"bukan json":          "not json",
		"larik":               "[1,2,3]",
		"tanpa objects":       `{"type":"bundle"}`,
		"objects bukan larik": `{"objects":5}`,
		"objek rusak":         `{"objects":[{"type":"attack-pattern","name":5}]}`,
		"terlalu kecil":       `{"objects":[{"type":"x-mitre-tactic","id":"a","x_mitre_shortname":"discovery","external_references":[{"source_name":"mitre-attack","external_id":"TA0007"}]}]}`,
		"terpotong":           `{"objects":[{"type":"attack-pattern"`,
	} {
		if _, err := Parse(strings.NewReader(in)); err == nil {
			t.Errorf("%s: harus ditolak", name)
		}
	}
}
