package api

import (
	"net"
	"net/netip"
)

// GuestURL is the address guests open: the IP the guest port is bound to on the host, and that port.
// It's "" when guests can't reach the server through it: no bind IP, a loopback one (the default,
// nothing open yet), or 0.0.0.0, which names no one address to give out.
func GuestURL(bindIP, port string) string {
	ip, err := netip.ParseAddr(bindIP)
	if err != nil || ip.IsLoopback() || ip.IsUnspecified() || port == "" {
		return ""
	}
	return "http://" + net.JoinHostPort(ip.String(), port)
}
