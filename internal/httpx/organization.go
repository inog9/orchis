package httpx

import (
	"github.com/google/uuid"
	"net/url"
)

// OrganizationParam menerima URL baru dan tautan lama tanpa memilih diam-diam antara dua scope berbeda.
func OrganizationParam(q url.Values, unassigned bool) (string, error) {
	value, legacy := q.Get("org"), q.Get("organization_id")
	if value != "" && legacy != "" && value != legacy {
		return "", Validation(FieldError{Name: "org", Message: "Choose one organisation filter."})
	}
	if value == "" {
		value = legacy
	}
	if value == "" || (unassigned && value == "unassigned") {
		return value, nil
	}
	if _, err := uuid.Parse(value); err != nil {
		return "", BadRequest("Invalid organization.")
	}
	return value, nil
}
