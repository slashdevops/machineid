package machineid

import (
	"testing"
)

// TestCollectMACAddresses tests network interface MAC address collection with default filter.
func TestCollectMACAddresses(t *testing.T) {
	macs, err := collectMACAddresses(MACFilterPhysical, nil)
	if err != nil {
		t.Logf("collectMACAddresses error (might be expected in some environments): %v", err)
	}
	t.Logf("Found %d physical MAC addresses", len(macs))
}

// TestCollectMACAddressesAllFilter tests that MACFilterAll returns >= physical count.
func TestCollectMACAddressesAllFilter(t *testing.T) {
	physical, err := collectMACAddresses(MACFilterPhysical, nil)
	if err != nil {
		t.Logf("physical filter error: %v", err)
	}

	all, err := collectMACAddresses(MACFilterAll, nil)
	if err != nil {
		t.Logf("all filter error: %v", err)
	}

	if len(all) < len(physical) {
		t.Errorf("MACFilterAll (%d) should return >= MACFilterPhysical (%d)", len(all), len(physical))
	}

	t.Logf("Physical: %d, All: %d", len(physical), len(all))
}

// TestCollectMACAddressesVirtualFilter tests that virtual filter excludes physical interfaces.
func TestCollectMACAddressesVirtualFilter(t *testing.T) {
	physical, _ := collectMACAddresses(MACFilterPhysical, nil)
	virtual, _ := collectMACAddresses(MACFilterVirtual, nil)
	all, _ := collectMACAddresses(MACFilterAll, nil)

	// Virtual + physical should equal all (no overlap since classification is binary)
	if len(virtual)+len(physical) != len(all) {
		t.Errorf("virtual (%d) + physical (%d) != all (%d)", len(virtual), len(physical), len(all))
	}

	t.Logf("Physical: %d, Virtual: %d, All: %d", len(physical), len(virtual), len(all))
}

// TestMACFilterString tests the String() method on MACFilter.
func TestMACFilterString(t *testing.T) {
	tests := []struct {
		filter MACFilter
		want   string
	}{
		{MACFilterPhysical, "physical"},
		{MACFilterAll, "all"},
		{MACFilterVirtual, "virtual"},
		{MACFilter(99), "physical"}, // unknown defaults to physical
	}

	for _, tt := range tests {
		got := tt.filter.String()
		if got != tt.want {
			t.Errorf("MACFilter(%d).String() = %q, want %q", tt.filter, got, tt.want)
		}
	}
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
