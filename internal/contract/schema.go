package contract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
)

// Skema sengaja terbatas: tidak ada referensi, kode, atau ekspresi reguler dari pengguna.
type Property struct {
	ErrorMessage string   `json:"error_message,omitempty"`
	Type         string   `json:"type"`
	Label        string   `json:"label"`
	Help         string   `json:"help,omitempty"`
	Enum         []string `json:"enum,omitempty"`
	EnumLabels   []string `json:"enum_labels,omitempty"`
	Pattern      string   `json:"pattern,omitempty"`
	ItemsEnum    []string `json:"items_enum,omitempty"`
	ItemsPattern string   `json:"items_pattern,omitempty"`
	MinItems     int      `json:"min_items,omitempty"`
	MaxItems     int      `json:"max_items,omitempty"`
	MaxLength    int      `json:"max_length,omitempty"`
	Min          *float64 `json:"minimum,omitempty"`
	Max          *float64 `json:"maximum,omitempty"`
	Default      any      `json:"default,omitempty"`
	ShowWhen     *struct {
		Field string `json:"field"`
		Value string `json:"value"`
	} `json:"show_when,omitempty"`
}
type Schema struct {
	Type                 string              `json:"type"`
	AdditionalProperties bool                `json:"additionalProperties"`
	Properties           map[string]Property `json:"properties"`
	Order                []string            `json:"order"`
}

func contains(values []string, s string) bool {
	for _, v := range values {
		if v == s {
			return true
		}
	}
	return false
}
func (d Def) ValidateDefinition() error {
	if d.Key == "" || d.Version < 1 {
		return fmt.Errorf("invalid action identity")
	}
	if !contains([]string{"hackbot", "nmap", "nuclei", "agent.listening_ports", "agent.system_info", AdapterOsquery}, d.Adapter) {
		return fmt.Errorf("adapter not allowed")
	}
	if d.Schema.Type != "object" || d.Schema.AdditionalProperties {
		return fmt.Errorf("only closed object schemas are supported")
	}
	for key, p := range d.Schema.Properties {
		if !contains([]string{"string", "boolean", "integer", "array"}, p.Type) {
			return fmt.Errorf("unsupported type for %s", key)
		}
		for _, pattern := range []string{p.Pattern, p.ItemsPattern} {
			if pattern != "" {
				if _, err := regexp.Compile(pattern); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
func (s Schema) Normalize(raw json.RawMessage) (json.RawMessage, error) {
	fail := func(k, msg string) (json.RawMessage, error) {
		return nil, &ParamError{Field: "params." + k, Message: msg}
	}
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	var obj map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	if err := dec.Decode(&obj); err != nil || obj == nil {
		return fail("", "Parameters must be an object.")
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return fail("", "Only one parameter object is allowed.")
	}
	for key := range obj {
		if _, ok := s.Properties[key]; !ok {
			return fail(key, "Unrecognised parameter.")
		}
	}
	for key, p := range s.Properties {
		if _, ok := obj[key]; !ok && p.Default != nil {
			data, _ := json.Marshal(p.Default)
			var value any
			_ = json.Unmarshal(data, &value)
			obj[key] = value
		}
	}
	for key, p := range s.Properties {
		value, ok := obj[key]
		if !ok {
			continue
		}
		switch p.Type {
		case "string":
			v, ok := value.(string)
			if !ok {
				return fail(key, "Must be a string.")
			}
			if len(p.Enum) > 0 && !contains(p.Enum, v) {
				return fail(key, "Choose an allowed value.")
			}
			if p.MaxLength > 0 && len(v) > p.MaxLength {
				return fail(key, "Value is too long.")
			}
			if p.Pattern != "" && !regexp.MustCompile(p.Pattern).MatchString(v) {
				if p.ErrorMessage != "" {
					return fail(key, p.ErrorMessage)
				}
				return fail(key, "Value does not match the required format.")
			}
		case "boolean":
			if _, ok := value.(bool); !ok {
				return fail(key, "Must be a boolean.")
			}
		case "integer":
			v, ok := value.(float64)
			if !ok || v != float64(int64(v)) {
				return fail(key, "Must be an integer.")
			}
			if p.Min != nil && v < *p.Min || p.Max != nil && v > *p.Max {
				return fail(key, "Number is outside the allowed range.")
			}
		case "array":
			list, ok := value.([]any)
			if !ok {
				return fail(key, "Must be an array.")
			}
			if len(list) < p.MinItems || p.MaxItems > 0 && len(list) > p.MaxItems {
				return fail(key, "Choose an allowed number of items.")
			}
			seen := map[string]bool{}
			out := []string{}
			for _, item := range list {
				v, ok := item.(string)
				if !ok {
					return fail(key, "Items must be strings.")
				}
				if len(p.ItemsEnum) > 0 && !contains(p.ItemsEnum, v) {
					return fail(key, "Choose an allowed item.")
				}
				if p.ItemsPattern != "" && !regexp.MustCompile(p.ItemsPattern).MatchString(v) {
					return fail(key, "Item does not match the required format.")
				}
				if !seen[v] {
					seen[v] = true
					out = append(out, v)
				}
			}
			sort.Strings(out)
			obj[key] = out
		}
	}
	return json.Marshal(obj)
}
func Adapter(key string) string {
	if d := Find(key); d != nil {
		return d.Adapter
	}
	return ""
}
