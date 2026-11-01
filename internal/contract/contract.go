// Package contract mendefinisikan kontrak aksi (apa yang boleh diminta ke injector) dan membangun argumen perintahnya.
// Server dan injector memakai paket yang sama, sehingga parameter divalidasi dengan aturan yang persis sama di kedua sisi.
//
// Aturan keamanan: parameter dari pengguna TIDAK PERNAH menjadi argumen mentah. Setiap nilai harus lolos daftar putih
// atau pola ketat, dan target selalu ditaruh setelah "--" (atau lewat flag bernilai) agar tidak terbaca sebagai opsi.
package contract

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"orchis-go/internal/osquery"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	KeyNmapTCP  = "nmap.tcp_connect"
	KeyNucleiVS = "nuclei.scan"

	// Aksi yang berjalan DI endpoint oleh agent (hanya-baca, tanpa parameter, tanpa menyentuh jaringan).
	KeyAgentPorts = "agent.listening_ports"
	KeyAgentInfo  = "agent.system_info"

	// Aksi osquery (hanya-baca, tanpa parameter): query bernama dari katalog beku internal/osquery. Dijalankan agent di endpoint.
	AdapterOsquery = "osquery"
)

// Def adalah satu kontrak yang disemai ke tabel contracts.
type Def struct {
	Adapter        string   `json:"adapter"`
	Category       string   `json:"category"`
	Schema         Schema   `json:"input_schema"`
	Key            string   `json:"key"`
	Integration    string   `json:"integration"` // kunci integrasi injector yang menjalankannya
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Kind           string   `json:"kind"` // scan | vulnerability_scan
	OutputTypes    []string `json:"output_types"`
	TimeoutSeconds int      `json:"timeout_seconds"`
	Version        int      `json:"version"`
	NeedsAgent     bool     `json:"needs_agent"` // berjalan di endpoint lewat agent, bukan lewat injector di jaringan
}

//go:embed catalog.json
var catalogJSON []byte
var Defs []Def

func init() {
	if err := json.Unmarshal(catalogJSON, &Defs); err != nil {
		panic(err)
	}
	addPackContracts()
	for _, d := range Defs {
		if err := d.ValidateDefinition(); err != nil {
			panic(err)
		}
	}
}

func Find(key string) *Def {
	var best *Def
	for i := range Defs {
		d := &Defs[i]
		if d.Key == key && (best == nil || d.Version > best.Version) {
			best = d
		}
	}
	return best
}
func FindVersion(key string, version int) *Def {
	for i := range Defs {
		if Defs[i].Key == key && Defs[i].Version == version {
			return &Defs[i]
		}
	}
	return nil
}

// ParamError menunjuk kolom parameter yang salah.
type ParamError struct{ Field, Message string }

func (e *ParamError) Error() string { return e.Field + ": " + e.Message }

type NmapParams struct {
	Profile          string `json:"profile"`           // top100 | top1000 | web | custom
	Ports            string `json:"ports,omitempty"`   // hanya untuk profile custom
	ServiceDetection bool   `json:"service_detection"` // -sV --version-light
}

type NucleiParams struct {
	TemplateProfile string   `json:"template_profile,omitempty"`
	Severity        []string `json:"severity"`
	Tags            []string `json:"tags,omitempty"`
}

var (
	portsRe = regexp.MustCompile(`^[0-9]{1,5}(-[0-9]{1,5})?(,[0-9]{1,5}(-[0-9]{1,5})?){0,49}$`)
	tagRe   = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)
)

var severities = []string{"info", "low", "medium", "high", "critical"}

func validSeverity(s string) bool {
	for _, v := range severities {
		if v == s {
			return true
		}
	}
	return false
}

func strict(raw json.RawMessage, dst any) error {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return &ParamError{Field: "params", Message: "Unrecognised or malformed parameters: " + err.Error()}
	}
	return nil
}

// Normalize memvalidasi params dan mengembalikan bentuk kanonik (nilai bawaan terisi, urutan stabil).
func Normalize(key string, raw json.RawMessage) (json.RawMessage, error) {
	return normalizeDefinition(Find(key), raw)
}
func NormalizeVersion(key string, version int, raw json.RawMessage) (json.RawMessage, error) {
	return normalizeDefinition(FindVersion(key, version), raw)
}
func normalizeDefinition(def *Def, raw json.RawMessage) (json.RawMessage, error) {
	if def == nil {
		return nil, &ParamError{Field: "contract", Message: "Unknown contract."}
	}
	canonical, err := def.Schema.Normalize(raw)
	if err != nil {
		return nil, err
	}
	raw = canonical
	switch def.Adapter {
	case "nmap":
		p := NmapParams{Profile: "top100"}
		if err := strict(raw, &p); err != nil {
			return nil, err
		}
		if err := p.validate(); err != nil {
			return nil, err
		}
		return json.Marshal(p)
	case "nuclei":
		p := NucleiParams{Severity: []string{"medium", "high", "critical"}}
		if err := strict(raw, &p); err != nil {
			return nil, err
		}
		if err := p.validate(); err != nil {
			return nil, err
		}
		return json.Marshal(p)
	case "agent.listening_ports", "agent.system_info", AdapterOsquery:
		// Tanpa parameter: terima hanya objek kosong, supaya tidak ada jalur untuk menyelipkan perintah ke agent.
		var none struct{}
		if err := strict(raw, &none); err != nil {
			return nil, err
		}
		return json.RawMessage(`{}`), nil
	}
	return nil, &ParamError{Field: "contract", Message: "Unknown contract."}
}

func (p *NmapParams) validate() error {
	switch p.Profile {
	case "top100", "top1000", "web":
		if p.Ports != "" {
			return &ParamError{Field: "params.ports", Message: "Ports are only allowed with the custom profile."}
		}
	case "custom":
		if !portsRe.MatchString(p.Ports) {
			return &ParamError{Field: "params.ports", Message: "Use ports like 22,80,8000-8100 (at most 50 entries)."}
		}
		for _, part := range strings.Split(p.Ports, ",") {
			lo, hi, isRange := strings.Cut(part, "-")
			a, _ := strconv.Atoi(lo)
			b := a
			if isRange {
				b, _ = strconv.Atoi(hi)
			}
			if a < 1 || b > 65535 || a > b {
				return &ParamError{Field: "params.ports", Message: "Ports must be between 1 and 65535, and a range must go upwards."}
			}
		}
	default:
		return &ParamError{Field: "params.profile", Message: "Choose top100, top1000, web or custom."}
	}
	return nil
}

func (p *NucleiParams) validate() error {
	if p.TemplateProfile != "" && p.TemplateProfile != "default" && p.TemplateProfile != "exposure" && p.TemplateProfile != "cves" && p.TemplateProfile != "misconfig" {
		return &ParamError{Field: "params.template_profile", Message: "Choose an allowed template profile."}
	}
	if len(p.Severity) == 0 {
		return &ParamError{Field: "params.severity", Message: "Choose at least one severity."}
	}
	seen := map[string]bool{}
	for _, s := range p.Severity {
		if !validSeverity(s) {
			return &ParamError{Field: "params.severity", Message: "Severity must be one of info, low, medium, high, critical."}
		}
		seen[s] = true
	}
	p.Severity = p.Severity[:0]
	for _, s := range severities { // urutan kanonik
		if seen[s] {
			p.Severity = append(p.Severity, s)
		}
	}
	if len(p.Tags) > 10 {
		return &ParamError{Field: "params.tags", Message: "At most 10 tags."}
	}
	tags := map[string]bool{}
	for _, t := range p.Tags {
		if !tagRe.MatchString(t) {
			return &ParamError{Field: "params.tags", Message: "Tags may only contain lowercase letters, digits, - and _."}
		}
		tags[t] = true
	}
	p.Tags = p.Tags[:0]
	for t := range tags {
		p.Tags = append(p.Tags, t)
	}
	sort.Strings(p.Tags)
	return nil
}

// NmapArgs membangun argumen nmap. Keluaran XML dikirim ke stdout. Target ditaruh setelah "--".
func NmapArgs(raw json.RawMessage, target string, timeoutSeconds int) ([]string, error) {
	var p NmapParams
	if err := strict(raw, &p); err != nil {
		return nil, err
	}
	if err := p.validate(); err != nil {
		return nil, err
	}
	args := []string{"-sT", "-Pn", "-n", "-oX", "-", "--host-timeout", fmt.Sprintf("%ds", timeoutSeconds)}
	switch p.Profile {
	case "top100":
		args = append(args, "--top-ports", "100")
	case "top1000":
		args = append(args, "--top-ports", "1000")
	case "web":
		args = append(args, "-p", "80,443,8000,8080,8443")
	case "custom":
		args = append(args, "-p", p.Ports)
	}
	if p.ServiceDetection {
		args = append(args, "-sV", "--version-light")
	}
	return append(args, "--", target), nil
}

// NucleiArgs membangun argumen nuclei. templatesDir kosong = template bawaan Nuclei.
func NucleiArgs(raw json.RawMessage, target, templatesDir string) ([]string, error) {
	var p NucleiParams
	if err := strict(raw, &p); err != nil {
		return nil, err
	}
	if err := p.validate(); err != nil {
		return nil, err
	}
	args := []string{"-u", target, "-jsonl", "-silent", "-nc", "-duc", "-severity", strings.Join(p.Severity, ","),
		"-rl", "30", "-c", "10", "-timeout", "5", "-retries", "1", "-max-host-error", "30"}
	if len(p.Tags) > 0 {
		args = append(args, "-tags", strings.Join(p.Tags, ","))
	}
	if p.TemplateProfile != "" && p.TemplateProfile != "default" {
		if templatesDir == "" {
			return nil, &ParamError{Field: "params.template_profile", Message: "A curated template directory is required."}
		}
		dirs := map[string]string{"exposure": "http/exposures", "cves": "http/cves", "misconfig": "http/misconfiguration"}
		templatesDir = filepath.Join(templatesDir, dirs[p.TemplateProfile])
	}
	if templatesDir != "" {
		args = append(args, "-t", templatesDir)
	}
	return args, nil
}

// addPackContracts membuat satu kontrak tanpa parameter untuk setiap query pack resmi yang dibekukan, dari bentuk kontrak
// osquery.inventory. Kunci kontrak = kunci query: server tidak pernah mengirim SQL, hanya kunci ini.
func addPackContracts() {
	var base *Def
	for i := range Defs {
		if Defs[i].Key == "osquery.inventory" {
			base = &Defs[i]
		}
	}
	if base == nil {
		panic("contract: osquery.inventory tidak ada")
	}
	for _, q := range osquery.Queries {
		if q.Pack == "" {
			continue
		}
		d := *base
		d.Key, d.Name, d.Description, d.Version = q.Key, q.Title+" ("+q.Pack+" pack)", q.Description, 1
		if d.Description == "" {
			d.Description = "Read-only check from the official osquery " + q.Pack + " pack."
		}
		d.Category, d.TimeoutSeconds = "Endpoint", q.TimeoutSeconds
		Defs = append(Defs, d)
	}
}
