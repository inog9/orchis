package execution

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestProvisionedTokenFormat(t *testing.T) {
	valid := InjectorTokenPrefix + base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("x", 32)))
	if !ValidInjectorToken(valid) {
		t.Fatal("issued token rejected")
	}
	for _, token := range []string{"", strings.Repeat("x", 43), "oij_" + strings.Repeat("x", 32), "oij_" + strings.Repeat("!", 43), valid + "=", valid + "x"} {
		if ValidInjectorToken(token) {
			t.Fatal("invalid provisioning token accepted")
		}
	}
}
