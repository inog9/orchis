package mitre

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"orchis-go/internal/osquery"
)

const frozenDir = "../../deploy/mitre"

// frozen memuat bundel beku di deploy/mitre. Bundelnya tidak ikut git (hash dipatok di manifest); tes ini dilewati
// bila belum diambil, dan make mitre-update memulihkannya.
func frozen(t *testing.T) (*Manifest, *Dataset) {
	t.Helper()
	m, err := LoadManifest(frozenDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(frozenDir, m.File)); err != nil {
		t.Skipf("bundel beku belum ada; jalankan make mitre-update VERSION=%s", m.Version)
	}
	m, ds, err := Verify(frozenDir)
	if err != nil {
		t.Fatal(err)
	}
	return m, ds
}

func TestFrozenBundleMatchesManifestAndSource(t *testing.T) {
	m, ds := frozen(t)
	c := ds.Count()
	// Angka-angka ini dicocokkan dengan sumber resmi MITRE v20.0 dan dengan OpenAEV (718 = 227+479+12 deprecated).
	if m.Version == "20.0" && (c.Tactics != 15 || c.Parents != 227 || c.Subs != 479 || c.Deprecated != 12 || ds.Revoked != 149) {
		t.Fatalf("hitungan v20.0 tidak sesuai sumber: %+v revoked=%d", c, ds.Revoked)
	}
	if len(ds.Techniques) != c.Parents+c.Subs+c.Deprecated {
		t.Fatalf("jumlah teknik tidak konsisten: %d", len(ds.Techniques))
	}
}

func TestFrozenTacticsAreOfficialOrderAndComplete(t *testing.T) {
	m, ds := frozen(t)
	if m.Version != "20.0" {
		t.Skip("urutan resmi diperiksa untuk v20.0")
	}
	var got []string
	for _, x := range ds.Tactics {
		got = append(got, x.Shortname)
	}
	want := "reconnaissance,resource-development,initial-access,execution,persistence,privilege-escalation,stealth,defense-impairment,credential-access,discovery,lateral-movement,collection,command-and-control,exfiltration,impact"
	if strings.Join(got, ",") != want {
		t.Fatalf("urutan taktik salah:\n got %s\nwant %s", strings.Join(got, ","), want)
	}
	for i, x := range ds.Tactics {
		if x.Order != i {
			t.Fatalf("sort_order %s = %d, harus %d", x.ID, x.Order, i)
		}
	}
}

func TestFrozenHierarchyTacticsAndNoRevoked(t *testing.T) {
	_, ds := frozen(t)
	tactics := map[string]bool{}
	for _, x := range ds.Tactics {
		if tactics[x.ID] {
			t.Fatalf("taktik ganda %s", x.ID)
		}
		tactics[x.ID] = true
	}
	have := map[string]Technique{}
	for _, x := range ds.Techniques {
		if _, dup := have[x.ID]; dup {
			t.Fatalf("teknik ganda %s", x.ID)
		}
		have[x.ID] = x
	}
	idRe := regexp.MustCompile(`^T[0-9]{4}(\.[0-9]{3})?$`)
	for _, x := range ds.Techniques {
		if !idRe.MatchString(x.ID) {
			t.Errorf("format ID salah: %s", x.ID)
		}
		if x.ParentID != "" {
			p, ok := have[x.ParentID]
			if !ok || !strings.HasPrefix(x.ID, x.ParentID+".") || p.ParentID != "" {
				t.Errorf("sub-teknik %s: induk %q tidak sah", x.ID, x.ParentID)
			}
		} else if strings.Contains(x.ID, ".") {
			t.Errorf("sub-teknik %s kehilangan induknya (yatim)", x.ID)
		}
		if len(x.TacticIDs) == 0 && x.ParentID == "" && !x.Deprecated {
			t.Errorf("teknik aktif %s tanpa taktik", x.ID)
		}
		for _, ta := range x.TacticIDs {
			if !tactics[ta] {
				t.Errorf("%s menunjuk taktik tak dikenal %s", x.ID, ta)
			}
		}
		if strings.Contains(x.Description, "(Citation:") {
			t.Errorf("%s: sitasi inline tidak dibersihkan", x.ID)
		}
		if !strings.HasPrefix(x.URL, "https://attack.mitre.org/techniques/") {
			t.Errorf("%s: tautan sumber salah %q", x.ID, x.URL)
		}
	}
	// Satu teknik bisa berada di banyak taktik (Valid Accounts).
	if va, ok := have["T1078"]; !ok || len(va.TacticIDs) < 3 {
		t.Errorf("T1078 harus berada di beberapa taktik: %+v", have["T1078"].TacticIDs)
	}
}

// Silang dengan relasi resmi "subtechnique-of" dan daftar revoked di bundel mentah.
func TestFrozenAgainstRawRelationshipsAndRevoked(t *testing.T) {
	m, ds := frozen(t)
	raw, err := os.ReadFile(filepath.Join(frozenDir, m.File))
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	var generic struct {
		Objects []map[string]json.RawMessage `json:"objects"`
	}
	if err := dec.Decode(&generic); err != nil {
		t.Fatal(err)
	}
	stixToID := map[string]string{}
	revokedIDs := map[string]bool{}
	type rel struct{ src, dst string }
	var subs []rel
	for _, o := range generic.Objects {
		var typ, id, rt, sr, tr string
		var revoked bool
		_ = json.Unmarshal(o["type"], &typ)
		_ = json.Unmarshal(o["id"], &id)
		_ = json.Unmarshal(o["revoked"], &revoked)
		switch typ {
		case "attack-pattern":
			var refs []struct {
				Source string `json:"source_name"`
				ID     string `json:"external_id"`
			}
			_ = json.Unmarshal(o["external_references"], &refs)
			for _, r := range refs {
				if r.Source == "mitre-attack" {
					stixToID[id] = r.ID
					if revoked {
						revokedIDs[r.ID] = true
					}
				}
			}
		case "relationship":
			_ = json.Unmarshal(o["relationship_type"], &rt)
			_ = json.Unmarshal(o["source_ref"], &sr)
			_ = json.Unmarshal(o["target_ref"], &tr)
			if rt == "subtechnique-of" && !revoked {
				subs = append(subs, rel{sr, tr})
			}
		}
	}
	have := map[string]Technique{}
	for _, x := range ds.Techniques {
		have[x.ID] = x
	}
	for id := range revokedIDs {
		if x, ok := have[id]; ok {
			t.Errorf("teknik revoked %s masih dimuat (%s)", id, x.Name)
		}
	}
	checked := 0
	for _, r := range subs {
		child, parent := stixToID[r.src], stixToID[r.dst]
		if revokedIDs[child] || revokedIDs[parent] {
			continue
		}
		x, ok := have[child]
		if !ok {
			t.Errorf("sub-teknik %s dari relasi resmi tidak dimuat", child)
			continue
		}
		if x.ParentID != parent {
			t.Errorf("%s: induk dimuat %q, relasi resmi %q", child, x.ParentID, parent)
		}
		checked++
	}
	if checked < 450 {
		t.Fatalf("hanya %d relasi sub-teknik diperiksa; pemeriksaan tidak masuk akal", checked)
	}
}

func TestFrozenMitigationsAttachedToTechniques(t *testing.T) {
	_, ds := frozen(t)
	withMit := 0
	for _, x := range ds.Techniques {
		if len(x.Mitigations) > 0 {
			withMit++
		}
		for _, mt := range x.Mitigations {
			if !regexp.MustCompile(`^M[0-9]{4}$`).MatchString(mt.ID) || mt.Name == "" {
				t.Errorf("%s: mitigasi tidak sah %+v", x.ID, mt)
			}
		}
	}
	if withMit < 500 {
		t.Fatalf("hanya %d teknik bermitigasi", withMit)
	}
}

// Setiap pemetaan aksi->teknik harus menunjuk teknik AKTIF di bundel beku, beralasan, dan tidak berlebihan.
func TestDefaultLinksAreValidReasonedAndConservative(t *testing.T) {
	_, ds := frozen(t)
	have := map[string]Technique{}
	for _, x := range ds.Techniques {
		have[x.ID] = x
	}
	seen := map[string]bool{}
	for _, l := range DefaultLinks {
		k := l.Contract + "|" + l.Technique
		if seen[k] {
			t.Errorf("tautan ganda %s", k)
		}
		seen[k] = true
		x, ok := have[l.Technique]
		if !ok || x.Deprecated {
			t.Errorf("%s -> %s: teknik tidak ada/deprecated di bundel beku", l.Contract, l.Technique)
		}
		if len(strings.TrimSpace(l.Reason)) < 30 {
			t.Errorf("%s -> %s: alasan terlalu pendek", l.Contract, l.Technique)
		}
		if l.Basis != "direct" && l.Basis != "estimate" {
			t.Errorf("%s -> %s: basis %q", l.Contract, l.Technique, l.Basis)
		}
		if l.Technique == "T1190" {
			t.Errorf("%s dipetakan ke T1190: deteksi kerentanan bukan eksploitasi (cakupan berlebihan)", l.Contract)
		}
	}
	want := 4
	for _, q := range osquery.Queries {
		want += len(q.Mitre)
	}
	if len(DefaultLinks) != want {
		t.Fatalf("setiap aksi bawaan harus punya pemetaan (%d diharapkan), ada %d", want, len(DefaultLinks))
	}
	for _, q := range osquery.Queries {
		linked := false
		for _, l := range DefaultLinks {
			linked = linked || l.Contract == q.Key
		}
		if !linked {
			t.Errorf("aksi %s tanpa pemetaan MITRE", q.Key)
		}
	}
}

func TestParseVerifiedRejectsWrongHash(t *testing.T) {
	raw, err := os.ReadFile("testdata/bundle.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseVerified(bytes.NewReader(raw), strings.Repeat("0", 64)); err == nil || !strings.Contains(err.Error(), "hash bundel tidak cocok") {
		t.Fatalf("hash salah harus ditolak: %v", err)
	}
	ds, err := ParseVerified(bytes.NewReader(raw), "")
	if err != nil || len(ds.SHA256) != 64 {
		t.Fatalf("hash harus dihitung: %v %q", err, ds.SHA256)
	}
	if _, err := ParseVerified(bytes.NewReader(raw), strings.ToUpper(ds.SHA256)); err != nil {
		t.Fatalf("hash benar (huruf besar) harus diterima: %v", err)
	}
}

func TestUpdateFreezesShowsDiffAndVerifyDetectsTamper(t *testing.T) {
	dir := t.TempDir()
	raw, err := os.ReadFile("testdata/bundle.json")
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(t.TempDir(), "in.json")
	if err := os.WriteFile(src, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	m, _, err := Update(context.Background(), dir, "99.0", src, &out)
	if err != nil {
		t.Fatal(err)
	}
	if m.Counts.Techniques == 0 || len(m.SHA256) != 64 {
		t.Fatalf("manifest: %+v", m)
	}
	if _, _, err := Verify(dir); err != nil {
		t.Fatalf("verifikasi bundel baru dibekukan: %v", err)
	}
	// Versi tidak cocok ditolak.
	if _, _, err := Update(context.Background(), t.TempDir(), "98.0", src, &out); err == nil {
		t.Fatal("versi bundel berbeda dari yang diminta harus ditolak")
	}
	// Bundel versi 99.1 dengan satu teknik diubah namanya: diff harus menampilkannya, dan berkas versi lama dihapus.
	changed := strings.Replace(strings.Replace(string(raw), `"x_mitre_version": "99.0"`, `"x_mitre_version": "99.1"`, 1), `"Network Service Discovery"`, `"Network Service Discovery RENAMED"`, 1)
	if changed == string(raw) {
		t.Skip("fixture tidak memuat teks yang diubah")
	}
	if err := os.WriteFile(src, []byte(changed), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if _, diff, err := Update(context.Background(), dir, "99.1", src, &out); err != nil {
		t.Fatalf("%v (%s)", err, out.String())
	} else if len(diff.Changed) != 1 || diff.Changed[0] != "T1046" || !strings.Contains(out.String(), "berubah") {
		t.Fatalf("diff salah: %+v\n%s", diff, out.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "enterprise-attack-99.0.json")); err == nil {
		t.Fatal("berkas versi lama harus dihapus setelah versi baru dibekukan")
	}
	// Berkas diubah diam-diam: verifikasi menolak.
	p := filepath.Join(dir, "enterprise-attack-99.1.json")
	b, _ := os.ReadFile(p)
	if err := os.WriteFile(p, bytes.Replace(b, []byte("Discovery"), []byte("Dizcovery"), 1), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Verify(dir); err == nil {
		t.Fatal("bundel yang diubah harus gagal verifikasi")
	}
}

// Urutan kolom harus mengikuti objek x-mitre-matrix di bundel, bukan daftar cadangan di kode: taktik baru yang belum
// dikenal kode (seperti defense-impairment dulu) tetap harus berada di tempat resmi.
func TestTacticOrderFollowsBundleMatrixNotFallback(t *testing.T) {
	obj := func(m map[string]any) json.RawMessage { b, _ := json.Marshal(m); return b }
	ref := func(id string) []map[string]string {
		return []map[string]string{{"source_name": "mitre-attack", "external_id": id, "url": "https://attack.mitre.org/x/" + id}}
	}
	objs := []json.RawMessage{
		obj(map[string]any{"type": "x-mitre-collection", "id": "x-mitre-collection--1", "x_mitre_version": "1.0"}),
		obj(map[string]any{"type": "x-mitre-tactic", "id": "x-mitre-tactic--a", "name": "Impact", "x_mitre_shortname": "impact", "external_references": ref("TA0040")}),
		obj(map[string]any{"type": "x-mitre-tactic", "id": "x-mitre-tactic--b", "name": "Brand New", "x_mitre_shortname": "brand-new", "external_references": ref("TA0900")}),
		obj(map[string]any{"type": "x-mitre-tactic", "id": "x-mitre-tactic--c", "name": "Reconnaissance", "x_mitre_shortname": "reconnaissance", "external_references": ref("TA0043")}),
		// Resmi: Brand New, Impact, Reconnaissance (berlawanan dengan urutan cadangan: reconnaissance ... impact, brand-new di belakang).
		obj(map[string]any{"type": "x-mitre-matrix", "id": "x-mitre-matrix--1", "tactic_refs": []string{"x-mitre-tactic--b", "x-mitre-tactic--a", "x-mitre-tactic--c"}}),
	}
	for i := 0; i < 12; i++ {
		id := "T" + strings.Repeat("0", 3) + string(rune('1'+i%9)) + string(rune('0'+i/9))
		objs = append(objs, obj(map[string]any{"type": "attack-pattern", "id": "attack-pattern--" + id, "name": "Tech " + id,
			"kill_chain_phases":   []map[string]string{{"kill_chain_name": "mitre-attack", "phase_name": "impact"}},
			"external_references": ref(id)}))
	}
	raw, _ := json.Marshal(map[string]any{"type": "bundle", "objects": objs})
	ds, err := Parse(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, x := range ds.Tactics {
		got = append(got, x.Shortname)
	}
	if strings.Join(got, ",") != "brand-new,impact,reconnaissance" {
		t.Fatalf("urutan harus mengikuti x-mitre-matrix: %v", got)
	}
}
