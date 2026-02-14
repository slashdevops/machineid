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

// collectMACAddresses retrieves MAC addresses from network interfaces filtered
// by the given [MACFilter]. Loopback and down interfaces are always excluded.
func collectMACAddresses(filter MACFilter, logger *slog.Logger) ([]string, error) {
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

		virtual := isVirtualInterface(i.Name)

		switch filter {
		case MACFilterPhysical:
			if virtual {
				if logger != nil {
					logger.Debug("skipping virtual interface", "interface", i.Name)
				}

				continue
			}
		case MACFilterVirtual:
			if !virtual {
				if logger != nil {
					logger.Debug("skipping physical interface", "interface", i.Name)
				}

				continue
			}
		case MACFilterAll:
			// Include everything that passed loopback/up checks.
		}

		if logger != nil {
			logger.Debug("including interface", "interface", i.Name, "mac", i.HardwareAddr.String(), "virtual", virtual)
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
