package authz_test

import (
	"orchis-go/internal/authz"
	"reflect"
	"testing"
)

func TestThreeRolesHaveProductPermissions(t *testing.T) {
	roles := map[string]authz.Role{}
	for _, r := range authz.SystemRoles {
		roles[r.Key] = r
	}
	if len(roles) != 3 {
		t.Fatalf("expected three roles, got %d", len(roles))
	}
	if !reflect.DeepEqual(roles["viewer"].Permissions, []string{"finding.read", "report.read"}) {
		t.Fatalf("Viewer must read findings and reports only: %v", roles["viewer"].Permissions)
	}
	for _, perm := range []string{"asset.write", "simulation.run", "assessment.run", "report.generate"} {
		if !authz.Has(roles["analyst"].Permissions, perm) {
			t.Fatalf("Analyst missing %s", perm)
		}
	}
	for _, perm := range []string{"asset.authorize", "agent.manage", "settings.manage", "user.manage", "ai.manage"} {
		if authz.Has(roles["analyst"].Permissions, perm) {
			t.Fatalf("Analyst must not grant %s", perm)
		}
		if !authz.Has(roles["admin"].Permissions, perm) {
			t.Fatalf("Admin missing %s", perm)
		}
	}
}
