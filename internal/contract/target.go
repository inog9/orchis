package contract

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
)

// Address adalah satu alamat aset (kind sesuai tabel asset_addresses).
type Address struct {
	Kind      string `json:"kind"`
	Value     string `json:"value"`
	IsPrimary bool   `json:"is_primary"`
}

var hostRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,62}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,62}[a-z0-9])?)*$`)

// Resolver dipakai untuk memeriksa hostname; di tes diganti pengganti.
type Resolver func(host string) ([]netip.Addr, error)

func SystemResolver(host string) ([]netip.Addr, error) {
	ips, err := net.LookupIP(host)
	if err != nil {
		return nil, err
	}
	out := make([]netip.Addr, 0, len(ips))
	for _, ip := range ips {
		if a, ok := netip.AddrFromSlice(ip); ok {
			out = append(out, a.Unmap())
		}
	}
	return out, nil
}

// Guard menolak sasaran yang tidak boleh dipindai apa pun izin asetnya.
type Guard struct {
	AllowLoopback bool
	Resolve       Resolver
}

var errBlocked = errors.New("blocked")

func (g Guard) checkIP(a netip.Addr) error {
	a = a.Unmap()
	switch {
	case a.IsUnspecified():
		return fmt.Errorf("%w: unspecified address", errBlocked)
	case a.IsMulticast():
		return fmt.Errorf("%w: multicast address", errBlocked)
	case a.IsLinkLocalUnicast():
		// Mencakup 169.254.169.254 (metadata cloud) dan fe80::/10.
		return fmt.Errorf("%w: link-local address (cloud metadata range)", errBlocked)
	case a.IsLoopback() && !g.AllowLoopback:
		return fmt.Errorf("%w: loopback address", errBlocked)
	case a == netip.MustParseAddr("fd00:ec2::254"):
		return fmt.Errorf("%w: cloud metadata address", errBlocked)
	}
	return nil
}

// CheckHost memeriksa host (IP atau nama). Nama diterjemahkan dan SEMUA hasilnya harus lolos.
func (g Guard) CheckHost(host string) error {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" || len(host) > 253 {
		return errors.New("empty or too long target")
	}
	if a, err := netip.ParseAddr(host); err == nil {
		return g.checkIP(a)
	}
	if !hostRe.MatchString(host) { // juga menolak awalan "-" yang dapat terbaca sebagai opsi
		return errors.New("target is not a valid host name")
	}
	res := g.Resolve
	if res == nil {
		res = SystemResolver
	}
	ips, err := res(host)
	if err != nil || len(ips) == 0 {
		return errors.New("target name does not resolve")
	}
	for _, ip := range ips {
		if err := g.checkIP(ip); err != nil {
			return err
		}
	}
	return nil
}

// Target memilih alamat yang cocok untuk kontrak dan memastikan alamat itu lolos penjaga.
// nmap: butuh host (ip/hostname/domain, atau host dari url). nuclei: lebih suka url, lalu nama, lalu ip.
func (g Guard) Target(key string, addrs []Address) (string, error) {
	pref := []string{"ip", "hostname", "domain", "url"}
	if Adapter(key) == "nuclei" {
		pref = []string{"url", "hostname", "domain", "ip"}
	}
	var lastErr error = errors.New("the asset has no address this action can use (needs an IP, host name or URL)")
	for _, kind := range pref {
		for _, ad := range addrs {
			if ad.Kind != kind {
				continue
			}
			target, host, err := normalizeTarget(key, ad)
			if err != nil {
				lastErr = err
				continue
			}
			if err := g.CheckHost(host); err != nil {
				lastErr = err
				continue
			}
			return target, nil
		}
	}
	return "", lastErr
}

func normalizeTarget(key string, ad Address) (target, host string, err error) {
	v := strings.TrimSpace(ad.Value)
	if ad.Kind == "url" {
		u, perr := url.Parse(v)
		if perr != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
			return "", "", errors.New("URL must be http or https without credentials")
		}
		if Adapter(key) == "nuclei" {
			return u.String(), u.Hostname(), nil
		}
		return u.Hostname(), u.Hostname(), nil
	}
	return v, v, nil
}

// HostOf mengambil host dari target yang tersimpan (URL penuh atau host polos).
func HostOf(target string) string {
	if strings.Contains(target, "://") {
		if u, err := url.Parse(target); err == nil {
			return u.Hostname()
		}
		return ""
	}
	return target
}
