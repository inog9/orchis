package defensebot

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// component is one piece of installed software that is sent to the vulnerability service.
type component struct {
	Pair     Pair
	Display  string   // name shown to people (binary package name when there is exactly one)
	Binaries []string // binary packages built from the source package
	Kernel   bool
}

type cpeComp struct{ Vendor, Product, Name, Version string }

type plan struct {
	CPE         []cpeComp
	Ecosystem   string
	Platform    string
	OSName      string
	OSVersion   string
	Components  []*component
	Notes       []Note
	Binary2Src  map[string]string // binary package -> source package
	Unmapped    int
	NewestKern  string // version of the newest installed kernel image package
	RunningABI  string
	KernelCount int // inactive kernel packages skipped
}

var kernelPkgRe = regexp.MustCompile(`^linux-(?:image|image-unsigned|modules|modules-extra|headers|tools|cloud-tools|buildinfo|lib-rust)-(\d+\.\d+\.\d+-\d+)(?:-(.+))?$`)
var runningKernelRe = regexp.MustCompile(`^(\d+\.\d+\.\d+-\d+)(?:-(.+))?$`)

// ecosystemFor maps an osquery os_version row to the OSV ecosystem name. It returns "" for distributions that are not
// supported yet; those are reported as a coverage limitation rather than guessed.
func ecosystemFor(row map[string]string) string {
	platform := strings.ToLower(row["platform"])
	switch platform {
	case "ubuntu":
		major, e1 := strconv.Atoi(row["major"])
		minor, e2 := strconv.Atoi(row["minor"])
		if e1 != nil || e2 != nil {
			return ""
		}
		v := fmt.Sprintf("Ubuntu:%d.%02d", major, minor)
		if strings.Contains(strings.ToUpper(row["version"]), "LTS") {
			v += ":LTS"
		}
		return v
	case "debian":
		if _, err := strconv.Atoi(row["major"]); err != nil {
			return ""
		}
		return "Debian:" + row["major"]
	}
	return ""
}

// systemSections are Debian sections of libraries, kernel and development material. orchis-defense-agent reports on applications and
// services someone installed or runs, not on the operating system's building blocks.
var systemSections = map[string]bool{"libs": true, "libdevel": true, "oldlibs": true, "kernel": true, "python": true, "perl": true, "ruby": true,
	"golang": true, "rust": true, "javascript": true, "doc": true, "fonts": true, "localization": true, "metapackages": true, "debug": true,
	"haskell": true, "ocaml": true, "lisp": true, "gnustep": true, "introspection": true, "interpreters": true}

var systemNameRe = regexp.MustCompile(`^(lib[a-z0-9]|linux-|python3-|perl-|golang-|ruby-|node-|fonts-|gir1\.2-|firmware-|.*-dev$|.*-dbg$|.*-doc$|.*-data$|.*-common$)`)

// isApplication tells application packages from system building blocks. Sections win; names are the fallback for agents
// that did not send a section.
func isApplication(name, section string) bool {
	sec := strings.ToLower(strings.TrimSpace(section))
	if i := strings.LastIndex(sec, "/"); i >= 0 {
		sec = sec[i+1:]
	}
	if systemSections[sec] {
		return false
	}
	if sec == "" || sec == "misc" || sec == "utils" || sec == "admin" {
		return !systemNameRe.MatchString(name)
	}
	return !strings.HasPrefix(name, "linux-") && !strings.HasPrefix(name, "lib") || cpeKnown(name)
}

func cpeKnown(name string) bool { _, _, ok := cpeFor(name, ""); return ok }

func okRows(in Input, key string) ([]map[string]string, DatasetInput, bool) {
	d, ok := in.Datasets[key]
	if !ok || d.Status != "ok" {
		return nil, d, false
	}
	return d.Rows, d, true
}

// buildPlan turns the stored datasets into the list of components to look up and the coverage notes.
func buildPlan(in Input, aiCPE map[string]cpeComp) *plan {
	p := &plan{Binary2Src: map[string]string{}}
	osRows, _, haveOS := okRows(in, QueryOSRelease)
	if haveOS && len(osRows) > 0 {
		p.Platform = strings.ToLower(osRows[0]["platform"])
		p.OSName = osRows[0]["name"]
		p.OSVersion = osRows[0]["version"]
		p.Ecosystem = ecosystemFor(osRows[0])
	} else if !haveOS {
		// os_release missing: infer the platform from the software inventory that was collected,
		// so platform-specific requirements (deb/rpm packages on macOS) are not flagged as gaps.
		if _, _, ok := okRows(in, QueryApps); ok {
			p.Platform = "darwin"
		} else if _, _, ok := okRows(in, QueryBrew); ok {
			p.Platform = "darwin"
		}
	}
	if kr, _, ok := okRows(in, QueryKernel); ok && len(kr) > 0 {
		if m := runningKernelRe.FindStringSubmatch(strings.TrimSpace(kr[0]["version"])); m != nil {
			p.RunningABI = m[1]
		}
	}
	byKey := map[Pair]*component{}
	add := func(eco, name, display, version, binary string, kernel bool) {
		name, version = strings.TrimSpace(name), strings.TrimSpace(version)
		if name == "" || version == "" || eco == "" {
			return
		}
		k := Pair{Ecosystem: eco, Name: name, Version: version}
		c := byKey[k]
		if c == nil {
			c = &component{Pair: k, Display: display}
			byKey[k] = c
			p.Components = append(p.Components, c)
		}
		if binary != "" {
			c.Binaries = append(c.Binaries, binary)
		}
		if kernel {
			c.Kernel = true
		}
	}

	if rows, d, ok := okRows(in, QueryPackages); ok {
		if d.Truncated {
			p.Notes = append(p.Notes, Note{Code: "packages_truncated", Count: len(rows), Limiting: true})
		}
		rpm, inactive, system := 0, 0, 0
		for _, r := range rows {
			name, version := r["name"], r["version"]
			typ := strings.ToLower(r["pkg_type"])
			source := strings.TrimSpace(r["source_name"])
			if typ == "" { // older agents sent a literal "deb"/"rpm" label in the source column
				typ = strings.ToLower(r["source"])
			}
			if typ == "rpm" {
				rpm++
				continue
			}
			if p.Ecosystem == "" {
				p.Unmapped++
				continue
			}
			if source == "" || source == "deb" {
				source = name
			}
			p.Binary2Src[name] = source
			if !isApplication(name, r["section"]) {
				system++
				continue
			}
			if v, pr, ok := cpeFor(name, ""); ok {
				p.CPE = append(p.CPE, cpeComp{Vendor: v, Product: pr, Name: name, Version: upstreamVersion(version)})
			}
			kernel := false
			if m := kernelPkgRe.FindStringSubmatch(name); m != nil {
				kernel = true
				if p.RunningABI != "" && m[1] != p.RunningABI {
					inactive++
					if strings.HasPrefix(name, "linux-image-") && (p.NewestKern == "" || CompareVersions(p.Ecosystem, version, p.NewestKern) > 0) {
						p.NewestKern = version
					}
					continue
				}
				if strings.HasPrefix(name, "linux-image-") && (p.NewestKern == "" || CompareVersions(p.Ecosystem, version, p.NewestKern) > 0) {
					p.NewestKern = version
				}
			}
			add(p.Ecosystem, source, name, version, name, kernel)
		}
		if rpm > 0 {
			p.Unmapped += rpm
			p.Notes = append(p.Notes, Note{Code: "rpm_unsupported", Count: rpm, Limiting: true})
		}
		if system > 0 {
			p.Notes = append(p.Notes, Note{Code: "system_packages_skipped", Count: system})
		}
		if inactive > 0 {
			p.KernelCount = inactive
			p.Notes = append(p.Notes, Note{Code: "old_kernels_skipped", Count: inactive})
		}
		if p.Ecosystem == "" && len(rows) > 0 && rpm < len(rows) {
			p.Notes = append(p.Notes, Note{Code: "ecosystem_unsupported", Detail: strings.TrimSpace(p.OSName + " " + p.OSVersion), Limiting: true})
		}
	}
	if rows, _, ok := okRows(in, QueryNPM); ok {
		for _, r := range rows {
			add("npm", r["name"], r["name"], r["version"], "", false)
		}
	}
	unmapped := 0
	if rows, _, ok := okRows(in, QueryApps); ok {
		for _, r := range rows {
			if v, pr, mapped := cpeFor(r["name"], r["bundle_identifier"]); mapped {
				p.CPE = append(p.CPE, cpeComp{Vendor: v, Product: pr, Name: r["name"], Version: strings.TrimSpace(r["version"])})
				continue
			}
			if c, ok := aiCPE[AppKey(r["name"])]; ok {
				p.CPE = append(p.CPE, c)
				continue
			}
			unmapped++
		}
	}
	if rows, _, ok := okRows(in, QueryBrew); ok {
		unmapped += len(rows)
	}
	if unmapped > 0 {
		p.Unmapped += unmapped
		p.Notes = append(p.Notes, Note{Code: "apps_unmapped", Count: unmapped, Limiting: true})
	}
	for _, c := range p.Components {
		sort.Strings(c.Binaries)
		if len(c.Binaries) == 1 {
			c.Display = c.Binaries[0]
		} else if len(c.Binaries) > 1 {
			c.Display = c.Pair.Name
		}
	}
	sort.Slice(p.Components, func(i, j int) bool {
		a, b := p.Components[i].Pair, p.Components[j].Pair
		if a.Ecosystem != b.Ecosystem {
			return a.Ecosystem < b.Ecosystem
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.Version < b.Version
	})
	return p
}
