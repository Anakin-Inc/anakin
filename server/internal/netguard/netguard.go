// SPDX-License-Identifier: AGPL-3.0-or-later

// Package netguard keeps scrape targets off the server's own network.
//
// Two layers, because neither is sufficient alone:
//
//   - ValidateHost runs at the API boundary. The caller gets a 400 instead of a
//     job that fails later, and it is the only protection available for the
//     browser handler, whose dialer lives in the Camoufox process.
//   - DialControl runs at connect time on the HTTP handler's dialer. It sees the
//     post-resolution IP of every hop, so it also covers redirect chains and DNS
//     rebinding, which a one-shot hostname check cannot see.
//
// KNOWN GAP: the browser handler has boundary validation only. A target whose DNS
// record flips to an internal address between validation and navigation, or that
// redirects there, is not caught. Closing that needs an egress proxy in front of
// the browser service.
package netguard

import (
	"context"
	"fmt"
	"net"
	"syscall"
)

// blockedNets holds ranges that are not routable public destinations but that none
// of the net.IP predicates classify. net.IP.IsPrivate covers only RFC 1918 and
// fc00::/7, so every other IANA special-purpose allocation has to be listed.
//
// IPNet.Contains normalises an IPv4-mapped IPv6 address before testing, so
// ::ffff:100.100.100.200 matches the 100.64.0.0/10 entry too.
var blockedNets = []*net.IPNet{
	// RFC 6598 shared address space, used for carrier-grade NAT. Alibaba Cloud's
	// instance metadata endpoint, 100.100.100.200, sits in here — the same class of
	// target as 169.254.169.254, which is already blocked.
	mustCIDR("100.64.0.0/10"),
	// RFC 6890 IETF protocol assignments.
	mustCIDR("192.0.0.0/24"),
	// RFC 2544 benchmarking range, routed internally on some networks.
	mustCIDR("198.18.0.0/15"),
	// RFC 1112 reserved space, which also covers the 255.255.255.255 broadcast
	// address — IsMulticast only spans 224.0.0.0/4.
	mustCIDR("240.0.0.0/4"),
}

func mustCIDR(s string) *net.IPNet {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		panic("netguard: bad CIDR " + s + ": " + err.Error())
	}
	return n
}

// Blocked reports whether ip is in a range a scrape target must never reach:
// loopback, private (RFC1918 and fc00::/7), link-local (which covers the
// 169.254.169.254 cloud metadata endpoint), unspecified, multicast, or one of the
// special-purpose ranges in blockedNets.
func Blocked(ip net.IP) bool {
	if ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified() {
		return true
	}

	for _, n := range blockedNets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// ValidateHost rejects a hostname or IP literal that points at a blocked address.
//
// A host that does not resolve is allowed through: DNS failures are transient and
// unreachable names reach nothing, so the scrape is left to fail with its real error
// rather than turning a lookup hiccup into a 400.
func ValidateHost(ctx context.Context, host string) error {
	if ip := net.ParseIP(host); ip != nil {
		if Blocked(ip) {
			return fmt.Errorf("%s is not a routable public address", ip)
		}
		return nil
	}

	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil
	}
	for _, addr := range ips {
		if Blocked(addr.IP) {
			return fmt.Errorf("%s resolves to %s, which is not a routable public address", host, addr.IP)
		}
	}
	return nil
}

// DialControl is a net.Dialer Control hook that refuses connections to blocked
// addresses. Install it on transports that dial the target directly — with a proxy
// the dial target is the proxy itself, which is legitimately private.
func DialControl(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("cannot parse dial address %q", address)
	}
	if Blocked(ip) {
		return fmt.Errorf("blocked connection to %s", ip)
	}
	return nil
}
