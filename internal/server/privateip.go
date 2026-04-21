package server

import (
	"fmt"
	"net"
	"sync"
)

// Pre-computed private IP ranges for SSRF protection.
// These are initialized on first use via sync.Once to avoid runtime parsing overhead.
var (
	privateIPv4Nets []*net.IPNet
	privateIPv6Nets []*net.IPNet
	privateIPOnce   sync.Once
)

// initPrivateIPRanges initializes the private IP range networks.
// This is called lazily on first use via sync.Once.
func initPrivateIPRanges() {
	// Parse private IPv4 ranges (RFC 1918 + link-local)
	privateIPv4CIDRs := []string{
		"10.0.0.0/8",     // Private class A
		"172.16.0.0/12",  // Private class B
		"192.168.0.0/16", // Private class C
		"169.254.0.0/16", // Link-local
	}

	for _, cidr := range privateIPv4CIDRs {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			// This should never happen with hardcoded CIDRs
			panic(fmt.Sprintf("invalid IPv4 CIDR in privateIPv4CIDRs: %s: %v", cidr, err))
		}
		privateIPv4Nets = append(privateIPv4Nets, network)
	}

	// Parse private IPv6 ranges
	privateIPv6CIDRs := []string{
		"fc00::/7",  // IPv6 unique local addresses (ULA)
		"fe80::/10", // IPv6 link-local
	}

	for _, cidr := range privateIPv6CIDRs {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			// This should never happen with hardcoded CIDRs
			panic(fmt.Sprintf("invalid IPv6 CIDR in privateIPv6CIDRs: %s: %v", cidr, err))
		}
		privateIPv6Nets = append(privateIPv6Nets, network)
	}
}

// IsPrivateIP checks if an IP address is in a private or reserved range.
func IsPrivateIP(ip net.IP) bool {
	if ip.IsLoopback() {
		return true
	}

	if isPrivateIPv4(ip) {
		return true
	}

	return isPrivateIPv6(ip)
}

// isPrivateIPv4 checks if an IPv4 address is in a private range (RFC 1918).
func isPrivateIPv4(ip net.IP) bool {
	privateIPOnce.Do(initPrivateIPRanges)
	for _, network := range privateIPv4Nets {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// isPrivateIPv6 checks if an IPv6 address is in a private range.
func isPrivateIPv6(ip net.IP) bool {
	privateIPOnce.Do(initPrivateIPRanges)
	// Only check IPv6 addresses
	if ip.To4() != nil {
		return false
	}

	for _, network := range privateIPv6Nets {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}
