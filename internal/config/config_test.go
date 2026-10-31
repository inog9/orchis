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
