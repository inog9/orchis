package osquery

import (
	"os"
	"strings"
	"testing"
)

func TestConsentDocumentMatchesPublishedTemplateAndRef(t *testing.T) {
	doc := Consent()
	if !doc.ConsentRef.Valid() || doc.Version != ConsentVersion || len(doc.SHA256) != 64 || !strings.Contains(doc.Text, "Version "+ConsentVersion) {
		t.Fatal("dokumen persetujuan tidak konsisten")
	}
	published, err := os.ReadFile("../../docs/ENDPOINT-CONSENT.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(published) != doc.Text {
		t.Fatal("dokumen publik berbeda dari yang disetujui di UI")
	}
	for _, ref := range []ConsentRef{{}, {Version: "old", SHA256: doc.SHA256}, {Version: doc.Version, SHA256: "bad"}} {
		if ref.Valid() {
			t.Fatal("referensi dokumen lama/tidak cocok diterima")
		}
	}
}

func TestBilingualConsentBindsBothLanguagesAndInvalidatesEarlierRef(t *testing.T) {
	doc := Consent()
	if !strings.Contains(doc.Translations["en"], "Purpose and timing") || !strings.Contains(doc.Translations["id"], "Tujuan dan waktu") {
		t.Fatal("missing reviewed consent translation")
	}
	for _, lang := range []string{"en", "id"} {
		if !strings.Contains(doc.Text, doc.Translations[lang]) {
			t.Fatal("translation outside canonical hash", lang)
		}
	}
	previous := ConsentRef{Version: "2026-11-03.1", SHA256: doc.SHA256}
	if previous.Valid() {
		t.Fatal("earlier document accepted without renewal")
	}
}
