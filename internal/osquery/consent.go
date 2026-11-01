package osquery

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
)

const ConsentVersion = "2026-11-01.1"

//go:embed consent.md
var consentText string

type ConsentRef struct {
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}
type ConsentDocument struct {
	ConsentRef
	Text string `json:"text"`
}

func Consent() ConsentDocument {
	sum := sha256.Sum256([]byte(consentText))
	return ConsentDocument{ConsentRef: ConsentRef{Version: ConsentVersion, SHA256: hex.EncodeToString(sum[:])}, Text: consentText}
}
func (c ConsentRef) Valid() bool {
	current := Consent()
	return c.Version == current.Version && c.SHA256 == current.SHA256
}
