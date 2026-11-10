package contract

import (
	"encoding/json"
	"errors"
	"net/netip"
	"strings"
	"testing"
)

func TestNmapParamsRejectInjection(t *testing.T) {
	bad := []string{
		`{"profile":"custom","ports":"22; rm -rf /"}`,
		`{"profile":"custom","ports":"22 -sC"}`,
		`{"profile":"custom","ports":"0"}`,
		`{"profile":"custom","ports":"80-20"}`,
		`{"profile":"custom","ports":"70000"}`,
		`{"profile":"custom"}`,
		`{"profile":"top100","ports":"22"}`,
		`{"profile":"--script=evil"}`,
		`{"profile":"top100","script":"vuln"}`,
		`{"profile":"top100","extra_args":["-oN","/etc/cron.d/x"]}`,
	}
	for _, b := range bad {
		if _, err := Normalize(KeyNmapTCP, json.RawMessage(b)); err == nil {
			t.Errorf("harus ditolak: %s", b)
		} else {
			var pe *ParamError
			if !errors.As(err, &pe) {
				t.Errorf("error harus ParamError: %v", err)
			}
		}
	}
}

func TestNmapArgsPutTargetAfterDoubleDashAndNeverShell(t *testing.T) {
	raw, err := Normalize(KeyNmapTCP, json.RawMessage(`{"profile":"custom","ports":"22,80,8000-8100","service_detection":true}`))
	if err != nil {
		t.Fatal(err)
	}
	args, err := NmapArgs(raw, "10.0.0.5", 120)
	if err != nil {
		t.Fatal(err)
	}
	n := len(args)
	if args[n-2] != "--" || args[n-1] != "10.0.0.5" {
		t.Fatalf("target harus setelah '--': %v", args)
	}
	joined := " " + strings.Join(args, " ") + " "
	for _, must := range []string{" -sT ", " -Pn ", " -sV ", " -p 22,80,8000-8100 "} {
		if !strings.Contains(joined, must) {
			t.Errorf("argumen kurang %q: %v", must, args)
		}
	}
}

func TestNucleiParamsCanonicalAndStrict(t *testing.T) {
	raw, err := Normalize(KeyNucleiVS, json.RawMessage(`{"severity":["high","medium","high"],"tags":["wordpress","cve","cve"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"severity":["medium","high"],"tags":["cve","wordpress"]}` {
		t.Errorf("bentuk kanonik salah: %s", raw)
	}
	for _, b := range []string{`{"severity":[]}`, `{"severity":["urgent"]}`, `{"tags":["a b"]}`, `{"tags":["--evil"]}`, `{"templates":"/etc/passwd"}`, `{"tags":["a;b"]}`} {
		if _, err := Normalize(KeyNucleiVS, json.RawMessage(b)); err == nil {
			t.Errorf("harus ditolak: %s", b)
		}
	}
	args, err := NucleiArgs(raw, "http://10.0.0.5", "")
	if err != nil {
		t.Fatal(err)
	}
	if args[0] != "-u" || args[1] != "http://10.0.0.5" {
		t.Errorf("target harus menjadi nilai -u: %v", args)
	}
	joined := " " + strings.Join(args, " ") + " "
	for _, flag := range []string{" -pt http ", " -ni "} {
		if !strings.Contains(joined, flag) {
			t.Errorf("HTTP-only Nuclei flag %q missing: %v", flag, args)
		}
	}
}

func TestGuardBlocksDangerousTargets(t *testing.T) {
	res := func(host string) ([]netip.Addr, error) {
		switch host {
		case "rebind.example":
			return []netip.Addr{netip.MustParseAddr("93.184.216.34"), netip.MustParseAddr("169.254.169.254")}, nil
		case "fine.example":
			return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
		}
		return nil, errors.New("nxdomain")
	}
	g := Guard{Resolve: res}
	for host, wantErr := range map[string]bool{
		"169.254.169.254": true, "127.0.0.1": true, "::1": true, "0.0.0.0": true, "224.0.0.1": true,
		"fd00:ec2::254": true, "fe80::1": true, "::ffff:169.254.169.254": true,
		"rebind.example": true, "nowhere.example": true, "-oN": true, "--script": true, "a b": true, "": true,
		"10.1.2.3": false, "192.168.1.10": false, "fine.example": false, "2001:db8::1": false,
	} {
		err := g.CheckHost(host)
		if (err != nil) != wantErr {
			t.Errorf("CheckHost(%q) = %v, wantErr=%v", host, err, wantErr)
		}
	}
	if err := (Guard{AllowLoopback: true, Resolve: res}).CheckHost("127.0.0.1"); err != nil {
		t.Errorf("loopback harus diizinkan bila diaktifkan eksplisit: %v", err)
	}
	if err := (Guard{AllowLoopback: true, Resolve: res}).CheckHost("169.254.169.254"); err == nil {
		t.Error("metadata cloud tidak boleh pernah diizinkan, bahkan dengan AllowLoopback")
	}
}

func TestTargetPicksUsableAddressPerContract(t *testing.T) {
	g := Guard{Resolve: func(string) ([]netip.Addr, error) { return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil }}
	addrs := []Address{{Kind: "mac", Value: "aa:bb:cc:dd:ee:ff"}, {Kind: "cidr", Value: "10.0.0.0/24"},
		{Kind: "url", Value: "https://shop.example/login"}, {Kind: "ip", Value: "10.0.0.5"}}
	if got, err := g.Target(KeyNmapTCP, addrs); err != nil || got != "10.0.0.5" {
		t.Errorf("nmap harus memilih ip: %q %v", got, err)
	}
	if got, err := g.Target(KeyNucleiVS, addrs); err != nil || got != "https://shop.example/login" {
		t.Errorf("nuclei harus memilih url: %q %v", got, err)
	}
	if _, err := g.Target(KeyNmapTCP, []Address{{Kind: "cidr", Value: "10.0.0.0/24"}, {Kind: "mac", Value: "aa:bb:cc:dd:ee:ff"}}); err == nil {
		t.Error("rentang CIDR tidak boleh dipindai pada MVP")
	}
	if got, err := g.Target(KeyNmapTCP, []Address{{Kind: "url", Value: "https://shop.example/login"}}); err != nil || got != "shop.example" {
		t.Errorf("nmap dari url harus memakai host saja: %q %v", got, err)
	}
	if _, err := g.Target(KeyNucleiVS, []Address{{Kind: "url", Value: "ftp://x.example"}}); err == nil {
		t.Error("skema selain http/https harus ditolak")
	}
	if _, err := g.Target(KeyNucleiVS, []Address{{Kind: "url", Value: "http://user:pw@x.example/"}}); err == nil {
		t.Error("URL dengan kredensial harus ditolak")
	}
}

func TestNmapRetainsClosedAndFilteredStatesForCoverage(t *testing.T) {
	args, err := NmapArgs([]byte(`{"profile":"custom","ports":"80"}`), "127.0.0.1", 120)
	if err != nil {
		t.Fatal(err)
	}
	for _, arg := range args {
		if arg == "--open" {
			t.Fatal("--open hides the distinction between closed and unreachable endpoints")
		}
	}
}

func TestDeferNamesOnlyWhenResolutionFails(t *testing.T) {
	fail := func(string) ([]netip.Addr, error) { return nil, errors.New("no dns") }
	if err := (Guard{Resolve: fail}).CheckHost("example.com"); err == nil {
		t.Fatal("strict guard must reject an unresolvable name")
	}
	if err := (Guard{Resolve: fail, DeferNames: true}).CheckHost("example.com"); err != nil {
		t.Fatalf("deferred name: %v", err)
	}
	private := func(string) ([]netip.Addr, error) { return []netip.Addr{netip.MustParseAddr("169.254.169.254")}, nil }
	if err := (Guard{Resolve: private, DeferNames: true}).CheckHost("example.com"); err == nil {
		t.Fatal("a name that resolves to a blocked address must stay blocked even when names are deferred")
	}
	if err := (Guard{Resolve: fail, DeferNames: true}).CheckHost("-oX"); err == nil {
		t.Fatal("syntax check must still apply")
	}
}
