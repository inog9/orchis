package contract

import (
	"fmt"
	"net/netip"
)

const DefaultScanMaxHosts = 256
const MaximumScanMaxHosts = 4096

// ScanScope dibekukan server dari izin admin; agent tidak dapat membuat bukti scope ini.
type ScanScope struct {
	Target            string `json:"target"`
	CIDR              string `json:"cidr,omitempty"`
	TestingAuthorized bool   `json:"testing_authorized"`
	InternetFacing    bool   `json:"internet_facing"`
	MaxHosts          int    `json:"max_hosts"`
}

// BoundedNetwork membatasi JUMLAH alamat, bukan angka prefix lintas keluarga IP.
func BoundedNetwork(raw string, maxHosts int, guard Guard) (netip.Prefix, error) {
	p, err := netip.ParsePrefix(raw)
	if err != nil || p != p.Masked() || p.Bits() == 0 || maxHosts < 1 || maxHosts > MaximumScanMaxHosts {
		return netip.Prefix{}, fmt.Errorf("invalid network or host limit")
	}
	bits := p.Addr().BitLen() - p.Bits()
	if bits > 12 || (1<<bits) > maxHosts {
		return netip.Prefix{}, fmt.Errorf("network exceeds the %d address limit", maxHosts)
	}
	for a := p.Addr(); p.Contains(a); a = a.Next() {
		if err := guard.CheckHost(a.String()); err != nil {
			return netip.Prefix{}, err
		}
		if !a.Next().IsValid() {
			break
		}
	}
	return p, nil
}

func (s ScanScope) Validate(target string, guard Guard) error {
	if !s.TestingAuthorized || target != s.Target {
		return fmt.Errorf("scan target is not authorized")
	}
	host := HostOf(target)
	if err := guard.CheckHost(host); err != nil {
		return err
	}
	a, err := netip.ParseAddr(host)
	if err != nil {
		if !s.InternetFacing {
			return fmt.Errorf("hostname targets require internet-facing authorization")
		}
	} else if !a.IsPrivate() && !a.IsLoopback() && !s.InternetFacing {
		return fmt.Errorf("public targets require internet-facing authorization")
	}
	if s.CIDR != "" {
		p, err := BoundedNetwork(s.CIDR, s.MaxHosts, guard)
		if err != nil {
			return err
		}
		if !a.IsValid() || !p.Contains(a) {
			return fmt.Errorf("scan target is outside the approved network")
		}
	}
	return nil
}
