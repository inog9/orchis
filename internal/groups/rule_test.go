package groups

import (
	"strings"
	"testing"
)

func TestNormalizeRejectsBadRules(t *testing.T) {
	cases := map[string]Rule{
		"mode":           {Mode: "xor"},
		"field":          {Filters: []Filter{{Field: "password", Operator: "eq", Values: []string{"x"}}}},
		"operator":       {Filters: []Filter{{Field: "platform", Operator: "contains", Values: []string{"x"}}}},
		"enum":           {Filters: []Filter{{Field: "platform", Operator: "eq", Values: []string{"solaris"}}}},
		"jumlah nilai":   {Filters: []Filter{{Field: "platform", Operator: "eq", Values: []string{"linux", "windows"}}}},
		"nilai kosong":   {Filters: []Filter{{Field: "tag", Operator: "eq", Values: []string{"  "}}}},
		"is_empty+nilai": {Filters: []Filter{{Field: "platform", Operator: "is_empty", Values: []string{"linux"}}}},
		"bool":           {Filters: []Filter{{Field: "is_internet_facing", Operator: "eq", Values: []string{"ya"}}}},
		"terlalu banyak": {Filters: make([]Filter, 11)},
	}
	for name, r := range cases {
		r := r
		if err := r.Normalize(); err == nil {
			t.Errorf("%s: aturan tidak valid seharusnya ditolak", name)
		}
	}
}

func TestNormalizeAcceptsAndNormalizes(t *testing.T) {
	r := Rule{Filters: []Filter{{Field: "hostname", Operator: "contains", Values: []string{"  WEB-01 "}}, {Field: "platform", Operator: "in", Values: []string{"linux", "macos"}}}}
	if err := r.Normalize(); err != nil {
		t.Fatal(err)
	}
	if r.Mode != "and" || r.Filters[0].Values[0] != "web-01" {
		t.Fatalf("hasil normalisasi salah: %+v", r)
	}
}

// Nilai pengguna TIDAK PERNAH masuk ke teks SQL, hanya lewat parameter.
func TestWhereIsParameterized(t *testing.T) {
	evil := "x'; DROP TABLE assets; --"
	for _, op := range []string{"eq", "contains"} {
		r := Rule{Mode: "and", Filters: []Filter{{Field: "hostname", Operator: op, Values: []string{evil}}}}
		if err := r.Normalize(); err != nil {
			t.Fatal(err)
		}
		sql, args := r.Where(2)
		if strings.Contains(strings.ToLower(sql), "drop table") || strings.Contains(sql, evil) {
			t.Fatalf("nilai bocor ke SQL: %s", sql)
		}
		if len(args) != 1 || !strings.Contains(strings.ToLower(args[0].(string)), "drop table") {
			t.Fatalf("nilai harus ada di argumen: %v", args)
		}
	}
	// placeholder berurutan dari titik awal
	r := Rule{Mode: "or", Filters: []Filter{{Field: "platform", Operator: "eq", Values: []string{"linux"}}, {Field: "arch", Operator: "in", Values: []string{"arm64", "x86_64"}}}}
	_ = r.Normalize()
	sql, args := r.Where(5)
	if !strings.Contains(sql, "$5") || !strings.Contains(sql, "$6") || len(args) != 2 || !strings.Contains(sql, " OR ") {
		t.Fatalf("placeholder/mode salah: %s %v", sql, args)
	}
	if s, a := (Rule{}).Where(2); s != "TRUE" || a != nil {
		t.Fatalf("aturan kosong harus cocok semua: %s", s)
	}
}
