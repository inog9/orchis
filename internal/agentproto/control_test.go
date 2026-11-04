package agentproto

import "testing"

func TestDiagnosticCodesNeverContainRawLogData(t *testing.T) {
	for _, v := range []string{"", "update_failed", "release_key_missing", "collection_failed", "uninstall_failed", "uninstall_unsupported"} {
		if DiagnosticCode(v) != v {
			t.Fatal("known code rejected", v)
		}
	}
	for _, v := range []string{"update_failed /root/secret", "oen_fixture-token", "password=private", "x\nnew log line", "UPDATE_FAILED", "<script>alert(1)</script>"} {
		if DiagnosticCode(v) != "" {
			t.Fatal("raw diagnostic accepted")
		}
	}
}
