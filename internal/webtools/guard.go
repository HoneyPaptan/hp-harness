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
	if len(ip) < net.IPv4len {
		return false
	}
	switch {
	case ip[0] == 127:
		return true
	case ip[0] == 10:
		return true
	case ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31:
		return true
	case ip[0] == 192 && ip[1] == 168:
		return true
	case ip[0] == 169 && ip[1] == 254:
		return true
	case ip[0] == 100 && ip[1] >= 64 && ip[1] <= 127:
		return true
	case ip[0] >= 224 && ip[0] < 240:
		return true
	case ip[0]|ip[1]|ip[2]|ip[3] == 0:
		return true
	default:
		return false
	}
}

func blockedIPv6(ip net.IP) bool {
	if len(ip) < net.IPv6len {
		return false
	}
	if ip[0]|ip[1]|ip[2]|ip[3]|ip[4]|ip[5]|ip[6]|ip[7]|ip[8]|ip[9]|ip[10]|ip[11]|ip[12]|ip[13]|ip[14]|ip[15] == 0 {
		return true
	}
	if ip[0] == 0 && ip[1] == 0 && ip[2] == 0 && ip[3] == 0 &&
		ip[4] == 0 && ip[5] == 0 && ip[6] == 0 && ip[7] == 0 &&
		ip[8] == 0 && ip[9] == 0 && ip[10] == 0 && ip[11] == 0 &&
		ip[12] == 0 && ip[13] == 0 && ip[14] == 0 && ip[15] == 1 {
		return true
	}
	if ip[0] == 0xfe && ip[1]&0xc0 == 0x80 {
		return true
	}
	if ip[0] == 0xfc || ip[0] == 0xfd {
		return true
	}
	if ip[0] == 0xff {
		return true
	}
	return false
}
