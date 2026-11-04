package webtools

import (
	"net"
	"testing"
)

func TestBlockedIP(t *testing.T) {
	tests := []struct {
		ip      string
		blocked bool
	}{
		{"127.0.0.1", true},
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"192.168.0.1", true},
		{"169.254.1.1", true},
		{"100.64.0.1", true},
		{"0.0.0.0", true},
		{"224.0.0.1", true},
		{"8.8.8.8", false},
		{"203.0.113.1", false},
		{"::1", true},
		{"fe80::1", true},
		{"fc00::1", true},
		{"fd12::1", true},
		{"::ffff:127.0.0.1", true},
		{"2001:db8::1", false},
	}
	for _, tc := range tests {
		t.Run(tc.ip, func(t *testing.T) {
			ip := net.ParseIP(tc.ip)
			if ip == nil {
				t.Fatal("parse ip")
			}
			got := blockedIP(ip)
			if got != tc.blocked {
				t.Fatalf("blockedIP(%q) = %v, want %v", tc.ip, got, tc.blocked)
			}
		})
	}
}

func TestDialControlLoopback(t *testing.T) {
	ctrl := dialControl(false)
	err := ctrl("tcp", "127.0.0.1:80", nil)
	if err == nil || err.Error() != refusedPrivate {
		t.Fatalf("got err %v, want %q", err, refusedPrivate)
	}
}

func TestDialControlPublic(t *testing.T) {
	ctrl := dialControl(false)
	if err := ctrl("tcp", "8.8.8.8:443", nil); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestDialControlAllowPrivate(t *testing.T) {
	ctrl := dialControl(true)
	if err := ctrl("tcp", "127.0.0.1:80", nil); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}
