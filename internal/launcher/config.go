package launcher

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ReadJSONC supports OpenCode's comments and trailing commas without changing
// quoted strings (including URLs, escaped quotes, and credential contents).
func ReadJSONC(data []byte, value any) error {
	clean := append([]byte(nil), data...)
	inString, escaped := false, false
	for i := 0; i < len(clean); i++ {
		c := clean[i]
		if inString {
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == '"' {
				inString = false
			}
			continue
		}
		if c == '"' {
			inString = true
			continue
		}
		if c != '/' || i+1 >= len(clean) {
			continue
		}
		if clean[i+1] == '/' {
			for ; i < len(clean) && clean[i] != '\n'; i++ {
				clean[i] = ' '
			}
		} else if clean[i+1] == '*' {
			clean[i], clean[i+1] = ' ', ' '
			i += 2
			for ; i+1 < len(clean) && !(clean[i] == '*' && clean[i+1] == '/'); i++ {
				clean[i] = ' '
			}
			if i+1 >= len(clean) {
				return errors.New("unterminated JSONC comment")
			}
			clean[i], clean[i+1] = ' ', ' '
			i++
		}
	}
	inString, escaped = false, false
	for i, c := range clean {
		if inString {
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == '"' {
				inString = false
			}
			continue
		}
		if c == '"' {
			inString = true
		} else if c == ',' {
			j := i + 1
			for j < len(clean) && bytes.ContainsRune([]byte(" \r\n\t"), rune(clean[j])) {
				j++
			}
			if j < len(clean) && (clean[j] == '}' || clean[j] == ']') {
				clean[i] = ' '
			}
		}
	}
	return json.Unmarshal(clean, value)
}

func configPaths(explicit, home string) []string {
	if explicit != "" {
		return []string{explicit}
	}
	if path := os.Getenv("OPENCODE_CONFIG"); path != "" {
		return []string{path}
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		base = filepath.Join(home, ".config")
	}
	return []string{filepath.Join(base, "opencode", "opencode.jsonc"), filepath.Join(base, "opencode", "opencode.json")}
}

// MCPConfig converts only enabled servers and the explicitly requested
// Burp/Caido/Textbee integrations. It never writes back to OpenCode or Pi config.
// Returned data may contain credentials and must only be stored privately.
func MCPConfig(explicit, home, agentDir string) (map[string]any, []string, error) {
	servers := map[string]any{}
	var existing struct {
		Servers            map[string]json.RawMessage `json:"mcpServers"`
		AutoEnableCodemode *bool                      `json:"autoEnableCodemode"`
	}
	if data, err := os.ReadFile(filepath.Join(agentDir, "mcp.json")); err == nil {
		if err := ReadJSONC(data, &existing); err != nil {
			return nil, nil, fmt.Errorf("invalid Pi MCP configuration: %w", err)
		}
		for name, raw := range existing.Servers {
			var server map[string]any
			if err := json.Unmarshal(raw, &server); err != nil || server == nil {
				return nil, nil, fmt.Errorf("MCP %s: expected a server object", name)
			}
			native, err := nativeMCPServer(server)
			if err != nil {
				return nil, nil, fmt.Errorf("MCP %s: %w", name, err)
			}
			// Native wildcard exposure rules are first-match-wins. Preserve the
			// original JSON key order instead of reordering them through a Go map.
			if server["toolExposure"] != nil && server["directTools"] == nil && server["excludeTools"] == nil {
				var fields map[string]json.RawMessage
				if err := json.Unmarshal(raw, &fields); err != nil {
					return nil, nil, fmt.Errorf("MCP %s: invalid server object", name)
				}
				native["toolExposure"] = fields["toolExposure"]
			}
			servers[name] = native
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, nil, err
	}
	var imported []string
	for _, path := range configPaths(explicit, home) {
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) && explicit == "" && os.Getenv("OPENCODE_CONFIG") == "" {
			continue
		}
		if err != nil {
			return nil, nil, fmt.Errorf("read OpenCode configuration: %w", err)
		}
		var config struct {
			MCP map[string]struct {
				Type        string            `json:"type"`
				Command     []string          `json:"command"`
				URL         string            `json:"url"`
				Environment map[string]string `json:"environment"`
				Headers     map[string]string `json:"headers"`
				Enabled     *bool             `json:"enabled"`
				OAuth       json.RawMessage   `json:"oauth"`
			} `json:"mcp"`
		}
		if err := ReadJSONC(data, &config); err != nil {
			return nil, nil, fmt.Errorf("invalid OpenCode configuration: %w", err)
		}
		for name, server := range config.MCP {
			lower := strings.ToLower(name)
			required := strings.Contains(lower, "burp") || strings.Contains(lower, "caido") || strings.Contains(lower, "textbee")
			if server.Enabled != nil && !*server.Enabled && !required {
				continue
			}
			entry := map[string]any{"exposure": "codemode"}
			if server.Type == "local" && len(server.Command) > 0 {
				entry["command"] = server.Command[0]
				entry["args"] = server.Command[1:]
				entry["cwd"] = filepath.Dir(path)
				entry["env"] = server.Environment
			} else if server.Type == "remote" && server.URL != "" {
				entry["url"] = server.URL
				entry["headers"] = server.Headers
				if len(server.OAuth) > 0 && string(server.OAuth) != "false" && string(server.OAuth) != "null" {
					if string(server.OAuth) != "true" {
						var oauth map[string]any
						if err := json.Unmarshal(server.OAuth, &oauth); err != nil {
							return nil, nil, fmt.Errorf("invalid OAuth configuration for %s", name)
						}
						entry["oauth"] = oauth
					}
				}
			} else {
				return nil, nil, fmt.Errorf("unsupported MCP configuration for %s", name)
			}
			resolved, err := resolveReferences(entry, filepath.Dir(path))
			if err != nil {
				return nil, nil, fmt.Errorf("MCP %s: %w", name, err)
			}
			servers[name] = resolved
			imported = append(imported, name)
		}
		break
	}
	result := map[string]any{"mcpServers": servers}
	if existing.AutoEnableCodemode != nil {
		result["autoEnableCodemode"] = *existing.AutoEnableCodemode
	}
	sort.Strings(imported)
	return result, imported, nil
}

// Keep native fields and translate common legacy extension exposure/auth options.
// Native Pi supplies stable mcp__server__tool names and connects enabled servers.
func nativeMCPServer(source map[string]any) (map[string]any, error) {
	if source["type"] == "sse" {
		return nil, errors.New("native MCP requires streamable HTTP or a stdio proxy for legacy SSE")
	}
	result := map[string]any{}
	for _, key := range []string{"type", "command", "args", "cwd", "env", "url", "headers", "enabled", "timeout", "description", "exposure", "toolExposure", "oauth"} {
		if value, ok := source[key]; ok {
			result[key] = value
		}
	}
	if _, ok := result["exposure"]; !ok {
		result["exposure"] = "codemode"
		if direct, ok := source["directTools"].(bool); ok && direct {
			result["exposure"] = "direct"
		}
	}
	overrides := map[string]any{}
	if previous, ok := result["toolExposure"].(map[string]any); ok {
		for name, value := range previous {
			overrides[name] = value
		}
	}
	if direct, ok := source["directTools"].([]any); ok {
		for _, name := range direct {
			if name, ok := name.(string); ok {
				overrides[name] = "direct"
			}
		}
	}
	if excluded, ok := source["excludeTools"].([]any); ok {
		for _, name := range excluded {
			if name, ok := name.(string); ok {
				overrides[name] = "hidden"
			}
		}
	}
	if len(overrides) > 0 {
		result["toolExposure"] = overrides
	}
	// Native provider credentials can only be selected in the global config.
	// A project-only preference override lets Pi retain that authenticated entry.
	if auth, ok := source["auth"].(map[string]any); ok && auth["provider"] != nil {
		preferences := map[string]any{}
		for _, key := range []string{"enabled", "exposure", "toolExposure"} {
			if value, ok := result[key]; ok {
				preferences[key] = value
			}
		}
		return preferences, nil
	}
	var bearer string
	if token, ok := source["bearerToken"].(string); ok && token != "" {
		bearer = token
	} else if name, ok := source["bearerTokenEnv"].(string); ok && name != "" {
		bearer = "${" + name + "}"
	}
	if bearer != "" {
		headers := map[string]any{}
		if previous, ok := source["headers"].(map[string]any); ok {
			for name, value := range previous {
				headers[name] = value
			}
		}
		headers["Authorization"] = "Bearer " + bearer
		result["headers"] = headers
	}
	return result, nil
}

func resolveReferences(value any, base string) (any, error) {
	switch v := value.(type) {
	case map[string]any:
		for key, item := range v {
			resolved, err := resolveReferences(item, base)
			if err != nil {
				return nil, err
			}
			v[key] = resolved
		}
	case map[string]string:
		for key, item := range v {
			resolved, err := resolveReferences(item, base)
			if err != nil {
				return nil, err
			}
			v[key] = resolved.(string)
		}
	case []string:
		for i, item := range v {
			resolved, err := resolveReferences(item, base)
			if err != nil {
				return nil, err
			}
			v[i] = resolved.(string)
		}
	case string:
		for _, prefix := range []string{"{env:", "{file:"} {
			for strings.Contains(v, prefix) {
				start := strings.Index(v, prefix)
				end := strings.Index(v[start:], "}")
				if end < 0 {
					return nil, errors.New("unterminated configuration reference")
				}
				end += start
				key := v[start+len(prefix) : end]
				var replacement string
				if prefix == "{env:" {
					var ok bool
					replacement, ok = os.LookupEnv(key)
					if !ok {
						return nil, fmt.Errorf("required environment variable %s is unset", key)
					}
				} else {
					path := key
					if strings.HasPrefix(path, "~/") {
						home, err := os.UserHomeDir()
						if err != nil {
							return nil, err
						}
						path = filepath.Join(home, path[2:])
					} else if !filepath.IsAbs(path) {
						path = filepath.Join(base, path)
					}
					data, err := os.ReadFile(path)
					if err != nil {
						return nil, fmt.Errorf("cannot read credential reference %s", key)
					}
					replacement = strings.TrimSpace(string(data))
				}
				// A replacement is a value, not another template to recursively execute.
				v = v[:start] + replacement + v[end+1:]
				if strings.Contains(replacement, prefix) {
					return nil, errors.New("recursive configuration reference")
				}
			}
		}
		return v, nil
	}
	return value, nil
}
