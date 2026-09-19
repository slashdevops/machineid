package machineid

import "testing"

func TestIsValidUUID(t *testing.T) {
	tests := []struct {
		name  string
		uuid  string
		valid bool
	}{
		{"uppercase", "4C4C4544-0058-5210-8048-B4C04F595031", true},
		{"lowercase", "4c4c4544-0058-5210-8048-b4c04f595031", true},
		{"braces", "{4C4C4544-0058-5210-8048-B4C04F595031}", true},
		{"no dashes", "4c4c45440058521080484b4c04f595031"[:32], true},
		{"urn prefix", "urn:uuid:4c4c4544-0058-5210-8048-b4c04f595031", true},
		{"empty", "", false},
		{"nil UUID", "00000000-0000-0000-0000-000000000000", false},
		{"max UUID", "FFFFFFFF-FFFF-FFFF-FFFF-FFFFFFFFFFFF", false},
		{"not a UUID", "abc123", false},
		{"OEM placeholder", "To be filled by O.E.M.", false},
		{"dmidecode not settable", "Not Settable", false},
		{"truncated", "4C4C4544-0058-5210-8048", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidUUID(tt.uuid); got != tt.valid {
				t.Errorf("isValidUUID(%q) = %v, want %v", tt.uuid, got, tt.valid)
			}
		})
	}
}
