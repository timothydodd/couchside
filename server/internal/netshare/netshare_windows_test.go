package netshare

import "testing"

// A sealed password opens again for the same account on the same machine,
// and the sealed bytes don't hold it in the clear.
func TestProtectRoundTrip(t *testing.T) {
	sealed, err := Protect("s3cret pw")
	if err != nil || len(sealed) == 0 {
		t.Fatalf("protect = %v, %d bytes", err, len(sealed))
	}
	if string(sealed) == "s3cret pw" {
		t.Fatal("sealed password is the password")
	}
	if pw, err := Unprotect(sealed); err != nil || pw != "s3cret pw" {
		t.Fatalf("unprotect = %q, %v", pw, err)
	}
	if _, err := Unprotect(append(sealed[:len(sealed)-1:len(sealed)-1], sealed[len(sealed)-1]^0xff)); err == nil {
		t.Fatal("a tampered secret opened")
	}
	if pw, err := Unprotect(nil); err != nil || pw != "" {
		t.Fatalf("no secret = %q, %v", pw, err)
	}
}
