package agent

import (
	"strings"
	"testing"
)

func TestParseOSRelease(t *testing.T) {
	ubuntu := "NAME=\"Ubuntu\"\nVERSION=\"24.04.1 LTS (Noble Numbat)\"\nID=ubuntu\nPRETTY_NAME=\"Ubuntu 24.04.1 LTS\"\nVERSION_ID=\"24.04\"\n"
	if got := parseOSRelease(ubuntu); got != "Ubuntu 24.04.1 LTS" {
		t.Fatalf("pretty name: %q", got)
	}
	if got := parseOSRelease("NAME=Alpine\nVERSION_ID=3.20.1\n# comment\n"); got != "Alpine 3.20.1" {
		t.Fatalf("fallback: %q", got)
	}
	if got := parseOSRelease("garbage without equals\n"); got != "" {
		t.Fatalf("garbage: %q", got)
	}
}

func TestParseSwVersAndWindowsVer(t *testing.T) {
	if got := parseSwVers("10.15.7\n"); got != "macOS 10.15.7" {
		t.Fatalf("sw_vers: %q", got)
	}
	if parseSwVers("  \n") != "" {
		t.Fatal("empty sw_vers must be unknown")
	}
	if got := parseWindowsVer("\r\nMicrosoft Windows [Version 10.0.19045.3693]\r\n"); got != "Windows 10.0.19045" {
		t.Fatalf("ver: %q", got)
	}
	if parseWindowsVer("something else") != "" {
		t.Fatal("unparseable ver must be unknown")
	}
}

func TestCleanOSVersionStripsControlCharactersAndCaps(t *testing.T) {
	if got := cleanOSVersion("  macOS 10.15.7\x00\n "); got != "macOS 10.15.7" {
		t.Fatalf("control chars: %q", got)
	}
	if got := cleanOSVersion(strings.Repeat("a", 500)); len(got) != osVersionMax {
		t.Fatalf("not capped: %d", len(got))
	}
}
