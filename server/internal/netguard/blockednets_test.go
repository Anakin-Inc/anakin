// SPDX-License-Identifier: AGPL-3.0-or-later

package netguard

import (
	"context"
	"net"
	"testing"
)

// TestBlockedSpecialPurposeRanges covers the IANA special-purpose allocations that
// the net.IP predicates do not classify. The boundary cases matter as much as the
// hits: over-blocking would break scraping legitimate public sites.
func TestBlockedSpecialPurposeRanges(t *testing.T) {
	tests := []struct {
		ip      string
		blocked bool
		why     string
	}{
		// RFC 6598 shared address space, 100.64.0.0/10.
		{"100.100.100.200", true, "Alibaba Cloud instance metadata"},
		{"100.64.0.0", true, "first address of 100.64.0.0/10"},
		{"100.127.255.255", true, "last address of 100.64.0.0/10"},
		{"::ffff:100.100.100.200", true, "IPv4-mapped Alibaba Cloud metadata"},
		{"100.63.255.255", false, "public, one below 100.64.0.0/10"},
		{"100.128.0.0", false, "public, one above 100.64.0.0/10"},
		{"101.0.0.1", false, "public"},

		// RFC 6890 IETF protocol assignments, 192.0.0.0/24.
		{"192.0.0.0", true, "first address of 192.0.0.0/24"},
		{"192.0.0.255", true, "last address of 192.0.0.0/24"},
		{"192.0.1.1", false, "public, just above 192.0.0.0/24"},
		{"192.0.2.1", false, "TEST-NET-1, documentation only, still routed nowhere but not a target"},

		// RFC 2544 benchmarking, 198.18.0.0/15.
		{"198.18.0.1", true, "benchmarking range"},
		{"198.19.255.255", true, "last address of 198.18.0.0/15"},
		{"198.17.255.255", false, "public, one below 198.18.0.0/15"},
		{"198.20.0.1", false, "public, just above 198.18.0.0/15"},

		// RFC 1112 reserved, 240.0.0.0/4, including the broadcast address.
		{"240.0.0.1", true, "reserved class E"},
		{"255.255.255.255", true, "limited broadcast"},
		{"239.255.255.255", true, "top of multicast, already covered by IsMulticast"},
		{"223.255.255.255", false, "public, below the multicast range"},

		// Ranges the original guard already handled must keep working.
		{"169.254.169.254", true, "AWS/GCP/Azure metadata"},
		{"127.0.0.1", true, "loopback"},
		{"10.0.0.1", true, "RFC 1918"},
		{"8.8.8.8", false, "public resolver"},
		{"93.184.216.34", false, "example.com"},
		{"2606:2800:220:1:248:1893:25c8:1946", false, "public IPv6"},
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("bad test fixture: %q is not an IP", tt.ip)
			}
			if got := Blocked(ip); got != tt.blocked {
				t.Errorf("Blocked(%s) = %v, want %v (%s)", tt.ip, got, tt.blocked, tt.why)
			}
		})
	}
}

// TestValidateHostRejectsSharedAddressSpace is the boundary check a scrape request
// hits first: POST /v1/scrape with the Alibaba metadata address must be a 400.
func TestValidateHostRejectsSharedAddressSpace(t *testing.T) {
	ctx := context.Background()

	for _, host := range []string{"100.100.100.200", "100.64.0.1", "198.18.0.1", "255.255.255.255"} {
		if err := ValidateHost(ctx, host); err == nil {
			t.Errorf("ValidateHost(%q) = nil, want an error", host)
		}
	}

	for _, host := range []string{"100.63.255.255", "100.128.0.1", "198.20.0.1"} {
		if err := ValidateHost(ctx, host); err != nil {
			t.Errorf("ValidateHost(%q) = %v, want nil", host, err)
		}
	}
}

// TestDialControlRejectsSharedAddressSpace covers the second layer, which sees the
// post-resolution IP and so also catches a redirect or a DNS record that flips to
// the metadata address after validation.
func TestDialControlRejectsSharedAddressSpace(t *testing.T) {
	for _, addr := range []string{"100.100.100.200:80", "100.64.0.1:443", "198.18.0.1:80", "240.0.0.1:80"} {
		if err := DialControl("tcp", addr, nil); err == nil {
			t.Errorf("DialControl(%q) = nil, want an error", addr)
		}
	}

	for _, addr := range []string{"100.63.255.255:80", "8.8.8.8:53"} {
		if err := DialControl("tcp", addr, nil); err != nil {
			t.Errorf("DialControl(%q) = %v, want nil", addr, err)
		}
	}
}

// TestBlockedNetsAreCanonical guards against a typo that would silently widen or
// narrow a range, e.g. writing 100.100.0.0/10 instead of 100.64.0.0/10.
func TestBlockedNetsAreCanonical(t *testing.T) {
	want := []string{
		"100.64.0.0/10",
		"192.0.0.0/24",
		"198.18.0.0/15",
		"240.0.0.0/4",
	}

	if len(blockedNets) != len(want) {
		t.Fatalf("blockedNets has %d entries, want %d", len(blockedNets), len(want))
	}
	for i, w := range want {
		if got := blockedNets[i].String(); got != w {
			t.Errorf("blockedNets[%d] = %s, want %s", i, got, w)
		}
	}
}
