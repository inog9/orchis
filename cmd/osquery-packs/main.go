// Perintah osquery-packs membekukan pack resmi osquery dari tarball yang DIPATOK (hash dicek terhadap katalog) menjadi
// internal/osquery/packs.json. Setiap query diklasifikasi (runnable/sensitive/not_reviewed/other_os/unverified); hanya yang
// lolos tinjauan dan menghasilkan sampel nyata pada osquery asli yang menjadi query katalog. Dijalankan oleh pemelihara, bukan saat runtime.
//
//	go run ./cmd/osquery-packs -tar deploy/osquery/osquery-5.24.0_1.linux_x86_64.tar.gz -osqueryd /path/osqueryd
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"orchis-go/internal/osquery"
)

var packNames = []string{"vuln-management", "it-compliance", "incident-response", "hardware-monitoring", "ossec-rootkit"}

var (
	lineComment = regexp.MustCompile(`(?m)^\s*//.*$`)
	nonKey      = regexp.MustCompile(`[^a-z0-9]+`)
)

type rawQuery struct {
	Query       string `json:"query"`
	Description string `json:"description"`
	Value       string `json:"value"`
	Platform    string `json:"platform"`
}
type rawPack struct {
	Platform string              `json:"platform"`
	Queries  map[string]rawQuery `json:"queries"`
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "osquery-packs: "+format+"\n", a...)
	os.Exit(1)
}

func main() {
	tarPath := flag.String("tar", "", "tarball osquery yang dipatok")
	bin := flag.String("osqueryd", "", "biner osquery untuk mengambil kolom dan sampel nyata")
	out := flag.String("out", "internal/osquery/packs.json", "berkas keluaran")
	flag.Parse()
	if *tarPath == "" || *bin == "" {
		fail("-tar dan -osqueryd wajib")
	}
	raw, err := os.ReadFile(*tarPath)
	if err != nil {
		fail("%v", err)
	}
	sum := sha256.Sum256(raw)
	pin, ok := osquery.Artifacts["linux/amd64"]
	if !ok || hex.EncodeToString(sum[:]) != pin.SHA256 {
		fail("tarball tidak sama dengan artefak yang dipatok")
	}
	files, err := readPacks(raw)
	if err != nil {
		fail("%v", err)
	}
	res := struct {
		OsqueryVersion string          `json:"osquery_version"`
		ArtifactSHA256 string          `json:"artifact_sha256"`
		Packs          []osquery.Pack  `json:"packs"`
		Queries        []osquery.Query `json:"queries"`
	}{OsqueryVersion: osquery.Version, ArtifactSHA256: pin.SHA256, Packs: []osquery.Pack{}, Queries: []osquery.Query{}}
	for _, name := range packNames {
		body, ok := files[name]
		if !ok {
			fail("pack %s tidak ada di tarball", name)
		}
		h := sha256.Sum256(body)
		var rp rawPack
		if err := json.Unmarshal([]byte(lineComment.ReplaceAllString(string(body), "")), &rp); err != nil {
			fail("pack %s tidak terbaca: %v", name, err)
		}
		pk := osquery.Pack{Name: name, File: name + ".conf", SHA256: hex.EncodeToString(h[:]), Platform: rp.Platform, Description: describe(name), Queries: []osquery.PackQuery{}}
		names := make([]string, 0, len(rp.Queries))
		for n := range rp.Queries {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			pq, q := classify(*bin, name, n, rp.Platform, rp.Queries[n])
			pk.Queries = append(pk.Queries, pq)
			if q != nil {
				res.Queries = append(res.Queries, *q)
			}
		}
		res.Packs = append(res.Packs, pk)
	}
	enc, _ := json.MarshalIndent(res, "", " ")
	if err := os.WriteFile(*out, append(enc, '\n'), 0o644); err != nil {
		fail("%v", err)
	}
	for _, p := range res.Packs {
		c := map[string]int{}
		for _, q := range p.Queries {
			c[q.Status]++
		}
		fmt.Printf("%-20s %v\n", p.Name, c)
	}
}

func describe(name string) string {
	switch name {
	case "vuln-management":
		return "Software and kernel inventory used for vulnerability management."
	case "it-compliance":
		return "Basic IT compliance facts: users, mounts, time, OS and kernel."
	case "incident-response":
		return "Triage facts used during incident response."
	case "hardware-monitoring":
		return "Hardware and device inventory."
	case "ossec-rootkit":
		return "Rootkit and tamper checks, based on reading system files."
	}
	return name
}

func readPacks(tgz []byte) (map[string][]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(tgz))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(zr)
	out := map[string][]byte{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		if h.Typeflag != tar.TypeReg || path.Dir(h.Name) != "opt/osquery/share/osquery/packs" || !strings.HasSuffix(h.Name, ".conf") {
			continue
		}
		b, err := io.ReadAll(io.LimitReader(tr, 1<<20))
		if err != nil {
			return nil, err
		}
		out[strings.TrimSuffix(path.Base(h.Name), ".conf")] = b
	}
}

func platformOK(p string) bool {
	if strings.TrimSpace(p) == "" {
		return true
	}
	for _, x := range strings.Split(strings.ToLower(p), ",") {
		switch strings.TrimSpace(x) {
		case "linux", "posix", "all", "any":
			return true
		}
	}
	return false
}

func classify(bin, pack, name, packPlatform string, rq rawQuery) (osquery.PackQuery, *osquery.Query) {
	desc := strings.TrimSpace(rq.Description)
	if desc == "" {
		desc = strings.TrimSpace(rq.Value)
	}
	if len(desc) > 300 {
		desc = desc[:300]
	}
	// Pack resmi mengakhiri SQL dengan ";". Katalog melarang ";" sama sekali, jadi hanya penutup akhir yang dibuang.
	sql := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(rq.Query), ";"))
	pq := osquery.PackQuery{Name: name, Description: desc, Tables: osquery.TablesIn(sql)}
	if pq.Tables == nil {
		pq.Tables = []string{}
	}
	plat := rq.Platform
	if plat == "" {
		plat = packPlatform
	}
	if !platformOK(plat) {
		pq.Status, pq.Reason = osquery.PackOtherOS, "Only for "+plat+", not proven yet."
		return pq, nil
	}
	for _, t := range pq.Tables {
		if osquery.IsDeniedTable(t) {
			pq.Status, pq.Reason = osquery.PackSensitive, "Skipped: sensitive data ("+t+")."
			return pq, nil
		}
	}
	for _, t := range pq.Tables {
		if !osquery.AllowedTables[t] {
			pq.Status, pq.Reason = osquery.PackNotReview, "Table "+t+" is not reviewed yet."
			return pq, nil
		}
	}
	rows, err := sample(bin, sql)
	if err != nil {
		pq.Status, pq.Reason = osquery.PackUnverified, "Could not be run when the pack was frozen: "+err.Error()
		return pq, nil
	}
	if len(rows) == 0 {
		pq.Status, pq.Reason = osquery.PackUnverified, "Returned no rows when the pack was frozen, so its columns could not be confirmed."
		return pq, nil
	}
	cols := make([]string, 0, len(rows[0]))
	for c := range rows[0] {
		cols = append(cols, c)
	}
	sort.Strings(cols)
	ex := rows
	if len(ex) > 3 {
		ex = ex[:3]
	}
	key := "osquery.pack." + strings.ReplaceAll(pack, "-", "_") + "." + strings.Trim(nonKey.ReplaceAllString(strings.ToLower(name), "_"), "_")
	title := strings.ReplaceAll(name, "_", " ")
	q := &osquery.Query{Key: key, Pack: pack, Title: title, Description: desc, Category: "Pack: " + pack, SQL: sql, Tables: pq.Tables, Columns: cols,
		OS: []string{"linux"}, Sensitivity: "medium", Load: "low", MaxRows: 1000, TimeoutSeconds: 60, Example: ex,
		Attributes: osquery.Attributes{Aggregates: []osquery.Aggregate{{Key: "row_count", Op: "count"}}}}
	q.Mitre = mitreFor(pq.Tables)
	if err := q.Validate(); err != nil {
		pq.Status, pq.Reason = osquery.PackSensitive, "Skipped: "+err.Error()
		return pq, nil
	}
	pq.Status, pq.Key, pq.Reason = osquery.PackRunnable, key, ""
	return pq, q
}

func sample(bin, sql string) ([]map[string]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "-S", "--json", "--ephemeral", "--disable_extensions", "--disable_events", "--disable_watchdog", "--disable_logging", sql)
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C"}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 100 {
			msg = msg[:100]
		}
		return nil, fmt.Errorf("%v %s", err, msg)
	}
	var rows []map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &rows); err != nil {
		return nil, err
	}
	out := make([]map[string]string, 0, len(rows))
	for _, r := range rows {
		m := map[string]string{}
		for k, v := range r {
			s := fmt.Sprint(v)
			if len(s) > 200 {
				s = s[:200]
			}
			m[k] = s
		}
		out = append(out, m)
	}
	return out, nil
}

// mitreByTable: teknik ATT&CK yang paling dekat dengan data yang dibaca, dengan alasan tertulis (pemetaan konservatif).
var mitreByTable = map[string]osquery.MitreRef{
	"deb_packages":    {ID: "T1518", Reason: "Listing installed software is Software Discovery."},
	"rpm_packages":    {ID: "T1518", Reason: "Listing installed software is Software Discovery."},
	"kernel_info":     {ID: "T1082", Reason: "Kernel and boot details are System Information Discovery."},
	"os_version":      {ID: "T1082", Reason: "Operating system version is System Information Discovery."},
	"kernel_modules":  {ID: "T1082", Reason: "Loaded kernel modules are System Information Discovery."},
	"disk_encryption": {ID: "T1082", Reason: "Disk encryption state is System Information Discovery."},
	"mounts":          {ID: "T1082", Reason: "Mounted file systems are System Information Discovery."},
	"time":            {ID: "T1082", Reason: "The system clock is System Information Discovery."},
	"etc_hosts":       {ID: "T1016", Reason: "Host name entries are System Network Configuration Discovery."},
	"listening_ports": {ID: "T1049", Reason: "Listening sockets are System Network Connections Discovery."},
	"usb_devices":     {ID: "T1120", Reason: "Attached USB devices are Peripheral Device Discovery."},
	"pci_devices":     {ID: "T1120", Reason: "Attached PCI devices are Peripheral Device Discovery."},
}

func mitreFor(tables []string) []osquery.MitreRef {
	out := []osquery.MitreRef{}
	seen := map[string]bool{}
	for _, t := range tables {
		if m, ok := mitreByTable[t]; ok && !seen[m.ID] {
			seen[m.ID] = true
			out = append(out, m)
		}
	}
	return out
}
