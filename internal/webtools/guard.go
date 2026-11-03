package webtools

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"syscall"
	"time"
)

const refusedPrivate = "refused: private address"

func NewClient(allowPrivate bool) *http.Client {
	d := guardedDialer(allowPrivate)
	tr := &http.Transport{
		Proxy:       nil,
		DialContext: d.DialContext,
	}
	return &http.Client{
		Timeout:       15 * time.Second,
		Transport:     tr,
		CheckRedirect: limitRedirects,
	}
}

func guardedDialer(allowPrivate bool) *net.Dialer {
	return &net.Dialer{
		Timeout: 15 * time.Second,
		Control: dialControl(allowPrivate),
	}
}

func dialControl(allowPrivate bool) func(string, string, syscall.RawConn) error {
	return func(network, address string, c syscall.RawConn) error {
		if allowPrivate {
			return nil
		}
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			host = address
		}
		ip := net.ParseIP(host)
		if ip == nil {
			return nil
		}
		if blockedIP(ip) {
			return errors.New(refusedPrivate)
		}
		return nil
	}
}

func limitRedirects(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return fmt.Errorf("stopped after 5 redirects")
	}
	return nil
}

func blockedIP(ip net.IP) bool {
	if ip4 := ip.To4(); ip4 != nil {
		return blockedIPv4(ip4)
	}
	return blockedIPv6(ip)
}

func blockedIPv4(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	if inRange(ip, 100, 64, 10) {
		return true
	}
	return false
}

func blockedIPv6(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	if ip.IsLinkLocalMulticast() || ip.IsLinkLocalUnicast() {
		return true
	}
	if len(ip) == net.IPv6len && ip[0] == 0xfc {
		return true
	}
	if len(ip) == net.IPv6len && ip[0] == 0xfd {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		return blockedIPv4(ip4)
	}
	return false
}

func inRange(ip net.IP, a, b byte, prefix int) bool {
	mask := net.CIDRMask(prefix, 32)
	n := net.IPNet{IP: net.IPv4(a, b, 0, 0), Mask: mask}
	return n.Contains(ip)
}
