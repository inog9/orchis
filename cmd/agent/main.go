// orchis-agent: agent endpoint ORCHIS. Satu biner statis untuk Windows, Linux, dan macOS.
//
//	orchis-agent enroll --server URL --token TOKEN   daftarkan agent (token sekali pakai dari konsol)
//	orchis-agent install | start | stop | restart | uninstall   kelola sebagai service (butuh hak admin)
//	orchis-agent run                                 jalankan di latar depan (mode user)
//	orchis-agent version
//
// Agent hanya melakukan koneksi KELUAR ke server dan hanya menjalankan aksi baku yang ia kenal.
package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/kardianos/service"

	"orchis-go/internal/agent"
)

// Diisi saat build (lihat cmd/agentrelease): -X main.version=... -X main.pubKeyB64=...
var (
	version   = "dev"
	pubKeyB64 = ""
)

const svcName = "orchis-agent"

// selfPath dihitung SEKALI saat proses mulai, sebelum ada rename: di Linux os.Executable() mengikuti berkas yang sudah
// di-rename (menjadi ".old"), sehingga memanggilnya lagi setelah update akan menimpa berkas yang salah.
var selfPath = func() string {
	p, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}
	p, _ = filepath.EvalSymlinks(p)
	return p
}()

func cfgPath() string { return filepath.Join(filepath.Dir(selfPath), "agent-config.json") }

func pubKey() ed25519.PublicKey {
	if pubKeyB64 == "" {
		return nil
	}
	b, err := base64.StdEncoding.DecodeString(pubKeyB64)
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil
	}
	return ed25519.PublicKey(b)
}

type program struct {
	cancel context.CancelFunc
	done   chan struct{}
	svc    service.Service
	cfg    *agent.Config
}

func (p *program) agent() *agent.Agent {
	a := &agent.Agent{Cfg: p.cfg, ConfigPath: cfgPath(), Version: version, PubKey: pubKey(), Exe: selfPath,
		// Unix exec preserves the supervised PID without a blocking self-restart.
		// Windows exits for the configured SCM recovery policy.
		Restart: func() { agent.Reexec(selfPath) }}

	if runtime.GOOS != "windows" {
		a.ServiceManaged = p.svc != nil
		a.StartUninstall = func() error { return agent.StartUninstallHelper(selfPath, p.cfg) }
	}
	return a
}

func (p *program) Start(s service.Service) error {
	p.svc = s
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel, p.done = cancel, make(chan struct{})
	go func() {
		defer close(p.done)
		_ = p.agent().Run(ctx)
	}()
	return nil
}

func (p *program) Stop(service.Service) error {
	p.cancel()
	select {
	case <-p.done:
	case <-time.After(5 * time.Second):
	}
	return nil
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("orchis-agent: ")
	cmd := ""
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	cfg, cfgErr := agent.LoadConfig(cfgPath())
	if cfg != nil && !agent.ValidInstanceName(cfg.Name) {
		log.Fatal("invalid instance name")
	}
	prg := &program{cfg: cfg}
	opt := service.KeyValue{"Restart": "always", "KeepAlive": true, "RunAtLoad": true, "OnFailure": "restart", "OnFailureDelayDuration": "5s"}
	if cfg != nil && cfg.Mode == "user" {
		opt["UserService"] = true
	}
	instanceService := svcName
	if cfg != nil && cfg.Name != "" {
		instanceService += "-" + cfg.Name
	}
	svc, err := service.New(prg, &service.Config{Name: instanceService, DisplayName: "ORCHIS agent", Description: "ORCHIS endpoint agent", Option: opt})
	if err != nil {
		log.Fatal(err)
	}
	switch cmd {
	case "version":
		fmt.Println(version, runtime.GOOS+"/"+runtime.GOARCH)
	case "remote-uninstall":
		if cfg == nil || cfg.Uninstall == nil || !agent.ValidInstanceName(cfg.Name) {
			log.Fatal("invalid uninstall receipt")
		}
		if err := prg.remoteUninstall(svc); err != nil {
			log.Fatal("uninstall did not complete")
		}
	case "enroll":
		fs := flag.NewFlagSet("enroll", flag.ExitOnError)
		name := fs.String("name", "", "unique agent instance name")
		server, token := fs.String("server", "", "URL server, mis. https://orchis.example.com"), fs.String("token", "", "token pendaftaran sekali pakai")
		caFile := fs.String("ca-file", "", "PEM CA berkas untuk server dengan CA HTTPS sendiri")
		_ = fs.Parse(os.Args[2:])
		if *server == "" || *token == "" {
			log.Fatal("--server dan --token wajib diisi")
		}
		var hc *http.Client
		if *caFile != "" {
			abs, err := filepath.Abs(*caFile)
			if err != nil {
				log.Fatal(err)
			}
			*caFile = abs
			if hc, err = agent.NewHTTPClient(abs, 30*time.Second); err != nil {
				log.Fatalf("CA tidak dapat dipakai: %v", err)
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		c, err := agent.EnrollNamed(ctx, *server, *token, version, *name, hc)
		if err != nil {
			log.Fatalf("enroll gagal: %v", err)
		}
		c.CAFile = *caFile
		if err := c.Save(cfgPath()); err != nil {
			log.Fatalf("tidak bisa menyimpan konfigurasi: %v", err)
		}
		fmt.Printf("enroll berhasil. agent_id=%s mode=%s\n", c.AgentID, c.Mode)
	case "install", "uninstall", "start", "stop", "restart":
		if err := service.Control(svc, cmd); err != nil {
			log.Fatal(err)
		}
		fmt.Println("ok:", cmd)
	case "run":
		if cfg == nil {
			log.Fatalf("belum terdaftar (%v). Jalankan: orchis-agent enroll --server URL --token TOKEN", cfgErr)
		}
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		_ = prg.agent().Run(ctx)
	default: // dijalankan oleh service manager
		if cfg == nil {
			// Belum terdaftar: tetap hidup tanpa melakukan apa pun, supaya service manager tidak menghidupkan ulang terus-menerus.
			log.Printf("belum terdaftar (%v); menunggu. Jalankan: orchis-agent enroll --server URL --token TOKEN", cfgErr)
			prg.cfg = &agent.Config{}
		}
		if err := svc.Run(); err != nil {
			log.Fatal(err)
		}
	}
}

// Fixed detached helper; no shell or remotely supplied arguments/paths.
func (p *program) remoteUninstall(svc service.Service) error {
	lockPath := filepath.Join(filepath.Dir(selfPath), ".orchis-uninstall.lock")
	unlock, err := agent.AcquireUninstallLock(lockPath)
	if err != nil {
		return err
	}
	defer unlock()
	receipt := *p.cfg.Uninstall
	a := p.agent()
	fail := func(e error) error {
		p.cfg.Uninstall = nil
		_ = p.cfg.Save(cfgPath())
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = a.UninstallAck(ctx, receipt, false)
		if receipt.Managed {
			_ = svc.Start()
		}
		return e
	}
	if receipt.Managed {
		if err = svc.Stop(); err != nil {
			// Status must prove it is stopped before unregistering. Never ignore a live
			// service stop error or delete the executable underneath another worker.
			status, e := svc.Status()
			if e != nil || status != service.StatusStopped {
				return fail(err)
			}
		}
		if err = svc.Uninstall(); err != nil {
			return fail(err)
		}
	}
	if err = a.RemoveOwnedFiles(); err != nil {
		return fail(err)
	}
	for attempt := 0; attempt < 3; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		err = a.UninstallAck(ctx, receipt, true)
		cancel()
		if err == nil {
			break
		}
		time.Sleep(time.Second)
	}
	_ = os.Remove(lockPath)
	return err
}
