// Package config membaca konfigurasi dari variabel lingkungan.
package config

import (
	"encoding/hex"
	"fmt"
	"net/netip"
	"net/url"
	"orchis-go/internal/envfile"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type S3Config struct {
	Endpoint, AccessKey, SecretKey, Bucket string
	TLS                                    bool
}

type Config struct {
	Production                bool
	TrustedProxies            []netip.Prefix
	HTTPAddr                  string
	DatabaseURL               string // peran aplikasi (tunduk pada RLS)
	MigrateURL                string // peran pemilik, hanya untuk migrasi
	AppRole                   string // nama peran aplikasi yang diberi hak akses setelah migrasi
	TenantName                string // tenant tunggal MVP
	AdminEmail                string
	AdminPassword             string
	CookieSecure              bool
	AdvancedFeatures          bool
	ScanMaxHosts              int
	ScanConcurrency           int
	EndpointCollectionSeconds int
	WebDir                    string // direktori hasil build frontend (opsional)

	// Infrastruktur opsional: tanpa ini server tetap jalan (aset, grup), hanya eksekusi/pencarian tidak aktif.
	AMQPURL string // RabbitMQ
	ESURL   string // Elasticsearch
	S3      S3Config
	// AllowLoopbackTargets mengizinkan aset beralamat 127.0.0.0/8 menjadi sasaran scan. Hanya untuk pengembangan dan tes.
	AllowLoopbackTargets bool
	// MitreSource: URL atau berkas STIX pada dev; produksi hanya path absolut berkas beku. "off" mematikan impor.
	MitreSource string
	// MitreSHA256: hash bundel ATT&CK yang dipatok (lihat deploy/mitre/manifest.json); bundel dengan hash lain ditolak.
	MitreSHA256 string
	// AgentReleaseDir: direktori rilis agent (hasil `agentrelease`); kosong = pemasangan agent dinonaktifkan.
	AgentReleaseDir string
}

func Load() (*Config, error) {
	if err := envfile.Load(); err != nil {
		return nil, err
	}
	c := &Config{
		HTTPAddr:      env("ORCHIS_HTTP_ADDR", "127.0.0.1:8090"),
		DatabaseURL:   env("ORCHIS_DATABASE_URL", "postgres://orchis_app:orchis_app@127.0.0.1:15432/orchis"),
		MigrateURL:    env("ORCHIS_MIGRATE_URL", "postgres://orchis:orchis@127.0.0.1:15432/orchis"),
		AppRole:       env("ORCHIS_APP_ROLE", "orchis_app"),
		TenantName:    env("ORCHIS_TENANT", "default"),
		AdminEmail:    env("ORCHIS_ADMIN_EMAIL", "admin@orchis.local"),
		AdminPassword: os.Getenv("ORCHIS_ADMIN_PASSWORD"),
		WebDir:        os.Getenv("ORCHIS_WEB_DIR"),
		AMQPURL:       os.Getenv("ORCHIS_AMQP_URL"),
		ESURL:         os.Getenv("ORCHIS_ES_URL"),
		S3: S3Config{
			Endpoint: os.Getenv("ORCHIS_S3_ENDPOINT"), AccessKey: os.Getenv("ORCHIS_S3_ACCESS_KEY"),
			SecretKey: os.Getenv("ORCHIS_S3_SECRET_KEY"), Bucket: env("ORCHIS_S3_BUCKET", "orchis"),
		},
	}
	c.MitreSource = env("ORCHIS_MITRE_URL", "default")
	c.MitreSHA256 = strings.ToLower(strings.TrimSpace(os.Getenv("ORCHIS_MITRE_SHA256")))
	c.AgentReleaseDir = os.Getenv("ORCHIS_AGENT_RELEASE_DIR")
	var err error
	if c.AdvancedFeatures, err = strconv.ParseBool(env("ORCHIS_ADVANCED_FEATURES", "false")); err != nil {
		return nil, fmt.Errorf("ORCHIS_ADVANCED_FEATURES is invalid: %w", err)
	}
	if c.AllowLoopbackTargets, err = strconv.ParseBool(env("ORCHIS_ALLOW_LOOPBACK_TARGETS", "false")); err != nil {
		return nil, fmt.Errorf("ORCHIS_ALLOW_LOOPBACK_TARGETS tidak valid: %w", err)
	}
	c.ScanMaxHosts, err = strconv.Atoi(env("ORCHIS_SCAN_MAX_HOSTS", "256"))
	if err != nil || c.ScanMaxHosts < 1 || c.ScanMaxHosts > 4096 {
		return nil, fmt.Errorf("ORCHIS_SCAN_MAX_HOSTS must be between 1 and 4096")
	}
	c.ScanConcurrency, err = strconv.Atoi(env("ORCHIS_SCAN_CONCURRENCY", "4"))
	if err != nil || c.ScanConcurrency < 1 || c.ScanConcurrency > 32 {
		return nil, fmt.Errorf("ORCHIS_SCAN_CONCURRENCY must be between 1 and 32")
	}
	c.EndpointCollectionSeconds, err = strconv.Atoi(env("ORCHIS_ENDPOINT_COLLECTION_SECONDS", "0"))
	// 0 = collection only when a user starts a DefenseBot run (no periodic collection).
	if err != nil || c.EndpointCollectionSeconds != 0 && (c.EndpointCollectionSeconds < 60 || c.EndpointCollectionSeconds > 86400) {
		return nil, fmt.Errorf("ORCHIS_ENDPOINT_COLLECTION_SECONDS must be 0 (manual only) or between 60 and 86400")
	}
	c.S3.TLS, _ = strconv.ParseBool(env("ORCHIS_S3_TLS", "false"))
	secure, err := strconv.ParseBool(env("ORCHIS_COOKIE_SECURE", "false"))
	if err != nil {
		return nil, fmt.Errorf("ORCHIS_COOKIE_SECURE tidak valid: %w", err)
	}
	c.CookieSecure = secure
	for _, raw := range strings.Split(os.Getenv("ORCHIS_TRUSTED_PROXIES"), ",") {
		if raw == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(strings.TrimSpace(raw))
		if err != nil || prefix.Bits() == 0 {
			return nil, fmt.Errorf("ORCHIS_TRUSTED_PROXIES must contain explicit CIDRs")
		}
		c.TrustedProxies = append(c.TrustedProxies, prefix)
	}
	c.Production, err = strconv.ParseBool(env("ORCHIS_PRODUCTION", "false"))
	if err != nil {
		return nil, fmt.Errorf("ORCHIS_PRODUCTION is invalid")
	}
	if c.Production {
		if !c.CookieSecure || c.AllowLoopbackTargets || os.Getenv("ORCHIS_AI_TEST_URL") != "" {
			return nil, fmt.Errorf("production requires secure cookies, scope enforcement and no test provider")
		}
		if c.MitreSource != "off" {
			pin, pinErr := hex.DecodeString(c.MitreSHA256)
			if !filepath.IsAbs(c.MitreSource) || pinErr != nil || len(pin) != 32 {
				return nil, fmt.Errorf("production requires an absolute local ATT&CK bundle path with a valid ORCHIS_MITRE_SHA256, or ORCHIS_MITRE_URL=off")
			}
		}
		if c.AdminPassword != "" && len(c.AdminPassword) < 24 {
			return nil, fmt.Errorf("production initial admin password must have at least 24 characters")
		}
		if c.AMQPURL != "" {
			u, err := url.Parse(c.AMQPURL)
			if err != nil || u.User == nil {
				return nil, fmt.Errorf("production AMQP credentials are invalid")
			}
			password, ok := u.User.Password()
			if !ok || len(password) < 24 || u.User.Username() == "guest" {
				return nil, fmt.Errorf("production AMQP requires a non-default account and a password of at least 24 characters")
			}
		}
		if c.S3.Endpoint != "" && (c.S3.AccessKey == "" || len(c.S3.SecretKey) < 24) {
			return nil, fmt.Errorf("production object storage requires explicit credentials and a secret of at least 24 characters")
		}
		if os.Getenv("ORCHIS_RENDERER_URL") != "" && len(os.Getenv("ORCHIS_RENDERER_TOKEN")) < 32 {
			return nil, fmt.Errorf("production renderer token must have at least 32 characters")
		}
		configured := false
		for _, key := range []string{"ORCHIS_DATABASE_URL", "ORCHIS_MIGRATE_URL"} {
			value := os.Getenv(key)
			if value == "" {
				continue
			}
			configured = true
			u, e := url.Parse(value)
			if e != nil || u.User == nil {
				return nil, fmt.Errorf("production database credentials are invalid")
			}
			password, ok := u.User.Password()
			if !ok || len(password) < 24 {
				return nil, fmt.Errorf("production database password must have at least 24 characters")
			}
		}
		if !configured {
			return nil, fmt.Errorf("production database URL must be configured explicitly")
		}
	}
	return c, nil
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
