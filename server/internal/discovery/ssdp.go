// Package discovery answers SSDP searches on the LAN, so TV apps can find a
// Couchside server without anyone typing its address. Only M-SEARCH is
// answered (no NOTIFY announcements): a setup screen asks when it opens.
package discovery

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// ST is the search target Couchside answers to (and to ssdp:all).
	ST = "urn:couchside-app:device:server:1"
	// Group is SSDP's multicast address.
	Group  = "239.255.255.250:1900"
	maxAge = 1800
)

// Responder answers SSDP M-SEARCH requests for Couchside.
type Responder struct {
	ID      string // stable server id (uuid-ish), the USN
	Name    string // shown in the TV's server list
	Version string
	Port    int    // HTTP port, used when URL is empty
	URL     string // base URL to advertise instead of http://<this machine's address>:<Port>
	// SignIn reports how to sign in: "passwordless" or "password".
	SignIn func(ctx context.Context) string
	// Group and Interface are for tests and multi-homed hosts; empty means
	// SSDP's group on the default multicast interface.
	Group     string
	Interface string
}

// Run answers searches until ctx ends. It returns an error only when it
// can't listen at all (port 1900 taken, no multicast route).
func (d *Responder) Run(ctx context.Context) error {
	group := d.Group
	if group == "" {
		group = Group
	}
	gaddr, err := net.ResolveUDPAddr("udp4", group)
	if err != nil {
		return err
	}
	var ifi *net.Interface
	if d.Interface != "" {
		if ifi, err = net.InterfaceByName(d.Interface); err != nil {
			return fmt.Errorf("discovery interface: %w", err)
		}
	}
	conn, err := net.ListenMulticastUDP("udp4", ifi, gaddr)
	if err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		conn.Close()
	}()
	slog.Info("lan discovery: answering SSDP searches", "group", group, "st", ST, "name", d.Name)
	buf := make([]byte, 2048)
	for {
		n, src, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			slog.Debug("lan discovery: read", "err", err)
			continue
		}
		st, mx, ok := parseSearch(buf[:n])
		if !ok {
			continue
		}
		go d.reply(ctx, src, st, mx)
	}
}

// parseSearch reads an M-SEARCH and returns the target to echo back and its
// MX (how long the asker waits). ok is false for anything that isn't a
// search for Couchside.
func parseSearch(b []byte) (st string, mx int, ok bool) {
	req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(b)))
	if err != nil || req.Method != "M-SEARCH" {
		return "", 0, false
	}
	if !strings.EqualFold(strings.Trim(req.Header.Get("MAN"), `"`), "ssdp:discover") {
		return "", 0, false
	}
	st = strings.TrimSpace(req.Header.Get("ST"))
	if st != ST && st != "ssdp:all" {
		return "", 0, false
	}
	mx, _ = strconv.Atoi(strings.TrimSpace(req.Header.Get("MX")))
	return st, mx, true
}

func (d *Responder) reply(ctx context.Context, to *net.UDPAddr, st string, mx int) {
	// Answer within MX, spread out as SSDP asks, but never make a TV wait long.
	if wait := min(max(mx, 0), 1); wait > 0 {
		time.Sleep(time.Duration(rand.Int64N(int64(wait) * int64(time.Second))))
	}
	conn, err := net.DialUDP("udp4", nil, to)
	if err != nil {
		slog.Debug("lan discovery: reply", "to", to, "err", err)
		return
	}
	defer conn.Close()
	// The address the asker can reach us on is the one we send from.
	local := conn.LocalAddr().(*net.UDPAddr).IP
	_, _ = conn.Write(d.response(ctx, st, local))
}

// response builds the reply to a search, advertising base URL local:Port
// unless URL is set.
func (d *Responder) response(ctx context.Context, st string, local net.IP) []byte {
	base := strings.TrimRight(d.URL, "/")
	if base == "" {
		base = "http://" + net.JoinHostPort(local.String(), strconv.Itoa(d.Port))
	}
	signIn := "password"
	if d.SignIn != nil {
		signIn = d.SignIn(ctx)
	}
	var b strings.Builder
	b.WriteString("HTTP/1.1 200 OK\r\n")
	fmt.Fprintf(&b, "CACHE-CONTROL: max-age=%d\r\n", maxAge)
	b.WriteString("EXT:\r\n")
	fmt.Fprintf(&b, "LOCATION: %s/api/discovery\r\n", base)
	fmt.Fprintf(&b, "SERVER: Couchside/%s UPnP/1.1\r\n", clean(d.Version))
	fmt.Fprintf(&b, "ST: %s\r\n", st)
	fmt.Fprintf(&b, "USN: uuid:%s::%s\r\n", d.ID, ST)
	fmt.Fprintf(&b, "X-COUCHSIDE-NAME: %s\r\n", clean(d.Name))
	fmt.Fprintf(&b, "X-COUCHSIDE-VERSION: %s\r\n", clean(d.Version))
	fmt.Fprintf(&b, "X-COUCHSIDE-URL: %s\r\n", base)
	fmt.Fprintf(&b, "X-COUCHSIDE-SIGNIN: %s\r\n", signIn)
	b.WriteString("\r\n")
	return []byte(b.String())
}

// clean keeps a header value on one line.
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' {
			return ' '
		}
		return r
	}, s)
}
