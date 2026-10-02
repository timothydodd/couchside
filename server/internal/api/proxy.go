package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// parseProxies reads COUCHSIDE_TRUSTED_PROXIES: CIDRs or single addresses,
// separated by commas or spaces.
func parseProxies(list []string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, v := range list {
		if strings.Contains(v, "/") {
			p, err := netip.ParsePrefix(v)
			if err != nil {
				return nil, fmt.Errorf("COUCHSIDE_TRUSTED_PROXIES: %q isn't a CIDR", v)
			}
			out = append(out, p.Masked())
			continue
		}
		a, err := netip.ParseAddr(v)
		if err != nil {
			return nil, fmt.Errorf("COUCHSIDE_TRUSTED_PROXIES: %q isn't an address", v)
		}
		out = append(out, netip.PrefixFrom(a.Unmap(), a.Unmap().BitLen()))
	}
	return out, nil
}

type forwardedKey struct{}

// viaHTTPS marks a request a trusted proxy received over HTTPS.
func viaHTTPS(r *http.Request) bool {
	v, _ := r.Context().Value(forwardedKey{}).(bool)
	return v
}

// realIP replaces chi's RealIP, which believed forwarding headers from anyone.
// Only when the peer is a trusted proxy does it take the client address from
// X-Forwarded-For (the right-most address that isn't a trusted proxy, since
// the left of the list is whatever the client sent) or X-Real-IP, and
// X-Forwarded-Proto for whether cookies are Secure.
func (s *Server) realIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		peer, ok := addrOf(clientIP(r))
		if !ok || !s.trusted(peer) {
			next.ServeHTTP(w, r)
			return
		}
		client := peer
		if hops := forwardedFor(r); len(hops) > 0 {
			client = hops[0] // all trusted: the furthest one we know
			for i := len(hops) - 1; i >= 0; i-- {
				if !s.trusted(hops[i]) {
					client = hops[i]
					break
				}
			}
		} else if a, ok := addrOf(strings.TrimSpace(r.Header.Get("X-Real-IP"))); ok {
			client = a
		}
		r2 := r.WithContext(context.WithValue(r.Context(), forwardedKey{},
			strings.EqualFold(strings.TrimSpace(lastValue(r.Header.Get("X-Forwarded-Proto"))), "https")))
		r2.RemoteAddr = net.JoinHostPort(client.String(), "0")
		next.ServeHTTP(w, r2)
	})
}

func (s *Server) trusted(a netip.Addr) bool {
	for _, p := range s.proxies {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// forwardedFor lists X-Forwarded-For's addresses over every copy of the header,
// in order. An unparseable entry ends the list there: nothing to its left can
// be placed.
func forwardedFor(r *http.Request) []netip.Addr {
	var raw []string
	for _, h := range r.Header.Values("X-Forwarded-For") {
		raw = append(raw, strings.Split(h, ",")...)
	}
	var out []netip.Addr
	for i := len(raw) - 1; i >= 0; i-- {
		a, ok := addrOf(strings.TrimSpace(raw[i]))
		if !ok {
			break
		}
		out = append([]netip.Addr{a}, out...)
	}
	return out
}

func addrOf(s string) (netip.Addr, bool) {
	if h, _, err := net.SplitHostPort(s); err == nil {
		s = h
	}
	a, err := netip.ParseAddr(strings.Trim(s, "[]"))
	if err != nil {
		return netip.Addr{}, false
	}
	return a.Unmap().WithZone(""), true
}

// lastValue is the last entry of a comma-separated header (the nearest proxy's).
func lastValue(h string) string {
	if i := strings.LastIndexByte(h, ','); i >= 0 {
		return h[i+1:]
	}
	return h
}

// publicAddr reports whether a client address is on the internet rather than
// a home or tailnet one.
func publicAddr(ip string) bool {
	a, ok := addrOf(ip)
	if !ok {
		return false
	}
	cgnat := netip.MustParsePrefix("100.64.0.0/10") // Tailscale and carrier NAT
	return !(a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast() || a.IsUnspecified() || cgnat.Contains(a))
}

// plainHTTPFromInternet is a sign-in that sends a password or token in the clear
// across the internet.
func plainHTTPFromInternet(r *http.Request) bool {
	return !isHTTPS(r) && publicAddr(clientIP(r))
}
