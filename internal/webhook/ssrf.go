package webhook

import (
	"fmt"
	"net"
	"sync"
)

// privateIPv4CIDRs enumerates IPv4 ranges that must never be reachable by a
// customer-supplied webhook target.
var privateIPv4CIDRs = []string{
	"10.0.0.0/8",      // RFC 1918 Class A
	"172.16.0.0/12",   // RFC 1918 Class B
	"192.168.0.0/16",  // RFC 1918 Class C
	"169.254.0.0/16",  // Link-local (includes cloud metadata 169.254.169.254)
	"100.64.0.0/10",   // Carrier-grade NAT
	"0.0.0.0/8",       // Current network
	"192.0.0.0/24",    // IETF protocol assignments
	"192.0.2.0/24",    // TEST-NET-1
	"198.18.0.0/15",   // Benchmarking
	"198.51.100.0/24", // TEST-NET-2
	"203.0.113.0/24",  // TEST-NET-3
	"224.0.0.0/4",     // Multicast
	"240.0.0.0/4",     // Reserved
}

// privateIPv6CIDRs enumerates IPv6 ranges that must never be reachable by a
// customer-supplied webhook target.
var privateIPv6CIDRs = []string{
	"fc00::/7",      // Unique local addresses
	"fe80::/10",     // Link-local
	"fec0::/10",     // Deprecated site-local
	"ff00::/8",      // Multicast
	"::/128",        // Unspecified
	"::1/128",       // Loopback
	"64:ff9b::/96",  // NAT64 well-known prefix
	"100::/64",      // Discard-only
	"2001:db8::/32", // Documentation
}

var (
	parsedPrivateIPv4Nets []*net.IPNet
	parsedPrivateIPv6Nets []*net.IPNet
	privateIPParseOnce    sync.Once
)

// parsePrivateRanges populates parsedPrivateIPv{4,6}Nets on first use. Panics
// on malformed CIDRs because every entry is a compile-time literal.
func parsePrivateRanges() {
	for _, cidr := range privateIPv4CIDRs {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			panic(fmt.Sprintf("invalid IPv4 CIDR %q: %v", cidr, err))
		}
		parsedPrivateIPv4Nets = append(parsedPrivateIPv4Nets, network)
	}
	for _, cidr := range privateIPv6CIDRs {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			panic(fmt.Sprintf("invalid IPv6 CIDR %q: %v", cidr, err))
		}
		parsedPrivateIPv6Nets = append(parsedPrivateIPv6Nets, network)
	}
}

// IsPrivateOrReservedIP reports whether ip is in any range that should be
// unreachable from a customer-supplied webhook target. This covers loopback,
// RFC 1918 private space, link-local (incl. cloud metadata 169.254.169.254),
// CGNAT, multicast, and IPv6 equivalents.
func IsPrivateOrReservedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsUnspecified() {
		return true
	}

	privateIPParseOnce.Do(parsePrivateRanges)

	if v4 := ip.To4(); v4 != nil {
		for _, network := range parsedPrivateIPv4Nets {
			if network.Contains(v4) {
				return true
			}
		}
		return false
	}

	for _, network := range parsedPrivateIPv6Nets {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}
