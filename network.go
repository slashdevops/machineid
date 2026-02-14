package machineid

import (
	"log/slog"
	"net"
	"strings"
)

// virtualInterfacePrefixes lists interface name prefixes that represent
// virtual, VPN, bridge, or ephemeral interfaces. These are excluded because
// they change when software is installed/removed or connections are started/stopped.
var virtualInterfacePrefixes = []string{
	// VPN and tunnel interfaces
	"utun", "tun", "tap", "ipsec", "ppp",
	// Docker and container bridges
	"docker", "br-", "veth",
	// Virtual bridges and switches
	"virbr", "vnet", "vmnet",
	// Thunderbolt bridge (changes with docking state)
	"bridge",
	// Loopback variants
	"lo",
	// WireGuard
	"wg",
	// Parallels / VirtualBox / VMware
	"vnic", "vboxnet",
}

// collectMACAddresses retrieves MAC addresses from physical network interfaces.
// Virtual, VPN, bridge, and container interfaces are excluded for stability.
func collectMACAddresses(logger *slog.Logger) ([]string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var macs []string

	for _, i := range interfaces {
		// Skip loopback interfaces and those without MAC addresses.
		if i.Flags&net.FlagLoopback != 0 || len(i.HardwareAddr) == 0 {
			continue
		}

		// Skip interfaces that are not up — they may be transient.
		if i.Flags&net.FlagUp == 0 {
			if logger != nil {
				logger.Debug("skipping interface (not up)", "interface", i.Name)
			}

			continue
		}

		// Skip virtual/VPN/bridge interfaces that are not stable hardware.
		if isVirtualInterface(i.Name) {
			if logger != nil {
				logger.Debug("skipping virtual interface", "interface", i.Name)
			}

			continue
		}

		if logger != nil {
			logger.Debug("including interface", "interface", i.Name, "mac", i.HardwareAddr.String())
		}

		macs = append(macs, i.HardwareAddr.String())
	}

	return macs, nil
}

// isVirtualInterface returns true if the interface name matches a known
// virtual, VPN, or bridge prefix.
func isVirtualInterface(name string) bool {
	lower := strings.ToLower(name)
	for _, prefix := range virtualInterfacePrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}

	return false
}
