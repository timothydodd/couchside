package api

import (
	"testing"
	"time"

	"github.com/timothydodd/couchside/internal/auth"
)

// codeFor is what the authenticator app shows now for a secret. Verification
// accepts the neighbouring steps, so the test's clock needn't line up.
func codeFor(t *testing.T, secret string) string {
	t.Helper()
	for i := 0; i < 1_000_000; i++ {
		c := []byte("000000")
		for j, n := 5, i; j >= 0; j, n = j-1, n/10 {
			c[j] = byte('0' + n%10)
		}
		if _, ok := auth.VerifyTOTP(secret, string(c), time.Now()); ok {
			return string(c)
		}
	}
	t.Fatal("no code verifies")
	return ""
}

func TestTwoStepSignIn(t *testing.T) {
	s, ts, admin := passwordlessServer(t)
	s.auth.byName = auth.NewLimiter(1000, time.Minute, time.Minute, time.Minute)
	s.auth.byIP = auth.NewLimiter(1000, time.Minute, time.Minute, time.Minute)
	if code := admin.do("POST", "/api/accounts", map[string]any{"name": "Kid", "password": "kid password"}, nil); code != 201 {
		t.Fatalf("create = %d", code)
	}
	login := func(extra map[string]string) (int, map[string]any, *testClient) {
		c := newClient(t, ts.URL)
		body := map[string]string{"name": "Kid", "password": "kid password"}
		for k, v := range extra {
			body[k] = v
		}
		var out map[string]any
		code := c.do("POST", "/api/auth/login", body, &out)
		return code, out, c
	}
	_, _, kid := login(nil)
	// A temporary password has to be replaced before anything else.
	if code := kid.do("POST", "/api/auth/password", map[string]string{"current": "kid password", "password": "kid's own password"}, nil); code != 204 && code != 200 {
		t.Fatalf("change password = %d", code)
	}
	login = func(extra map[string]string) (int, map[string]any, *testClient) {
		c := newClient(t, ts.URL)
		body := map[string]string{"name": "Kid", "password": "kid's own password"}
		for k, v := range extra {
			body[k] = v
		}
		var out map[string]any
		code := c.do("POST", "/api/auth/login", body, &out)
		return code, out, c
	}
	code, _, kid := login(nil)
	if code != 200 {
		t.Fatalf("login before two-step = %d", code)
	}

	// Enrol: a secret, then a code from it turns two-step on and gives recovery codes.
	var setup struct{ Secret, URI string }
	if code := kid.do("POST", "/api/auth/totp/setup", nil, &setup); code != 200 || setup.Secret == "" {
		t.Fatalf("setup = %d %+v", code, setup)
	}
	if code, _, _ := login(nil); code != 200 {
		t.Fatalf("login while enrolment is unfinished = %d: it must still work with just the password", code)
	}
	if code := kid.do("POST", "/api/auth/totp/enable", map[string]string{"code": "000000"}, nil); code != 400 {
		t.Fatalf("enable with a wrong code = %d", code)
	}
	var enabled struct{ RecoveryCodes []string }
	app := codeFor(t, setup.Secret)
	if code := kid.do("POST", "/api/auth/totp/enable", map[string]string{"code": app}, &enabled); code != 200 || len(enabled.RecoveryCodes) != 8 {
		t.Fatalf("enable = %d %+v", code, enabled)
	}

	// Signing in now: the password alone asks for the code; a wrong code and
	// the code just used (a replay) are refused.
	if code, out, _ := login(nil); code != 401 || out["code"] != "totp_required" {
		t.Fatalf("password only = %d %v", code, out)
	}
	if code, out, _ := login(map[string]string{"code": "123456"}); code != 401 || out["code"] != "bad_code" {
		t.Fatalf("wrong code = %d %v", code, out)
	}
	if code, out, _ := login(map[string]string{"code": app}); code != 401 {
		t.Fatalf("the code used to enrol, again = %d %v", code, out)
	}
	if code, _, _ := login(map[string]string{"password": "wrong", "code": enabled.RecoveryCodes[0]}); code != 401 {
		t.Fatalf("a recovery code with the wrong password = %d", code)
	}
	// A recovery code works once, typed any way.
	rc := enabled.RecoveryCodes[0]
	if code, out, _ := login(map[string]string{"code": " " + rc[:4] + rc[5:] + " "}); code != 200 {
		t.Fatalf("recovery code = %d %v", code, out)
	}
	if code, _, _ := login(map[string]string{"code": rc}); code != 401 {
		t.Fatalf("the same recovery code again = %d", code)
	}
	var st struct {
		Enabled           bool
		RecoveryCodesLeft int
	}
	if code := kid.do("GET", "/api/auth/totp", nil, &st); code != 200 || !st.Enabled || st.RecoveryCodesLeft != 7 {
		t.Fatalf("status = %d %+v", code, st)
	}

	// An admin can switch it off for someone locked out; then the password is enough.
	if code := admin.do("POST", "/api/accounts/2/totp/reset", nil, nil); code != 204 {
		t.Fatalf("admin reset = %d", code)
	}
	if code, _, _ := login(nil); code != 200 {
		t.Fatalf("login after the reset = %d", code)
	}
	// A profile without a password can't enrol.
	if code := admin.do("POST", "/api/auth/totp/setup", nil, nil); code != 400 {
		t.Fatalf("setup without a password = %d", code)
	}
}
