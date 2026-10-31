package agent

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"orchis-go/internal/agentproto"
	"orchis-go/internal/contract"
	"orchis-go/internal/execution"
)

const procTCP = `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0100007F:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 12345 1 0000000000000000 100 0 0 10 0
   1: 00000000:0016 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 22222 1 0000000000000000 100 0 0 10 0
   2: 0100007F:D431 0100007F:1F90 01 00000000:00000000 00:00000000 00000000  1000        0 33333 1 0000000000000000 100 0 0 10 0
   3: 0100007F:0050 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 44444 1 0000000000000000 100 0 0 10 0
`

const procTCP6 = `  sl  local_address                         remote_address                        st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000000000000000000001000000:0277 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 55555 1 0000000000000000 100 0 0 10 0
   1: 00000000000000000000000000000000:0016 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 66666 1 0000000000000000 100 0 0 10 0
`

func TestParseProcNetKeepsOnlyListeningSockets(t *testing.T) {
	v4 := ParseProcNet(strings.NewReader(procTCP), false)
	if len(v4) != 3 { // 3 LISTEN (0A); baris ESTABLISHED (01) dilewati
		t.Fatalf("v4: %+v", v4)
	}
	want := map[int]string{8080: "127.0.0.1", 22: "0.0.0.0", 80: "127.0.0.1"}
	for _, b := range v4 {
		if want[b.port] != b.addr {
			t.Errorf("port %d: alamat %q, harapan %q", b.port, b.addr, want[b.port])
		}
	}
	v6 := ParseProcNet(strings.NewReader(procTCP6), true)
	if len(v6) != 2 || v6[0].port != 0x277 || v6[0].addr != "::1" || v6[1].port != 22 || v6[1].addr != "::" {
		t.Fatalf("v6: %+v", v6)
	}
	if got := ParseProcNet(strings.NewReader("header only\n"), false); len(got) != 0 {
		t.Errorf("tanpa soket: %+v", got)
	}
	if got := ParseProcNet(strings.NewReader("h\ngarbage line\n 0: ZZZZ:1F90 x 0A\n"), false); len(got) != 0 {
		t.Errorf("baris rusak harus dilewati, bukan menyebabkan panik atau temuan: %+v", got)
	}
}

func TestParseNetstatMacAndWindows(t *testing.T) {
	mac := `Active Internet connections (including servers)
Proto Recv-Q Send-Q  Local Address          Foreign Address        (state)
tcp4       0      0  *.22                   *.*                    LISTEN
tcp6       0      0  ::1.631                *.*                    LISTEN
tcp4       0      0  127.0.0.1.8080         *.*                    LISTEN
tcp4       0      0  192.168.1.5.52000      17.1.1.1.443           ESTABLISHED
udp4       0      0  *.5353                 *.*
`
	got := ParseNetstat(mac)
	if len(got) != 3 || got[0].port != 22 || got[0].addr != "*" || got[1].port != 631 || got[1].addr != "::1" || got[2].port != 8080 || got[2].addr != "127.0.0.1" {
		t.Fatalf("macOS: %+v", got)
	}
	win := `Active Connections

  Proto  Local Address          Foreign Address        State           PID
  TCP    0.0.0.0:135            0.0.0.0:0              LISTENING       1032
  TCP    [::]:445               [::]:0                 LISTENING       4
  TCP    127.0.0.1:5432         0.0.0.0:0              LISTENING       2200
  TCP    192.168.1.5:50000      52.1.1.1:443           ESTABLISHED     4000
`
	got = ParseNetstat(win)
	if len(got) != 3 || got[0].port != 135 || got[1].port != 445 || got[1].addr != "::" || got[2].port != 5432 {
		t.Fatalf("Windows: %+v", got)
	}
}

func TestPortFindingsMergeBindsAndDescribeScope(t *testing.T) {
	fs := portFindings([]bind{{22, "0.0.0.0"}, {22, "::"}, {8080, "127.0.0.1"}, {5000, "127.0.0.1"}, {5000, "192.168.1.5"}})
	if len(fs) != 3 || fs[0].Value != "tcp/22" || fs[1].Value != "tcp/5000" || fs[2].Value != "tcp/8080" {
		t.Fatalf("urutan dan gabungan: %+v", fs)
	}
	if !strings.Contains(fs[0].Title, "reachable from the network") || !strings.Contains(fs[2].Title, "loopback only") || !strings.Contains(fs[1].Title, "reachable from the network") {
		t.Fatalf("cakupan: %q %q %q", fs[0].Title, fs[1].Title, fs[2].Title)
	}
	for _, f := range fs {
		if f.Type != "open_port" || f.Severity != "info" || !json.Valid(f.Details) {
			t.Errorf("finding tidak sesuai kontrak: %+v", f)
		}
	}
}

func TestTasksAreTheOnlyThingTheAgentCanRun(t *testing.T) {
	if _, err := Task(context.Background(), "nmap.tcp_connect"); err == nil {
		t.Fatal("aksi injector tidak boleh dijalankan agent")
	}
	if _, err := Task(context.Background(), "rm -rf /"); err == nil {
		t.Fatal("kunci arbitrer harus ditolak")
	}
	fs, err := Task(context.Background(), contract.KeyAgentInfo)
	if err != nil || len(fs) != 1 || fs[0].Type != "text" || !strings.HasPrefix(fs[0].Value, "system:") {
		t.Fatalf("system_info: %+v %v", fs, err)
	}
	var facts map[string]any
	if json.Unmarshal(fs[0].Details, &facts) != nil || facts["arch"] != runtime.GOARCH || facts["os"] != runtime.GOOS {
		t.Fatalf("fakta: %s", fs[0].Details)
	}
}

func TestListeningPortsSeesARealListener(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("tes ini membaca /proc/net/tcp")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	fs, err := Task(context.Background(), contract.KeyAgentPorts)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range fs {
		if f.Value == "tcp/"+itoa(port) {
			found = strings.Contains(f.Title, "loopback only")
		}
	}
	if !found {
		t.Fatalf("port %d (loopback) harus terdeteksi", port)
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

// ---- loop agent terhadap server palsu ----

type fakeServer struct {
	mu        sync.Mutex
	srv       *httptest.Server
	results   []execution.Result
	resultIDs []string
	claims    int32
	beats     int32
	revoke    atomic.Bool
	latest    string
	task      string
}

func newFakeServer(t *testing.T, task string) *fakeServer {
	f := &fakeServer{task: task}
	id := uuid.New()
	mux := http.NewServeMux()
	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if f.revoke.Load() || r.Header.Get("Authorization") != "Bearer oag_secret" {
				w.WriteHeader(401)
				return
			}
			h(w, r)
		}
	}
	mux.HandleFunc("POST /api/v1/agent/heartbeat", auth(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&f.beats, 1)
		json.NewEncoder(w).Encode(map[string]any{"poll_seconds": 2, "latest_version": f.latest})
	}))
	mux.HandleFunc("POST /api/v1/agent/executions/claim", auth(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&f.claims, 1) > 1 || f.task == "" {
			w.WriteHeader(204)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"execution_id": id, "contract_key": f.task, "contract_version": 1, "target": "local", "params": map[string]any{}, "timeout_seconds": 30})
	}))
	mux.HandleFunc("POST /api/v1/agent/executions/{id}/result", auth(func(w http.ResponseWriter, r *http.Request) {
		var res execution.Result
		json.NewDecoder(r.Body).Decode(&res)
		f.mu.Lock()
		f.results = append(f.results, res)
		f.resultIDs = append(f.resultIDs, r.PathValue("id"))
		f.mu.Unlock()
		w.WriteHeader(204)
	}))
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func newAgent(t *testing.T, f *fakeServer) *Agent {
	dir := t.TempDir()
	cfg := &Config{ServerURL: f.srv.URL, AgentID: "a1", Secret: "oag_secret", Mode: "user"}
	path := filepath.Join(dir, "agent-config.json")
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}
	return &Agent{Cfg: cfg, ConfigPath: path, Version: "1.0.0", Exe: filepath.Join(dir, "orchis-agent"), Logf: func(string, ...any) {}}
}

func TestAgentLoopRunsTheClaimedTaskAndReportsFindings(t *testing.T) {
	f := newFakeServer(t, contract.KeyAgentInfo)
	a := newAgent(t, f)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() { a.Run(ctx); close(done) }()
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		n := len(f.results)
		f.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	cancel()
	<-done
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.results) != 1 {
		t.Fatalf("satu hasil diharapkan: %d", len(f.results))
	}
	r := f.results[0]
	if r.ExitCode == nil || *r.ExitCode != 0 || r.Error != "" || len(r.Findings) != 1 || r.Findings[0].Type != "text" {
		t.Fatalf("hasil: %+v", r)
	}
	if atomic.LoadInt32(&f.beats) < 1 {
		t.Fatal("agent harus berdetak")
	}
}

func TestAgentReportsErrorForUnknownTaskInsteadOfRunningIt(t *testing.T) {
	f := newFakeServer(t, "nmap.tcp_connect")
	a := newAgent(t, f)
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	go a.Run(ctx)
	for i := 0; i < 100; i++ {
		f.mu.Lock()
		n := len(f.results)
		f.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.results) != 1 || f.results[0].Error == "" || len(f.results[0].Findings) != 0 || *f.results[0].ExitCode != 1 {
		t.Fatalf("aksi tak dikenal harus dilaporkan sebagai galat tanpa temuan: %+v", f.results)
	}
}

func TestRevokedAgentWipesItsSecretAndGoesQuiet(t *testing.T) {
	f := newFakeServer(t, "")
	f.revoke.Store(true)
	a := newAgent(t, f)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { a.Run(ctx); close(done) }()
	time.Sleep(700 * time.Millisecond)
	select {
	case <-done:
		t.Fatal("agent yang dicabut tidak boleh keluar (service manager akan menghidupkannya terus)")
	default:
	}
	c, err := LoadConfig(a.ConfigPath)
	if err != nil || !c.Revoked || c.Secret != "" {
		t.Fatalf("rahasia harus dihapus dari disk: %+v %v", c, err)
	}
	cancel()
	<-done
	// dijalankan lagi dengan konfigurasi itu: diam, tidak menghubungi server
	b := &Agent{Cfg: c, ConfigPath: a.ConfigPath, Version: "1.0.0", Exe: a.Exe, Logf: func(string, ...any) {}}
	before := atomic.LoadInt32(&f.beats)
	ctx2, cancel2 := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel2()
	b.Run(ctx2)
	if atomic.LoadInt32(&f.beats) != before {
		t.Fatal("agent yang sudah dicabut tidak boleh menghubungi server lagi")
	}
}

// ---- update diri ----

type release struct {
	version, content, sig string
	headerVersion         string
}

func serveRelease(t *testing.T, r release) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Authorization") != "Bearer oag_secret" {
			w.WriteHeader(401)
			return
		}
		hv := r.headerVersion
		if hv == "" {
			hv = r.version
		}
		if hv != "-" {
			w.Header().Set("X-Release-Version", hv)
		}
		if r.sig != "-" {
			w.Header().Set("X-Signature-Ed25519", r.sig)
		}
		w.Write([]byte(r.content))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func signed(priv ed25519.PrivateKey, version, content string) string {
	sum := sha256.Sum256([]byte(content))
	return base64.StdEncoding.EncodeToString(ed25519.Sign(priv, agentproto.SignedMessage(version, runtime.GOOS, runtime.GOARCH, hex.EncodeToString(sum[:]))))
}

func script(version string) string {
	return "#!/bin/sh\necho \"" + version + " " + runtime.GOOS + "/" + runtime.GOARCH + "\"\n"
}

func TestSelfUpdateInstallsOnlyGenuineNewerReleases(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("memakai skrip shell sebagai biner uji")
	}
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	_, otherPriv, _ := ed25519.GenerateKey(rand.Reader)
	good := script("1.1.0")
	cases := []struct {
		name    string
		rel     release
		want    string
		wantErr string
	}{
		{"rilis sah", release{version: "1.1.0", content: good, sig: signed(priv, "1.1.0", good)}, "1.1.0", ""},
		{"ditandatangani kunci lain", release{version: "1.1.0", content: good, sig: signed(otherPriv, "1.1.0", good)}, "1.1.0", "TIDAK cocok"},
		{"isi diubah setelah ditandatangani", release{version: "1.1.0", content: good + "# x", sig: signed(priv, "1.1.0", good)}, "1.1.0", "TIDAK cocok"},
		{"tanpa tanda tangan", release{version: "1.1.0", content: good, sig: "-"}, "1.1.0", "tidak mengirim"},
		{"tanpa versi", release{version: "1.1.0", content: good, sig: signed(priv, "1.1.0", good), headerVersion: "-"}, "1.1.0", "tidak mengirim"},
		{"tanda tangan bukan base64", release{version: "1.1.0", content: good, sig: "!!!"}, "1.1.0", "base64"},
		{"versi di header berbeda dari yang ditawarkan", release{version: "1.1.0", content: good, sig: signed(priv, "1.2.0", good), headerVersion: "1.2.0"}, "1.1.0", "tidak sama"},
		{"tanda tangan untuk versi lain", release{version: "1.1.0", content: good, sig: signed(priv, "9.9.9", good)}, "1.1.0", "TIDAK cocok"},
		{"downgrade", release{version: "0.9.0", content: script("0.9.0"), sig: signed(priv, "0.9.0", script("0.9.0"))}, "0.9.0", "anti-downgrade"},
		{"versi sama", release{version: "1.0.0", content: script("1.0.0"), sig: signed(priv, "1.0.0", script("1.0.0"))}, "1.0.0", "anti-downgrade"},
		{"gagal uji asap (biner melapor versi lain)", release{version: "1.1.0", content: script("7.7.7"), sig: signed(priv, "1.1.0", script("7.7.7"))}, "1.1.0", "uji asap"},
		{"biner yang tidak bisa dijalankan", release{version: "1.1.0", content: "bukan program", sig: signed(priv, "1.1.0", "bukan program")}, "1.1.0", "uji asap"},
	}
	for _, c := range cases {
		dir := t.TempDir()
		exe := filepath.Join(dir, "orchis-agent")
		os.WriteFile(exe, []byte("ORIGINAL"), 0o755)
		srv := serveRelease(t, c.rel)
		a := &Agent{Cfg: &Config{ServerURL: srv.URL, Secret: "oag_secret"}, Version: "1.0.0", PubKey: pub, Exe: exe, Logf: func(string, ...any) {}}
		err := a.selfUpdate(context.Background(), c.want)
		b, _ := os.ReadFile(exe)
		if c.wantErr == "" {
			if err != nil || string(b) != good {
				t.Errorf("%s: harus terpasang: err=%v isi=%q", c.name, err, b)
			}
			if _, err := os.Stat(exe + ".old"); err != nil {
				t.Errorf("%s: biner lama harus disimpan sebagai .old: %v", c.name, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), c.wantErr) {
			t.Errorf("%s: harus ditolak dengan %q, dapat %v", c.name, c.wantErr, err)
		}
		if string(b) != "ORIGINAL" {
			t.Errorf("%s: biner terpasang TIDAK boleh berubah saat update ditolak: %q", c.name, b)
		}
		if left, _ := filepath.Glob(filepath.Join(dir, ".stage-*")); len(left) != 0 {
			t.Errorf("%s: berkas sementara harus dibersihkan: %v", c.name, left)
		}
	}
}

func TestAgentWithoutEmbeddedKeyNeverUpdates(t *testing.T) {
	f := newFakeServer(t, "")
	f.latest = "9.9.9"
	a := newAgent(t, f)
	restarted := false
	a.Restart = func() { restarted = true }
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	a.Run(ctx)
	if restarted {
		t.Fatal("tanpa kunci rilis tertanam, agent tidak boleh memperbarui diri")
	}
}

func TestEnrollRejectsUnknownServerAnswers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		json.NewDecoder(r.Body).Decode(&in)
		if in["token"] != "oen_ok" {
			w.WriteHeader(400)
			w.Write([]byte(`{"title":"Enrollment failed","detail":"This enrollment token is invalid, already used, or expired."}`))
			return
		}
		if in["protocol_version"] != float64(agentproto.ProtocolVersion) || in["hostname"] == "" {
			w.WriteHeader(422)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"agent_id": "a1", "secret": "oag_x", "mode": "service"})
	}))
	defer srv.Close()
	if _, err := Enroll(context.Background(), srv.URL, "oen_bad", "1.0.0", nil); err == nil || !strings.Contains(err.Error(), "already used") {
		t.Fatalf("pesan server harus sampai ke pengguna: %v", err)
	}
	c, err := Enroll(context.Background(), srv.URL, "oen_ok", "1.0.0", nil)
	if err != nil || c.Secret != "oag_x" || c.Mode != "service" || c.AgentID != "a1" {
		t.Fatalf("enroll: %+v %v", c, err)
	}
	path := filepath.Join(t.TempDir(), "c.json")
	if err := c.Save(path); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(path)
	if runtime.GOOS != "windows" && st.Mode().Perm() != 0o600 {
		t.Fatalf("berkas konfigurasi berisi rahasia harus 0600: %v", st.Mode().Perm())
	}
}
