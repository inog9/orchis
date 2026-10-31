package mitre

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
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

// DefaultLinks menautkan kontrak bawaan ke teknik ATT&CK yang dilatihnya.
var DefaultLinks = map[string][]string{
	"nmap.tcp_connect": {"T1046"},
	"nuclei.scan":      {"T1595.002"},
}

type Importer struct {
	DB     *db.DB
	Tenant uuid.UUID
	Source string
	HTTP   *http.Client
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
		if _, err := tx.Exec(ctx, `INSERT INTO mitre_techniques (id, stix_id, name, description, parent_id, tactic_ids, platforms, is_deprecated, source_version, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,now())
ON CONFLICT (id) DO UPDATE SET stix_id=EXCLUDED.stix_id, name=EXCLUDED.name, description=EXCLUDED.description, parent_id=EXCLUDED.parent_id,
  tactic_ids=EXCLUDED.tactic_ids, platforms=EXCLUDED.platforms, is_deprecated=EXCLUDED.is_deprecated, source_version=EXCLUDED.source_version, updated_at=now()`,
			t.ID, t.StixID, t.Name, t.Description, parent, t.TacticIDs, t.Platforms, t.Deprecated, nullable(ds.Version)); err != nil {
			return fail(err)
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE mitre_techniques SET is_deprecated=true, updated_at=now() WHERE NOT (id = ANY($1)) AND NOT is_deprecated`, keep); err != nil {
		return fail(err)
	}
	nt, nc := len(ds.Techniques), len(ds.Tactics)
	run.Status, run.Techniques, run.Tactics = "succeeded", &nt, &nc
	if ds.Version != "" {
		run.Version = &ds.Version
	}
	if _, err := tx.Exec(ctx, `UPDATE mitre_import_runs SET status='succeeded', techniques=$2, tactics=$3, source_version=$4, finished_at=now() WHERE id=$1`, run.ID, nt, nc, nullable(ds.Version)); err != nil {
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, im.Source, nil)
	if err != nil {
		return nil, err
	}
	resp, err := im.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("unduhan gagal: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("sumber menjawab %d", resp.StatusCode)
	}
	return Parse(io.LimitReader(resp.Body, maxDownload))
}

// LinkContracts menautkan kontrak bawaan ke teknik yang ada. Aman dijalankan berulang dan sebelum teknik diimpor (dilewati diam-diam).
func (im *Importer) LinkContracts(ctx context.Context) error {
	return im.DB.InTenant(ctx, im.Tenant, func(tx pgx.Tx) error {
		for key, techs := range DefaultLinks {
			for _, tid := range techs {
				if _, err := tx.Exec(ctx, `INSERT INTO contract_techniques (tenant_id, contract_id, technique_id)
SELECT c.tenant_id, c.id, t.id FROM contracts c, mitre_techniques t WHERE c.tenant_id=$1 AND c.key=$2 AND t.id=$3
ON CONFLICT DO NOTHING`, im.Tenant, key, tid); err != nil {
					return err
				}
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
