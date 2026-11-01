package osquery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Runner menjalankan query bernama SEKALI JALAN lewat biner osquery (osqueryd -S, sama dengan osqueryi). Tidak ada daemon,
// tidak ada ekstensi, tidak ada peristiwa. Teks SQL selalu berasal dari katalog beku, tidak pernah dari argumen luar.
type Runner struct {
	Bin string
	// IsRoot dan GOOS dapat diganti di tes; nil/kosong memakai nilai proses sebenarnya.
	IsRoot func() bool
	GOOS   string
}

var errTooLarge = errors.New("osquery output exceeded the size limit")

type capWriter struct {
	buf bytes.Buffer
	max int
}

func (w *capWriter) Write(p []byte) (int, error) {
	if w.buf.Len()+len(p) > w.max {
		return 0, errTooLarge
	}
	return w.buf.Write(p)
}

func (r *Runner) goos() string {
	if r.GOOS != "" {
		return r.GOOS
	}
	return runtime.GOOS
}

func (r *Runner) root() bool {
	if r.IsRoot != nil {
		return r.IsRoot()
	}
	return os.Geteuid() == 0
}

func skipped(q *Query, reason string) *Dataset {
	return &Dataset{Query: q.Key, Status: StatusSkipped, Reason: reason, Rows: []map[string]string{}, CollectedAt: time.Now().UTC(), Version: Version}
}

func failed(q *Query, reason string) *Dataset {
	return &Dataset{Query: q.Key, Status: StatusFailed, Reason: clip(Redact(reason), 200), Rows: []map[string]string{}, CollectedAt: time.Now().UTC(), Version: Version}
}

// Run menjalankan satu query dan SELALU mengembalikan dataset: ok, dilewati (dengan alasan), atau gagal. Tidak ada kegagalan diam-diam.
func (r *Runner) Run(ctx context.Context, q *Query) *Dataset {
	if err := q.Validate(); err != nil { // pertahanan berlapis: periksa lagi tepat sebelum menjalankan
		return skipped(q, ReasonSensitive+" ("+err.Error()+")")
	}
	goos := r.goos()
	if !q.SupportsOS(goos) {
		return skipped(q, ReasonUnsupported)
	}
	if q.NeedsRoot && !r.root() {
		return skipped(q, ReasonNeedsRoot)
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(q.TimeoutSeconds)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, r.Bin, "-S", "--json", "--ephemeral", "--disable_extensions", "--disable_events", "--disable_watchdog", "--disable_logging", q.SQL)
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C"}
	out := &capWriter{max: maxResult}
	stderr := &capWriter{max: 64 << 10}
	cmd.Stdout, cmd.Stderr = out, stderr
	if err := cmd.Run(); err != nil {
		switch {
		case errors.Is(ctx.Err(), context.DeadlineExceeded):
			return failed(q, "the query exceeded its time limit")
		case errors.Is(err, errTooLarge) || out.buf.Len() >= maxResult || stderr.buf.Len() >= stderr.max:
			return failed(q, "the result was too large")
		}
		return failed(q, "osquery failed: "+strings.TrimSpace(clip(Redact(stderr.buf.String()), 120)))
	}
	var rows []map[string]any
	if err := json.Unmarshal(out.buf.Bytes(), &rows); err != nil {
		return failed(q, "osquery returned unreadable output")
	}
	ds := &Dataset{Query: q.Key, Status: StatusOK, Rows: make([]map[string]string, 0, len(rows)), CollectedAt: time.Now().UTC(), Version: Version}
	for i, row := range rows {
		if i >= q.MaxRows {
			ds.Truncated = true
			break
		}
		clean := make(map[string]string, len(row))
		for k, v := range row {
			clean[k] = fmt.Sprint(v)
		}
		ds.Rows = append(ds.Rows, clean)
	}
	// Bentuk akhir selalu lewat Sanitize (skema kolom, panjang sel, redaksi): sama dengan yang dilakukan server.
	safe, err := Sanitize(q, ds)
	if err != nil {
		return failed(q, err.Error())
	}
	return safe
}
