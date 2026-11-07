// Package server merakit seluruh handler HTTP.
package server

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/netip"
	"orchis-go/internal/ai"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"orchis-go/internal/agents"
	"orchis-go/internal/alerts"
	"orchis-go/internal/assets"
	"orchis-go/internal/auditview"
	"orchis-go/internal/auth"
	"orchis-go/internal/contract"
	"orchis-go/internal/db"
	"orchis-go/internal/demo"
	"orchis-go/internal/endpoint"
	"orchis-go/internal/execution"
	"orchis-go/internal/groups"
	"orchis-go/internal/home"
	"orchis-go/internal/httpx"
	"orchis-go/internal/integrations"
	"orchis-go/internal/mitre"
	"orchis-go/internal/organizations"
	"orchis-go/internal/people"
	"orchis-go/internal/reporting"
	"orchis-go/internal/retention"
	"orchis-go/internal/scancontrol"
	"orchis-go/internal/scans"
	"orchis-go/internal/scenario"
	"orchis-go/internal/settings"
	"orchis-go/internal/simulation"
	"orchis-go/internal/tags"
	"orchis-go/internal/users"
)

type Options struct {
	Now                       func() time.Time
	DB                        *db.DB
	Tenant                    uuid.UUID
	PublicRateLimit           bool
	TrustedProxies            []netip.Prefix
	CookieSecure              bool
	AdvancedFeatures          bool
	ScanMaxHosts              int
	ScanConcurrency           int
	EndpointCollectionSeconds int
	WebDir                    string
	Version                   string
	Checks                    []settings.Check
	AgentRelease              string // direktori rilis agent (manifest.json + biner); kosong = tidak ada rilis
	MitreSource               string // sumber STIX (URL atau berkas beku); kosong = impor manual dinonaktifkan
	MitreSHA256               string // hash bundel yang dipatok; kosong = tidak diperiksa (hanya pengembangan)

	// Eksekusi. Guard menentukan sasaran yang boleh dipindai; Search/Store boleh nil.
	Guard         contract.Guard
	Search        execution.Searcher
	AIKey         []byte
	AIWorkerToken string
	Store         execution.ObjectStore
}

func New(o Options) http.Handler {
	mux := http.NewServeMux()
	authSvc := auth.NewService(o.DB)
	ah := &auth.Handlers{Svc: authSvc, Tenant: o.Tenant, CookieSecure: o.CookieSecure, AdvancedFeatures: o.AdvancedFeatures}

	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := o.DB.Pool.Ping(ctx); err != nil {
			httpx.Write(w, r, httpx.NewProblem(503, "Database unavailable", ""))
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	ah.Register(mux)
	(&scans.Handlers{Svc: ScanService(o)}).Register(mux)
	(&scancontrol.Handlers{Svc: ScanControlService(o)}).Register(mux)
	(&retention.Handlers{Svc: RetentionService(o)}).Register(mux)
	(&auditview.Handler{DB: o.DB}).Register(mux)
	(&ai.Handlers{Svc: AIService(o), Reports: ReportService(o)}).Register(mux)
	(&organizations.Handlers{Svc: OrganizationService(o)}).Register(mux)
	(&demo.Handlers{Svc: DemoService(o)}).Register(mux)
	(&endpoint.Handlers{Svc: EndpointService(o)}).Register(mux)
	(&reporting.Handlers{Svc: ReportService(o)}).Register(mux)
	(&assets.Handlers{Svc: &assets.Service{DB: o.DB}}).Register(mux)
	(&groups.Handlers{Svc: &groups.Service{DB: o.DB}}).Register(mux)
	(&tags.Handlers{Svc: &tags.Service{DB: o.DB}}).Register(mux)
	(&execution.Handlers{Svc: &execution.Service{Now: o.Now, DB: o.DB, Guard: o.Guard}, Search: o.Search}).Register(mux)
	(&scenario.Handlers{Svc: &scenario.Service{DB: o.DB}}).Register(mux)
	(&simulation.Handlers{Svc: SimulationService(o)}).Register(mux)
	(&alerts.Handlers{Svc: &alerts.Service{DB: o.DB}}).Register(mux)
	(&users.Handlers{Svc: &users.Service{DB: o.DB, Tenant: o.Tenant}}).Register(mux)
	(&people.Handlers{Svc: &people.Service{DB: o.DB}}).Register(mux)
	(&integrations.Handlers{Svc: &integrations.Service{DB: o.DB}}).Register(mux)
	(&settings.Handlers{Svc: &settings.Service{DB: o.DB, Version: o.Version, Checks: o.Checks}}).Register(mux)
	(&mitre.Handlers{Svc: mitreService(o)}).Register(mux)
	(&agents.Handlers{Svc: AgentService(o), RT: Runtime(o)}).Register(mux)
	(&home.Handlers{Svc: &home.Service{DB: o.DB}}).Register(mux)
	registerInstallScript(mux)
	(&execution.InjectorHandlers{RT: Runtime(o)}).Register(mux)

	// Semua jalur /api yang tidak dikenal -> 404 seragam; sisanya berkas statis SPA.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		httpx.Write(w, r, httpx.NotFound("Endpoint"))
	})
	if o.WebDir != "" {
		mux.Handle("/", spa(o.WebDir))
	}
	var handler http.Handler = httpx.CSRF(ah.Middleware(mux))
	if o.PublicRateLimit {
		handler = auth.NewPublicLimiter().Middleware(handler)
	}
	return auth.TrustedProxy(o.TrustedProxies, securityHeaders(httpx.Base(handler)))
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; font-src 'self'; connect-src 'self'; frame-ancestors 'none'")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

// spa menyajikan hasil build frontend; jalur tanpa berkas dialihkan ke index.html (routing sisi klien).
func spa(dir string) http.Handler {
	fs := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := filepath.Join(dir, filepath.Clean("/"+r.URL.Path))
		if st, err := os.Stat(p); err != nil || st.IsDir() {
			if r.URL.Path != "/" {
				http.ServeFile(w, r, filepath.Join(dir, "index.html"))
				return
			}
		}
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		fs.ServeHTTP(w, r)
	})
}

// Runtime membangun sisi server untuk injector dari opsi yang sama dengan handler HTTP.
func Runtime(o Options) *execution.Runtime {
	eng := &simulation.Engine{Guard: o.Guard, Now: o.Now}
	return &execution.Runtime{Now: o.Now, DB: o.DB, Tenant: o.Tenant, Guard: o.Guard, Store: o.Store, ScanMaxHosts: o.ScanMaxHosts, AfterEnd: eng.Advance}
}

// SimulationService membangun layanan simulasi dengan mesin chain yang memakai penjaga scope yang sama.
func SimulationService(o Options) *simulation.Service {
	return &simulation.Service{DB: o.DB, Engine: &simulation.Engine{Guard: o.Guard, Now: o.Now}}
}

func mitreService(o Options) *mitre.Service {
	svc := &mitre.Service{DB: o.DB}
	if o.MitreSource != "" {
		svc.Importer = &mitre.Importer{DB: o.DB, Tenant: o.Tenant, Source: o.MitreSource, ExpectedSHA256: o.MitreSHA256}
	}
	return svc
}

// AgentService membangun layanan agent; AfterEnd menjaga chain maju saat eksekusi agent dibatalkan karena pencabutan.
func AgentService(o Options) *agents.Service {
	eng := &simulation.Engine{Guard: o.Guard, Now: o.Now}
	return &agents.Service{Now: o.Now, CollectNow: EndpointService(o).CollectNow, DB: o.DB, Tenant: o.Tenant, Release: &agents.Release{Dir: o.AgentRelease}, AfterEnd: eng.Advance}
}

func OrganizationService(o Options) *organizations.Service {
	return &organizations.Service{Now: o.Now, DB: o.DB, Engine: &simulation.Engine{Guard: o.Guard, Now: o.Now}}
}

func ReportService(o Options) *reporting.Service {
	return &reporting.Service{DB: o.DB, Store: o.Store, Script: os.Getenv("ORCHIS_REPORT_RENDER_SCRIPT"), RendererURL: os.Getenv("ORCHIS_RENDERER_URL"), RendererToken: os.Getenv("ORCHIS_RENDERER_TOKEN")}
}

// AIService dinonaktifkan secara efektif tanpa key dan worker token; tidak melakukan panggilan provider di server.
func AIService(o Options) *ai.Service {
	key := o.AIKey
	if key == nil {
		encoded := os.Getenv("ORCHIS_AI_ENCRYPTION_KEY")
		if path := os.Getenv("ORCHIS_AI_ENCRYPTION_KEY_FILE"); path != "" {
			b, err := os.ReadFile(path)
			if err == nil && len(b) < 128 {
				encoded = strings.TrimSpace(string(b))
			} else {
				encoded = ""
			}
		}
		key, _ = base64.StdEncoding.DecodeString(encoded)
	}
	token := o.AIWorkerToken
	if token == "" {
		token = os.Getenv("ORCHIS_AI_WORKER_TOKEN")
	}
	return &ai.Service{Now: o.Now, DB: o.DB, Tenant: o.Tenant, Key: key, WorkerToken: token, Guard: o.Guard}
}

// DemoService also wires cleanup capabilities; never report external cleanup on
// the strength of deleting database rows alone.
func DemoService(o Options) *demo.Service {
	s := &demo.Service{DB: o.DB}
	if store, ok := o.Store.(demo.BlobCleaner); ok {
		s.Blob = store
	}
	if search, ok := o.Search.(demo.SearchCleaner); ok {
		s.Search = search
	}
	return s
}

func ScanService(o Options) *scans.Service {
	return &scans.Service{Now: o.Now, DB: o.DB, Guard: o.Guard, Engine: &simulation.Engine{Guard: o.Guard, Now: o.Now}, Reports: ReportService(o), MaxHosts: o.ScanMaxHosts, Concurrency: o.ScanConcurrency, Collector: EndpointService(o)}
}

func EndpointService(o Options) *endpoint.Service {
	seconds := o.EndpointCollectionSeconds
	if seconds == 0 {
		seconds = 900
	}
	return &endpoint.Service{DB: o.DB, Exec: &execution.Service{Now: o.Now, DB: o.DB, Guard: o.Guard}, CollectionInterval: time.Duration(seconds) * time.Second}
}

func ScanControlService(o Options) *scancontrol.Service {
	eng := &simulation.Engine{Guard: o.Guard, Now: o.Now}
	return &scancontrol.Service{DB: o.DB, Now: o.Now, Advance: eng.Advance}
}

func RetentionService(o Options) *retention.Service {
	s := &retention.Service{DB: o.DB, Now: o.Now}
	if b, ok := o.Store.(retention.BlobCleaner); ok {
		s.Blob = b
	}
	if es, ok := o.Search.(retention.SearchCleaner); ok {
		s.Search = es
	}
	return s
}
