package mitre

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/reugn/go-quartz/quartz"

	"orchis-go/internal/db"
	"orchis-go/internal/ids"
)

// DefaultSource = bundel resmi ATT&CK Enterprise (STIX 2.1) dari MITRE.
const DefaultSource = "https://raw.githubusercontent.com/mitre-attack/attack-stix-data/master/enterprise-attack/enterprise-attack.json"

const (
	maxDownload = 120 << 20
	lockKey     = 74120931 // kunci advisory: hanya satu impor pada satu waktu di seluruh instance server
	staleAfter  = 15 * time.Minute
)

// Link menautkan satu kontrak bawaan ke satu teknik ATT&CK beserta alasannya.
// Hanya perilaku yang BENAR-BENAR dilakukan aksi itu yang dipetakan (basis "direct"); jangan menandai cakupan berlebihan.
type Link struct {
	Contract  string
	Technique string
	Basis     string // direct | estimate
	Reason    string
}

// DefaultLinks adalah sumber kebenaran pemetaan aksi->teknik; LinkContracts menyelaraskan database dengannya
// (tautan lama yang tidak ada lagi di sini dihapus, sehingga koreksi pemetaan ikut berlaku).
//
// Sengaja TIDAK dipetakan: nuclei.scan -> T1190 (Exploit Public-Facing Application). Nuclei mendeteksi kerentanan,
// tidak mengeksploitasinya, jadi menautkannya akan melebih-lebihkan cakupan. T1190 hanya muncul sebagai perkiraan
// pada temuan kerentanan di laporan (lihat internal/reporting).
var DefaultLinks = []Link{
	{"nmap.tcp_connect", "T1046", "direct", "A TCP connect scan enumerates open ports and, optionally, service versions on a remote host. That is the discovery behaviour of Network Service Discovery."},
	{"nuclei.scan", "T1595.002", "direct", "Nuclei sends template-based probes to a target to find known vulnerabilities, which is Active Scanning: Vulnerability Scanning."},
	{"agent.system_info", "T1082", "direct", "The agent reads hostname, OS, architecture, kernel and memory from the endpoint, which is System Information Discovery."},
	{"agent.listening_ports", "T1049", "direct", "The agent lists listening sockets from /proc/net/tcp*, which is System Network Connections Discovery."},
}

type Importer struct {
	DB     *db.DB
	Tenant uuid.UUID
	// Source: URL http(s) atau jalur berkas lokal (bundel beku di deploy/mitre). Berkas lokal adalah jalur produksi.
	Source string
	// ExpectedSHA256: bila diisi, bundel yang hash-nya berbeda ditolak sebelum menyentuh database.
	ExpectedSHA256 string
	HTTP           *http.Client
}

func (im *Importer) client() *http.Client {
	if im.HTTP != nil {
		return im.HTTP
	}
	return &http.Client{Timeout: 3 * time.Minute}
}

type Run struct {
	ID         uuid.UUID  `json:"id"`
	Status     string     `json:"status"`
	Version    *string    `json:"source_version"`
	Techniques *int       `json:"techniques"`
	Tactics    *int       `json:"tactics"`
	Subs       *int       `json:"subtechniques"`
	Deprecated *int       `json:"deprecated"`
	SHA256     *string    `json:"source_sha256"`
	Error      *string    `json:"error"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
}

var ErrBusy = errors.New("an import is already running")

// Import mengunduh, memvalidasi, lalu menyimpan dalam SATU transaksi: gagal di tengah tidak meninggalkan data setengah jadi.
// Teknik yang hilang dari sumber ditandai deprecated (bukan dihapus) agar riwayat dan tautan tetap utuh.
func (im *Importer) Import(ctx context.Context) (*Run, error) {
	run := &Run{ID: ids.New(), Status: "running", StartedAt: time.Now()}
	// Klaim berseri: kunci advisory membuat dua permintaan serentak tidak sama-sama melihat "tidak ada impor berjalan".
	claim, err := im.DB.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer claim.Rollback(ctx)
	if _, err := claim.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, lockKey); err != nil {
		return nil, err
	}
	var busy bool
	if err := claim.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM mitre_import_runs WHERE status='running' AND started_at > now() - make_interval(secs => $1))`, staleAfter.Seconds()).Scan(&busy); err != nil {
		return nil, err
	}
	if busy {
		return nil, ErrBusy
	}
	if _, err := claim.Exec(ctx, `INSERT INTO mitre_import_runs (id, source, status) VALUES ($1,$2,'running')`, run.ID, im.Source); err != nil {
		return nil, err
	}
	if err := claim.Commit(ctx); err != nil {
		return nil, err
	}
	fail := func(e error) (*Run, error) {
		msg := e.Error()
		if len(msg) > 500 {
			msg = msg[:500]
		}
		run.Status, run.Error = "failed", &msg
		_, _ = im.DB.Pool.Exec(context.WithoutCancel(ctx), `UPDATE mitre_import_runs SET status='failed', error=$2, finished_at=now() WHERE id=$1`, run.ID, msg)
		return run, e
	}
	ds, err := im.fetch(ctx)
	if err != nil {
		return fail(err)
	}
	tx, err := im.DB.Pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, lockKey); err != nil {
		return fail(err)
	}
	for _, t := range ds.Tactics {
		if _, err := tx.Exec(ctx, `INSERT INTO mitre_tactics (id, stix_id, name, shortname, sort_order) VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (id) DO UPDATE SET stix_id=EXCLUDED.stix_id, name=EXCLUDED.name, shortname=EXCLUDED.shortname, sort_order=EXCLUDED.sort_order`,
			t.ID, t.StixID, t.Name, t.Shortname, t.Order); err != nil {
			return fail(err)
		}
	}
	keep := make([]string, 0, len(ds.Techniques))
	for _, t := range ds.Techniques {
		keep = append(keep, t.ID)
		var parent *string
		if t.ParentID != "" {
			parent = &t.ParentID
		}
		mits, err := json.Marshal(t.Mitigations)
		if err != nil {
			return fail(err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO mitre_techniques (id, stix_id, name, description, parent_id, tactic_ids, platforms, is_deprecated, source_version, url, mitigations, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,''),$11,now())
ON CONFLICT (id) DO UPDATE SET stix_id=EXCLUDED.stix_id, name=EXCLUDED.name, description=EXCLUDED.description, parent_id=EXCLUDED.parent_id,
  tactic_ids=EXCLUDED.tactic_ids, platforms=EXCLUDED.platforms, is_deprecated=EXCLUDED.is_deprecated, source_version=EXCLUDED.source_version,
  url=EXCLUDED.url, mitigations=EXCLUDED.mitigations, updated_at=now()`,
			t.ID, t.StixID, t.Name, t.Description, parent, t.TacticIDs, t.Platforms, t.Deprecated, nullable(ds.Version), t.URL, mits); err != nil {
			return fail(err)
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE mitre_techniques SET is_deprecated=true, updated_at=now() WHERE NOT (id = ANY($1)) AND NOT is_deprecated`, keep); err != nil {
		return fail(err)
	}
	cnt := ds.Count()
	nt, nc := len(ds.Techniques), len(ds.Tactics)
	run.Status, run.Techniques, run.Tactics, run.Subs, run.Deprecated = "succeeded", &nt, &nc, &cnt.Subs, &cnt.Deprecated
	run.SHA256 = nullable(ds.SHA256)
	if ds.Version != "" {
		run.Version = &ds.Version
	}
	if _, err := tx.Exec(ctx, `UPDATE mitre_import_runs SET status='succeeded', techniques=$2, tactics=$3, source_version=$4, subtechniques=$5, deprecated=$6, source_sha256=$7, finished_at=now() WHERE id=$1`,
		run.ID, nt, nc, nullable(ds.Version), cnt.Subs, cnt.Deprecated, nullable(ds.SHA256)); err != nil {
		return fail(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	if err := im.LinkContracts(ctx); err != nil {
		slog.Warn("menautkan kontrak ke teknik gagal", "err", err)
	}
	return run, nil
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (im *Importer) fetch(ctx context.Context) (*Dataset, error) {
	var body io.ReadCloser
	if strings.HasPrefix(im.Source, "http://") || strings.HasPrefix(im.Source, "https://") {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, im.Source, nil)
		if err != nil {
			return nil, err
		}
		resp, err := im.client().Do(req)
		if err != nil {
			return nil, fmt.Errorf("unduhan gagal: %w", err)
		}
		if resp.StatusCode != 200 {
			resp.Body.Close()
			return nil, fmt.Errorf("sumber menjawab %d", resp.StatusCode)
		}
		body = resp.Body
	} else {
		f, err := os.Open(strings.TrimPrefix(im.Source, "file://"))
		if err != nil {
			return nil, fmt.Errorf("bundel lokal tidak bisa dibuka: %w", err)
		}
		body = f
	}
	defer body.Close()
	return ParseVerified(io.LimitReader(body, maxDownload), im.ExpectedSHA256)
}

// ParseVerified mem-parse bundel sambil menghitung SHA-256 seluruh isinya. Bila expected diisi dan hash berbeda,
// hasil ditolak: bundel yang dibekukan tidak boleh berubah diam-diam.
func ParseVerified(r io.Reader, expected string) (*Dataset, error) {
	h := sha256.New()
	ds, err := Parse(io.TeeReader(r, h))
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(io.Discard, io.TeeReader(r, h)); err != nil { // sisa berkas setelah objek JSON terakhir
		return nil, err
	}
	ds.SHA256 = hex.EncodeToString(h.Sum(nil))
	if expected != "" && !strings.EqualFold(expected, ds.SHA256) {
		return nil, fmt.Errorf("hash bundel tidak cocok (diharapkan %s, dapat %s)", expected, ds.SHA256)
	}
	return ds, nil
}

// LinkContracts menyelaraskan tautan kontrak bawaan->teknik dengan DefaultLinks (menambah, memperbarui alasan, dan
// menghapus tautan bawaan yang sudah tidak ada). Aman dijalankan berulang dan sebelum teknik diimpor (dilewati diam-diam).
func (im *Importer) LinkContracts(ctx context.Context) error {
	return im.DB.InTenant(ctx, im.Tenant, func(tx pgx.Tx) error {
		want := map[string][]string{}
		for _, l := range DefaultLinks {
			want[l.Contract] = append(want[l.Contract], l.Technique)
			if _, err := tx.Exec(ctx, `INSERT INTO contract_techniques (tenant_id, contract_id, technique_id, reason, basis)
SELECT c.tenant_id, c.id, t.id, $4, $5 FROM contracts c, mitre_techniques t WHERE c.tenant_id=$1 AND c.key=$2 AND t.id=$3
ON CONFLICT (tenant_id, contract_id, technique_id) DO UPDATE SET reason=EXCLUDED.reason, basis=EXCLUDED.basis`,
				im.Tenant, l.Contract, l.Technique, l.Reason, l.Basis); err != nil {
				return err
			}
		}
		for key, techs := range want {
			if _, err := tx.Exec(ctx, `DELETE FROM contract_techniques ct USING contracts c
WHERE ct.tenant_id=$1 AND c.tenant_id=ct.tenant_id AND c.id=ct.contract_id AND c.key=$2 AND NOT (ct.technique_id = ANY($3))`, im.Tenant, key, techs); err != nil {
				return err
			}
		}
		return nil
	})
}

// LastSuccess mengembalikan waktu impor sukses terakhir (nil bila belum pernah).
func (im *Importer) LastSuccess(ctx context.Context) (*time.Time, error) {
	var t *time.Time
	err := im.DB.Pool.QueryRow(ctx, `SELECT max(finished_at) FROM mitre_import_runs WHERE status='succeeded'`).Scan(&t)
	return t, err
}

// RunIfDue menjalankan impor bila impor sukses terakhir lebih tua dari maxAge. Satu klaim per hari (job_runs) mencegah beberapa
// instance server mengimpor bersamaan, dan menjaga kegagalan tidak diulang terus-menerus dalam satu hari.
func (im *Importer) RunIfDue(ctx context.Context, maxAge time.Duration, instance string, now time.Time) (ran bool, err error) {
	last, err := im.LastSuccess(ctx)
	if err != nil {
		return false, err
	}
	if last != nil && now.Sub(*last) < maxAge {
		return false, nil
	}
	bucket := now.UTC().Truncate(24 * time.Hour)
	tag, err := im.DB.Pool.Exec(ctx, `INSERT INTO job_runs (job, bucket, instance) VALUES ('mitre_import', $1, $2) ON CONFLICT DO NOTHING`, bucket, instance)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	if _, err := im.Import(ctx); err != nil && !errors.Is(err, ErrBusy) {
		return true, err
	}
	return true, nil
}

type job struct {
	im       *Importer
	instance string
}

func (j job) Execute(ctx context.Context) error {
	ran, err := j.im.RunIfDue(ctx, 7*24*time.Hour, j.instance, time.Now())
	if err != nil {
		slog.Warn("impor MITRE gagal", "err", err)
	} else if ran {
		slog.Info("impor MITRE selesai")
	}
	return nil
}
func (j job) Description() string { return "mitre_import" }

// Schedule memasang pekerjaan berkala (go-quartz): memeriksa tiap 12 jam, mengimpor bila data lebih tua dari 7 hari.
// Pemeriksaan pertama dilakukan sebentar setelah server naik, sehingga instalasi baru terisi tanpa menunggu.
func (im *Importer) Schedule(ctx context.Context, instance string) (quartz.Scheduler, error) {
	sched, err := quartz.NewStdScheduler()
	if err != nil {
		return nil, err
	}
	sched.Start(ctx)
	j := job{im: im, instance: instance}
	if err := sched.ScheduleJob(quartz.NewJobDetail(j, quartz.NewJobKey("mitre-import")), quartz.NewSimpleTrigger(12*time.Hour)); err != nil {
		return nil, err
	}
	go func() {
		select {
		case <-ctx.Done():
		case <-time.After(15 * time.Second):
			_ = j.Execute(ctx)
		}
	}()
	return sched, nil
}
