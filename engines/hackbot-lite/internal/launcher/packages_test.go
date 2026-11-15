package launcher_test

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	pentestharnes "github.com/r1pu5u/pentest-harnes"
	"github.com/r1pu5u/pentest-harnes/internal/launcher"
)

func fixtureFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func fixturePackage(t *testing.T, root, name string) string {
	t.Helper()
	entry := map[string]string{
		"pi-subagents":                       "bundle/index.js",
		"billion-context":                    "dist/agent/pi-native.js",
		"@xynogen/pix-todo":                  "src/index.ts",
		"@juicesharp/rpiv-ask-user-question": "index.ts",
	}[name]
	fixtureFile(t, filepath.Join(root, "package.json"), `{"name":"`+name+`","pi":{"extensions":["`+entry+`"]}}`)
	// Detection must not execute or import the entry (even a broken one).
	fixtureFile(t, filepath.Join(root, filepath.FromSlash(entry)), `throw new Error("package detection executed JavaScript");`)
	return root
}

func commandEnv(command *exec.Cmd, key string) string {
	for _, item := range command.Env {
		if value, ok := strings.CutPrefix(item, key+"="); ok {
			return value
		}
	}
	return ""
}

func extensions(command *exec.Cmd) []string {
	var paths []string
	for i := 1; i+1 < len(command.Args); i++ {
		if command.Args[i] == "--extension" {
			paths = append(paths, command.Args[i+1])
		}
	}
	return paths
}

func runPackageFixture(t *testing.T, execute func(*exec.Cmd) error) string {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := launcher.Run(launcher.Options{Dir: t.TempDir(), Pi: executable, New: true, Execute: execute}, pentestharnes.Skills(), pentestharnes.Runtime(), strings.NewReader(""), &output, &output); err != nil {
		t.Fatal(err)
	}
	return output.String()
}

func readJSONObject(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestInstalledOptionalPackagesAndNativeLaunch(t *testing.T) {
	isolate(t)
	agentDir := os.Getenv("PI_CODING_AGENT_DIR")
	roots := map[string]string{}
	for _, name := range []string{"pi-subagents", "billion-context", "@xynogen/pix-todo", "@juicesharp/rpiv-ask-user-question"} {
		roots[name] = fixturePackage(t, filepath.Join(agentDir, "npm", "node_modules", filepath.FromSlash(name)), name)
	}
	// Installed packages are loaded through global settings, with Pi owning
	// normal resource deduplication. Only Fleet needs an explicit launch path.
	settingsPath := filepath.Join(agentDir, "settings.json")
	settings := `{"packages":["npm:pi-subagents","npm:billion-context","npm:@xynogen/pix-todo","npm:@juicesharp/rpiv-ask-user-question"]}`
	fixtureFile(t, settingsPath, settings)
	var doctor bytes.Buffer
	if err := launcher.Doctor("", &doctor); err != nil {
		t.Fatal(err)
	}
	for name, root := range roots {
		if !strings.Contains(doctor.String(), name+": "+root) {
			t.Fatalf("doctor omitted installed %s: %s", name, doctor.String())
		}
	}
	output := runPackageFixture(t, func(command *exec.Cmd) error {
		if got := commandEnv(command, "PENTEST_HARNES_SUBAGENTS_DIR"); got != roots["pi-subagents"] {
			t.Fatalf("native backend root: %q", got)
		}
		want := filepath.Join(roots["pi-subagents"], "bundle", "index.js")
		count := 0
		for _, extension := range extensions(command) {
			if extension == want {
				count++
			}
			if strings.Contains(extension, "billion-context") || strings.Contains(extension, "pix-todo") {
				t.Fatalf("launcher bypassed user's package loading: %q", extension)
			}
		}
		if count != 1 {
			t.Fatalf("Fleet explicit extension count = %d: %v", count, command.Args)
		}
		if commandEnv(command, "BILI_CONFIG_FILE") == "" {
			t.Fatal("installed compression package missing private config")
		}
		return nil
	})
	if !strings.Contains(output, "Estimated spend:") || !strings.Contains(output, "no hard monetary cap") {
		t.Fatalf("incorrect spending diagnostic: %s", output)
	}
	data, _ := os.ReadFile(settingsPath)
	if string(data) != settings {
		t.Fatal("global package settings changed")
	}
}

func TestPackageDetectionOverrideCanonicalizationAndFallbacks(t *testing.T) {
	for _, layout := range []string{"npm/node_modules", "node_modules", "extensions", "override"} {
		t.Run(layout, func(t *testing.T) {
			isolate(t)
			root := filepath.Join(os.Getenv("PI_CODING_AGENT_DIR"), filepath.FromSlash(layout), "pi-subagents")
			if layout == "override" {
				root = filepath.Join(t.TempDir(), "fixture with spaces")
			}
			fixturePackage(t, root, "pi-subagents")
			if layout == "override" {
				alias := filepath.Join(t.TempDir(), "linked package")
				if err := os.Symlink(root, alias); err != nil {
					t.Fatal(err)
				}
				t.Setenv("PENTEST_HARNES_SUBAGENTS_DIR", alias)
				// Override wins over another complete active installation.
				fixturePackage(t, filepath.Join(os.Getenv("PI_CODING_AGENT_DIR"), "npm", "node_modules", "pi-subagents"), "pi-subagents")
			}
			// Empty package selection still needs the explicit Fleet extension.
			fixtureFile(t, filepath.Join(os.Getenv("PI_CODING_AGENT_DIR"), "settings.json"), `{"packages":[]}`)
			runPackageFixture(t, func(command *exec.Cmd) error {
				if got := commandEnv(command, "PENTEST_HARNES_SUBAGENTS_DIR"); got != root {
					t.Fatalf("package root was not canonical/selected: %q, want %q", got, root)
				}
				want := filepath.Join(root, "bundle", "index.js")
				if !strings.Contains(strings.Join(extensions(command), "\n"), want) {
					t.Fatalf("explicit native extension missing: %v", command.Args)
				}
				return nil
			})
		})
	}
	t.Run("npm installation wins", func(t *testing.T) {
		isolate(t)
		agentDir := os.Getenv("PI_CODING_AGENT_DIR")
		root := fixturePackage(t, filepath.Join(agentDir, "npm", "node_modules", "pi-subagents"), "pi-subagents")
		fixturePackage(t, filepath.Join(agentDir, "extensions", "pi-subagents"), "pi-subagents")
		runPackageFixture(t, func(command *exec.Cmd) error {
			if commandEnv(command, "PENTEST_HARNES_SUBAGENTS_DIR") != root {
				t.Fatal("npm installation did not win")
			}
			return nil
		})
	})
	t.Run("active agent directory", func(t *testing.T) {
		isolate(t)
		fixturePackage(t, filepath.Join(os.Getenv("PI_CODING_AGENT_DIR"), "npm", "node_modules", "pi-subagents"), "pi-subagents")
		t.Setenv("PI_CODING_AGENT_DIR", t.TempDir())
		runPackageFixture(t, func(command *exec.Cmd) error {
			if got := commandEnv(command, "PENTEST_HARNES_SUBAGENTS_DIR"); got != "" {
				t.Fatalf("inactive installation selected: %q", got)
			}
			return nil
		})
	})
}

func TestMissingOrIncompletePackagesRetainCLIWorkers(t *testing.T) {
	for _, state := range []string{"missing", "wrong name", "missing entry", "invalid manifest", "invalid override"} {
		t.Run(state, func(t *testing.T) {
			isolate(t)
			root := filepath.Join(os.Getenv("PI_CODING_AGENT_DIR"), "npm", "node_modules", "pi-subagents")
			switch state {
			case "wrong name":
				fixturePackage(t, root, "billion-context")
			case "missing entry":
				fixtureFile(t, filepath.Join(root, "package.json"), `{"name":"pi-subagents"}`)
			case "invalid manifest":
				fixtureFile(t, filepath.Join(root, "package.json"), `{`)
			case "invalid override":
				fixturePackage(t, root, "pi-subagents")
				t.Setenv("PENTEST_HARNES_SUBAGENTS_DIR", filepath.Join(t.TempDir(), "missing"))
			}
			runPackageFixture(t, func(command *exec.Cmd) error {
				if commandEnv(command, "PENTEST_HARNES_SUBAGENTS_DIR") != "" {
					t.Fatal("unavailable package enabled native backend")
				}
				if commandEnv(command, "PENTEST_HARNES_PI") == "" {
					t.Fatal("CLI worker executable missing")
				}
				for _, extension := range extensions(command) {
					if strings.Contains(extension, "pi-subagents") {
						t.Fatal("missing package became a required extension")
					}
				}
				if commandEnv(command, "BILI_CONFIG_FILE") != "" {
					t.Fatal("absent compression package got config override")
				}
				if _, err := os.Stat(filepath.Join(commandEnv(command, "PENTEST_HARNES_SESSION"), "private", "billion-context.json")); !os.IsNotExist(err) {
					t.Fatal("absent compression package wrote config")
				}
				return nil
			})
			var doctor bytes.Buffer
			if err := launcher.Doctor("", &doctor); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(doctor.String(), "native Fleet unavailable; CLI subprocess workers available") || !strings.Contains(doctor.String(), "pi install npm:pi-subagents") {
				t.Fatalf("inaccurate fallback diagnostic: %s", doctor.String())
			}
		})
	}
}

func TestBillionConfigPreservesCustomValuesAndUsesPrivateOverride(t *testing.T) {
	for _, location := range []string{"explicit", "xdg", "home default"} {
		t.Run(location, func(t *testing.T) {
			home := isolate(t)
			fixturePackage(t, filepath.Join(os.Getenv("PI_CODING_AGENT_DIR"), "npm", "node_modules", "billion-context"), "billion-context")
			path := filepath.Join(home, ".config", "billion-context", "billion-context.json")
			if location == "explicit" {
				path = filepath.Join(t.TempDir(), "custom.json")
				t.Setenv("BILI_CONFIG_FILE", path)
				fixtureFile(t, filepath.Join(home, ".config", "billion-context", "billion-context.json"), `{"port":7777}`)
			} else if location == "xdg" {
				t.Setenv("XDG_CONFIG_HOME", t.TempDir())
				path = filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "billion-context", "billion-context.json")
			} else {
				t.Setenv("XDG_CONFIG_HOME", "")
			}
			const original = `{
				"port":9876,"custom":{"precise":9007199254740993,"future":[true,"keep"]},
				"providers":{"https://provider.invalid/v1":{"models":{"custom-model":{"context":123456}},"credentialRef":"existing-reference"}},
				"pi":{"inline":true,"subagents":{"maxConcurrent":3}},
				"guardrails":{"bashTimeout":60,"maxOutputBytes":1234},
				"compress":{"maxContextLimit":"65%","externalSummary":{"enabled":true,"targets":[{"model":"custom-summary"}]}},
				"cacheKeepalive":{"enabled":true,"intervalMs":12345}
			}`
			fixtureFile(t, path, original)
			want := readJSONObject(t, path)
			want["pi"].(map[string]any)["subagents"] = false
			want["guardrails"].(map[string]any)["bashTimeout"] = json.Number("0")
			runPackageFixture(t, func(command *exec.Cmd) error {
				private := commandEnv(command, "BILI_CONFIG_FILE")
				if private != filepath.Join(commandEnv(command, "PENTEST_HARNES_SESSION"), "private", "billion-context.json") || private == path {
					t.Fatalf("config override is not session-private: %q", private)
				}
				if got := readJSONObject(t, private); !reflect.DeepEqual(got, want) {
					t.Fatalf("custom config changed: got %#v, want %#v", got, want)
				}
				info, err := os.Stat(private)
				if err != nil || info.Mode().Perm() != 0600 {
					t.Fatalf("config is not private: %v, %v", info, err)
				}
				info, err = os.Stat(filepath.Dir(private))
				if err != nil || info.Mode().Perm() != 0700 {
					t.Fatalf("config directory is not private: %v, %v", info, err)
				}
				return nil
			})
			data, err := os.ReadFile(path)
			if err != nil || string(data) != original {
				t.Fatal("source config was modified")
			}
		})
	}
}

func TestBillionConfigDefaultsDoNotChangeCompression(t *testing.T) {
	for _, initial := range []string{"", `{"pi":{"subagents":true},"compress":{"maxContextLimit":"80%","externalSummary":{"targets":[]}},"cacheKeepalive":{"intervalMs":12345}}`} {
		t.Run(initial, func(t *testing.T) {
			isolate(t)
			fixturePackage(t, filepath.Join(os.Getenv("PI_CODING_AGENT_DIR"), "npm", "node_modules", "billion-context"), "billion-context")
			if initial != "" {
				path := filepath.Join(t.TempDir(), "config.json")
				fixtureFile(t, path, initial)
				t.Setenv("BILI_CONFIG_FILE", path)
			}
			runPackageFixture(t, func(command *exec.Cmd) error {
				config := readJSONObject(t, commandEnv(command, "BILI_CONFIG_FILE"))
				if config["pi"].(map[string]any)["subagents"] != false || config["guardrails"].(map[string]any)["bashTimeout"] != json.Number("0") {
					t.Fatalf("redundant delegates or command timeout enabled: %#v", config)
				}
				if config["cacheKeepalive"].(map[string]any)["enabled"] != false || config["compress"].(map[string]any)["externalSummary"].(map[string]any)["enabled"] != false {
					t.Fatalf("background model calls defaulted on: %#v", config)
				}
				if initial != "" && config["compress"].(map[string]any)["maxContextLimit"] != "80%" {
					t.Fatal("compression configuration lost")
				}
				if _, exists := config["providers"]; exists {
					t.Fatal("launcher set providers")
				}
				return nil
			})
		})
	}
}

func TestInvalidBillionConfigIsNotSilentlyDiscarded(t *testing.T) {
	for _, invalid := range []string{`{`, `null`, `[]`, `{"pi":false}`, `{"guardrails":[]}`, `{"compress":{"externalSummary":false}}`} {
		t.Run(invalid, func(t *testing.T) {
			isolate(t)
			fixturePackage(t, filepath.Join(os.Getenv("PI_CODING_AGENT_DIR"), "npm", "node_modules", "billion-context"), "billion-context")
			path := filepath.Join(t.TempDir(), "config.json")
			fixtureFile(t, path, invalid)
			t.Setenv("BILI_CONFIG_FILE", path)
			executable, _ := os.Executable()
			var output bytes.Buffer
			err := launcher.Run(launcher.Options{Dir: t.TempDir(), Pi: executable, New: true, Execute: func(*exec.Cmd) error { t.Fatal("invalid config started Pi"); return nil }}, pentestharnes.Skills(), pentestharnes.Runtime(), strings.NewReader(""), &output, &output)
			if err == nil || !strings.Contains(err.Error(), "invalid billion-context configuration") {
				t.Fatalf("invalid config accepted: %v", err)
			}
			// Doctor only detects files; it need not parse private package config.
			if err := launcher.Doctor("", &output); err != nil {
				t.Fatalf("doctor read package configuration: %v", err)
			}
		})
	}
}

func TestFullscreenLeafProfileAndEstimateRPCOutput(t *testing.T) {
	isolate(t)
	executable, _ := os.Executable()
	var stdout, stderr bytes.Buffer
	err := launcher.Run(launcher.Options{Dir: t.TempDir(), Pi: executable, New: true, RPC: true, Execute: func(command *exec.Cmd) error {
		settings := readJSONObject(t, filepath.Join(command.Dir, ".pi", "settings.json"))
		if settings["tuiMode"] != "fullscreen" || settings["retry"].(map[string]any)["enabled"] != false || settings["retry"].(map[string]any)["provider"].(map[string]any)["maxRetries"] != json.Number("0") {
			t.Fatalf("incorrect interactive/retry settings: %#v", settings)
		}
		data, err := os.ReadFile(filepath.Join(command.Dir, ".pi", "agents", "pentest-harness-leaf.md"))
		if err != nil {
			t.Fatal(err)
		}
		profile := string(data)
		for _, field := range []string{"name: pentest-harness-leaf", "description: Constrained harness worker", "async: true", "defaultContext: fresh", "allowNestedSubagents: false", "inheritProjectContext: false", "inheritSkills: true"} {
			if !strings.Contains(profile, "\n"+field+"\n") {
				t.Fatalf("leaf missing %s: %s", field, profile)
			}
		}
		frontmatter := strings.SplitN(profile, "---", 3)[1]
		if strings.Contains(frontmatter, "\ntools:") || strings.Contains(frontmatter, "\nextensions:") || strings.Contains(frontmatter, "extension.ts") {
			t.Fatalf("leaf restricted tools/ambient extensions or duplicated runtime guard: %s", frontmatter)
		}
		specialist, err := fs.ReadFile(pentestharnes.Runtime(), "SPECIALIST.md")
		if err != nil || !strings.HasSuffix(profile, string(specialist)) {
			t.Fatalf("leaf did not include runtime specialist prompt: %v", err)
		}
		launch := readJSONObject(t, commandEnv(command, "PENTEST_HARNES_LAUNCH"))
		if launch["budgetMode"] != "estimate" || launch["budgetUSD"] != json.Number("15") || launch["spentUSD"] != json.Number("0") {
			t.Fatalf("incorrect estimate ledger: %#v", launch)
		}
		return nil
	}}, pentestharnes.Skills(), pentestharnes.Runtime(), strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "Estimated spend:") {
		t.Fatalf("launcher diagnostics polluted typed RPC output: stdout %q, stderr %q", stdout.String(), stderr.String())
	}
}

func TestLeafProfileHasTemplateWhenRuntimeOmitsSpecialist(t *testing.T) {
	isolate(t)
	executable, _ := os.Executable()
	var output bytes.Buffer
	err := launcher.Run(launcher.Options{Dir: t.TempDir(), Pi: executable, New: true, Execute: func(command *exec.Cmd) error {
		data, err := os.ReadFile(filepath.Join(command.Dir, ".pi", "agents", "pentest-harness-leaf.md"))
		if err != nil || !strings.Contains(string(data), "bounded specialist contract") {
			t.Fatalf("fallback leaf profile missing: %s, %v", data, err)
		}
		return nil
	}}, pentestharnes.Skills(), fstest.MapFS{}, strings.NewReader(""), &output, &output)
	if err != nil {
		t.Fatal(err)
	}
}
