package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRealIP(t *testing.T) {
	proxies, err := parseProxies([]string{"10.42.0.0/16", "192.168.1.5"})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{proxies: proxies}
	for _, c := range []struct {
		name, peer, xff, realIP, proto string
		wantIP                         string
		wantHTTPS                      bool
	}{
		{"untrusted peer: headers ignored", "203.0.113.9:5000", "1.2.3.4", "5.6.7.8", "https", "203.0.113.9", false},
		{"trusted proxy", "10.42.0.7:5000", "198.51.100.20", "", "https", "198.51.100.20", true},
		{"client-supplied entries left of ours are skipped", "10.42.0.7:5000", "1.2.3.4, 198.51.100.20", "", "http", "198.51.100.20", false},
		{"chain of trusted proxies", "192.168.1.5:5000", "198.51.100.20, 10.42.0.3", "", "https", "198.51.100.20", true},
		{"all trusted: furthest one", "10.42.0.7:5000", "10.42.0.9", "", "", "10.42.0.9", false},
		{"garbage stops the walk", "10.42.0.7:5000", "1.2.3.4, nonsense, 198.51.100.20", "", "", "198.51.100.20", false},
		{"X-Real-IP from a trusted proxy", "10.42.0.7:5000", "", "198.51.100.30", "", "198.51.100.30", false},
		{"no headers", "10.42.0.7:5000", "", "", "", "10.42.0.7", false},
	} {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = c.peer
		if c.xff != "" {
			r.Header.Set("X-Forwarded-For", c.xff)
		}
		if c.realIP != "" {
			r.Header.Set("X-Real-IP", c.realIP)
		}
		if c.proto != "" {
			r.Header.Set("X-Forwarded-Proto", c.proto)
		}
		var gotIP string
		var gotHTTPS bool
		s.realIP(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			gotIP, gotHTTPS = clientIP(r), isHTTPS(r)
		})).ServeHTTP(httptest.NewRecorder(), r)
		if gotIP != c.wantIP || gotHTTPS != c.wantHTTPS {
			t.Errorf("%s: ip %s https %v, want %s %v", c.name, gotIP, gotHTTPS, c.wantIP, c.wantHTTPS)
		}
	}
	if _, err := parseProxies([]string{"10.0.0.0/33"}); err == nil {
		t.Error("bad CIDR accepted")
	}
}

func TestPublicAddr(t *testing.T) {
	for ip, want := range map[string]bool{
		"8.8.8.8": true, "2001:4860::8888": true,
		"127.0.0.1": false, "192.168.1.2": false, "10.1.2.3": false, "100.101.102.103": false, "fe80::1": false, "::1": false,
	} {
		if got := publicAddr(ip); got != want {
			t.Errorf("publicAddr(%s) = %v", ip, got)
		}
	}
}
