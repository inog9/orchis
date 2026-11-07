package server_test

import (
	"encoding/csv"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestThreeRolePolicyEveryAPIEndpoint(t *testing.T) {
	f, err := os.Open("testdata/role-api-policy.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	a, _ := setup(t)
	cookies := map[string]string{}
	for _, role := range []string{"admin", "analyst", "viewer"} {
		cookies[role] = a.login(role + "@a.test")
	}
	seen := map[string]bool{}
	for _, row := range rows[1:] {
		pattern, allowed := row[0], row[1]
		seen[pattern] = true
		parts := strings.SplitN(pattern, " ", 2)
		path := parts[1]
		for _, key := range []string{"id", "org", "agent"} {
			path = strings.ReplaceAll(path, "{"+key+"}", "00000000-0000-0000-0000-000000000000")
		}
		path = strings.ReplaceAll(path, "{format}", "json")
		path = strings.ReplaceAll(path, "{key}", "osquery.inventory")
		path = strings.ReplaceAll(path, "{pack}", "linux")
		for _, role := range []string{"admin", "analyst", "viewer"} {
			cookie := cookies[role]
			if pattern == "POST /api/v1/auth/logout" {
				cookie = a.login(role + "@a.test")
			}
			r := a.do(parts[0], path, map[string]any{}, cookie)
			switch {
			case allowed == "machine":
				if r.Status != 401 {
					t.Errorf("%s %s must reject user credentials: %d", role, pattern, r.Status)
				}
			case allowed == "public":
				if r.Status == 401 || r.Status == 403 || r.Status >= 500 {
					t.Errorf("%s public %s: %d", role, pattern, r.Status)
				}
			case strings.Contains(allowed, role):
				if r.Status == 401 || r.Status == 403 || r.Status >= 500 {
					t.Errorf("%s allowed %s: %d %s", role, pattern, r.Status, r.Body)
				}
			default:
				if r.Status != 403 {
					t.Errorf("%s forbidden %s: %d %s", role, pattern, r.Status, r.Body)
				}
			}
		}
	}
	// Fail when a new literal API registration is omitted from the independent policy.
	err = filepath.WalkDir("..", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		tree, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(tree, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "HandleFunc" {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			s, _ := strconv.Unquote(lit.Value)
			if strings.Contains(s, " /api/") && !seen[s] {
				t.Errorf("API missing policy: %s (%s)", s, path)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != 168 {
		t.Fatalf("route inventory changed: got %d; review literal and five dynamic registrations", len(seen))
	}
}
