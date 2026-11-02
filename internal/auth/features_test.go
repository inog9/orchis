package auth

import "testing"

func TestRuntimeFeaturesFollowServerConfiguration(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		h := Handlers{AdvancedFeatures: enabled}
		dto := h.toDTO(&Principal{})
		if dto.Features["advanced"] != enabled {
			t.Fatalf("runtime flag does not follow server option: %v", enabled)
		}
	}
}
