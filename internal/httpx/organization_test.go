package httpx

import (
	"net/url"
	"testing"
)

func TestOrganizationParamRejectsConflictingScopeAndInvalidIdentifiers(t *testing.T) {
	for _, test := range []struct {
		query             string
		unassigned, valid bool
		want              string
	}{
		{"", false, true, ""}, {"org=11111111-1111-4111-8111-111111111111", false, true, "11111111-1111-4111-8111-111111111111"},
		{"organization_id=unassigned", true, true, "unassigned"}, {"org=unassigned", false, false, ""}, {"org=x", true, false, ""},
		{"org=11111111-1111-4111-8111-111111111111&organization_id=22222222-2222-4222-8222-222222222222", true, false, ""},
	} {
		q, _ := url.ParseQuery(test.query)
		value, err := OrganizationParam(q, test.unassigned)
		if (err == nil) != test.valid || value != test.want {
			t.Fatalf("scope %q: %q %v", test.query, value, err)
		}
	}
}
