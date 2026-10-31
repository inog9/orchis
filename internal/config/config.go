// Package config membaca konfigurasi dari variabel lingkungan.
package config

import (
	"fmt"
	"net/netip"
	"net/url"
	"orchis-go/internal/envfile"
	"os"
	"strconv"
	"strings"
)

type S3Config struct {
	Endpoint, AccessKey, SecretKey, Bucket string
	TLS                                    bool
}

type Config struct {
	Production     bool
	TrustedProxies []netip.Prefix
	HTTPAddr       string
	DatabaseURL    string // peran aplikasi (tunduk pada RLS)
	MigrateURL     string // peran pemilik, hanya untuk migrasi
	AppRole        string // nama peran aplikasi yang diberi hak akses setelah migrasi
	TenantName     string // tenant tunggal MVP
	AdminEmail     string
	AdminPassword  string
	CookieSecure   bool
	WebDir         string // direktori hasil build frontend (opsional)

	// Infrastruktur opsional: tanpa ini server tetap jalan (aset, grup), hanya eksekusi/pencarian tidak aktif.
	AMQPURL string // RabbitMQ
	ESURL   string // Elasticsearch
	S3      S3Config
	// AllowLoopbackTargets mengizinkan aset beralamat 127.0.0.0/8 menjadi sasaran scan. Hanya untuk pengembangan dan tes.
	AllowLoopbackTargets bool
	// MitreSource: URL bundel STIX ATT&CK. Kosong = bawaan (MITRE); "off" = impor dimatikan (mis. lingkungan tanpa internet).
	MitreSource string
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
	c.AgentReleaseDir = os.Getenv("ORCHIS_AGENT_RELEASE_DIR")
	var err error
	if c.AllowLoopbackTargets, err = strconv.ParseBool(env("ORCHIS_ALLOW_LOOPBACK_TARGETS", "false")); err != nil {
		return nil, fmt.Errorf("ORCHIS_ALLOW_LOOPBACK_TARGETS tidak valid: %w", err)
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
