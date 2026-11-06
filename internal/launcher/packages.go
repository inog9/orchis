package launcher

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Optional packages are discovered passively: only manifests and entry-file
// metadata are inspected. Pi remains responsible for loading enabled packages.
type optionalPackages struct {
	Subagents, BillionContext, Todo, AskUserQuestion string
}

func installedPackages(agentDir string) optionalPackages {
	subagents := ""
	if override := os.Getenv("PENTEST_HARNES_SUBAGENTS_DIR"); override != "" {
		// An explicit fixture root is authoritative, including when incomplete.
		subagents = packageRoot(override, "pi-subagents", "bundle/index.js")
	} else {
		subagents = findPackage(agentDir, "pi-subagents", "bundle/index.js")
	}
	return optionalPackages{
		Subagents:       subagents,
		BillionContext:  findPackage(agentDir, "billion-context", "dist/agent/pi-native.js"),
		Todo:            findPackage(agentDir, "@xynogen/pix-todo", "src/index.ts"),
		AskUserQuestion: findPackage(agentDir, "@juicesharp/rpiv-ask-user-question", "index.ts"),
	}
}

func findPackage(agentDir, name, entry string) string {
	for _, base := range []string{"npm/node_modules", "node_modules", "extensions"} {
		if root := packageRoot(filepath.Join(agentDir, filepath.FromSlash(base), filepath.FromSlash(name)), name, entry); root != "" {
			return root
		}
	}
	return ""
}

func packageRoot(directory, name, entry string) string {
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return ""
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(canonical, "package.json"))
	if err != nil {
		return ""
	}
	var manifest struct{ Name string }
	if json.Unmarshal(data, &manifest) != nil || manifest.Name != name {
		return ""
	}
	info, err := os.Stat(filepath.Join(canonical, filepath.FromSlash(entry)))
	if err != nil || !info.Mode().IsRegular() {
		return ""
	}
	return canonical
}

func writeLeafProfile(workspace string, runtime fs.FS) error {
	body, err := fs.ReadFile(runtime, "SPECIALIST.md")
	if errors.Is(err, fs.ErrNotExist) {
		body = []byte("Execute only the parent's bounded specialist contract with the full available tools. Read its assigned skill, respect scope and task bounds, keep secrets private, and return evidence, coverage, and blockers. Do not delegate further or edit central ledgers.\n")
	} else if err != nil {
		return err
	}
	// No tools allowlist: retain Pi's normal tools and ambient package tools.
	// Omitted extensions preserves ambient discovery for background children.
	// SDK children supply codemode; the mandatory harness guard installs the
	// official native MCP/tool-search factories after admission.
	const header = `---
name: pentest-harness-leaf
description: Constrained harness worker
async: true
defaultContext: fresh
allowNestedSubagents: false
inheritProjectContext: false
inheritSkills: true
---

`
	path := filepath.Join(workspace, ".pi", "agents", "pentest-harness-leaf.md")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, append([]byte(header), body...), 0600)
}

// billionConfig writes a private, session-local override, never the user's
// config or any credential store. Unknown/custom fields are retained verbatim
// as JSON values, including precise numbers and provider/model configuration.
func billionConfig(sessionRoot, home string) (string, error) {
	source := os.Getenv("BILI_CONFIG_FILE")
	if source == "" {
		base := os.Getenv("XDG_CONFIG_HOME")
		if base == "" {
			base = filepath.Join(home, ".config")
		}
		source = filepath.Join(base, "billion-context", "billion-context.json")
	}
	config := map[string]json.RawMessage{}
	if data, err := os.ReadFile(source); err == nil {
		if json.Unmarshal(data, &config) != nil || config == nil {
			return "", errors.New("invalid billion-context configuration: expected a JSON object")
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("read billion-context configuration: %w", err)
	}
	for _, override := range []struct {
		path         []string
		value        json.RawMessage
		onlyIfAbsent bool
	}{
		{[]string{"pi", "subagents"}, json.RawMessage(`false`), false},
		{[]string{"guardrails", "bashTimeout"}, json.RawMessage(`0`), false},
		{[]string{"cacheKeepalive", "enabled"}, json.RawMessage(`false`), true},
		{[]string{"compress", "externalSummary", "enabled"}, json.RawMessage(`false`), true},
	} {
		if err := setConfigField(config, override.path, override.value, override.onlyIfAbsent); err != nil {
			return "", err
		}
	}
	destination := filepath.Join(sessionRoot, "private", "billion-context.json")
	if err := privateJSON(destination, config); err != nil {
		return "", err
	}
	return destination, nil
}

func setConfigField(config map[string]json.RawMessage, path []string, value json.RawMessage, onlyIfAbsent bool) error {
	key := path[0]
	if len(path) == 1 {
		if _, exists := config[key]; !onlyIfAbsent || !exists {
			config[key] = value
		}
		return nil
	}
	child := map[string]json.RawMessage{}
	if raw, exists := config[key]; exists {
		if json.Unmarshal(raw, &child) != nil || child == nil {
			return fmt.Errorf("invalid billion-context configuration: %s must be an object", key)
		}
	}
	if err := setConfigField(child, path[1:], value, onlyIfAbsent); err != nil {
		return err
	}
	encoded, err := json.Marshal(child)
	if err != nil {
		return err
	}
	config[key] = encoded
	return nil
}
