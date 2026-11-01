package osquery

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCatalogQueriesAreValidNamedAndFrozen(t *testing.T) {
	if Version == "" || len(Queries) < 6 {
		t.Fatalf("katalog kosong: %q %d", Version, len(Queries))
	}
	seen := map[string]bool{}
	for i := range Queries {
		q := &Queries[i]
		if seen[q.Key] {
			t.Errorf("kunci ganda %s", q.Key)
		}
		seen[q.Key] = true
		if err := q.Validate(); err != nil {
			t.Errorf("%s: %v", q.Key, err)
		}
		if len(q.Mitre) == 0 || q.Mitre[0].Reason == "" {
			t.Errorf("%s: pemetaan MITRE wajib beralasan", q.Key)
		}
		for _, r := range q.Rules {
			if r.Finding.Type == "vulnerability" {
				t.Errorf("%s/%s: aturan tidak boleh mengarang kerentanan/CVE tanpa data", q.Key, r.ID)
			}
			if r.Explain == "" {
				t.Errorf("%s/%s: aturan wajib punya penjelasan awam", q.Key, r.ID)
			}
		}
	}
	if Find("osquery.inventory") == nil || Find("SELECT 1") != nil || Find("osquery.nope") != nil {
		t.Fatal("Find hanya boleh mengenali kunci katalog")
	}
}

func TestValidateRejectsSensitiveAndFreeFormSQL(t *testing.T) {
	base := func(sql string, tables ...string) *Query {
		return &Query{Key: "osquery.x", SQL: sql, Tables: tables, Columns: []string{"a"}, OS: []string{"linux"}, MaxRows: 10, TimeoutSeconds: 10, Example: []map[string]string{{"a": "x"}}}
	}
	cases := []struct {
		name string
		q    *Query
	}{
		{"kunci SSH", base("SELECT a FROM user_ssh_keys", "user_ssh_keys")},
		{"ekstensi peramban (awalan)", base("SELECT a FROM chrome_extensions", "chrome_extensions")},
		{"environment proses", base("SELECT a FROM process_envs", "process_envs")},
		{"isi file", base("SELECT a FROM file", "file")},
		{"tabel di luar daftar izin", base("SELECT a FROM arp_cache", "arp_cache")},
		{"selundup lewat subquery", base("SELECT username AS a FROM users WHERE uid IN (SELECT uid FROM process_envs)", "users")},
		{"selundup lewat join koma", base("SELECT username AS a FROM users u, file f", "users")},
		{"selundup lewat join", base("SELECT username AS a FROM users u JOIN shadow s ON s.username = u.username", "users")},
		{"kolom cmdline", base("SELECT cmdline AS a FROM processes", "processes")},
		{"kolom password", base("SELECT password AS a FROM users", "users")},
		{"banyak pernyataan", base("SELECT a FROM users; DROP TABLE x", "users")},
		{"komentar", base("SELECT a FROM users -- x", "users")},
		{"bukan SELECT", base("PRAGMA table_info(users)", "users")},
		{"ATTACH", base("SELECT a FROM users WHERE 1 UNION SELECT 1 FROM x ATTACH", "users")},
		{"kunci bukan osquery.", &Query{Key: "other.x", SQL: "SELECT a FROM users", Tables: []string{"users"}, Columns: []string{"a"}, OS: []string{"linux"}, MaxRows: 1, TimeoutSeconds: 1}},
		{"tabel dideklarasikan tak cocok", base("SELECT a FROM users", "groups")},
		{"tanpa batas baris", &Query{Key: "osquery.x", SQL: "SELECT a FROM users", Tables: []string{"users"}, Columns: []string{"a"}, OS: []string{"linux"}, TimeoutSeconds: 1}},
	}
	for _, c := range cases {
		if err := c.q.Validate(); err == nil {
			t.Errorf("%s: harus ditolak", c.name)
		}
	}
	if err := base("SELECT a FROM users", "users").Validate(); err != nil {
		t.Fatalf("query sah ditolak: %v", err)
	}
}

func TestTablesInParsesJoinsCommasAliasesAndSubqueries(t *testing.T) {
	cases := map[string]string{
		"SELECT * FROM a x, b y, c z WHERE 1":                             "a,b,c",
		"SELECT * FROM a LEFT JOIN b ON a.id = b.id JOIN c USING (id)":    "a,b,c",
		"SELECT (SELECT count(*) FROM q WHERE 1) FROM p AS r":             "p,q",
		"SELECT 1 FROM a UNION ALL SELECT 2 FROM b":                       "a,b",
		"SELECT x FROM a WHERE y IN (SELECT y FROM secret_table) LIMIT 1": "a,secret_table",
	}
	for sql, want := range cases {
		if got := strings.Join(TablesIn(sql), ","); got != want {
			t.Errorf("%q: dapat %q, mau %q", sql, got, want)
		}
	}
}

func TestSanitizeTreatsAgentDataAsUntrusted(t *testing.T) {
	q := Find("osquery.users")
	long := strings.Repeat("x", 2000)
	in := &Dataset{Query: q.Key, Status: StatusOK, Rows: []map[string]string{
		{"username": "alice", "uid": "1000", "shell": "/bin/bash", "groups": "sudo", "password_hash": "$6$secret", "extra": "injected"},
		{"username": long, "uid": "1", "shell": "token=abcd1234", "groups": ""},
	}}
	out, err := Sanitize(q, in)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := out.Rows[0]["password_hash"]; ok || out.Rows[0]["extra"] != "" {
		t.Fatalf("kolom di luar skema harus dibuang: %v", out.Rows[0])
	}
	if len([]rune(out.Rows[1]["username"])) != maxCell {
		t.Fatalf("sel harus dipotong ke %d", maxCell)
	}
	if strings.Contains(out.Rows[1]["shell"], "abcd1234") || !strings.Contains(out.Rows[1]["shell"], "[redacted]") {
		t.Fatalf("nilai rahasia harus diredaksi: %q", out.Rows[1]["shell"])
	}
	if _, still := in.Rows[0]["password_hash"]; !still {
		t.Fatal("masukan tidak boleh diubah")
	}
	// Batas baris.
	many := &Dataset{Query: q.Key, Status: StatusOK}
	for i := 0; i < q.MaxRows+50; i++ {
		many.Rows = append(many.Rows, map[string]string{"username": fmt.Sprint(i)})
	}
	if o, _ := Sanitize(q, many); len(o.Rows) != q.MaxRows || !o.Truncated {
		t.Fatalf("baris harus dibatasi %d dan ditandai terpotong: %d %v", q.MaxRows, len(o.Rows), o.Truncated)
	}
	// Dataset salah query, status asing, dan status non-ok.
	if _, err := Sanitize(q, &Dataset{Query: "osquery.network", Status: StatusOK}); err == nil {
		t.Fatal("dataset milik query lain harus ditolak")
	}
	if _, err := Sanitize(q, &Dataset{Query: q.Key, Status: "hacked"}); err == nil {
		t.Fatal("status asing harus ditolak")
	}
	if o, _ := Sanitize(q, &Dataset{Query: q.Key, Status: StatusSkipped, Reason: ReasonNeedsRoot, Rows: []map[string]string{{"username": "leak"}}}); len(o.Rows) != 0 || o.Reason != ReasonNeedsRoot {
		t.Fatalf("dataset dilewati tidak boleh membawa baris: %+v", o)
	}
	// Waktu pengambilan palsu di masa depan dikoreksi.
	if o, _ := Sanitize(q, &Dataset{Query: q.Key, Status: StatusOK, CollectedAt: time.Now().Add(24 * time.Hour)}); o.CollectedAt.After(time.Now().Add(time.Minute)) {
		t.Fatal("waktu masa depan harus dikoreksi")
	}
}

func rows(kv ...[]string) []map[string]string {
	var out []map[string]string
	for _, r := range kv {
		m := map[string]string{}
		for i := 0; i+1 < len(r); i += 2 {
			m[r[i]] = r[i+1]
		}
		out = append(out, m)
	}
	return out
}

func TestMapProducesAttributesAndFindingsFromRules(t *testing.T) {
	u := Find("osquery.users")
	ds := &Dataset{Query: u.Key, Status: StatusOK, Rows: rows(
		[]string{"username", "root", "uid", "0", "shell", "/bin/bash", "groups", "root"},
		[]string{"username", "toor", "uid", "0", "shell", "/bin/sh", "groups", "root"},
		[]string{"username", "a", "uid", "1000", "shell", "/bin/bash", "groups", "sudo"},
		[]string{"username", "b", "uid", "1001", "shell", "/bin/bash", "groups", "wheel,dev"},
		[]string{"username", "c", "uid", "1002", "shell", "/bin/zsh", "groups", "admin"},
		[]string{"username", "d", "uid", "1003", "shell", "/usr/sbin/nologin", "groups", "sudo"},
	)}
	m := u.Map(ds)
	if m.Attributes["local_account_count"] != 6 {
		t.Fatalf("jumlah akun: %v", m.Attributes["local_account_count"])
	}
	var titles []string
	for _, f := range m.Findings {
		titles = append(titles, f.Severity+"|"+f.Title)
	}
	got := strings.Join(titles, ";")
	if !strings.Contains(got, "high|Account toor has superuser (UID 0) rights") || strings.Contains(got, "Account root has") {
		t.Fatalf("hanya akun UID 0 selain root yang jadi temuan: %s", got)
	}
	if !strings.Contains(got, "medium|4 local accounts have administrator group membership") {
		t.Fatalf("agregat admin salah: %s", got)
	}
	// Data jinak: tidak ada temuan.
	benign := u.Map(&Dataset{Query: u.Key, Status: StatusOK, Rows: rows([]string{"username", "root", "uid", "0", "shell", "/bin/bash", "groups", "root"}, []string{"username", "a", "uid", "1000", "shell", "/bin/bash", "groups", "sudo"})})
	if len(benign.Findings) != 0 {
		t.Fatalf("data jinak tidak boleh menghasilkan temuan: %+v", benign.Findings)
	}
	// Dataset tidak ok tidak menghasilkan apa pun.
	if mm := u.Map(&Dataset{Query: u.Key, Status: StatusSkipped}); len(mm.Findings) != 0 || len(mm.Attributes) != 0 {
		t.Fatal("dataset dilewati tidak boleh dipetakan")
	}
}

func TestMapHardeningThresholdsAndDedupe(t *testing.T) {
	h := Find("osquery.hardening")
	m := h.Map(&Dataset{Query: h.Key, Status: StatusOK, Rows: rows(
		[]string{"item", "sysctl", "target", "kernel.randomize_va_space", "value", "0"},
		[]string{"item", "sysctl", "target", "net.ipv4.ip_forward", "value", "1"},
		[]string{"item", "sysctl", "target", "net.ipv4.tcp_syncookies", "value", "1"},
		[]string{"item", "disk", "target", "/dev/sda", "value", "0"},
		[]string{"item", "disk", "target", "/dev/sdb", "value", "0"},
		[]string{"item", "disk", "target", "/dev/sdc", "value", "1"},
	)})
	titles := map[string]string{}
	for _, f := range m.Findings {
		titles[f.Rule] = f.Severity + "|" + f.Title
	}
	if titles["aslr-weak"] != "high|Address space randomisation (ASLR) is not fully enabled" || titles["ip-forward-on"] == "" ||
		titles["disk-unencrypted"] != "low|2 storage devices report no encryption" || titles["syncookies-off"] != "" {
		t.Fatalf("temuan hardening salah: %v", titles)
	}
	n := Find("osquery.network")
	nm := n.Map(&Dataset{Query: n.Key, Status: StatusOK, Rows: rows(
		[]string{"port", "22", "address", "0.0.0.0", "protocol", "6", "process", "sshd"},
		[]string{"port", "22", "address", "::", "protocol", "6", "process", "sshd"},
		[]string{"port", "53", "address", "0.0.0.0", "protocol", "17", "process", "dns"},
		[]string{"port", "8090", "address", "127.0.0.1", "protocol", "6", "process", "server"},
	)})
	if len(nm.Findings) != 1 || nm.Findings[0].Value != "tcp/22" || nm.Findings[0].Type != "open_port" {
		t.Fatalf("v4 dan v6 pada port yang sama harus satu temuan; UDP dan loopback tidak: %+v", nm.Findings)
	}
}

// Biner palsu: skrip yang mencetak JSON, untuk menguji batas tanpa osquery sungguhan.
func fakeBin(t *testing.T, script string) (bin, argsFile string) {
	t.Helper()
	dir := t.TempDir()
	argsFile = filepath.Join(dir, "args")
	bin = filepath.Join(dir, "osqueryd")
	body := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + argsFile + "\n" + script + "\n"
	if err := os.WriteFile(bin, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, argsFile
}

func TestRunnerOneShotArgumentsAndLimits(t *testing.T) {
	q := *Find("osquery.users")
	q.TimeoutSeconds = 1
	bin, argsFile := fakeBin(t, `echo '[{"username":"a","uid":"1","shell":"/bin/sh","groups":"","bogus":"x"}]'`)
	r := &Runner{Bin: bin, IsRoot: func() bool { return true }, GOOS: "linux"}
	ds := r.Run(context.Background(), &q)
	if ds.Status != StatusOK || len(ds.Rows) != 1 || ds.Rows[0]["bogus"] != "" {
		t.Fatalf("hasil: %+v", ds)
	}
	raw, _ := os.ReadFile(argsFile)
	args := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if args[len(args)-1] != q.SQL {
		t.Fatalf("SQL katalog harus argumen TERAKHIR: %v", args)
	}
	for _, want := range []string{"-S", "--json", "--ephemeral", "--disable_extensions", "--disable_events", "--disable_watchdog", "--disable_logging"} {
		if !strings.Contains(string(raw), want+"\n") {
			t.Errorf("flag %s hilang: %v", want, args)
		}
	}
	// Batas waktu.
	slow, _ := fakeBin(t, "sleep 5")
	if ds := (&Runner{Bin: slow, IsRoot: func() bool { return true }, GOOS: "linux"}).Run(context.Background(), &q); ds.Status != StatusFailed || !strings.Contains(ds.Reason, "time limit") {
		t.Fatalf("batas waktu: %+v", ds)
	}
	// Keluaran terlalu besar.
	big, _ := fakeBin(t, "head -c 9000000 /dev/zero | tr '\\0' 'a'")
	if ds := (&Runner{Bin: big, IsRoot: func() bool { return true }, GOOS: "linux"}).Run(context.Background(), &q); ds.Status != StatusFailed || !strings.Contains(ds.Reason, "too large") {
		t.Fatalf("keluaran besar: %+v", ds)
	}
	// JSON rusak dan keluar tidak nol.
	bad, _ := fakeBin(t, "echo not-json")
	if ds := (&Runner{Bin: bad, IsRoot: func() bool { return true }, GOOS: "linux"}).Run(context.Background(), &q); ds.Status != StatusFailed {
		t.Fatalf("JSON rusak: %+v", ds)
	}
	fail, _ := fakeBin(t, "echo boom >&2; exit 3")
	if ds := (&Runner{Bin: fail, IsRoot: func() bool { return true }, GOOS: "linux"}).Run(context.Background(), &q); ds.Status != StatusFailed || !strings.Contains(ds.Reason, "boom") {
		t.Fatalf("keluar nol: %+v", ds)
	}
	// Biner tidak ada.
	if ds := (&Runner{Bin: "/nonexistent/osqueryd", IsRoot: func() bool { return true }, GOOS: "linux"}).Run(context.Background(), &q); ds.Status != StatusFailed {
		t.Fatalf("biner hilang harus gagal tercatat: %+v", ds)
	}
}

func TestRunnerSkipsWithReasonInsteadOfFailingSilently(t *testing.T) {
	bin, argsFile := fakeBin(t, `echo '[]'`)
	fw := Find("osquery.firewall")
	if ds := (&Runner{Bin: bin, IsRoot: func() bool { return false }, GOOS: "linux"}).Run(context.Background(), fw); ds.Status != StatusSkipped || ds.Reason != ReasonNeedsRoot {
		t.Fatalf("tanpa hak admin harus dilewati dengan alasan: %+v", ds)
	}
	if _, err := os.Stat(argsFile); err == nil {
		t.Fatal("query yang dilewati tidak boleh dijalankan")
	}
	if ds := (&Runner{Bin: bin, IsRoot: func() bool { return true }, GOOS: "windows"}).Run(context.Background(), Find("osquery.inventory")); ds.Status != StatusSkipped || ds.Reason != ReasonUnsupported {
		t.Fatalf("OS tidak didukung harus dilewati: %+v", ds)
	}
	// Pertahanan berlapis: query rakitan yang menyentuh tabel terlarang tidak dijalankan.
	evil := &Query{Key: "osquery.evil", SQL: "SELECT a FROM user_ssh_keys", Tables: []string{"user_ssh_keys"}, Columns: []string{"a"}, OS: []string{"linux"}, MaxRows: 5, TimeoutSeconds: 5}
	if ds := (&Runner{Bin: bin, IsRoot: func() bool { return true }, GOOS: "linux"}).Run(context.Background(), evil); ds.Status != StatusSkipped || !strings.Contains(ds.Reason, "sensitive") {
		t.Fatalf("tabel terlarang harus dilewati sebagai sensitif: %+v", ds)
	}
	if _, err := os.Stat(argsFile); err == nil {
		t.Fatal("query terlarang tidak boleh sampai ke biner")
	}
}

// Uji dengan osquery sungguhan (opsional): ORCHIS_TEST_OSQUERY=/jalur/osqueryd.
func TestRealOsqueryBinary(t *testing.T) {
	bin := os.Getenv("ORCHIS_TEST_OSQUERY")
	if bin == "" {
		t.Skip("ORCHIS_TEST_OSQUERY tidak diisi")
	}
	r := &Runner{Bin: bin}
	for i := range Queries {
		q := &Queries[i]
		ds := r.Run(context.Background(), q)
		if ds.Status == StatusFailed {
			t.Errorf("%s gagal: %s", q.Key, ds.Reason)
		}
		if ds.Status == StatusOK && len(ds.Rows) == 0 && q.Key != "osquery.hardening" {
			t.Errorf("%s: tidak ada baris", q.Key)
		}
		for _, row := range ds.Rows {
			for col := range row {
				ok := false
				for _, c := range q.Columns {
					ok = ok || c == col
				}
				if !ok {
					t.Errorf("%s: kolom %s di luar skema lolos", q.Key, col)
				}
			}
		}
	}
	inv := Find("osquery.inventory")
	ds := r.Run(context.Background(), inv)
	if m := inv.Map(ds); m.Attributes["os_name"] == "" || m.Attributes["kernel"] == "" {
		t.Fatalf("atribut inventaris kosong: %v", m.Attributes)
	}
}

// Denylist adalah jaring pengaman kedua: bila daftar izin keliru ditambah tabel sensitif, query tetap ditolak.
func TestDenylistWinsEvenIfAllowlistIsMistakenlyExtended(t *testing.T) {
	for _, table := range []string{"user_ssh_keys", "chrome_extensions", "process_envs", "file", "shadow"} {
		AllowedTables[table] = true
		q := &Query{Key: "osquery.x", SQL: "SELECT a FROM " + table, Tables: []string{table}, Columns: []string{"a"}, OS: []string{"linux"}, MaxRows: 1, TimeoutSeconds: 1}
		err := q.Validate()
		delete(AllowedTables, table)
		if err == nil || !strings.Contains(err.Error(), "terlarang") {
			t.Errorf("%s: denylist harus menang atas daftar izin: %v", table, err)
		}
	}
}

func TestExamplesAreSafeAndMapToAttributes(t *testing.T) {
	for i := range Queries {
		q := &Queries[i]
		m := q.Map(&Dataset{Query: q.Key, Status: StatusOK, Rows: q.Example})
		if len(q.Attributes.FirstRow)+len(q.Attributes.Aggregates) > 0 && len(m.Attributes) == 0 {
			t.Errorf("%s: contoh tidak menghasilkan atribut", q.Key)
		}
		for _, row := range q.Example {
			for col, val := range row {
				if strings.Contains(strings.ToLower(val), "[redacted]") || strings.Contains(strings.ToLower(col+val), "password") {
					t.Errorf("%s: contoh memuat nilai sensitif: %s=%s", q.Key, col, val)
				}
			}
		}
	}
	// Contoh dengan kolom di luar skema ditolak validasi.
	q := *Find("osquery.firewall")
	q.Example = []map[string]string{{"rules": "0", "password_hash": "x"}}
	if err := q.Validate(); err == nil {
		t.Fatal("contoh dengan kolom di luar skema harus ditolak")
	}
	q.Example = nil
	if err := q.Validate(); err == nil {
		t.Fatal("contoh data wajib")
	}
}

func TestComparePortsAndBuildPosture(t *testing.T) {
	pc := ComparePorts([]int{22, 80, 8080}, []int{22, 80, 5432})
	if Ints(pc.Both) != "22, 80" || Ints(pc.ScanOnly) != "8080" || Ints(pc.HostOnly) != "5432" || pc.Note == "" {
		t.Fatalf("perbandingan port: %+v", pc)
	}
	if Ints(nil) != "none" {
		t.Fatal("daftar kosong harus 'none'")
	}
	net := Find("osquery.network")
	hard := Find("osquery.hardening")
	inv := Find("osquery.inventory")
	sets := map[string]*Dataset{
		net.Key: {Query: net.Key, Status: StatusOK, Rows: rows(
			[]string{"port", "22", "address", "0.0.0.0", "protocol", "6", "process", "sshd"},
			[]string{"port", "5432", "address", "::", "protocol", "6", "process", "postgres"},
			[]string{"port", "53", "address", "0.0.0.0", "protocol", "17", "process", "dns"},
			[]string{"port", "9000", "address", "127.0.0.1", "protocol", "6", "process", "local"})},
		hard.Key:        {Query: hard.Key, Status: StatusOK, Rows: rows([]string{"item", "sysctl", "target", "kernel.randomize_va_space", "value", "0"})},
		inv.Key:         {Query: inv.Key, Status: StatusOK, Version: "5.24.0", Rows: rows([]string{"hostname", "secret-host", "os_name", "Ubuntu", "kernel", "7.0", "uptime_seconds", "10"})},
		"osquery.users": {Query: "osquery.users", Status: StatusSkipped, Reason: ReasonDisabled},
	}
	p := BuildPosture(sets, []int{22, 80})
	if p == nil || p.Source != "osquery" || p.Version != "5.24.0" {
		t.Fatalf("postur: %+v", p)
	}
	if p.Attributes["os_name"] != "Ubuntu" || p.Attributes["hostname"] != nil {
		t.Fatalf("hanya atribut daftar putih yang masuk (tanpa nama host): %v", p.Attributes)
	}
	if p.Ports == nil || Ints(p.Ports.Both) != "22" || Ints(p.Ports.ScanOnly) != "80" || Ints(p.Ports.HostOnly) != "5432" {
		t.Fatalf("port: %+v", p.Ports)
	}
	if len(p.Findings) == 0 || p.Findings[0].Severity != "high" {
		t.Fatalf("temuan harus terurut dari yang terparah: %+v", p.Findings)
	}
	if BuildPosture(map[string]*Dataset{"osquery.users": {Query: "osquery.users", Status: StatusSkipped}}, nil) != nil {
		t.Fatal("tanpa dataset ok tidak ada postur")
	}
	if BuildPosture(nil, nil) != nil {
		t.Fatal("tanpa data tidak ada postur")
	}
}

func TestFrozenPacksAreSafeAndConsistent(t *testing.T) {
	if len(Packs) != 5 {
		t.Fatalf("lima pack resmi diharapkan: %d", len(Packs))
	}
	runnable := map[string]bool{}
	for _, p := range Packs {
		if len(p.SHA256) != 64 || p.File != p.Name+".conf" || len(p.Queries) == 0 {
			t.Errorf("pack %s tidak lengkap", p.Name)
		}
		for _, pq := range p.Queries {
			switch pq.Status {
			case PackRunnable:
				q := Find(pq.Key)
				if q == nil || q.Pack != p.Name || q.Sensitivity == "low" || pq.Reason != "" {
					t.Errorf("%s/%s: query pack harus ada di katalog, milik pack, dan butuh persetujuan", p.Name, pq.Name)
					continue
				}
				runnable[pq.Key] = true
			case PackSensitive, PackNotReview, PackOtherOS, PackUnverified:
				if pq.Key != "" || pq.Reason == "" || Find("osquery.pack."+p.Name+"."+pq.Name) != nil {
					t.Errorf("%s/%s: query yang dilewati wajib beralasan dan tidak boleh ada di katalog", p.Name, pq.Name)
				}
			default:
				t.Errorf("%s/%s: status %q tidak dikenal", p.Name, pq.Name, pq.Status)
			}
			// Query yang menyentuh tabel terlarang TIDAK PERNAH runnable, apa pun status lain.
			for _, tb := range pq.Tables {
				if IsDeniedTable(tb) && pq.Status == PackRunnable {
					t.Errorf("%s/%s membaca tabel terlarang %s", p.Name, pq.Name, tb)
				}
			}
		}
	}
	for _, key := range PackQueryKeys() {
		if !runnable[key] {
			t.Errorf("%s ada di katalog tetapi tidak ditandai runnable di packnya", key)
		}
	}
	// ossec-rootkit hanya membaca berkas (tabel file/hash): seluruhnya dilewati sebagai data sensitif.
	if len(FindPack("ossec-rootkit").RunnableKeys()) != 0 || FindPack("nope") != nil {
		t.Fatal("ossec-rootkit tidak boleh punya query yang berjalan")
	}
	// Pack yang dibekukan hanya boleh memuat tabel yang sudah ditinjau.
	for _, key := range PackQueryKeys() {
		for _, tb := range Find(key).Tables {
			if !AllowedTables[tb] || IsDeniedTable(tb) {
				t.Errorf("%s membaca tabel %s di luar daftar izin", key, tb)
			}
		}
	}
}

func TestPackQueriesCannotBeLoadedWithTamperedMetadata(t *testing.T) {
	q := *Find(PackQueryKeys()[0])
	bad := []Query{}
	for _, mut := range []func(*Query){
		func(q *Query) { q.SQL = "SELECT * FROM file" },
		func(q *Query) { q.SQL += "; SELECT 1" },
		func(q *Query) { q.Tables = append(q.Tables, "hash") },
		func(q *Query) { q.Key = "other.pack" },
	} {
		c := q
		mut(&c)
		bad = append(bad, c)
	}
	for i, c := range bad {
		if c.Validate() == nil {
			t.Errorf("perubahan %d lolos validasi", i)
		}
	}
}

func TestParsePacksRejectsTamperedFiles(t *testing.T) {
	good := func() map[string]any {
		return map[string]any{"osquery_version": Version, "artifact_sha256": Artifacts["linux/amd64"].SHA256, "packs": []any{}, "queries": []any{}}
	}
	enc := func(m map[string]any) []byte { b, _ := json.Marshal(m); return b }
	if _, _, err := parsePacks(enc(good()), Version, Artifacts, Queries); err != nil {
		t.Fatalf("berkas baik ditolak: %v", err)
	}
	pq := *Find(PackQueryKeys()[0])
	withQuery := func(q Query) map[string]any { m := good(); m["queries"] = []any{q}; return m }
	cases := map[string][]byte{}
	m := good()
	m["osquery_version"] = "9.9.9"
	cases["versi lain"] = enc(m)
	m = good()
	m["artifact_sha256"] = strings.Repeat("0", 64)
	cases["hash artefak lain"] = enc(m)
	cases["kunci ganda"] = enc(withQuery(pq))
	bad := pq
	bad.Key = "osquery.pack.x.y"
	bad.SQL = "SELECT * FROM file"
	cases["tabel terlarang"] = enc(withQuery(bad))
	bad = pq
	bad.Key = "osquery.pack.x.z"
	bad.Pack = ""
	cases["tanpa nama pack"] = enc(withQuery(bad))
	bad = pq
	bad.Key = "osquery.inventory2"
	cases["bukan kunci pack"] = enc(withQuery(bad))
	cases["bukan json"] = []byte("{")
	for name, raw := range cases {
		if _, _, err := parsePacks(raw, Version, Artifacts, Queries); err == nil {
			t.Errorf("%s: seharusnya ditolak", name)
		}
	}
	if _, _, err := parsePacks(enc(good()), Version, map[string]Artifact{}, Queries); err == nil {
		t.Error("tanpa artefak yang dipatok seharusnya ditolak")
	}
}
