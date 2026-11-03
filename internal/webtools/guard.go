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
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsMulticast() || ip.IsUnspecified() || carrierNAT(ip)
}

func carrierNAT(ip net.IP) bool {
	ip4 := ip.To4()
	return ip4 != nil && ip4[0] == 100 && ip4[1]&0xc0 == 64
}
