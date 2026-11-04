package reporting

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"orchis-go/internal/contract"
	"sort"
	"strings"
	"time"
)

// Counts concern findings (including discovery observations), not vulnerability risk.
type ChangeCounts struct {
	New        int `json:"new"`
	Persistent int `json:"persistent"`
	Resolved   int `json:"resolved"`
	Uncompared int `json:"uncompared"`
}
type FindingChange struct {
	AssetID   uuid.UUID `json:"asset_id"`
	AssetName string    `json:"asset_name"`
	FindingID string    `json:"finding_id"`
	Type      string    `json:"type"`
	Value     string    `json:"value"`
	Title     string    `json:"title"`
	Severity  string    `json:"severity"`
	Change    string    `json:"change"`
	Reason    string    `json:"reason,omitempty"`
	Actions   []string  `json:"actions"`
}
type ComparisonCheck struct {
	AssetID              uuid.UUID  `json:"asset_id"`
	Action               string     `json:"action"`
	Target               string     `json:"target"`
	BaselineAssessmentID *uuid.UUID `json:"baseline_assessment_id,omitempty"`
	BaselineReportID     *uuid.UUID `json:"baseline_report_id,omitempty"`
	BaselineAt           *time.Time `json:"baseline_at,omitempty"`
	Comparable           bool       `json:"comparable"`
	Reason               string     `json:"reason,omitempty"`
}
type Comparison struct {
	Version    int                     `json:"version"`
	Totals     ChangeCounts            `json:"totals"`
	BySeverity map[string]ChangeCounts `json:"by_severity"`
	Rows       []FindingChange         `json:"rows"`
	Checks     []ComparisonCheck       `json:"checks"`
}

// CoverageKey is intentionally unavailable for redacted/truncated targets. Equality
// of redacted addresses must never establish equality of the original coverage.
func CoverageKey(m Method) string {
	if m.Target == "" || strings.Contains(m.Target, "[REDACTED") || strings.Contains(m.Target, "…") || Redact(m.Target, 1000) != m.Target {
		return ""
	}
	if contract.FindVersion(m.Action, m.Version) == nil {
		return ""
	}
	raw, e := contract.NormalizeVersion(m.Action, m.Version, m.Params)
	if e != nil {
		return ""
	}
	var value any
	if e = json.Unmarshal(raw, &value); e != nil {
		return ""
	}
	canonical, e := json.Marshal(value)
	if e != nil {
		return ""
	}
	data, _ := json.Marshal([]any{m.Action, m.Version, m.Target, json.RawMessage(canonical)})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func severityRank(s string) int {
	for i, v := range []string{"unknown", "info", "low", "medium", "high", "critical"} {
		if s == v {
			return i
		}
	}
	return 0
}
func severity(s string) string {
	if severityRank(s) == 0 {
		return "unknown"
	}
	return s
}
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
func unique(in []string) []string {
	m := map[string]bool{}
	for _, v := range in {
		if v != "" {
			m[v] = true
		}
	}
	return sortedKeys(m)
}
func findingOrder(f Finding) string { raw, _ := json.Marshal(f); return string(raw) }
func mergeFinding(a, b Finding) Finding {
	sources := unique(append(append([]string(nil), a.Coverage...), b.Coverage...))
	// A repeated observation can carry a different severity. Freeze the highest;
	// lexical tie-breaking makes job/row order irrelevant without mutating inputs.
	if severityRank(b.Severity) > severityRank(a.Severity) || (severityRank(a.Severity) == severityRank(b.Severity) && findingOrder(b) < findingOrder(a)) {
		a = b
	}
	a.Coverage = sources
	return a
}

// MergeFindings preserves all producing checks and counts one identity per asset.
func MergeFindings(in []Finding) []Finding {
	byID := map[string]Finding{}
	for _, f := range in {
		if old, ok := byID[f.ID]; ok {
			byID[f.ID] = mergeFinding(old, f)
		} else {
			f.Coverage = unique(f.Coverage)
			byID[f.ID] = f
		}
	}
	out := make([]Finding, 0, len(byID))
	for _, id := range sortedKeys(byID) {
		out = append(out, byID[id])
	}
	return out
}

type methodState struct {
	method Method
	valid  bool
}

func methods(a Asset) map[string]methodState {
	out := map[string]methodState{}
	for _, m := range a.Methods {
		k := CoverageKey(m)
		if k == "" {
			continue
		}
		if old, ok := out[k]; ok {
			old.valid = false
			out[k] = old
		} else {
			out[k] = methodState{m, true}
		}
	}
	return out
}
func sources(a Asset, f Finding) []string {
	ms := methods(a)
	if len(f.Coverage) > 0 {
		for _, k := range f.Coverage {
			m, ok := ms[k]
			if !ok || !m.valid {
				return nil
			}
			def := contract.FindVersion(m.method.Action, m.method.Version)
			allowed := false
			for _, typ := range def.OutputTypes {
				allowed = allowed || typ == f.Type
			}
			if !allowed {
				return nil
			}
		}
		return unique(f.Coverage)
	}
	// Historical snapshots did not store producers. Only a single compatible
	// check is unambiguous; never guess between two checks of the same tool.
	var keys []string
	for k, m := range ms {
		if !m.valid {
			continue
		}
		for _, typ := range contract.FindVersion(m.method.Action, m.method.Version).OutputTypes {
			if typ == f.Type {
				keys = append(keys, k)
				break
			}
		}
	}
	if len(keys) != 1 {
		return nil
	}
	return keys
}
func canUse(a Asset, m methodState) bool {
	if !m.valid || a.SkipReason != "" || a.Status == "skipped" || m.method.Outcome != "success" {
		return false
	}
	if a.Status == "finished" {
		return true
	}
	if a.Status != "incomplete" {
		return false
	}
	// A partial multi-tool result can still prove a completed check. An unexplained
	// incomplete asset with all-success methods is inconsistent and fails closed.
	for _, other := range a.Methods {
		if other.Outcome != "success" {
			return true
		}
	}
	return false
}

type baselineCheck struct {
	epoch      int64
	asset      Asset
	state      methodState
	findings   map[string]Finding
	assessment uuid.UUID
	report     uuid.UUID
	at         time.Time
}
type comparisonAsset struct {
	asset      Asset
	methods    map[string]methodState
	previous   map[string]baselineCheck
	hadHistory bool
	hadProfile bool
}

// ComparisonBuilder accepts prior snapshots newest first. It keeps only the
// latest matching check, including failed attempts (never cherry-pick a success).
type ComparisonBuilder struct {
	epoch   int64
	profile string
	assets  map[string]*comparisonAsset
}

func NewComparison(current Snapshot) *ComparisonBuilder {
	b := &ComparisonBuilder{epoch: current.FindingsEpoch, profile: current.Profile, assets: map[string]*comparisonAsset{}}
	counts := map[string]int{}
	for _, a := range current.Assets {
		counts[a.ID.String()]++
	}
	for _, a := range current.Assets {
		key := a.ID.String()
		a.Findings = MergeFindings(a.Findings)
		ms := methods(a)
		if counts[key] > 1 {
			for k, m := range ms {
				m.valid = false
				ms[k] = m
			}
			if existing := b.assets[key]; existing != nil {
				existing.asset.Findings = MergeFindings(append(existing.asset.Findings, a.Findings...))
				continue
			}
		}
		b.assets[key] = &comparisonAsset{asset: a, methods: ms, previous: map[string]baselineCheck{}}
	}
	return b
}
func (b *ComparisonBuilder) Add(previous Snapshot, report uuid.UUID) {
	counts := map[string]int{}
	for _, a := range previous.Assets {
		counts[a.ID.String()]++
	}
	seen := map[string]bool{}
	for _, a := range previous.Assets {
		id := a.ID.String()
		current := b.assets[id]
		if current == nil || seen[id] {
			continue
		}
		seen[id] = true
		current.hadHistory = true
		if previous.Profile != b.profile {
			continue
		}
		current.hadProfile = true
		oldMethods := methods(a)
		if counts[id] > 1 {
			for k, m := range oldMethods {
				m.valid = false
				oldMethods[k] = m
			}
		}
		a.Findings = MergeFindings(a.Findings)
		for key, m := range oldMethods {
			if _, wanted := current.methods[key]; !wanted {
				continue
			}
			if _, found := current.previous[key]; found {
				continue
			}
			metadata := a
			metadata.Findings = nil
			base := baselineCheck{epoch: previous.FindingsEpoch, asset: metadata, state: m, findings: map[string]Finding{}, assessment: previous.AssessmentID, report: report, at: previous.CreatedAt}
			for _, f := range a.Findings {
				if counts[id] > 1 {
					continue
				}
				keys := sources(a, f)
				for _, source := range keys {
					if source == key {
						f.Coverage = keys
						base.findings[f.ID] = f
						break
					}
				}
			}
			current.previous[key] = base
		}
	}
}

// Done bounds database reading once all current checks have their latest baseline.
func (b *ComparisonBuilder) Done() bool {
	for _, a := range b.assets {
		for k := range a.methods {
			if _, ok := a.previous[k]; !ok {
				return false
			}
		}
	}
	return true
}
func rowFor(a Asset, f Finding, change, reason string, keys []string) FindingChange {
	actions := []string{}
	ms := methods(a)
	for _, k := range keys {
		if m, ok := ms[k]; ok {
			actions = append(actions, m.method.Action)
		}
	}
	return FindingChange{AssetID: a.ID, AssetName: Redact(a.Name, 200), FindingID: f.ID, Type: f.Type, Value: Redact(f.Value, 1000), Title: Redact(f.Title, 500), Severity: severity(f.Severity), Change: change, Reason: reason, Actions: unique(actions)}
}
func (c *Comparison) add(row FindingChange) {
	c.Rows = append(c.Rows, row)
	counts := c.BySeverity[row.Severity]
	for _, n := range []*ChangeCounts{&c.Totals, &counts} {
		switch row.Change {
		case "new":
			n.New++
		case "persistent":
			n.Persistent++
		case "resolved":
			n.Resolved++
		default:
			n.Uncompared++
		}
	}
	c.BySeverity[row.Severity] = counts
}
func (b *ComparisonBuilder) Result() Comparison {
	out := Comparison{Version: 1, BySeverity: map[string]ChangeCounts{}, Rows: []FindingChange{}, Checks: []ComparisonCheck{}}
	for _, s := range []string{"critical", "high", "medium", "low", "info"} {
		out.BySeverity[s] = ChangeCounts{}
	}
	for _, id := range sortedKeys(b.assets) {
		a := b.assets[id]
		oldFindings := map[string]Finding{}
		for _, key := range sortedKeys(a.methods) {
			method := a.methods[key]
			base, found := a.previous[key]
			check := ComparisonCheck{AssetID: a.asset.ID, Action: method.method.Action, Target: Redact(method.method.Target, 1000)}
			if found {
				if base.assessment != uuid.Nil {
					v := base.assessment
					check.BaselineAssessmentID = &v
				}
				if base.report != uuid.Nil {
					v := base.report
					check.BaselineReportID = &v
				}
				if !base.at.IsZero() {
					v := base.at
					check.BaselineAt = &v
				}
			}
			switch {
			case !canUse(a.asset, method):
				check.Reason = "scan_not_completed"
			case !found:
				check.Reason = "no_comparable_baseline"
			case base.epoch != b.epoch:
				check.Reason = "identity_changed"
			case !canUse(base.asset, base.state):
				check.Reason = "previous_not_completed"
			default:
				check.Comparable = true
			}
			out.Checks = append(out.Checks, check)
			if found {
				for fid, f := range base.findings {
					if old, ok := oldFindings[fid]; ok {
						oldFindings[fid] = mergeFinding(old, f)
					} else {
						oldFindings[fid] = f
					}
				}
			}
		}
		present := map[string]bool{}
		for _, f := range a.asset.Findings {
			present[f.ID] = true
			keys := sources(a.asset, f)
			comparable, known, persistent, observed, identityChanged := false, false, false, false, false
			for _, k := range keys {
				m := a.methods[k]
				if !canUse(a.asset, m) {
					continue
				}
				observed = true
				base, ok := a.previous[k]
				if !ok {
					continue
				}
				known = true
				if base.epoch != b.epoch {
					identityChanged = true
					continue
				}
				if !canUse(base.asset, base.state) {
					continue
				}
				comparable = true
				if _, ok = base.findings[f.ID]; ok {
					persistent = true
				}
			}
			change, reason := "uncompared", "coverage_changed"
			switch {
			case len(keys) == 0:
				reason = "ambiguous_source"
			case !observed:
				reason = "scan_not_completed"
			case persistent:
				change, reason = "persistent", ""
			case comparable:
				change, reason = "new", ""
			case !known && !a.hadHistory:
				change, reason = "new", "initial_coverage"
			case !known && !a.hadProfile:
				reason = "profile_changed"
			case identityChanged:
				reason = "identity_changed"
			case known:
				reason = "previous_not_completed"
			}
			out.add(rowFor(a.asset, f, change, reason, keys))
		}
		for _, fid := range sortedKeys(oldFindings) {
			if present[fid] {
				continue
			}
			f := oldFindings[fid]
			complete := len(f.Coverage) > 0
			reason := "scan_not_completed"
			for _, k := range f.Coverage {
				m, hasCurrent := a.methods[k]
				base, hasBaseline := a.previous[k]
				if !hasCurrent || !hasBaseline {
					complete = false
					reason = "coverage_changed"
					break
				}
				if base.epoch != b.epoch {
					complete = false
					reason = "identity_changed"
					break
				}
				if !canUse(a.asset, m) || !canUse(base.asset, base.state) {
					complete = false
					break
				}
			}
			if complete {
				out.add(rowFor(a.asset, f, "resolved", "", f.Coverage))
			} else {
				out.add(rowFor(a.asset, f, "uncompared", reason, f.Coverage))
			}
		}
	}
	sort.Slice(out.Rows, func(i, j int) bool {
		a, b := out.Rows[i], out.Rows[j]
		return a.AssetID.String()+"\x00"+a.Change+"\x00"+a.FindingID < b.AssetID.String()+"\x00"+b.Change+"\x00"+b.FindingID
	})
	return out
}
func (c Comparison) Summary() string {
	compared := 0
	for _, check := range c.Checks {
		if check.Comparable {
			compared++
		}
	}
	return fmt.Sprintf("Compared %d completed target/tool checks: %d new, %d persistent, %d resolved findings; %d not compared. Failed, skipped, stopped and differently scoped checks never establish resolved findings.", compared, c.Totals.New, c.Totals.Persistent, c.Totals.Resolved, c.Totals.Uncompared)
}
func Compare(current, previous Snapshot) string {
	b := NewComparison(current)
	b.Add(previous, uuid.Nil)
	return b.Result().Summary()
}
