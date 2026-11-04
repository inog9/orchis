package httpx

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestProblemCodesKeepDiagnosticsSeparateFromLocalizedContract(t *testing.T) {
	for _, test := range []struct {
		status       int
		detail, code string
	}{{400, "unrecognized diagnostic", "invalid_request"}, {409, "Already running.", "already_running"}, {422, "Choose English or Indonesia.", "language_invalid"}, {500, "unrecognized diagnostic", "request_failed"}} {
		p := NewProblem(test.status, "Diagnostic title", test.detail)
		if p.Code != test.code {
			t.Fatal(p.Code, test.code)
		}
		w := httptest.NewRecorder()
		Write(w, httptest.NewRequest("GET", "/", nil), p)
		var body Problem
		if e := json.Unmarshal(w.Body.Bytes(), &body); e != nil || body.Code != test.code {
			t.Fatal("wire code", body.Code, e)
		}
	}
	p := Validation(FieldError{Name: "name", Message: "Name is required (max 200 characters)."}, FieldError{Name: "custom", Message: "Arbitrary internal text"})
	if p.Code != "validation_failed" || p.Fields[0].Code != "name_invalid" || p.Fields[1].Code != "field_invalid" {
		t.Fatal("field codes", p)
	}
}

func TestKnownMessageCodesCoverCuratedAdminMessages(t *testing.T) {
	for message, code := range map[string]string{
		"This server has no agent release to install. An administrator must publish one first.": "release_unavailable",
		"This agent is already revoked.":                                       "agent_revoked",
		"An account with this email already exists.":                           "account_email_exists",
		"You can’t change your own role or status. Ask another administrator.": "own_role_change",
		"Use at least 12 characters.":                                          "password_too_short",
		"That isn’t your current password.":                                    "current_password_wrong",
		"Someone with this email already exists.":                              "person_email_exists",
		"Invalid IP address.":                                                  "address_ip_invalid",
		"Invalid CIDR.":                                                        "address_cidr_invalid",
		"Invalid hostname.":                                                    "address_hostname_invalid",
		"URL must start with http:// or https://.":                             "address_url_invalid",
		"Invalid MAC address.":                                                 "address_mac_invalid",
		"Unknown address kind.":                                                "address_kind_invalid",
		"At most 20 addresses.":                                                "addresses_too_many",
	} {
		if got := knownMessageCode(message); got != code {
			t.Fatal(message, got, code)
		}
	}
}
