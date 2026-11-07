// Perintah: server serve | server migrate
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/google/uuid"

	"orchis-go/internal/blob"
	"orchis-go/internal/bootstrap"
	"orchis-go/internal/config"
	"orchis-go/internal/contract"
	"orchis-go/internal/db"
	"orchis-go/internal/demo"
	"orchis-go/internal/execution"
	"orchis-go/internal/indexer"
	"orchis-go/internal/mitre"
	"orchis-go/internal/mq"
	"orchis-go/internal/search"
	"orchis-go/internal/server"
	"orchis-go/internal/settings"
	"orchis-go/internal/telemetry"
)

// version diisi saat build: go build -ldflags "-X main.version=1.2.3".
var version = "dev"

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	cfg, err := config.Load()
	if err != nil {
		fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	switch cmd {
	case "ai-rekey":
		if err := rekeyAI(ctx, cfg); err != nil {
			fatal(err)
		}
	case "object-digests":
		run, cancel := context.WithTimeout(ctx, 90*time.Second)
		defer cancel()
		digests, e := objectDigests(run, cfg)
		if e != nil {
			fatal(e)
		}
		if e = json.NewEncoder(os.Stdout).Encode(digests); e != nil {
			fatal(e)
		}
	case "healthcheck":
		client := &http.Client{Timeout: 3 * time.Second}
		res, e := client.Get("http://127.0.0.1:8090/api/healthz")
		if e != nil {
			os.Exit(1)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusOK {
			os.Exit(1)
		}
	case "provision":
		// Explicit, idempotent setup. Raw tokens never enter stdout or the database.
		if err := withTenant(ctx, cfg, func(d *db.DB, tenant uuid.UUID) error {
			for _, kind := range []string{"nmap", "nuclei", "hackbot"} {
				token := os.Getenv("ORCHIS_PROVISION_" + map[string]string{"hackbot": "HACKBOT", "nmap": "NMAP", "nuclei": "NUCLEI"}[kind] + "_TOKEN")
				if !execution.ValidInjectorToken(token) {
					return errors.New("provisioning token is missing")
				}
				if err := execution.ProvisionToken(ctx, d, tenant, kind, token); err != nil {
					return err
				}

			}
			return nil
		}); err != nil {
			fatal(err)
		}
	case "migrate":
		if err := db.Migrate(ctx, cfg.MigrateURL, cfg.AppRole); err != nil {
			fatal(err)
		}
		slog.Info("migrasi selesai")
	case "injector-token":
		// server injector-token <nmap|nuclei>: membuat token baru (yang lama tidak berlaku lagi) dan mencetaknya sekali.
		if len(os.Args) < 3 {
			fatal(fmt.Errorf("pemakaian: server injector-token <nmap|nuclei>"))
		}
		if err := withTenant(ctx, cfg, func(d *db.DB, tenant uuid.UUID) error {
			tok, err := execution.IssueToken(ctx, d, tenant, os.Args[2])
			if err == nil {
				fmt.Println(tok)
			}
			return err
		}); err != nil {
			fatal(err)
		}
	case "demo-seed", "demo-purge", "demo-status":
		// server demo-seed|demo-purge|demo-status: data dummy bertanda is_demo (lihat internal/demo). Seed dari DEMO_SEED (bawaan 1).
		if err := withTenant(ctx, cfg, func(d *db.DB, tenant uuid.UUID) error {
			opts := server.Options{DB: d, Tenant: tenant}
			if cfg.S3.Endpoint != "" {
				store, e := blob.New(blob.Options{Endpoint: cfg.S3.Endpoint, AccessKey: cfg.S3.AccessKey, SecretKey: cfg.S3.SecretKey, Bucket: cfg.S3.Bucket, TLS: cfg.S3.TLS})
				if e != nil {
					return e
				}
				opts.Store = store
			}
			if cfg.ESURL != "" {
				opts.Search = search.New(cfg.ESURL)
			}
			svc := server.DemoService(opts)
			var st *demo.Status
			var err error
			switch cmd {
			case "demo-seed":
				seed := 1
				if v := os.Getenv("DEMO_SEED"); v != "" {
					if seed, err = strconv.Atoi(v); err != nil {
						return fmt.Errorf("DEMO_SEED harus bilangan bulat: %w", err)
					}
				}
				if err = svc.Seed(ctx, tenant, seed, nil, "", "cli"); err != nil {
					return err
				}
				st, err = svc.Status(ctx, tenant)
			case "demo-purge":
				st, err = svc.Purge(ctx, tenant, nil, "", "cli")
			default:
				st, err = svc.Status(ctx, tenant)
			}
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(st)
		}); err != nil {
			fatal(err)
		}
	case "reindex":
		if err := withTenant(ctx, cfg, func(d *db.DB, tenant uuid.UUID) error {
			if cfg.ESURL == "" {
				return fmt.Errorf("ORCHIS_ES_URL belum diisi")
			}
			n, err := (&indexer.Indexer{DB: d, Tenant: tenant, ES: search.New(cfg.ESURL)}).Reindex(ctx)
			slog.Info("reindex selesai", "documents", n)
			return err
		}); err != nil {
			fatal(err)
		}
	case "serve":
		if cfg.Production && (os.Getenv("ORCHIS_DATABASE_URL") == "" || len(cfg.AdminPassword) < 24) {
			fatal(errors.New("production requires explicit application database and random initial administrator credentials"))
		}
		if err := serve(ctx, cfg); err != nil {
			fatal(err)
		}
	default:
		fatal(fmt.Errorf("perintah tidak dikenal %q (gunakan: serve | migrate | injector-token | reindex | demo-seed | demo-purge | demo-status)", cmd))
	}
}

func serve(ctx context.Context, cfg *config.Config) error {
	d, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("tidak bisa tersambung ke database: %w", err)
	}
	defer d.Close()
	tenant, err := bootstrap.EnsureTenant(ctx, d, cfg.TenantName)
	if err != nil {
		return err
	}
	created, generated, err := bootstrap.EnsureAdmin(ctx, d, tenant, cfg.AdminEmail, cfg.AdminPassword)
	if err != nil {
		return err
	}
	if created && generated != "" {
		// Satu-satunya tempat kata sandi awal tampil. Segera ganti setelah masuk.
		fmt.Printf("\n  Admin pertama dibuat.\n  Email   : %s\n  Password: %s\n  (hanya ditampilkan sekali; simpan atau segera ganti)\n\n", cfg.AdminEmail, generated)
	}
	opts := server.Options{DB: d, Tenant: tenant, TrustedProxies: cfg.TrustedProxies, PublicRateLimit: cfg.Production, CookieSecure: cfg.CookieSecure, AdvancedFeatures: cfg.AdvancedFeatures, ScanMaxHosts: cfg.ScanMaxHosts, ScanConcurrency: cfg.ScanConcurrency, EndpointCollectionSeconds: cfg.EndpointCollectionSeconds, WebDir: cfg.WebDir,
		Guard: contract.Guard{AllowLoopback: cfg.AllowLoopbackTargets, DeferNames: true}, Version: version, AgentRelease: cfg.AgentReleaseDir}
	opts.Checks = []settings.Check{{Name: "PostgreSQL", Fn: d.Pool.Ping}}
	var es *search.Client
	if cfg.ESURL != "" {
		es = search.New(cfg.ESURL)
		opts.Search = es
		opts.Checks = append(opts.Checks, settings.Check{Name: "Elasticsearch", Fn: es.Ping})
	} else {
		opts.Checks = append(opts.Checks, settings.Check{Name: "Elasticsearch"})
	}
	if cfg.AMQPURL != "" {
		amqpURL := cfg.AMQPURL
		opts.Checks = append(opts.Checks, settings.Check{Name: "RabbitMQ", Fn: func(context.Context) error {
			c, err := mq.Dial(amqpURL)
			if err == nil {
				c.Close()
			}
			return err
		}})
	} else {
		opts.Checks = append(opts.Checks, settings.Check{Name: "RabbitMQ"})
	}
	if cfg.S3.Endpoint != "" {
		store, err := blob.Open(ctx, blob.Options{Endpoint: cfg.S3.Endpoint, AccessKey: cfg.S3.AccessKey, SecretKey: cfg.S3.SecretKey, Bucket: cfg.S3.Bucket, TLS: cfg.S3.TLS})
		if err != nil {
			return fmt.Errorf("penyimpanan objek tidak bisa dipakai: %w", err)
		}
		opts.Store = store
		opts.Checks = append(opts.Checks, settings.Check{Name: "MinIO", Fn: store.Ping})
	} else {
		opts.Checks = append(opts.Checks, settings.Check{Name: "MinIO"})
	}
	if cfg.MitreSource != "off" {
		src := cfg.MitreSource
		if src == "" || src == "default" {
			src = mitre.DefaultSource
		}
		opts.MitreSource = src
		opts.MitreSHA256 = cfg.MitreSHA256
		host, _ := os.Hostname()
		sched, err := (&mitre.Importer{DB: d, Tenant: tenant, Source: src, ExpectedSHA256: cfg.MitreSHA256}).Schedule(ctx, fmt.Sprintf("%s-%d", host, os.Getpid()))
		if err != nil {
			return fmt.Errorf("menjadwalkan impor MITRE: %w", err)
		}
		defer sched.Stop()
	}
	go server.DemoService(opts).RunCleanup(ctx, tenant)
	go server.RetentionService(opts).Run(ctx, tenant)
	go server.OrganizationService(opts).RunTicker(ctx, tenant)
	go server.ScanService(opts).Run(ctx, tenant)
	go server.EndpointService(opts).RunCollection(ctx, tenant, time.Duration(cfg.EndpointCollectionSeconds)*time.Second)
	go server.ReportService(opts).Run(ctx, tenant)
	go server.Runtime(opts).RunSweeper(ctx, 15*time.Second)
	go server.SimulationService(opts).RunTicker(ctx, tenant, 15*time.Second)
	go server.AgentService(opts).RunSweeper(ctx, 15*time.Second, func(err error) { slog.Error("sweep agent gagal", "err", err) })
	if cfg.AMQPURL != "" {
		go (&mq.Relay{DB: d, Tenant: tenant, URL: cfg.AMQPURL}).Run(ctx)
		if es != nil {
			go (&indexer.Indexer{DB: d, Tenant: tenant, ES: es}).Run(ctx, cfg.AMQPURL)
		}
	} else {
		slog.Warn("ORCHIS_AMQP_URL kosong: eksekusi tidak akan diteruskan ke injector")
	}
	metrics, err := telemetry.Start(ctx)
	if err != nil {
		return err
	}
	go collectMetrics(ctx, metrics, d, tenant, cfg.AMQPURL)
	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           metrics.HTTP(server.New(opts)),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	slog.Info("server berjalan", "addr", cfg.HTTPAddr, "tenant", cfg.TenantName)
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdown); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		slog.Info("server berhenti dengan rapi")
		return nil
	}
}

// withTenant membuka database dan memastikan tenant ada, untuk perintah satu kali.
func withTenant(ctx context.Context, cfg *config.Config, fn func(*db.DB, uuid.UUID) error) error {
	d, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer d.Close()
	tenant, err := bootstrap.EnsureTenant(ctx, d, cfg.TenantName)
	if err != nil {
		return err
	}
	return fn(d, tenant)
}

func fatal(err error) {
	slog.Error("fatal", "err", err)
	os.Exit(1)
}
