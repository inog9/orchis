package reporting

import (
	"context"
	"encoding/json"
	"errors"
	"html/template"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// AIAdvice adalah suplemen advisori remediasi hasil AI untuk satu laporan: job AI
// kind=remediation yang sudah disetujui manusia (status='approved'). Ia tidak mengubah
// hasil beku; hanya dilampirkan sebagai bagian terpisah dan berlabel jelas.
type AIAdvice struct {
	JobID      string         `json:"job_id"`
	Model      string         `json:"model"`
	ApprovedAt *time.Time    `json:"approved_at,omitempty"`
	Summary    string         `json:"summary"`
	Items      []AIAdviceItem `json:"items"`
}

// AIAdviceItem merujuk satu temuan nyata di snapshot (alias AI dipetakan ke ID asli
// lewat kolom mapping pada ai_jobs).
type AIAdviceItem struct {
	FindingID      string `json:"finding_id"`
	Title          string `json:"title,omitempty"`
	Severity       string `json:"severity,omitempty"`
	Recommendation string `json:"recommendation"`
	Validation     string `json:"validation"`
}

// attachAIAdvice memuat job remediasi AI terbaru yang sudah approved untuk laporan ini
// dan menempelkannya ke snapshot sebelum render. Kegagalan membaca advice tidak
// membatalkan render laporan.
func attachAIAdvice(ctx context.Context, tx pgx.Tx, tenant, reportID uuid.UUID, snap *Snapshot) error {
	var jobID, model string
	var approvedAt *time.Time
	var outB, mapB []byte
	err := tx.QueryRow(ctx, `SELECT id,model,approved_at,output,mapping FROM ai_jobs WHERE tenant_id=$1 AND report_id=$2 AND kind='remediation' AND status='approved' ORDER BY approved_at DESC LIMIT 1`, tenant, reportID).Scan(&jobID, &model, &approvedAt, &outB, &mapB)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return nil // advice opsional: jangan gagalkan render
	}
	var out struct {
		Summary      string `json:"summary"`
		Remediations []struct {
			FindingID      string `json:"finding_id"`
			Recommendation string `json:"recommendation"`
			Validation     string `json:"validation"`
		} `json:"remediations"`
	}
	if e := json.Unmarshal(outB, &out); e != nil || len(out.Remediations) == 0 {
		return nil
	}
	var mapping struct {
		Findings map[string]string `json:"findings"` // alias AI -> finding ID asli
	}
	_ = json.Unmarshal(mapB, &mapping)
	byID := map[string]Finding{}
	for _, a := range snap.Assets {
		for _, f := range a.Findings {
			byID[f.ID] = f
		}
	}
	adv := &AIAdvice{JobID: jobID, Model: Redact(model, 100), ApprovedAt: approvedAt, Summary: Redact(out.Summary, 2000)}
	for _, r := range out.Remediations {
		real := r.FindingID
		if v, ok := mapping.Findings[r.FindingID]; ok && v != "" {
			real = v
		}
		it := AIAdviceItem{FindingID: real, Recommendation: Redact(r.Recommendation, 4000), Validation: Redact(r.Validation, 2000)}
		if f, ok := byID[real]; ok {
			it.Title = Redact(f.Title, 500)
			it.Severity = f.Severity
		}
		adv.Items = append(adv.Items, it)
	}
	sort.SliceStable(adv.Items, func(i, j int) bool { return sevOrder[adv.Items[i].Severity] > sevOrder[adv.Items[j].Severity] })
	snap.AIAdvice = adv
	return nil
}

// aiAdviceHTML merender blok suplemen AI di section Remediation.
func aiAdviceHTML(lang string, s Snapshot) template.HTML {
	a := s.AIAdvice
	if a == nil || len(a.Items) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<section class="ai-advice"><h3>` + esc(tr(lang, "AI-generated remediation advice", "Saran remediasi hasil AI")) + `</h3>`)
	b.WriteString(`<p class="section-note">` + esc(tr(lang, "Advisory supplement produced by "+a.Model+" and approved by a human. It does not modify the frozen results above.", "Suplemen advisori dari "+a.Model+" yang sudah ditinjau dan disetujui manusia. Tidak mengubah hasil beku di atas.")) + `</p>`)
	if a.Summary != "" {
		b.WriteString("<p><strong>" + esc(tr(lang, "Summary", "Ringkasan")) + "</strong> " + esc(a.Summary) + "</p>")
	}
	b.WriteString(`<table class="ai-advice-table"><thead><tr><th>` + esc(tr(lang, "Finding", "Temuan")) + `</th><th>` + esc(tr(lang, "Severity", "Severitas")) + `</th><th>` + esc(tr(lang, "Recommendation", "Rekomendasi")) + `</th><th>` + esc(tr(lang, "Validation", "Validasi")) + `</th></tr></thead><tbody>`)
	for _, it := range a.Items {
		b.WriteString("<tr><td>" + esc(it.Title) + "</td><td>" + esc(it.Severity) + "</td><td>" + esc(it.Recommendation) + "</td><td>" + esc(it.Validation) + "</td></tr>")
	}
	b.WriteString("</tbody></table></section>")
	return template.HTML(b.String())
}
