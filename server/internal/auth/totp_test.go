package auth

import (
	"testing"
	"time"
)

// The RFC 6238 test vectors (SHA-1, the ASCII secret "12345678901234567890"),
// cut to six digits.
func TestTOTPVectors(t *testing.T) {
	secret := b32.EncodeToString([]byte("12345678901234567890"))
	for unix, want := range map[int64]string{59: "287082", 1111111109: "081804", 1111111111: "050471", 1234567890: "005924", 2000000000: "279037"} {
		got, err := totpAt(secret, unix/totpStep)
		if err != nil || got != want {
			t.Errorf("at %d: %s (%v), want %s", unix, got, err, want)
		}
	}
}

func TestVerifyTOTP(t *testing.T) {
	secret := NewTOTPSecret()
	now := time.Unix(1_700_000_000, 0)
	step := now.Unix() / totpStep
	code, _ := totpAt(secret, step)
	if s, ok := VerifyTOTP(secret, " "+code[:3]+" "+code[3:], now); !ok || s != step {
		t.Fatalf("the current code, typed with a space: step %d ok=%v", s, ok)
	}
	prev, _ := totpAt(secret, step-1)
	if s, ok := VerifyTOTP(secret, prev, now); !ok || s != step-1 {
		t.Fatalf("the previous step's code: step %d ok=%v", s, ok)
	}
	old, _ := totpAt(secret, step-3)
	if _, ok := VerifyTOTP(secret, old, now); ok {
		t.Fatal("a code from 90 seconds ago was accepted")
	}
	for _, bad := range []string{"", "12345", "1234567", "abcdef"} {
		if _, ok := VerifyTOTP(secret, bad, now); ok {
			t.Fatalf("%q was accepted", bad)
		}
	}
	if _, ok := VerifyTOTP("not base32!", code, now); ok {
		t.Fatal("a broken secret verified a code")
	}
}
