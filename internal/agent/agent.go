package agent

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"orchis-go/internal/agentproto"
	"orchis-go/internal/contract"
	"orchis-go/internal/execution"
	"orchis-go/internal/osquery"
)

// Config disimpan di samping biner (izin 0600). Secret adalah kredensial per agent yang diterbitkan saat pendaftaran.
type Config struct {
	Name      string            `json:"name,omitempty"`
	ServerURL string            `json:"server_url"`
	AgentID   string            `json:"agent_id"`
	Secret    string            `json:"secret"`
	Mode      string            `json:"mode"`
	Revoked   bool              `json:"revoked,omitempty"`
	Uninstall *UninstallReceipt `json:"uninstall,omitempty"`
	CAFile    string            `json:"ca_file,omitempty"` // PEM CA of an installation with its own HTTPS CA
}

func LoadConfig(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) Save(path string) error {
	b, _ := json.MarshalIndent(c, "", "  ")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

var ErrRevoked = errors.New("kredensial agent dicabut")

type httpError struct {
	Status int
	Body   string
}

func (e *httpError) Error() string { return fmt.Sprintf("server menjawab %d: %s", e.Status, e.Body) }

type Agent struct {
	Cfg        *Config
	ConfigPath string
	Version    string
	PubKey     ed25519.PublicKey // kunci rilis tepercaya yang tertanam di biner; kosong = update dimatikan
	HTTP       *http.Client
	Exe        string // jalur biner yang berjalan, dihitung SEKALI saat proses mulai (lihat catatan spike)
	// Restart dipanggil setelah update terpasang: service manager menghidupkan ulang, atau proses menjalankan diri lagi.
	StartUninstall func() error
	ServiceManaged bool
	Restart        func()
	Logf           func(format string, a ...any)
	// Optional dependency injection for a local query runner; never set from a remote command.
	RunNamedQuery func(context.Context, string) ([]osquery.Dataset, error)

	mu             sync.Mutex
	diagnosticCode string
	osqueryVersion string
}

func (a *Agent) logf(f string, v ...any) {
	if a.Logf != nil {
		a.Logf(f, v...)
		return
	}
	log.Printf(f, v...)
}

func (a *Agent) client() *http.Client {
	if a.HTTP != nil {
		return a.HTTP
	}
	return clientFor(a.caFile(), 30*time.Second)
}

func (a *Agent) do(ctx context.Context, method, path string, in any, out any) (int, error) {
	var rd io.Reader
	if in != nil {
		b, _ := json.Marshal(in)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(a.Cfg.ServerURL, "/")+"/api/v1/agent/"+path, rd)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.Cfg.Secret)
	resp, err := a.client().Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return resp.StatusCode, ErrRevoked
	case resp.StatusCode >= 300:
		return resp.StatusCode, &httpError{resp.StatusCode, strings.TrimSpace(string(body))}
	}
	if out != nil && len(body) > 0 {
		return resp.StatusCode, json.Unmarshal(body, out)
	}
	return resp.StatusCode, nil
}

// Enroll mendaftarkan agent dengan token sekali pakai dan mengembalikan config yang siap disimpan.
func Enroll(ctx context.Context, server, token, version string, hc *http.Client) (*Config, error) {
	return EnrollNamed(ctx, server, token, version, "", hc)
}

func EnrollNamed(ctx context.Context, server, token, version, name string, hc *http.Client) (*Config, error) {
	goos, ok1 := agentproto.PlatformOS(runtime.GOOS)
	goarch, ok2 := agentproto.PlatformArch(runtime.GOARCH)
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("platform %s/%s tidak didukung", runtime.GOOS, runtime.GOARCH)
	}
	host, err := os.Hostname()
	if err != nil {
		return nil, err
	}
	b, _ := json.Marshal(map[string]any{"name": name, "token": token, "hostname": host, "os": goos, "arch": goarch, "version": version, "protocol_version": agentproto.ProtocolVersion})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(server, "/")+"/api/v1/agent/enroll", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != 200 {
		var p struct {
			Detail string `json:"detail"`
			Title  string `json:"title"`
		}
		_ = json.Unmarshal(body, &p)
		msg := p.Detail
		if msg == "" {
			msg = p.Title
		}
		return nil, fmt.Errorf("pendaftaran ditolak (HTTP %d): %s", resp.StatusCode, msg)
	}
	var out struct {
		AgentID string `json:"agent_id"`
		Secret  string `json:"secret"`
		Mode    string `json:"mode"`
	}
	if err := json.Unmarshal(body, &out); err != nil || out.Secret == "" {
		return nil, errors.New("jawaban server tidak dikenali")
	}
	return &Config{Name: name, ServerURL: strings.TrimRight(server, "/"), AgentID: out.AgentID, Secret: out.Secret, Mode: out.Mode}, nil
}

type heartbeatReply struct {
	LatestVersion string                     `json:"latest_version"`
	PollSeconds   int                        `json:"poll_seconds"`
	Paused        bool                       `json:"paused"`
	Command       *agentproto.ControlCommand `json:"command,omitempty"`
}

// Run berjalan sampai ctx selesai: detak berkala, ambil tugas, jalankan, laporkan, dan perbarui diri bila ada rilis baru.
func (a *Agent) Run(ctx context.Context) error {
	if a.Cfg.Revoked || a.Cfg.Secret == "" {
		a.logf("agent ini tidak aktif (kredensial dicabut atau belum terdaftar); tidak melakukan apa pun")
		<-ctx.Done()
		return nil
	}
	a.logf("agent %s versi %s (%s/%s) berjalan", a.Cfg.AgentID, a.Version, runtime.GOOS, runtime.GOARCH)
	if a.Cfg.Uninstall != nil {
		if a.StartUninstall == nil {
			return errors.New("pending uninstall requires the supported helper")
		}
		return a.StartUninstall()
	}
	if inst := a.osqueryInstaller(); inst.installedOK(ctx) {
		a.osqueryVersion = inst.version()
	}
	paused := false
	poll := 5 * time.Second
	backoff := time.Second
	var working chan struct{}
	var cancelWork context.CancelFunc
	var pending *agentproto.ControlCommand
	var collect *agentproto.ControlCommand
	defer func() {
		if cancelWork != nil {
			cancelWork()
		}
		if working != nil {
			<-working
		}
	}()
	var lastBeat time.Time
	for ctx.Err() == nil {
		if working != nil {
			select {
			case <-working:
				working = nil
				cancelWork = nil
			default:
			}
		}
		if pending != nil {
			if working != nil {
				cancelWork()
				sleep(ctx, poll)
				continue
			}
			if err := a.beginUninstall(ctx, pending); err == nil {
				return nil
			} else {
				a.setDiagnostic("uninstall_failed")
				a.logf("uninstall helper could not start")
				pending = nil
			}
		}
		if time.Since(lastBeat) >= 15*time.Second {
			var r heartbeatReply
			a.mu.Lock()
			code := a.diagnosticCode
			osqueryVersion := a.osqueryVersion
			a.mu.Unlock()
			_, err := a.do(ctx, http.MethodPost, "heartbeat", map[string]any{"version": a.Version, "protocol_version": agentproto.ProtocolVersion, "control_protocol": a.controlProtocol(), "osquery_version": osqueryVersion, "diagnostic_code": code, "os_version": OSVersion()}, &r)
			switch {
			case errors.Is(err, ErrRevoked):
				if cancelWork != nil {
					cancelWork()
				}
				if working != nil {
					<-working
					working = nil
				}
				return a.revoked(ctx)
			case err != nil:
				a.logf("heartbeat gagal: %v", err)
				if backoff < 30*time.Second {
					backoff *= 2
				}
				sleep(ctx, backoff)
				continue
			}
			paused = r.Paused
			if r.Command != nil {
				if r.Command.Kind == "uninstall" && r.Command.AgentID == a.Cfg.AgentID {
					pending = r.Command
					continue
				}
				if validCollectCommand(r.Command, a.Cfg.AgentID, runtime.GOOS) {
					collect = r.Command
				} else {
					a.setDiagnostic("collection_failed")
				}
			}
			backoff, lastBeat = time.Second, time.Now()
			if r.PollSeconds >= 2 && r.PollSeconds <= 60 {
				poll = time.Duration(r.PollSeconds) * time.Second
			}
			if r.LatestVersion != "" && a.PubKey != nil && working == nil {
				if err := a.selfUpdate(ctx, r.LatestVersion); err != nil {
					a.setDiagnostic("update_failed")
					a.logf("update ditolak: %v", err)
				} else {
					a.logf("update ke %s terpasang; memulai ulang", r.LatestVersion)
					if a.Restart != nil {
						a.Restart()
					}
					return nil
				}
			} else if r.LatestVersion != "" && a.PubKey == nil {
				a.setDiagnostic("release_key_missing")
				a.logf("ada rilis %s, tetapi biner ini tidak membawa kunci rilis; update dimatikan", r.LatestVersion)
			}
		}
		if paused || working != nil {
			sleep(ctx, poll)
			continue
		}
		var spec execution.ClaimSpec
		claimPath := "executions/claim"
		requested := collect
		collect = nil
		if requested != nil {
			claimPath = "executions/" + requested.ID + "/claim"
			if len(requested.Queries) > 1 {
				collect = &agentproto.ControlCommand{Kind: "collect", AgentID: requested.AgentID, ID: requested.ExecutionIDs[1], Queries: requested.Queries[1:], ExecutionIDs: requested.ExecutionIDs[1:]}
			}
		}
		code, err := a.do(ctx, http.MethodPost, claimPath, nil, &spec)
		if err == nil && code != http.StatusNoContent && requested != nil && (spec.ExecutionID.String() != requested.ID || spec.ContractKey != requested.Queries[0]) {
			a.setDiagnostic("collection_failed")
			a.logf("collection command did not match its claimed execution")
			continue
		}
		switch {
		case errors.Is(err, ErrRevoked):
			if cancelWork != nil {
				cancelWork()
			}
			if working != nil {
				<-working
				working = nil
			}
			return a.revoked(ctx)
		case err != nil:
			a.logf("mengambil tugas gagal: %v", err)
			sleep(ctx, poll)
		case code == http.StatusNoContent:
			sleep(ctx, poll)
		default:
			taskCtx, cancel := context.WithCancel(ctx)
			cancelWork = cancel
			working = make(chan struct{})
			done := working
			go func() { defer close(done); defer cancel(); a.execute(taskCtx, &spec) }()
			sleep(ctx, poll)
		}
	}
	return nil
}

// A command is only a hint to claim one previously authorized execution; it never contains SQL.
func validCollectCommand(c *agentproto.ControlCommand, agentID, platform string) bool {
	if c == nil || c.Kind != "collect" || c.AgentID != agentID || len(c.Queries) == 0 || len(c.Queries) > 8 {
		return false
	}
	if _, err := uuid.Parse(c.ID); err != nil {
		return false
	}
	if len(c.Queries) > 1 && len(c.ExecutionIDs) != len(c.Queries) {
		return false
	}
	if len(c.ExecutionIDs) > 0 && (len(c.ExecutionIDs) != len(c.Queries) || c.ExecutionIDs[0] != c.ID) {
		return false
	}
	ids := map[string]bool{}
	for i, key := range c.Queries {
		q := osquery.Find(key)
		if q == nil || !q.SupportsOS(platform) {
			return false
		}
		if len(c.ExecutionIDs) > 0 {
			v := c.ExecutionIDs[i]
			if _, err := uuid.Parse(v); err != nil || ids[v] {
				return false
			}
			ids[v] = true
		}
	}
	return true
}

// revoked: kredensial dicabut selamanya. Rahasia dihapus dari disk, lalu agent diam (tidak keluar, agar service manager tidak
// menghidupkannya ulang terus-menerus). Pemulihan = pendaftaran ulang dengan token baru.
func (a *Agent) revoked(ctx context.Context) error {
	a.logf("kredensial agent dicabut oleh administrator; agent berhenti bekerja. Daftarkan ulang dengan token baru bila perlu.")
	a.Cfg.Revoked, a.Cfg.Secret = true, ""
	if err := a.Cfg.Save(a.ConfigPath); err != nil {
		a.logf("tidak bisa menghapus kredensial dari disk: %v", err)
	}
	<-ctx.Done()
	return nil
}

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

// execute menjalankan satu tugas dan melaporkan hasilnya. Detak per-eksekusi memperpanjang lease dan membawa kabar pembatalan.
func (a *Agent) execute(ctx context.Context, spec *execution.ClaimSpec) {
	id := spec.ExecutionID
	timeout := time.Duration(spec.TimeoutSeconds) * time.Second
	if timeout <= 0 || timeout > 10*time.Minute {
		timeout = time.Minute
	}
	tctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	stop := make(chan struct{})
	go func() {
		t := time.NewTicker(10 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tctx.Done():
				return
			case <-t.C:
				var r struct {
					Cancel bool `json:"cancel"`
				}
				if _, err := a.do(tctx, http.MethodPost, "executions/"+id.String()+"/heartbeat", nil, &r); err == nil && r.Cancel {
					cancel()
					return
				}
			}
		}
	}()
	var findings []execution.NewFinding
	var datasets []osquery.Dataset
	var terr error
	if def := contract.FindVersion(spec.ContractKey, spec.ContractVersion); def == nil || !def.NeedsAgent {
		terr = fmt.Errorf("unsupported agent contract version: %s v%d", spec.ContractKey, spec.ContractVersion)
	} else if _, terr = contract.NormalizeVersion(spec.ContractKey, spec.ContractVersion, spec.Params); terr == nil {
		if osquery.IsOsquery(spec.ContractKey) {
			runner := a.runOsquery
			if a.RunNamedQuery != nil {
				runner = a.RunNamedQuery
			}
			datasets, terr = runner(tctx, spec.ContractKey)
		} else {
			findings, terr = Task(tctx, spec.ContractKey)
		}
	}
	close(stop)
	if ctx.Err() != nil {
		return
	}
	res := execution.Result{Findings: findings, Datasets: datasets}
	code := 0
	if terr != nil {
		code = 1
		res.Error = terr.Error()
		res.Findings = nil
	}
	if errors.Is(tctx.Err(), context.DeadlineExceeded) {
		res.TimedOut, res.Error = true, "The check exceeded its time limit."
	}
	res.ExitCode = &code
	res.Logs = []execution.LogLine{{Level: "info", Message: fmt.Sprintf("Agent %s ran %s.", a.Version, spec.ContractKey)}}
	for try := 0; try < 3; try++ {
		_, err := a.do(context.WithoutCancel(ctx), http.MethodPost, "executions/"+id.String()+"/result", res, nil)
		var he *httpError
		if err == nil || errors.Is(err, ErrRevoked) || (errors.As(err, &he) && he.Status < 500) {
			if err != nil {
				a.logf("hasil ditolak server: %v", err)
			}
			return
		}
		time.Sleep(time.Duration(try+1) * time.Second)
	}
}

// ---------- update diri ----------

func (a *Agent) selfUpdate(ctx context.Context, want string) error {
	if !agentproto.ValidVersion(want) || !agentproto.Newer(want, a.Version) {
		return fmt.Errorf("versi %s bukan versi yang lebih baru dari %s, menolak (anti-downgrade)", want, a.Version)
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(a.Cfg.ServerURL, "/")+"/api/v1/agent/download?version="+want, nil)
	req.Header.Set("Authorization", "Bearer "+a.Cfg.Secret)
	resp, err := clientFor(a.caFile(), 5*time.Minute).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("unduh gagal: HTTP %d", resp.StatusCode)
	}
	relVer, sigB64 := resp.Header.Get("X-Release-Version"), resp.Header.Get("X-Signature-Ed25519")
	if relVer == "" || sigB64 == "" {
		return errors.New("server tidak mengirim versi/tanda tangan rilis, menolak")
	}
	sig, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		return errors.New("tanda tangan bukan base64 yang valid")
	}
	dir := filepath.Dir(a.Exe)
	stage, err := os.CreateTemp(dir, ".stage-*")
	if err != nil {
		return err
	}
	defer os.Remove(stage.Name())
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(stage, h), io.LimitReader(resp.Body, 128<<20)); err != nil {
		stage.Close()
		return err
	}
	stage.Close()
	goos, goarch := runtime.GOOS, runtime.GOARCH
	if !ed25519.Verify(a.PubKey, agentproto.SignedMessage(relVer, goos, goarch, hex.EncodeToString(h.Sum(nil))), sig) {
		return errors.New("tanda tangan TIDAK cocok dengan kunci rilis tepercaya, menolak")
	}
	if relVer != want {
		return fmt.Errorf("versi rilis %s tidak sama dengan yang diminta %s, menolak", relVer, want)
	}
	if !agentproto.Newer(relVer, a.Version) {
		return fmt.Errorf("rilis %s tidak lebih baru dari terpasang %s, menolak downgrade", relVer, a.Version)
	}
	if err := os.Chmod(stage.Name(), 0o755); err != nil {
		return err
	}
	// Uji asap: biner baru harus bisa dijalankan dan melaporkan versi yang diharapkan sebelum dipasang.
	out, err := exec.CommandContext(ctx, stage.Name(), "version").Output()
	if err != nil || !strings.HasPrefix(strings.TrimSpace(string(out)), relVer+" ") {
		return fmt.Errorf("biner baru gagal uji asap (err=%v, keluaran=%q), menolak", err, strings.TrimSpace(string(out)))
	}
	// Ganti biner dengan dua langkah: nama lama dipindah dulu (di Windows biner yang berjalan boleh di-rename, tidak boleh ditimpa).
	old := a.Exe + ".old"
	os.Remove(old)
	if err := os.Rename(a.Exe, old); err != nil {
		return err
	}
	if err := os.Rename(stage.Name(), a.Exe); err != nil {
		_ = os.Rename(old, a.Exe) // kembalikan
		return err
	}
	return nil
}

func (a *Agent) controlProtocol() int {
	if a.StartUninstall != nil {
		return agentproto.ControlUninstallAndCollect
	}
	return agentproto.ControlCollectOnly
}
