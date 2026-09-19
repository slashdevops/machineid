package machineid

import (
	"log/slog"
	"net"
	"strings"
)

// virtualInterfacePrefixes lists kernel-style interface name prefixes (no
// spaces, as on Linux and macOS) that represent virtual, VPN, bridge,
// container or ephemeral interfaces. These are excluded because they change
// when software is installed/removed or connections are started/stopped.
// Names are compared in lower case.
var virtualInterfacePrefixes = []string{
	// VPN and tunnel interfaces
	"utun", "tun", "tap", "ipsec", "ppp", "wg", "tailscale", "zt", "nebula", "gre", "sit", "ip6tnl", "erspan", "vxlan", "geneve",
	// Docker, Podman, Kubernetes and container networking
	"docker", "br-", "veth", "cni", "flannel", "cali", "kube", "podman", "lxc", "lxd",
	// Virtual bridges, switches and dummies
	"virbr", "vnet", "vmnet", "bridge", "dummy", "ifb", "nlmon", "teql", "bond", "macvtap",
	// Parallels / VirtualBox / VMware
	"vnic", "vboxnet",
	// macOS: Apple Wireless Direct Link, low-latency WLAN, access point and Apple silicon debug interfaces
	"awdl", "llw", "ap", "anpi",
}

// windowsVirtualPrefixes lists prefixes of Windows friendly adapter names
// that are virtual: Hyper-V and WSL switches, Bluetooth PAN, Npcap, and
// hypervisor host adapters.
var windowsVirtualPrefixes = []string{"vethernet", "bluetooth", "npcap", "vmware", "virtualbox", "hyper-v", "tap-windows"}

// windowsVirtualSubstrings lists fragments that mark a Windows friendly name
// as virtual regardless of position, e.g. "Microsoft Wi-Fi Direct Virtual
// Adapter" or "Local Area Connection* 2" (Windows only uses the asterisk for
// virtual adapters).
var windowsVirtualSubstrings = []string{"virtual", "wi-fi direct", "loopback", "*"}

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

// isVirtualInterface reports whether the interface name denotes a virtual,
// VPN, bridge or container interface.
//
// Windows friendly names contain spaces or parentheses ("vEthernet (WSL)",
// "Local Area Connection") and are matched against the Windows rules only, so
// short kernel prefixes such as "lo" cannot misclassify "Local Area
// Connection". Kernel-style names are matched against the prefix list and an
// exact loopback pattern (lo, lo0, lo1, …).
func isVirtualInterface(name string) bool {
	lower := strings.ToLower(name)

	if strings.ContainsAny(lower, " (") {
		if hasAnyPrefix(lower, windowsVirtualPrefixes) {
			return true
		}
		for _, fragment := range windowsVirtualSubstrings {
			if strings.Contains(lower, fragment) {
				return true
			}
		}

		return false
	}

	return isLoopbackName(lower) || hasAnyPrefix(lower, virtualInterfacePrefixes) || hasAnyPrefix(lower, windowsVirtualPrefixes)
}

// isLoopbackName reports whether name is "lo" followed only by digits.
func isLoopbackName(name string) bool {
	rest, ok := strings.CutPrefix(name, "lo")
	if !ok {
		return false
	}
	for _, r := range rest {
		if r < '0' || r > '9' {
			return false
		}
	}

	return true
}
