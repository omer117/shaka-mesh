package ratelimit

import (
	"net"
	"net/http"
	"strings"
)

// ClientIP returns the address to rate-limit a request by.
//
// trustedProxyHops is the number of reverse proxies that sit in front of the
// gateway and append to X-Forwarded-For (0 when clients connect directly, 1
// behind the Caddy edge in the cloud compose). With 0 the header is ignored
// entirely, because any client can send a forged X-Forwarded-For and would
// otherwise get a fresh rate-limit bucket per request. With N > 0 we take the
// Nth entry from the right: those entries were appended by our own proxies,
// while anything further left was supplied by the client and cannot be trusted.
func ClientIP(r *http.Request, trustedProxyHops int) string {
	if trustedProxyHops > 0 {
		var hops []string
		for _, value := range r.Header.Values("X-Forwarded-For") {
			for _, part := range strings.Split(value, ",") {
				if part = strings.TrimSpace(part); part != "" {
					hops = append(hops, part)
				}
			}
		}
		if len(hops) >= trustedProxyHops {
			if ip := net.ParseIP(hops[len(hops)-trustedProxyHops]); ip != nil {
				return ip.String()
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
