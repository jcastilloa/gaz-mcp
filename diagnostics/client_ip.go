package diagnostics

import (
	"net"
	"net/http"
	"net/netip"
)

const (
	trueClientIPHeader  = "True-Client-IP"
	xForwardedForHeader = "X-Forwarded-For"
)

func clientIP(request *http.Request) (netip.Addr, bool) {
	value := request.Header.Get(trueClientIPHeader)
	if value == "" {
		value = request.Header.Get(xForwardedForHeader)
	}
	if value != "" {
		address, err := netip.ParseAddr(value)
		return address, err == nil
	}

	host, _, err := net.SplitHostPort(request.RemoteAddr)
	if err != nil {
		host = request.RemoteAddr
	}
	address, err := netip.ParseAddr(host)
	return address, err == nil
}
