package launcher

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
)

// Doctor checks configuration and executable availability without contacting
// targets, starting MCP servers, reading inboxes, or sending SMS.
func Doctor(config string, stdout io.Writer) error {
	for _, name := range []string{"pi", "node", "interceptor", "gog", "totp-cli", "jxscout", "gau", "curl", "jsluice", "npxconfuse"} {
		path, err := exec.LookPath(name)
		if err != nil {
			fmt.Fprintf(stdout, "%s: MISSING\n", name)
		} else {
			fmt.Fprintf(stdout, "%s: %s\n", name, path)
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	agentDir := agentDirectory(home)
	packages := installedPackages(agentDir)
	if packages.Subagents == "" {
		fmt.Fprintln(stdout, "pi-subagents: MISSING/incomplete; native Fleet unavailable; CLI subprocess workers available. Install: pi install npm:pi-subagents")
	} else {
		fmt.Fprintf(stdout, "pi-subagents: %s (native Fleet click-inspection)\n", packages.Subagents)
	}
	if packages.BillionContext != "" {
		fmt.Fprintf(stdout, "billion-context: %s (context compression via enabled Pi package; session-private config override)\n", packages.BillionContext)
	}
	if packages.Todo != "" {
		fmt.Fprintf(stdout, "@xynogen/pix-todo: %s (durable todo UI via enabled Pi package)\n", packages.Todo)
	}
	if packages.AskUserQuestion != "" {
		fmt.Fprintf(stdout, "@juicesharp/rpiv-ask-user-question: %s (visible user decision dialogs)\n", packages.AskUserQuestion)
	} else {
		fmt.Fprintln(stdout, "ask_user_question: not installed; install with pi install npm:@juicesharp/rpiv-ask-user-question")
	}
	fmt.Fprintln(stdout, "MCP: Pi built-in support (requires Pi 1.5.0+; project config .pi/mcp.json)")
	mcp, _, err := MCPConfig(config, home, agentDir)
	if err != nil {
		return err
	}
	servers := mcp["mcpServers"].(map[string]any)
	var names []string
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintf(stdout, "MCP %s: configured (connection not tested)\n", name)
	}
	for _, required := range []string{"burp", "caido", "textbee"} {
		found := false
		for _, name := range names {
			found = found || strings.Contains(strings.ToLower(name), required)
		}
		if !found {
			fmt.Fprintf(stdout, "MCP %s: MISSING configuration\n", required)
		}
	}
	fmt.Fprintln(stdout, "Bundled integration skills: interceptor, interceptor-browser, interceptor-research")
	for _, path := range externalSkills(home) {
		fmt.Fprintf(stdout, "Integration skill: %s\n", path)
	}
	fmt.Fprintln(stdout, "Spend reporting: estimates only ($15 reporting reference, no hard monetary cap). Run /mcp to inspect live connections.")
	return nil
}
