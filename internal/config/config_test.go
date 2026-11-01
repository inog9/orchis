package config

import (
	"strings"
	"testing"
)

func TestProductionRejectsUnsafeDefaults(t *testing.T) {
	t.Setenv("ORCHIS_ENV_FILE", "")
	t.Setenv("ORCHIS_PRODUCTION", "true")
	t.Setenv("ORCHIS_COOKIE_SECURE", "true")
	t.Setenv("ORCHIS_ALLOW_LOOPBACK_TARGETS", "false")
	t.Setenv("ORCHIS_AI_TEST_URL", "")
	t.Setenv("ORCHIS_MIGRATE_URL", "")
	t.Setenv("ORCHIS_DATABASE_URL", "")
	t.Setenv("ORCHIS_TRUSTED_PROXIES", "")
	t.Setenv("ORCHIS_MITRE_URL", "off")
	t.Setenv("ORCHIS_MITRE_SHA256", "")
	if _, e := Load(); e == nil {
		t.Fatal("development database fallback accepted")
	}
	t.Setenv("ORCHIS_DATABASE_URL", "postgres://orchis_app:short@pg/orchis")
	if _, e := Load(); e == nil {
		t.Fatal("weak password accepted")
	}
	t.Setenv("ORCHIS_DATABASE_URL", "postgres://orchis_app:"+strings.Repeat("r", 32)+"@pg/orchis")
	if _, e := Load(); e != nil {
		t.Fatal(e)
	}
	t.Setenv("ORCHIS_ALLOW_LOOPBACK_TARGETS", "true")
	if _, e := Load(); e == nil {
		t.Fatal("production loopback bypass accepted")
	}
	t.Setenv("ORCHIS_ALLOW_LOOPBACK_TARGETS", "false")
	t.Setenv("ORCHIS_AI_TEST_URL", "http://fake")
	if _, e := Load(); e == nil {
		t.Fatal("test provider accepted")
	}
	t.Setenv("ORCHIS_AI_TEST_URL", "")
	t.Setenv("ORCHIS_COOKIE_SECURE", "false")
	if _, e := Load(); e == nil {
		t.Fatal("insecure cookies accepted")
	}
}

func TestProductionRequiresPinnedMitreBundle(t *testing.T) {
	t.Setenv("ORCHIS_ENV_FILE", "")
	t.Setenv("ORCHIS_PRODUCTION", "true")
	t.Setenv("ORCHIS_COOKIE_SECURE", "true")
	t.Setenv("ORCHIS_ALLOW_LOOPBACK_TARGETS", "false")
	t.Setenv("ORCHIS_AI_TEST_URL", "")
	t.Setenv("ORCHIS_MIGRATE_URL", "")
	t.Setenv("ORCHIS_TRUSTED_PROXIES", "")
	t.Setenv("ORCHIS_DATABASE_URL", "postgres://orchis_app:"+strings.Repeat("r", 32)+"@pg/orchis")
	for _, src := range []string{"default", "https://example.invalid/enterprise-attack.json", "/mitre/enterprise-attack-20.0.json"} {
		t.Setenv("ORCHIS_MITRE_URL", src)
		t.Setenv("ORCHIS_MITRE_SHA256", "")
		if _, e := Load(); e == nil {
			t.Fatalf("production accepted unpinned ATT&CK source %q", src)
		}
		t.Setenv("ORCHIS_MITRE_SHA256", strings.Repeat("A", 64))
		c, e := Load()
		if strings.HasPrefix(src, "/") {
			if e != nil || c.MitreSHA256 != strings.Repeat("a", 64) {
				t.Fatalf("local pinned source %q rejected or hash not normalised: %v", src, e)
			}
		} else if e == nil {
			t.Fatalf("production accepted runtime network source %q even with a pin", src)
		}
	}
	t.Setenv("ORCHIS_MITRE_URL", "/mitre/enterprise-attack-20.0.json")
	t.Setenv("ORCHIS_MITRE_SHA256", strings.Repeat("z", 64))
	if _, e := Load(); e == nil {
		t.Fatal("invalid SHA-256 accepted")
	}
	t.Setenv("ORCHIS_MITRE_URL", "off")
	t.Setenv("ORCHIS_MITRE_SHA256", "")
	if _, e := Load(); e != nil {
		t.Fatalf("explicitly disabled ATT&CK import must be allowed: %v", e)
	}
	// Pengembangan tidak dibatasi.
	t.Setenv("ORCHIS_PRODUCTION", "false")
	t.Setenv("ORCHIS_MITRE_URL", "default")
	if _, e := Load(); e != nil {
		t.Fatalf("development must keep the default source: %v", e)
	}
}

func TestProductionRejectsDefaultServiceAndInitialAdminCredentials(t *testing.T) {
	for _, tc := range []struct{ name, key, value string }{
		{"initial-admin", "ORCHIS_ADMIN_PASSWORD", "Orchis-Dev-Pass-1"},
		{"broker", "ORCHIS_AMQP_URL", "amqp://orchis:orchis@mq:5672/"},
		{"broker-guest", "ORCHIS_AMQP_URL", "amqp://guest:" + strings.Repeat("r", 32) + "@mq:5672/"},
		{"object-store", "ORCHIS_S3_SECRET_KEY", "orchis-dev-secret"},
		{"renderer", "ORCHIS_RENDERER_TOKEN", "sample-renderer-token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ORCHIS_ENV_FILE", "")
			t.Setenv("ORCHIS_PRODUCTION", "true")
			t.Setenv("ORCHIS_COOKIE_SECURE", "true")
			t.Setenv("ORCHIS_ALLOW_LOOPBACK_TARGETS", "false")
			t.Setenv("ORCHIS_AI_TEST_URL", "")
			t.Setenv("ORCHIS_MIGRATE_URL", "")
			t.Setenv("ORCHIS_MITRE_URL", "off")
			t.Setenv("ORCHIS_TRUSTED_PROXIES", "")
			t.Setenv("ORCHIS_DATABASE_URL", "postgres://orchis_app:"+strings.Repeat("r", 32)+"@pg/orchis")
			t.Setenv("ORCHIS_ADMIN_PASSWORD", strings.Repeat("a", 32))
			t.Setenv("ORCHIS_AMQP_URL", "amqp://orchis:"+strings.Repeat("m", 32)+"@mq:5672/")
			t.Setenv("ORCHIS_S3_ENDPOINT", "minio:9000")
			t.Setenv("ORCHIS_S3_ACCESS_KEY", "orchis")
			t.Setenv("ORCHIS_S3_SECRET_KEY", strings.Repeat("s", 32))
			t.Setenv("ORCHIS_RENDERER_URL", "http://renderer:8091/render")
			t.Setenv("ORCHIS_RENDERER_TOKEN", strings.Repeat("t", 32))
			if _, err := Load(); err != nil {
				t.Fatalf("valid generated-style credentials rejected: %v", err)
			}
			t.Setenv(tc.key, tc.value)
			if _, err := Load(); err == nil || strings.Contains(err.Error(), tc.value) {
				t.Fatalf("unsafe credential accepted or disclosed: %v", err)
			}
		})
	}
}
