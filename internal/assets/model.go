// Package assets: aset yang dapat menjadi sasaran pengujian.
package assets

import (
	"encoding/json"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"orchis-go/internal/domain"
	"orchis-go/internal/httpx"
)

var (
	Categories    = domain.Categories
	platforms     = domain.Platforms
	arches        = domain.Arches
	criticalities = domain.Criticalities
	addrKinds     = []string{"ip", "cidr", "hostname", "domain", "url", "mac"}
	hostnameRe    = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)*$`)
	macRe         = regexp.MustCompile(`^([0-9a-f]{2}:){5}[0-9a-f]{2}$`)
)

const (
	maxAddresses  = 20
	maxAttrBytes  = 16 << 10
	maxNameLength = 200
)

type Address struct {
	ScanApproved bool   `json:"scan_approved"`
	Kind         string `json:"kind"`
	Value        string `json:"value"`
	IsPrimary    bool   `json:"is_primary"`
}

type TagRef struct {
	ID    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Color string    `json:"color"`
}

type GroupRef struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	IsSystem bool      `json:"is_system"`
}

type CreateInput struct {
	OrganizationID    *uuid.UUID      `json:"organization_id"`
	Category          string          `json:"category"`
	Name              string          `json:"name"`
	Description       *string         `json:"description"`
	Platform          *string         `json:"platform"`
	Arch              *string         `json:"arch"`
	Criticality       string          `json:"criticality"`
	IsInternetFacing  bool            `json:"is_internet_facing"`
	TestingAuthorized bool            `json:"testing_authorized"`
	Attributes        json.RawMessage `json:"attributes"`
	Addresses         []Address       `json:"addresses"`
	TagIDs            []uuid.UUID     `json:"tag_ids"`
}

type Asset struct {
	OrganizationID    *uuid.UUID      `json:"organization_id"`
	OrganizationName  *string         `json:"organization_name"`
	ID                uuid.UUID       `json:"id"`
	Category          string          `json:"category"`
	Name              string          `json:"name"`
	Description       *string         `json:"description"`
	Platform          *string         `json:"platform"`
	Arch              *string         `json:"arch"`
	Criticality       string          `json:"criticality"`
	IsInternetFacing  bool            `json:"is_internet_facing"`
	TestingAuthorized bool            `json:"testing_authorized"`
	Source            string          `json:"source"`
	AgentState        string          `json:"agent_state"` // online | offline | none (agentless)
	Attributes        json.RawMessage `json:"attributes"`
	Addresses         []Address       `json:"addresses"`
	Tags              []TagRef        `json:"tags"`
	Groups            []GroupRef      `json:"groups,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
	Version           int             `json:"version"`
}

func oneOf(v string, set []string) bool { return domain.OneOf(v, set) }

// NormalizeAddress mengembalikan bentuk kanonik, atau pesan kesalahan bila tidak valid.
func NormalizeAddress(a Address) (Address, string) {
	v := strings.TrimSpace(a.Value)
	switch a.Kind {
	case "ip":
		ip, err := netip.ParseAddr(v)
		if err != nil {
			return a, "Invalid IP address."
		}
		a.Value = ip.Unmap().String()
	case "cidr":
		p, err := netip.ParsePrefix(v)
		if err != nil {
			return a, "Invalid CIDR."
		}
		a.Value = p.Masked().String()
	case "hostname", "domain":
		v = strings.ToLower(strings.TrimSuffix(v, "."))
		if len(v) > 253 || !hostnameRe.MatchString(v) {
			return a, "Invalid hostname."
		}
		a.Value = v
	case "url":
		u, err := url.Parse(v)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return a, "URL must start with http:// or https://."
		}
		u.Host = strings.ToLower(u.Host)
		a.Value = u.String()
	case "mac":
		v = strings.ToLower(strings.ReplaceAll(v, "-", ":"))
		if !macRe.MatchString(v) {
			return a, "Invalid MAC address."
		}
		a.Value = v
	default:
		return a, "Unknown address kind."
	}
	return a, ""
}

// PatchInput: kolom yang tidak dikirim tidak diubah. Version wajib (kontrol konkurensi optimistis).
type PatchInput struct {
	Version           *int            `json:"version"`
	Name              *string         `json:"name"`
	Description       *string         `json:"description"`
	Platform          *string         `json:"platform"`
	Arch              *string         `json:"arch"`
	Criticality       *string         `json:"criticality"`
	IsInternetFacing  *bool           `json:"is_internet_facing"`
	TestingAuthorized *bool           `json:"testing_authorized"`
	Attributes        json.RawMessage `json:"attributes"`
	Addresses         *[]Address      `json:"addresses"`
	TagIDs            *[]uuid.UUID    `json:"tag_ids"`
	ClearPlatform     bool            `json:"clear_platform"`
	ClearArch         bool            `json:"clear_arch"`
}

// Validate memeriksa dan menormalkan input. Mengembalikan *httpx.Problem bila ada kolom yang salah.
func (in *CreateInput) Validate() error {
	var fe []httpx.FieldError
	add := func(n, m string) { fe = append(fe, httpx.FieldError{Name: n, Message: m}) }

	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > maxNameLength {
		add("name", "Name is required (max 200 characters).")
	}
	if !oneOf(in.Category, Categories) {
		add("category", "Unknown category.")
	}
	if in.Criticality == "" {
		in.Criticality = "medium"
	}
	if !oneOf(in.Criticality, criticalities) {
		add("criticality", "Must be low, medium, high or critical.")
	}
	if in.Platform != nil && !oneOf(*in.Platform, platforms) {
		add("platform", "Unknown platform.")
	}
	if in.Arch != nil && !oneOf(*in.Arch, arches) {
		add("arch", "Unknown architecture.")
	}
	if len(in.Attributes) == 0 || string(in.Attributes) == "null" {
		in.Attributes = json.RawMessage(`{}`)
	}
	if len(in.Attributes) > maxAttrBytes {
		add("attributes", "Attributes are too large (max 16 KB).")
	} else {
		var obj map[string]any
		if err := json.Unmarshal(in.Attributes, &obj); err != nil {
			add("attributes", "Attributes must be a JSON object.")
		}
	}
	if len(in.Addresses) > maxAddresses {
		add("addresses", "At most 20 addresses.")
	}
	seen := map[string]bool{}
	for i := range in.Addresses {
		if !oneOf(in.Addresses[i].Kind, addrKinds) {
			add("addresses", "Unknown address kind.")
			continue
		}
		na, msg := NormalizeAddress(in.Addresses[i])
		if msg != "" {
			add("addresses", msg)
			continue
		}
		key := na.Kind + "|" + na.Value
		if seen[key] {
			add("addresses", "Duplicate address: "+na.Value)
			continue
		}
		seen[key] = true
		in.Addresses[i] = na
	}
	if len(fe) > 0 {
		return httpx.Validation(fe...)
	}
	return nil
}

// Validate untuk PatchInput: hanya kolom yang dikirim yang diperiksa.
func (in *PatchInput) Validate() error {
	var fe []httpx.FieldError
	add := func(n, m string) { fe = append(fe, httpx.FieldError{Name: n, Message: m}) }
	if in.Version == nil {
		add("version", "The version you are editing is required.")
	}
	if in.Name != nil {
		n := strings.TrimSpace(*in.Name)
		in.Name = &n
		if n == "" || len(n) > maxNameLength {
			add("name", "Name is required (max 200 characters).")
		}
	}
	if in.Criticality != nil && !oneOf(*in.Criticality, criticalities) {
		add("criticality", "Must be low, medium, high or critical.")
	}
	if in.Platform != nil && !oneOf(*in.Platform, platforms) {
		add("platform", "Unknown platform.")
	}
	if in.Arch != nil && !oneOf(*in.Arch, arches) {
		add("arch", "Unknown architecture.")
	}
	if len(in.Attributes) > maxAttrBytes {
		add("attributes", "Attributes are too large (max 16 KB).")
	} else if len(in.Attributes) > 0 {
		var obj map[string]any
		if json.Unmarshal(in.Attributes, &obj) != nil {
			add("attributes", "Attributes must be a JSON object.")
		}
	}
	if in.Addresses != nil {
		if len(*in.Addresses) > maxAddresses {
			add("addresses", "At most 20 addresses.")
		}
		seen := map[string]bool{}
		for i := range *in.Addresses {
			a := &(*in.Addresses)[i]
			if !oneOf(a.Kind, addrKinds) {
				add("addresses", "Unknown address kind.")
				continue
			}
			na, msg := NormalizeAddress(*a)
			if msg != "" {
				add("addresses", msg)
				continue
			}
			if k := na.Kind + "|" + na.Value; seen[k] {
				add("addresses", "Duplicate address: "+na.Value)
				continue
			} else {
				seen[k] = true
			}
			*a = na
		}
	}
	if in.TagIDs != nil && len(*in.TagIDs) > 20 {
		add("tag_ids", "At most 20 tags.")
	}
	if len(fe) > 0 {
		return httpx.Validation(fe...)
	}
	return nil
}
