// Package mitre: impor data MITRE ATT&CK (STIX 2.1) ke PostgreSQL, penautan aksi ke teknik, dan matriks cakupan.
package mitre

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
)

const (
	maxTechniques  = 5000
	maxDescription = 4000
	minTechniques  = 10 // bundel yang lebih kecil dari ini dianggap rusak/bukan ATT&CK Enterprise
)

type Tactic struct {
	ID        string // TA0007
	StixID    string
	Name      string
	Shortname string
	Order     int
}

// Mitigation: satu mitigasi resmi ATT&CK (M####) beserta penjelasannya untuk satu teknik.
type Mitigation struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type Technique struct {
	ID          string // T1046, T1021.002
	StixID      string
	Name        string
	Description string
	ParentID    string
	TacticIDs   []string
	Platforms   []string
	URL         string
	Mitigations []Mitigation
	Deprecated  bool
}

// Dataset: isi bundel yang berlaku. Objek revoked tidak dimuat; yang deprecated dimuat dengan penanda.
// Counts mencatat apa yang dibaca dari bundel agar bisa dicocokkan dengan manifest (lihat deploy/mitre).
type Dataset struct {
	Version    string
	Tactics    []Tactic
	Techniques []Technique
	Revoked    int    // teknik yang di-revoke (tidak dimuat)
	SHA256     string // hash seluruh bundel (diisi ParseVerified)
}

// Count merangkum jumlah teknik induk/sub-teknik aktif dan yang deprecated.
type Count struct{ Tactics, Parents, Subs, Deprecated int }

func (d *Dataset) Count() Count {
	c := Count{Tactics: len(d.Tactics)}
	for _, t := range d.Techniques {
		switch {
		case t.Deprecated:
			c.Deprecated++
		case t.ParentID != "":
			c.Subs++
		default:
			c.Parents++
		}
	}
	return c
}

var citationRe = regexp.MustCompile(`\s*\(Citation: [^)]*\)`)

// cleanText membuang sitasi inline "(Citation: ...)" yang tidak berguna tanpa daftar rujukan.
func cleanText(s string) string { return strings.TrimSpace(citationRe.ReplaceAllString(s, "")) }

type stixObject struct {
	Type         string   `json:"type"`
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Revoked      bool     `json:"revoked"`
	Deprecated   bool     `json:"x_mitre_deprecated"`
	IsSub        bool     `json:"x_mitre_is_subtechnique"`
	Platforms    []string `json:"x_mitre_platforms"`
	TacticRefs   []string `json:"tactic_refs"`
	RelType      string   `json:"relationship_type"`
	SourceRef    string   `json:"source_ref"`
	TargetRef    string   `json:"target_ref"`
	Shortname    string   `json:"x_mitre_shortname"`
	Version      string   `json:"x_mitre_version"`
	ExternalRefs []struct {
		Source string `json:"source_name"`
		ID     string `json:"external_id"`
		URL    string `json:"url"`
	} `json:"external_references"`
	KillChain []struct {
		Name  string `json:"kill_chain_name"`
		Phase string `json:"phase_name"`
	} `json:"kill_chain_phases"`
}

func (o *stixObject) attackURL() string {
	for _, r := range o.ExternalRefs {
		if r.Source == "mitre-attack" && strings.HasPrefix(r.URL, "https://attack.mitre.org/") {
			return r.URL
		}
	}
	return ""
}

func (o *stixObject) attackID() string {
	for _, r := range o.ExternalRefs {
		if r.Source == "mitre-attack" && r.ID != "" {
			return r.ID
		}
	}
	return ""
}

// kanonik: urutan kolom matriks Enterprise; yang tidak dikenal ditaruh di belakang menurut abjad.
var tacticOrder = []string{"reconnaissance", "resource-development", "initial-access", "execution", "persistence", "privilege-escalation",
	"stealth", "defense-evasion", "defense-impairment", "credential-access", "discovery", "lateral-movement", "collection", "command-and-control", "exfiltration", "impact"}

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// Parse membaca bundel STIX secara streaming (berkas aslinya puluhan MB) dan hanya menyimpan taktik dan teknik yang masih berlaku.
func Parse(r io.Reader) (*Dataset, error) {
	dec := json.NewDecoder(r)
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, fmt.Errorf("bukan bundel STIX (JSON objek diharapkan)")
	}
	ds := &Dataset{}
	found := false
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("JSON rusak: %w", err)
		}
		if key != "objects" {
			var skip json.RawMessage
			if err := dec.Decode(&skip); err != nil {
				return nil, fmt.Errorf("JSON rusak: %w", err)
			}
			continue
		}
		if tok, err := dec.Token(); err != nil || tok != json.Delim('[') {
			return nil, fmt.Errorf("\"objects\" harus berupa larik")
		}
		found = true
		shortToID := map[string]string{}
		var techs []stixObject
		var tactics []stixObject
		var matrixOrder []string // urutan kolom resmi dari objek x-mitre-matrix
		coas := map[string]stixObject{}
		var mitigates []stixObject
		for dec.More() {
			var o stixObject
			if err := dec.Decode(&o); err != nil {
				return nil, fmt.Errorf("objek STIX rusak: %w", err)
			}
			switch o.Type {
			case "x-mitre-tactic":
				if o.attackID() != "" && o.Shortname != "" && !o.Revoked {
					tactics = append(tactics, o)
					shortToID[o.Shortname] = o.attackID()
				}
			case "attack-pattern":
				if o.Revoked {
					ds.Revoked++
				} else if o.attackID() != "" {
					techs = append(techs, o)
				}
			case "x-mitre-matrix":
				if matrixOrder == nil {
					matrixOrder = o.TacticRefs
				}
			case "course-of-action":
				if !o.Revoked && !o.Deprecated && o.attackID() != "" {
					coas[o.ID] = o
				}
			case "relationship":
				if o.RelType == "mitigates" && !o.Revoked {
					mitigates = append(mitigates, o)
				}
			case "x-mitre-collection":
				if ds.Version == "" {
					ds.Version = o.Version
				}
			}
			if len(techs) > maxTechniques {
				return nil, fmt.Errorf("terlalu banyak teknik (> %d)", maxTechniques)
			}
		}
		if _, err := dec.Token(); err != nil {
			return nil, fmt.Errorf("JSON rusak: %w", err)
		}
		pos := func(short string) int {
			for i, s := range tacticOrder {
				if s == short {
					return i
				}
			}
			return len(tacticOrder)
		}
		matrixPos := map[string]int{}
		for i, ref := range matrixOrder {
			matrixPos[ref] = i
		}
		sort.SliceStable(tactics, func(i, j int) bool {
			if mi, ok := matrixPos[tactics[i].ID]; ok {
				if mj, ok2 := matrixPos[tactics[j].ID]; ok2 {
					return mi < mj
				}
			}
			a, b := pos(tactics[i].Shortname), pos(tactics[j].Shortname)
			if a != b {
				return a < b
			}
			return tactics[i].Shortname < tactics[j].Shortname
		})
		for i, t := range tactics {
			ds.Tactics = append(ds.Tactics, Tactic{ID: t.attackID(), StixID: t.ID, Name: t.Name, Shortname: t.Shortname, Order: i})
		}
		mitByTech := map[string][]Mitigation{}
		for _, r := range mitigates {
			if c, ok := coas[r.SourceRef]; ok {
				mitByTech[r.TargetRef] = append(mitByTech[r.TargetRef], Mitigation{ID: c.attackID(), Name: clip(c.Name, 120), Description: clip(cleanText(r.Description), 600)})
			}
		}
		seen := map[string]bool{}
		for _, t := range techs {
			id := t.attackID()
			if seen[id] || !strings.HasPrefix(id, "T") {
				continue
			}
			seen[id] = true
			x := Technique{ID: id, StixID: t.ID, Name: clip(t.Name, 200), Description: clip(cleanText(t.Description), maxDescription), Platforms: t.Platforms,
				URL: t.attackURL(), Deprecated: t.Deprecated}
			ms := mitByTech[t.ID]
			sort.SliceStable(ms, func(i, j int) bool { return ms[i].ID < ms[j].ID })
			x.Mitigations = ms
			if x.Mitigations == nil {
				x.Mitigations = []Mitigation{}
			}
			if x.Platforms == nil {
				x.Platforms = []string{}
			}
			if dot := strings.IndexByte(id, '.'); dot > 0 {
				x.ParentID = id[:dot]
			}
			for _, k := range t.KillChain {
				if k.Name == "mitre-attack" {
					if tid, ok := shortToID[k.Phase]; ok {
						x.TacticIDs = append(x.TacticIDs, tid)
					}
				}
			}
			if x.TacticIDs == nil {
				x.TacticIDs = []string{}
			}
			ds.Techniques = append(ds.Techniques, x)
		}
	}
	if !found {
		return nil, fmt.Errorf("bundel STIX tanpa \"objects\"")
	}
	if len(ds.Tactics) == 0 || len(ds.Techniques) < minTechniques {
		return nil, fmt.Errorf("bundel tidak tampak seperti ATT&CK Enterprise (%d taktik, %d teknik)", len(ds.Tactics), len(ds.Techniques))
	}
	// sub-teknik yang induknya tidak ada diperlakukan sebagai teknik mandiri (menjaga kunci asing)
	have := map[string]bool{}
	for _, t := range ds.Techniques {
		have[t.ID] = true
	}
	for i := range ds.Techniques {
		if ds.Techniques[i].ParentID != "" && !have[ds.Techniques[i].ParentID] {
			ds.Techniques[i].ParentID = ""
		}
	}
	// induk dulu agar kunci asing parent_id terpenuhi saat dimasukkan berurutan
	sort.SliceStable(ds.Techniques, func(i, j int) bool { return (ds.Techniques[i].ParentID == "") && (ds.Techniques[j].ParentID != "") })
	return ds, nil
}
