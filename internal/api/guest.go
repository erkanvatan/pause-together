package api

import (
	"net"
	"net/netip"
	"strings"
)

// GuestURL is the address guests open: the IP the guest port is bound to on the host, and that port.
// It's "" when there's no one address to give out: no bind IP, a loopback one (the default, nothing
// open yet), or 0.0.0.0. An IPv6 bind comes in brackets, as compose's ports need it.
func GuestURL(bindIP, port string) string {
	if strings.HasPrefix(bindIP, "[") && strings.HasSuffix(bindIP, "]") {
		bindIP = bindIP[1 : len(bindIP)-1]
	}
	ip, err := netip.ParseAddr(bindIP)
	if err != nil || ip.IsLoopback() || ip.IsUnspecified() || port == "" {
		return ""
	}
	return "http://" + net.JoinHostPort(ip.String(), port)
}
