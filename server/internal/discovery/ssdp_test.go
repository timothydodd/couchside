package discovery

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

func search(st string) []byte {
	return []byte("M-SEARCH * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nMAN: \"ssdp:discover\"\r\nMX: 1\r\nST: " + st + "\r\n\r\n")
}

func TestParseSearch(t *testing.T) {
	if st, mx, ok := parseSearch(search(ST)); !ok || st != ST || mx != 1 {
		t.Errorf("Couchside search: %q %d %v", st, mx, ok)
	}
	if _, _, ok := parseSearch(search("ssdp:all")); !ok {
		t.Error("ssdp:all not answered")
	}
	for name, b := range map[string][]byte{
		"other device": search("urn:dial-multiscreen-org:service:dial:1"),
		"notify":       []byte("NOTIFY * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nNT: " + ST + "\r\nNTS: ssdp:alive\r\n\r\n"),
		"no MAN":       []byte("M-SEARCH * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nMX: 1\r\nST: " + ST + "\r\n\r\n"),
		"garbage":      []byte("\x00\x01hello"),
	} {
		if _, _, ok := parseSearch(b); ok {
			t.Errorf("%s was answered", name)
		}
	}
}

func TestResponse(t *testing.T) {
	d := &Responder{ID: "abc", Name: "Den\r\nX-Evil: 1", Version: "v0.11.0", Port: 8080,
		SignIn: func(context.Context) string { return "passwordless" }}
	got := string(d.response(context.Background(), ST, net.ParseIP("192.168.2.63")))
	for _, want := range []string{
		"HTTP/1.1 200 OK\r\n", "LOCATION: http://192.168.2.63:8080/api/discovery\r\n", "ST: " + ST + "\r\n",
		"USN: uuid:abc::" + ST + "\r\n", "X-COUCHSIDE-URL: http://192.168.2.63:8080\r\n", "X-COUCHSIDE-SIGNIN: passwordless\r\n",
		"X-COUCHSIDE-NAME: Den  X-Evil: 1\r\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("response lacks %q:\n%s", want, got)
		}
	}
	d.URL = "http://nas.lan:8095/"
	if got := string(d.response(context.Background(), ST, net.ParseIP("10.0.0.5"))); !strings.Contains(got, "LOCATION: http://nas.lan:8095/api/discovery\r\n") {
		t.Errorf("advertised URL not used:\n%s", got)
	}
}

// A search sent to the group gets an answer back, over real sockets.
func TestRoundTrip(t *testing.T) {
	probe, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero})
	if err != nil {
		t.Skip(err)
	}
	defer probe.Close()
	port := 30000 + int(time.Now().UnixNano()%20000)
	group := "239.255.255.250:" + strconv.Itoa(port)
	d := &Responder{ID: "abc", Name: "Den", Version: "dev", Port: 8080, Group: group}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errs := make(chan error, 1)
	go func() { errs <- d.Run(ctx) }()

	gaddr, _ := net.ResolveUDPAddr("udp4", group)
	buf := make([]byte, 2048)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-errs:
			t.Skipf("no multicast here: %v", err)
		default:
		}
		if _, err := probe.WriteToUDP(search(ST), gaddr); err != nil {
			t.Skipf("can't send to the group: %v", err)
		}
		probe.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
		if n, _, err := probe.ReadFromUDP(buf); err == nil {
			if got := string(buf[:n]); !strings.Contains(got, "USN: uuid:abc::"+ST) {
				t.Fatalf("answer:\n%s", got)
			}
			return
		}
	}
	t.Skip("no answer: multicast loopback isn't available here")
}

// One answer per address per second, and the sign-in mode isn't read for
// every packet.
func TestRepliesAreLimited(t *testing.T) {
	reads := 0
	d := &Responder{SignIn: func(context.Context) string { reads++; return "passwordless" }}
	now := time.Now()
	if !d.admit("192.168.1.20", now) {
		t.Fatal("the first search wasn't answered")
	}
	if d.admit("192.168.1.20", now.Add(100*time.Millisecond)) {
		t.Fatal("a second search from the same address within a second was answered")
	}
	if !d.admit("192.168.1.21", now) {
		t.Fatal("another address was held back")
	}
	if !d.admit("192.168.1.20", now.Add(replyEvery)) {
		t.Fatal("the address wasn't answered again after a second")
	}
	for range 3 {
		d.done()
	}
	// No more than maxReplies at once.
	for i := range maxReplies {
		if !d.admit(fmt.Sprintf("10.0.%d.%d", i/250, i%250), now.Add(time.Minute)) {
			t.Fatalf("reply %d refused below the limit", i)
		}
	}
	if d.admit("10.9.9.9", now.Add(time.Minute)) {
		t.Fatal("a reply was started past the limit")
	}
	for range 5 {
		d.response(context.Background(), ST, net.IPv4(192, 168, 1, 5))
	}
	if reads != 1 {
		t.Fatalf("the sign-in mode was read %d times for 5 replies", reads)
	}
}
