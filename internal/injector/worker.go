package injector

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"

	"orchis-go/internal/contract"
	"orchis-go/internal/execution"
	"orchis-go/internal/mq"
	"orchis-go/internal/telemetry"
)

const (
	maxStdout = 4 << 20
	maxStderr = 8 << 10
)

type Worker struct {
	Concurrency  int // jumlah consumer independen per injector; 0 memakai bawaan4
	Metrics      *telemetry.Registry
	Kind         string // nmap | nuclei
	Client       *Client
	AMQPURL      string
	Version      string
	Guard        contract.Guard
	TemplatesDir string // nuclei: direktori template (kosong = bawaan nuclei)
	Binary       string // jalur biner; kosong = nama default di PATH
}

// Run: heartbeat berkala + konsumen antrean exec.<kind>, consumer paralel terbatas, masing-masing satu tugas pada satu waktu.
func (w *Worker) Run(ctx context.Context) {
	go func() {
		t := time.NewTicker(15 * time.Second)
		defer t.Stop()
		for {
			if err := w.Client.Heartbeat(ctx, w.Version); err != nil && ctx.Err() == nil {
				slog.Warn("heartbeat gagal", "err", err)
			} else if err == nil && w.Metrics != nil {
				w.Metrics.Heartbeat()
			}
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
	slog.Info("injector berjalan", "kind", w.Kind)
	n := w.Concurrency
	if n == 0 {
		n = 4
	}
	if n < 1 || n > 32 {
		slog.Error("invalid injector concurrency")
		return
	}
	var workers sync.WaitGroup
	for i := 0; i < n; i++ {
		workers.Add(1)
		go func() { defer workers.Done(); mq.Consume(ctx, w.AMQPURL, "exec."+w.Kind, 1, w.handle) }()
	}
	workers.Wait()
}

func (w *Worker) handle(ctx context.Context, m mq.Message) error {
	var p struct {
		ExecutionID uuid.UUID `json:"execution_id"`
	}
	if err := json.Unmarshal(m.Body, &p); err != nil || p.ExecutionID == uuid.Nil {
		return fmt.Errorf("pesan dispatch tidak valid")
	}
	var spec *execution.ClaimSpec
	var err error
	for attempt := 0; attempt < 4; attempt++ {
		if spec, err = w.Client.Claim(ctx, p.ExecutionID); err == nil {
			break
		}
		var ae *APIError
		if errors.As(err, &ae) && ae.Status < 500 {
			// 404/409: sudah diklaim, dibatalkan, atau ditolak penjaga scope. Pesan selesai, tidak ada yang dijalankan.
			slog.Info("tugas dilewati", "execution", p.ExecutionID, "reason", ae.Body)
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 2 * time.Second):
		}
	}
	if err != nil {
		return err
	}
	res := w.execute(ctx, spec)
	if w.Metrics != nil {
		w.Metrics.Add("orchis_scan_completed_total", 1)
		if res.Error != "" {
			w.Metrics.Add("orchis_scan_failures_total", 1)
		}
	}
	for attempt := 0; attempt < 4; attempt++ {
		err = w.Client.Result(context.WithoutCancel(ctx), spec.ExecutionID, res)
		var ae *APIError
		if err == nil || (errors.As(err, &ae) && ae.Status < 500) {
			if err != nil {
				slog.Info("hasil ditolak server", "execution", spec.ExecutionID, "reason", ae.Body)
			}
			return nil
		}
		time.Sleep(time.Duration(attempt+1) * 2 * time.Second)
	}
	return err
}

// execute menjalankan alat untuk satu tugas dan mengembalikan hasil terstruktur. Tidak pernah panik ke pemanggil.
func (w *Worker) execute(ctx context.Context, spec *execution.ClaimSpec) execution.Result {
	res := execution.Result{}
	logf := func(level, format string, a ...any) {
		res.Logs = append(res.Logs, execution.LogLine{Level: level, Message: fmt.Sprintf(format, a...)})
	}
	fail := func(format string, a ...any) execution.Result {
		res.Error = fmt.Sprintf(format, a...)
		return res
	}
	def := contract.FindVersion(spec.ContractKey, spec.ContractVersion)
	if def == nil {
		return fail("unsupported contract version: %s v%d", spec.ContractKey, spec.ContractVersion)
	}
	normalized, err := contract.NormalizeVersion(spec.ContractKey, spec.ContractVersion, spec.Params)
	if err != nil {
		return fail("invalid parameters: %v", err)
	}
	spec.Params = normalized
	// Pertahanan berlapis: server sudah memeriksa, injector memeriksa lagi sebelum menyentuh jaringan.
	if spec.ScanScope != nil {
		if err := spec.ScanScope.Validate(spec.Target, w.Guard); err != nil {
			return fail("refused by the injector scan scope: %v", err)
		}
	}
	host := contract.HostOf(spec.Target)
	if err := w.Guard.CheckHost(host); err != nil {
		return fail("refused by the injector scope guard: %v", err)
	}
	var bin string
	var args []string
	err = nil
	switch {
	case w.Kind == "nmap" && def.Adapter == "nmap":
		bin = firstNonEmpty(w.Binary, "nmap")
		args, err = contract.NmapArgs(spec.Params, spec.Target, spec.TimeoutSeconds)
	case w.Kind == "nuclei" && def.Adapter == "nuclei":
		bin = firstNonEmpty(w.Binary, "nuclei")
		args, err = contract.NucleiArgs(spec.Params, spec.Target, w.TemplatesDir)
	default:
		return fail("contract %q is not handled by the %s injector", spec.ContractKey, w.Kind)
	}
	if err != nil {
		return fail("invalid parameters: %v", err)
	}
	logf("info", "Running %s against %s.", w.Kind, spec.Target)

	runCtx, cancel := context.WithTimeout(ctx, time.Duration(spec.TimeoutSeconds)*time.Second)
	defer cancel()
	canceled := make(chan struct{})
	defer close(canceled)
	go w.watch(runCtx, spec.ExecutionID, cancel, canceled)

	var stdout, stderr limitedBuffer
	stdout.max, stderr.max = maxStdout, maxStderr
	cmd := exec.CommandContext(runCtx, bin, args...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + firstNonEmpty(os.Getenv("HOME"), "/tmp")}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) } // matikan seluruh grup proses
	cmd.WaitDelay = 5 * time.Second

	if w.Kind == "nuclei" {
		if err := checkWebEndpoint(runCtx, spec.Target, w.Guard); err != nil {
			return fail("%v", err)
		}
	}
	runErr := cmd.Run()
	res.Output = stdout.String()
	code := 0
	if runErr != nil {
		var ee *exec.ExitError
		if errors.As(runErr, &ee) {
			code = ee.ExitCode()
		} else {
			code = -1
		}
	}
	res.ExitCode = &code
	if s := strings.TrimSpace(stderr.String()); s != "" {
		logf("warn", "stderr: %s", tail(s, 1500))
	}
	switch {
	case errors.Is(runCtx.Err(), context.DeadlineExceeded):
		res.TimedOut = true
		res.Error = "The scan exceeded its time limit and was stopped."
		return res
	case runCtx.Err() != nil:
		res.Error = "The scan was stopped."
		return res
	case runErr != nil:
		res.Error = fmt.Sprintf("%s failed: %v", w.Kind, runErr)
		return res
	}
	if w.Kind == "nuclei" {
		if err := checkWebEndpoint(runCtx, spec.Target, w.Guard); err != nil {
			return fail("%v", err)
		}
	}
	switch w.Kind {
	case "nmap":
		res.Findings, err = ParseNmap([]byte(res.Output))
	case "nuclei":
		res.Findings, err = ParseNuclei([]byte(res.Output))
	}
	if err != nil {
		res.Error = err.Error()
		return res
	}
	logf("info", "%s finished with %d finding(s).", w.Kind, len(res.Findings))
	return res
}

// watch memperpanjang lease dan menghentikan proses bila server memberi tahu eksekusi dibatalkan.
func (w *Worker) watch(ctx context.Context, id uuid.UUID, stop context.CancelFunc, done <-chan struct{}) {
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case <-ctx.Done():
			return
		case <-t.C:
			cancel, err := w.Client.ExecHeartbeat(ctx, id)
			if err != nil {
				var ae *APIError
				if errors.As(err, &ae) && ae.Status == http.StatusNotFound {
					cancel = true
				}
			}
			if cancel {
				slog.Info("eksekusi dibatalkan oleh server, proses dihentikan", "execution", id)
				stop()
				return
			}
		}
	}
}

type limitedBuffer struct {
	bytes.Buffer
	max       int
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if room := b.max - b.Len(); room < len(p) {
		b.truncated = true
		if room > 0 {
			b.Buffer.Write(p[:room])
		}
		return len(p), nil // pura-pura sukses supaya proses tidak macet
	}
	return b.Buffer.Write(p)
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}

func firstNonEmpty(a ...string) string {
	for _, s := range a {
		if s != "" {
			return s
		}
	}
	return ""
}
