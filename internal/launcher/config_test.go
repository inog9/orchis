package launcher_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/r1pu5u/pentest-harnes/internal/launcher"
)

func TestOpenCodeMCPConversionPreservesSecretsPrivatelyAndEnabledSelection(t *testing.T) {
	home := isolate(t)
	config := filepath.Join(home, "opencode.jsonc")
	t.Setenv("HARNESS_TEST_TOKEN", "test-secret-not-for-logs")
	data := `{
// OpenCode JSONC
"mcp": {
 "burpsuite-mcp": {"type":"local", "enabled":false, "command":["java","-jar","proxy.jar"], "environment":{"TOKEN":"{env:HARNESS_TEST_TOKEN}"}},
 "caido-mcp": {"type":"remote","enabled":false,"url":"http://127.0.0.1:3333/mcp","headers":{"Authorization":"Bearer {env:HARNESS_TEST_TOKEN}"}},
 "textbee":{"type":"local","command":["npx","-y","@textbee/mcp"],},
 "unrelated":{"type":"local","enabled":false,"command":["do-not-start"]},
},}`
	if err := os.WriteFile(config, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	converted, imported, err := launcher.MCPConfig(config, home, filepath.Join(home, ".pi", "agent"))
	if err != nil {
		t.Fatal(err)
	}
	servers := converted["mcpServers"].(map[string]any)
	if len(servers) != 3 || len(imported) != 3 {
		t.Fatalf("incorrect server selection: %v", imported)
	}
	if _, ok := converted["settings"]; ok {
		t.Fatal("native MCP configuration contains extension-only settings")
	}
	burp := servers["burpsuite-mcp"].(map[string]any)
	if _, ok := burp["lifecycle"]; ok || burp["exposure"] != "codemode" {
		t.Fatal("server was not converted to native MCP exposure")
	}
	if burp["command"] != "java" || burp["env"].(map[string]string)["TOKEN"] != "test-secret-not-for-logs" {
		t.Fatal("command or environment reference conversion failed")
	}
	caido := servers["caido-mcp"].(map[string]any)
	if caido["url"] != "http://127.0.0.1:3333/mcp" || caido["headers"].(map[string]string)["Authorization"] != "Bearer test-secret-not-for-logs" {
		t.Fatal("remote conversion failed")
	}
	got, _ := os.ReadFile(config)
	if string(got) != data {
		t.Fatal("source configuration was changed")
	}
}

func TestNativeMCPOptionsAndLegacyExposureMigration(t *testing.T) {
	home := isolate(t)
	agentDir := filepath.Join(home, ".pi", "agent")
	if err := os.MkdirAll(agentDir, 0700); err != nil {
		t.Fatal(err)
	}
	data := `{"autoEnableCodemode":false,"settings":{"toolPrefix":"short"},"mcpServers":{
	 "native":{"url":"https://mcp.example.test/mcp","exposure":"deferred","timeout":12,"oauth":{"clientId":"fixture"},"toolExposure":{"danger_*":"hidden","*":"codemode"}},
	 "legacy":{"command":"node","args":["fixture.mjs"],"lifecycle":"lazy","directTools":["probe"],"excludeTools":["delete_*"],"idleTimeout":10},
	 "bearer":{"url":"https://mcp.example.test/auth","auth":"bearer","bearerTokenEnv":"FIXTURE_TOKEN"},
	 "provider-auth":{"url":"https://mcp.example.test/provider","auth":{"provider":"fixture-provider"},"exposure":"direct"}
	}}`
	if err := os.WriteFile(filepath.Join(agentDir, "mcp.json"), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	converted, _, err := launcher.MCPConfig("", home, agentDir)
	if err != nil {
		t.Fatal(err)
	}
	if converted["autoEnableCodemode"] != false {
		t.Fatal("native codemode preference lost")
	}
	servers := converted["mcpServers"].(map[string]any)
	native := servers["native"].(map[string]any)
	if native["exposure"] != "deferred" || native["timeout"] != float64(12) || native["oauth"].(map[string]any)["clientId"] != "fixture" {
		t.Fatal("native MCP settings were not preserved")
	}
	rules, err := json.Marshal(native["toolExposure"])
	if err != nil || string(rules) != `{"danger_*":"hidden","*":"codemode"}` {
		t.Fatalf("native wildcard priority changed: %s (%v)", rules, err)
	}
	legacy := servers["legacy"].(map[string]any)
	for _, key := range []string{"lifecycle", "idleTimeout", "directTools", "excludeTools"} {
		if _, ok := legacy[key]; ok {
			t.Fatalf("legacy extension option remains: %s", key)
		}
	}
	exposure := legacy["toolExposure"].(map[string]any)
	if exposure["probe"] != "direct" || exposure["delete_*"] != "hidden" {
		t.Fatal("legacy tool exposure was lost")
	}
	bearer := servers["bearer"].(map[string]any)
	if bearer["headers"].(map[string]any)["Authorization"] != "Bearer ${FIXTURE_TOKEN}" {
		t.Fatal("bearer auth was not converted to a native header")
	}
	providerAuth := servers["provider-auth"].(map[string]any)
	if len(providerAuth) != 1 || providerAuth["exposure"] != "direct" {
		t.Fatal("project copy must retain provider credentials through a global-entry override")
	}
}

func TestJSONCLeavesQuotedCommentAndCommaCharactersIntact(t *testing.T) {
	var parsed map[string]any
	input := `{"url":"https://example.test/a//b/*c*/", "secret":"quoted\\\"//token,}", /* comment */ "values":[1,2,],}`
	if err := launcher.ReadJSONC([]byte(input), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed["url"] != "https://example.test/a//b/*c*/" || !strings.Contains(parsed["secret"].(string), "//token,}") {
		t.Fatalf("quoted strings were changed: %v", parsed)
	}
	var direct map[string]any
	if err := json.Unmarshal([]byte(`{"value":"//literal"}`), &direct); err != nil {
		t.Fatal(err)
	}
	if err := launcher.ReadJSONC([]byte(`{"x": 1 /* unterminated`), &parsed); err == nil {
		t.Fatal("unterminated JSONC comment accepted")
	}
}

func TestMissingEnvironmentReferenceIsAnErrorWithoutLeakingValues(t *testing.T) {
	home := isolate(t)
	path := filepath.Join(home, "opencode.json")
	if err := os.WriteFile(path, []byte(`{"mcp":{"textbee":{"type":"local","command":["npx"],"environment":{"KEY":"{env:HARNESS_UNSET_VALUE}"}}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HARNESS_UNSET_VALUE", "")
	os.Unsetenv("HARNESS_UNSET_VALUE")
	if _, _, err := launcher.MCPConfig(path, home, filepath.Join(home, ".pi")); err == nil || !strings.Contains(err.Error(), "HARNESS_UNSET_VALUE") {
		t.Fatalf("missing reference not surfaced: %v", err)
	}
}
