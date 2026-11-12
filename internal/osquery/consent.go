package osquery

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"strings"
)

const ConsentVersion = "2026-11-13.1"

//go:embed consent.md
var consentText string

type ConsentRef struct {
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}
type ConsentDocument struct {
	ConsentRef
	Text         string            `json:"text"`
	Translations map[string]string `json:"translations"`
}

func Consent() ConsentDocument {
	sum := sha256.Sum256([]byte(consentText))
	return ConsentDocument{ConsentRef: ConsentRef{Version: ConsentVersion, SHA256: hex.EncodeToString(sum[:])}, Text: consentText, Translations: consentTranslations()}
}
func (c ConsentRef) Valid() bool {
	current := Consent()
	return c.Version == current.Version && c.SHA256 == current.SHA256
}

// One canonical hash binds both reviewed language versions. Existing approvals
// must renew when either text changes; a translation cannot bypass consent.
func consentTranslations() map[string]string {
	en, id, ok := strings.Cut(consentText, "<!-- LANGUAGE: id -->")
	if !ok {
		panic("bilingual collection document lacks Indonesian text")
	}
	return map[string]string{"en": strings.TrimSpace(en), "id": strings.TrimSpace(id)}
}
