package auth

import (
	"strings"
	"testing"
	"time"
)

func TestPasswordHash(t *testing.T) {
	h, err := HashPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("hash = %q", h)
	}
	if ok, rehash := VerifyPassword(h, "correct horse"); !ok || rehash {
		t.Fatalf("verify = %v, %v", ok, rehash)
	}
	if ok, _ := VerifyPassword(h, "correct horsE"); ok {
		t.Fatal("wrong password accepted")
	}
	h2, _ := HashPassword("correct horse")
	if h2 == h {
		t.Fatal("salts should differ")
	}
	if ok, _ := VerifyPassword("", "anything"); ok {
		t.Fatal("empty hash accepted")
	}
	if ok, _ := VerifyPassword("$argon2id$v=19$m=999999999,t=1,p=1$AAAAAAAAAAA$AAAA", "x"); ok {
		t.Fatal("absurd parameters accepted")
	}
}

func TestOldParamsWantRehash(t *testing.T) {
	saved := current
	current = params{memory: 8 * 1024, time: 1, threads: 1}
	h, _ := HashPassword("pass phrase")
	current = saved
	if ok, rehash := VerifyPassword(h, "pass phrase"); !ok || !rehash {
		t.Fatalf("verify = %v, %v; want ok and rehash", ok, rehash)
	}
}

func TestCheckPassword(t *testing.T) {
	if CheckPassword("short") != ErrPasswordShort {
		t.Error("short password allowed")
	}
	if CheckPassword("        x") != ErrPasswordShort {
		t.Error("spaces shouldn't count")
	}
	if CheckPassword(strings.Repeat("a", MaxPassword+1)) != ErrPasswordLong {
		t.Error("huge password allowed")
	}
	if CheckPassword("long enough") != nil {
		t.Error("good password refused")
	}
}

func TestSigner(t *testing.T) {
	s := NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	now := time.Unix(1_000_000, 0)
	tok := s.Sign(Claims{Session: "abc", Profile: 3, Expires: now.Add(time.Minute).Unix()})
	c, err := s.Verify(tok, now)
	if err != nil || c.Session != "abc" || c.Profile != 3 {
		t.Fatalf("verify = %+v, %v", c, err)
	}
	if _, err := s.Verify(tok, now.Add(2*time.Minute)); err == nil {
		t.Error("expired token accepted")
	}
	other := NewSigner([]byte("another key another key another k"))
	if _, err := other.Verify(tok, now); err == nil {
		t.Error("token from another key accepted")
	}
	// Change the claims but keep the signature.
	forged := s.Sign(Claims{Session: "abc", Profile: 1, Expires: now.Add(time.Minute).Unix()})
	parts := strings.Split(tok, ".")
	fparts := strings.Split(forged, ".")
	if _, err := s.Verify(parts[0]+"."+fparts[1]+"."+parts[2], now); err == nil {
		t.Error("tampered claims accepted")
	}
	for _, bad := range []string{"", "v1.", "v1..", "garbage", "v2." + parts[1] + "." + parts[2]} {
		if _, err := s.Verify(bad, now); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestLoadKey(t *testing.T) {
	p := t.TempDir() + "/auth.key"
	k1, err := LoadKey(p)
	if err != nil || len(k1) != 32 {
		t.Fatalf("LoadKey = %d bytes, %v", len(k1), err)
	}
	k2, _ := LoadKey(p)
	if string(k1) != string(k2) {
		t.Fatal("key should persist")
	}
}

func TestTokens(t *testing.T) {
	a, b := NewToken(), NewToken()
	if a == b || len(a) != 43 {
		t.Fatalf("tokens %q %q", a, b)
	}
	if HashToken(a) == a || len(HashToken(a)) != 64 {
		t.Fatal("bad token hash")
	}
	c := SetupCode()
	if len(c) != 14 || c[4] != '-' || c[9] != '-' {
		t.Fatalf("setup code %q", c)
	}
	if !SameCode(c, strings.ToLower(strings.ReplaceAll(c, "-", ""))) || SameCode(c, SetupCode()) {
		t.Fatal("SameCode")
	}
}

func TestLimiter(t *testing.T) {
	now := time.Unix(0, 0)
	l := NewLimiter(3, time.Second, 8*time.Second, time.Minute)
	l.now = func() time.Time { return now }
	for i := 0; i < 3; i++ {
		if d := l.Fail("ip"); d != 0 {
			t.Fatalf("free failure %d locked for %v", i, d)
		}
	}
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 8 * time.Second}
	for _, w := range want {
		if d := l.Fail("ip"); d != w {
			t.Fatalf("lock = %v, want %v", d, w)
		}
	}
	if l.Wait("ip") != 8*time.Second || l.Wait("other") != 0 {
		t.Fatal("Wait")
	}
	now = now.Add(9 * time.Second)
	if l.Wait("ip") != 0 {
		t.Fatal("lock should have passed")
	}
	now = now.Add(2 * time.Minute) // past the window: forgotten
	if d := l.Fail("ip"); d != 0 {
		t.Fatalf("failures should be forgotten, locked %v", d)
	}
	l.Success("ip")
	if len(l.keys) != 0 {
		t.Fatal("Success should forget the key")
	}
}
