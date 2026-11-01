package simulation

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"orchis-go/internal/chain"
	"orchis-go/internal/contract"
	"sort"
	"strconv"
	"strings"
)

// mapInput tidak menerima URL arbitrer dari finding: host berasal dari aset beku dan port dari hasil induk langsung.
func mapInput(s stepRow, t target, edges []chain.Edge, found []chain.Found) (stepRow, target, error) {
	var mapper *chain.Mapper
	parent := ""
	for _, edge := range edges {
		if edge.Child == s.Key && edge.Mapper != nil {
			if mapper != nil {
				return s, t, fmt.Errorf("only one mapper may supply a step")
			}
			mapper = edge.Mapper
			parent = edge.Parent
		}
	}
	if mapper == nil {
		return s, t, nil
	}
	if mapper.Source == "installed_software" {
		return mapSoftware(s, t, mapper, parent, found)
	}
	ports := map[int]bool{}
	for _, f := range found {
		if f.Step != parent || f.Type != "open_port" {
			continue
		}
		v := strings.TrimPrefix(strings.ToLower(f.Value), "tcp/")
		n, e := strconv.Atoi(v)
		if e == nil && n > 0 && n <= 65535 {
			ports[n] = true
		}
	}
	ordered := []int{}
	for n := range ports {
		ordered = append(ordered, n)
	}
	sort.Ints(ordered)
	if len(ordered) == 0 {
		return s, t, fmt.Errorf("parent returned no usable open ports")
	}
	if len(ordered) > 50 {
		return s, t, fmt.Errorf("mapping exceeds 50 ports")
	}
	if mapper.Destination == "ports" {
		if contract.Adapter(s.ContractKey) != "nmap" {
			return s, t, fmt.Errorf("ports mapping needs the nmap adapter")
		}
		var params map[string]any
		if e := json.Unmarshal(s.Params, &params); e != nil {
			return s, t, e
		}
		list := []string{}
		for _, n := range ordered {
			list = append(list, strconv.Itoa(n))
		}
		params["profile"] = "custom"
		params["ports"] = strings.Join(list, ",")
		raw, _ := json.Marshal(params)
		normalized, e := contract.Normalize(s.ContractKey, raw)
		if e != nil {
			return s, t, e
		}
		s.Params = normalized
		return s, t, nil
	}
	if mapper.Destination != "target" || contract.Adapter(s.ContractKey) != "nuclei" || mapper.Source != "web_urls" {
		return s, t, fmt.Errorf("target mapping needs web_urls and the nuclei adapter")
	}
	// Satu URL per eksekusi: port web terkecil dipilih deterministik dan ditampilkan dalam pratinjau.
	base := ""
	for _, f := range found {
		if f.Step != parent || f.Type != "open_port" || f.Target == "" || f.Target == "local" {
			continue
		}
		host := contract.HostOf(f.Target)
		if base != "" && base != host {
			return s, t, fmt.Errorf("parent results refer to multiple hosts")
		}
		base = host
	}
	if base != "" {
		allowed := false
		for _, a := range t.Addrs {
			if a.Kind == "ip" || a.Kind == "hostname" || a.Kind == "domain" || a.Kind == "url" {
				if strings.EqualFold(contract.HostOf(a.Value), base) {
					allowed = true
				}
			}
		}
		if !allowed {
			return s, t, fmt.Errorf("parent target is outside the frozen asset scope")
		}
	} else {
		for _, kind := range []string{"ip", "hostname", "domain", "url"} {
			for _, a := range t.Addrs {
				if a.Kind == kind {
					base = contract.HostOf(a.Value)
					break
				}
			}
			if base != "" {
				break
			}
		}
	}
	if base == "" {
		return s, t, fmt.Errorf("asset has no usable host")
	}
	port := 0
	for _, n := range ordered {
		if n == 80 || n == 443 || n == 8000 || n == 8080 || n == 8443 {
			port = n
			break
		}
	}
	if port == 0 {
		return s, t, fmt.Errorf("no web port from the supported mapping profile")
	}
	scheme := "http"
	if port == 443 || port == 8443 {
		scheme = "https"
	}
	u := url.URL{Scheme: scheme, Host: net.JoinHostPort(base, strconv.Itoa(port))}
	t.Addrs = []contract.Address{{Kind: "url", Value: u.String(), IsPrimary: true}}
	return s, t, nil
}
func scoped(execs []chain.Exec, found []chain.Found, asset string) ([]chain.Exec, []chain.Found) {
	xs := []chain.Exec{}
	fs := []chain.Found{}
	for _, x := range execs {
		if x.Asset == asset {
			xs = append(xs, x)
		}
	}
	for _, f := range found {
		if f.Asset == asset {
			fs = append(fs, f)
		}
	}
	return xs, fs
}

// mappedInputs menghasilkan satu masukan per URL web yang teramati (maksimal lima port profil web).
func mappedInputs(s stepRow, t target, edges []chain.Edge, found []chain.Found) ([]struct {
	step   stepRow
	target target
}, error) {
	type input = struct {
		step   stepRow
		target target
	}
	var m *chain.Mapper
	parent := ""
	for _, e := range edges {
		if e.Child == s.Key && e.Mapper != nil {
			m = e.Mapper
			parent = e.Parent
		}
	}
	if m == nil || m.Destination != "target" {
		step, target, err := mapInput(s, t, edges, found)
		if err != nil {
			return nil, err
		}
		return []input{{step, target}}, nil
	}
	out := []input{}
	for _, port := range []int{80, 443, 8000, 8080, 8443} {
		fs := []chain.Found{}
		for _, f := range found {
			if f.Step == parent && f.Type == "open_port" && strings.EqualFold(f.Value, "tcp/"+strconv.Itoa(port)) {
				fs = append(fs, f)
			}
		}
		if len(fs) == 0 {
			continue
		}
		step, target, err := mapInput(s, t, edges, fs)
		if err != nil {
			return nil, err
		}
		out = append(out, input{step, target})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("parent returned no web ports supported by this mapping profile")
	}
	return out, nil
}

// mapSoftware mengubah paket server yang dilaporkan osquery menjadi tag Nuclei lewat tabel beku; nama paket mentah tidak pernah dipakai.
func mapSoftware(s stepRow, t target, m *chain.Mapper, parent string, found []chain.Found) (stepRow, target, error) {
	if m.Destination != "tags" || contract.Adapter(s.ContractKey) != "nuclei" {
		return s, t, fmt.Errorf("software mapping needs tags and the nuclei adapter")
	}
	set := map[string]bool{}
	for _, f := range found {
		if f.Step != parent {
			continue
		}
		if tag, ok := chain.SoftwareTags[chain.InstalledSoftware(f)]; ok {
			set[tag] = true
		}
	}
	if len(set) == 0 {
		return s, t, fmt.Errorf("parent reported no installed software with a supported scan tag")
	}
	tags := []string{}
	for tag := range set {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	var params map[string]any
	if e := json.Unmarshal(s.Params, &params); e != nil {
		return s, t, e
	}
	params["tags"] = tags
	raw, _ := json.Marshal(params)
	normalized, e := contract.Normalize(s.ContractKey, raw)
	if e != nil {
		return s, t, e
	}
	s.Params = normalized
	return s, t, nil
}
