package api

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/timothydodd/couchside/internal/db"
)

// A webOS app runs from file:// (Origin "null"): it can read answers, but
// other sites can't, and cookies are never allowed along.
func TestTVAppCORS(t *testing.T) {
	_, ts, admin := passwordlessServer(t)
	var kid db.Profile
	if code := admin.do("POST", "/api/accounts", map[string]any{"name": "Kid"}, &kid); code != 201 {
		t.Fatalf("create = %d", code)
	}
	var tk tokens
	if code := newClient(t, ts.URL).do("POST", "/api/auth/pick", map[string]any{"profileId": kid.ID, "client": "tv"}, &tk); code != 200 {
		t.Fatalf("pick = %d", code)
	}
	send := func(method, path, origin, bearer string, hdr map[string]string) *http.Response {
		t.Helper()
		req, _ := http.NewRequest(method, ts.URL+path, nil)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res
	}

	pre := send("OPTIONS", "/api/home", "null", "", map[string]string{
		"Access-Control-Request-Method": "GET", "Access-Control-Request-Headers": "authorization"})
	if pre.StatusCode != 204 || pre.Header.Get("Access-Control-Allow-Origin") != "null" ||
		pre.Header.Get("Access-Control-Allow-Headers") == "" {
		t.Fatalf("preflight = %d %v", pre.StatusCode, pre.Header)
	}
	if pre.Header.Get("Access-Control-Allow-Credentials") != "" {
		t.Fatal("preflight allows credentials")
	}
	home := send("GET", "/api/home", "null", tk.AccessToken, nil)
	if home.StatusCode != 200 || home.Header.Get("Access-Control-Allow-Origin") != "null" {
		t.Fatalf("home from the app = %d %v", home.StatusCode, home.Header)
	}
	if res := send("GET", "/api/server", "null", "", nil); res.Header.Get("Access-Control-Allow-Origin") != "null" {
		t.Fatalf("/api/server from the app has no CORS: %v", res.Header)
	}
	if res := send("GET", "/api/home", "https://evil.example", tk.AccessToken, nil); res.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("another site may read answers")
	}
	if res := send("OPTIONS", "/api/home", "https://evil.example", "", map[string]string{"Access-Control-Request-Method": "GET"}); res.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("another site passed the preflight")
	}
}

// A <video> can't send a header, so the stream and subtitle routes take the
// access token in the URL; other routes don't.
func TestQueryToken(t *testing.T) {
	_, ts, admin := passwordlessServer(t)
	var kid db.Profile
	if code := admin.do("POST", "/api/accounts", map[string]any{"name": "Kid"}, &kid); code != 201 {
		t.Fatalf("create = %d", code)
	}
	var tk tokens
	if code := newClient(t, ts.URL).do("POST", "/api/auth/pick", map[string]any{"profileId": kid.ID, "client": "tv"}, &tk); code != 200 {
		t.Fatalf("pick = %d", code)
	}
	tv := newClient(t, ts.URL)
	q := "?access_token=" + url.QueryEscape(tk.AccessToken)
	if code := tv.do("GET", "/api/files/999/stream", nil, nil); code != 401 {
		t.Fatalf("stream without a token = %d", code)
	}
	if code := tv.do("GET", "/api/files/999/stream"+q, nil, nil); code == 401 || code == 403 {
		t.Fatalf("stream with a token in the URL = %d", code)
	}
	if code := tv.do("GET", "/api/files/999/subtitles/s0.vtt"+q, nil, nil); code == 401 || code == 403 {
		t.Fatalf("subtitles with a token in the URL = %d", code)
	}
	if code := tv.do("GET", "/api/files/999/stream?access_token=v1.bogus.sig", nil, nil); code != 401 {
		t.Fatalf("stream with a bad token = %d", code)
	}
	if code := tv.do("GET", "/api/home"+q, nil, nil); code != 401 {
		t.Fatalf("home took the token from the URL: %d", code)
	}
}
