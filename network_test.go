package machineid

import (
	"testing"
)

// TestCollectMACAddresses tests network interface MAC address collection.
func TestCollectMACAddresses(t *testing.T) {
	macs, err := collectMACAddresses(nil)
	if err != nil {
		t.Logf("collectMACAddresses error (might be expected in some environments): %v", err)
	}
	t.Logf("Found %d MAC addresses (filtered)", len(macs))
}

// TestIsVirtualInterface tests virtual interface detection.
func TestIsVirtualInterface(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"utun0", true},
		{"utun1", true},
		{"docker0", true},
		{"br-abc123", true},
		{"veth1234", true},
		{"bridge0", true},
		{"vmnet1", true},
		{"lo0", true},
		{"wg0", true},
		{"vnic0", true},
		{"en0", false},
		{"en1", false},
		{"eth0", false},
		{"wlan0", false},
		{"Wi-Fi", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isVirtualInterface(tt.name)
			if result != tt.expected {
				t.Errorf("isVirtualInterface(%q) = %v, want %v", tt.name, result, tt.expected)
			}
		})
	}
}
