package injector

import (
	"bufio"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strings"

	"orchis-go/internal/execution"
)

type nmapRun struct {
	XMLName xml.Name `xml:"nmaprun"`
	Hosts   []struct {
		TimedOut string `xml:"timedout,attr"`
		Status   struct {
			State string `xml:"state,attr"`
		} `xml:"status"`
		ExtraPorts []struct {
			State string `xml:"state,attr"`
			Count int    `xml:"count,attr"`
		} `xml:"ports>extraports"`
		Ports []struct {
			Protocol string `xml:"protocol,attr"`
			ID       int    `xml:"portid,attr"`
			State    struct {
				State string `xml:"state,attr"`
			} `xml:"state"`
			Service struct {
				Name    string `xml:"name,attr"`
				Product string `xml:"product,attr"`
				Version string `xml:"version,attr"`
			} `xml:"service"`
		} `xml:"ports>port"`
	} `xml:"host"`
}

// ParseNmap mengubah keluaran XML nmap menjadi finding open_port (dan service bila produknya terdeteksi).
func ParseNmap(out []byte) ([]execution.NewFinding, error) {
	var run nmapRun
	dec := xml.NewDecoder(bytes.NewReader(out))
	dec.Strict = false
	if err := dec.Decode(&run); err != nil {
		return nil, fmt.Errorf("keluaran nmap tidak bisa dibaca: %w", err)
	}
	if len(run.Hosts) == 0 {
		return nil, fmt.Errorf("TCP scan inconclusive: no host response was recorded")
	}
	var fs []execution.NewFinding
	for _, h := range run.Hosts {
		responded := false
		for _, p := range h.Ports {
			if p.State.State == "open" || p.State.State == "closed" || p.State.State == "unfiltered" {
				responded = true
			}
		}
		for _, p := range h.ExtraPorts {
			if p.Count > 0 && (p.State == "closed" || p.State == "unfiltered") {
				responded = true
			}
		}
		if h.TimedOut == "true" || h.Status.State != "up" || !responded {
			return nil, fmt.Errorf("TCP scan inconclusive: the endpoint did not respond on the selected ports; no clean result can be inferred")
		}
		for _, p := range h.Ports {
			if p.State.State != "open" {
				continue
			}
			base := fmt.Sprintf("%s/%d", p.Protocol, p.ID)
			svc := p.Service.Name
			title := fmt.Sprintf("Open port %d/%s", p.ID, p.Protocol)
			if svc != "" {
				title += " (" + svc + ")"
			}
			details, _ := json.Marshal(map[string]any{"port": p.ID, "protocol": p.Protocol, "service": svc,
				"product": p.Service.Product, "version": p.Service.Version})
			fs = append(fs, execution.NewFinding{Type: "open_port", Value: base, Title: title, Severity: "info", Details: details})
			if p.Service.Product != "" {
				name := strings.TrimSpace(p.Service.Product + " " + p.Service.Version)
				fs = append(fs, execution.NewFinding{Type: "service", Value: base + " " + name,
					Title: name + " on " + fmt.Sprintf("%d/%s", p.ID, p.Protocol), Severity: "info", Details: details})
			}
		}
	}
	return fs, nil
}

type nucleiLine struct {
	TemplateID string `json:"template-id"`
	Info       struct {
		Name        string   `json:"name"`
		Severity    string   `json:"severity"`
		Description string   `json:"description"`
		Reference   []string `json:"reference"`
		Tags        []string `json:"tags"`
	} `json:"info"`
	Type             string   `json:"type"`
	MatchedAt        string   `json:"matched-at"`
	MatcherName      string   `json:"matcher-name"`
	ExtractedResults []string `json:"extracted-results"`
}

// ParseNuclei mengubah keluaran JSONL nuclei menjadi finding vulnerability. Permintaan/respons mentah sengaja
// tidak diteruskan (bisa berisi data sensitif); hanya metadata kecil.
func ParseNuclei(out []byte) ([]execution.NewFinding, error) {
	var fs []execution.NewFinding
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 0, 64<<10), 4<<20)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var n nucleiLine
		if err := json.Unmarshal(line, &n); err != nil || n.TemplateID == "" {
			continue
		}
		sev := strings.ToLower(n.Info.Severity)
		if sev == "unknown" {
			sev = "info"
		}
		title := n.Info.Name
		if title == "" {
			title = n.TemplateID
		}
		desc := n.Info.Description
		if len(desc) > 1000 {
			desc = desc[:1000]
		}
		refs := n.Info.Reference
		if len(refs) > 5 {
			refs = refs[:5]
		}
		extracted := n.ExtractedResults
		if len(extracted) > 10 {
			extracted = extracted[:10]
		}
		details, _ := json.Marshal(map[string]any{"template_id": n.TemplateID, "matched_at": n.MatchedAt, "matcher_name": n.MatcherName,
			"type": n.Type, "description": desc, "reference": refs, "tags": n.Info.Tags, "extracted_results": extracted})
		fs = append(fs, execution.NewFinding{Type: "vulnerability", Value: n.TemplateID + "@" + n.MatchedAt, Title: title, Severity: sev, Details: details})
	}
	return fs, sc.Err()
}
