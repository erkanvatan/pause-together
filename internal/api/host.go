package api

import (
	"net"
	"net/http"
	"strings"
)

// localHostOnly rejects requests whose Host isn't localhost, 127.0.0.1 or [::1], port ignored.
// This stops DNS rebinding: a page pointing its own domain at 127.0.0.1 looks same-origin, so
// CrossOriginProtection alone can't tell.
func localHostOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLocalHost(r.Host) {
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isLocalHost(hostport string) bool {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		// No port: "localhost", "127.0.0.1" or "[::1]".
		host = strings.TrimSuffix(strings.TrimPrefix(hostport, "["), "]")
	}
	switch strings.ToLower(host) {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}
