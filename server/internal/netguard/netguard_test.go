// SPDX-License-Identifier: AGPL-3.0-or-later

package netguard

import (
	"context"
	"net"
	"testing"
)

func TestBlocked(t *testing.T) {
	tests := []struct {
		ip      string
		blocked bool
	}{
		{"127.0.0.1", true},
		{"127.53.0.1", true},
		{"::1", true},
		{"::ffff:127.0.0.1", true},
		{"169.254.169.254", true}, // AWS/GCP metadata
		{"::ffff:169.254.169.254", true},
		{"10.0.0.5", true},
		{"172.16.4.1", true},
		{"192.168.1.1", true},
		{"fc00::1", true},
		{"fe80::1", true},
		{"0.0.0.0", true},
		{"::", true},
		{"224.0.0.1", true},
		{"93.184.216.34", false}, // example.com
		{"8.8.8.8", false},
		{"2606:2800:220:1:248:1893:25c8:1946", false},
		{"172.32.0.1", false}, // just outside 172.16/12
	}

	for _, tt := range tests {
		ip := net.ParseIP(tt.ip)
		if ip == nil {
			t.Fatalf("bad test fixture: %q is not an IP", tt.ip)
		}
		if got := Blocked(ip); got != tt.blocked {
			t.Errorf("Blocked(%s) = %v, want %v", tt.ip, got, tt.blocked)
		}
	}
}

func TestValidateHost(t *testing.T) {
	ctx := context.Background()

	for _, host := range []string{"127.0.0.1", "169.254.169.254", "10.1.2.3", "::1", "localhost"} {
		if err := ValidateHost(ctx, host); err == nil {
			t.Errorf("ValidateHost(%q) = nil, want an error", host)
		}
	}

	for _, host := range []string{"93.184.216.34", "8.8.8.8"} {
		if err := ValidateHost(ctx, host); err != nil {
			t.Errorf("ValidateHost(%q) = %v, want nil", host, err)
		}
	}

	// Unresolvable hosts are left to fail during the scrape, not at the boundary.
	if err := ValidateHost(ctx, "no-such-host.invalid"); err != nil {
		t.Errorf("ValidateHost on unresolvable host = %v, want nil", err)
	}
}

func TestDialControl(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:8080", "169.254.169.254:80", "[::1]:443"} {
		if err := DialControl("tcp", addr, nil); err == nil {
			t.Errorf("DialControl(%q) = nil, want an error", addr)
		}
	}

	if err := DialControl("tcp", "93.184.216.34:80", nil); err != nil {
		t.Errorf("DialControl on a public address = %v, want nil", err)
	}
}
