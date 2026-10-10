package api

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// fakeProvider is a small OpenID Connect provider: it "signs in" whoever the
// test says, and hands out an ID token for them.
type fakeProvider struct {
	srv      *httptest.Server
	claims   map[string]any // added to the ID token
	lastForm url.Values     // the last token request
	nonce    string
}

func newProvider(t *testing.T) *fakeProvider {
	p := &fakeProvider{claims: map[string]any{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"issuer": p.srv.URL, "authorization_endpoint": p.srv.URL + "/authorize", "token_endpoint": p.srv.URL + "/token"})
	})
	mux.HandleFunc("/authorize", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		p.nonce = q.Get("nonce")
		http.Redirect(w, r, q.Get("redirect_uri")+"?code=the-code&state="+url.QueryEscape(q.Get("state")), http.StatusFound)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		p.lastForm = r.PostForm
		claims := map[string]any{"iss": p.srv.URL, "aud": r.PostForm.Get("client_id"), "exp": time.Now().Add(time.Minute).Unix(), "nonce": p.nonce}
		for k, v := range p.claims {
			claims[k] = v
		}
		body, _ := json.Marshal(claims)
		tok := "e30." + base64.RawURLEncoding.EncodeToString(body) + ".sig"
		json.NewEncoder(w).Encode(map[string]string{"id_token": tok, "access_token": "x"})
	})
	p.srv = httptest.NewServer(mux)
	t.Cleanup(p.srv.Close)
	return p
}

// A browser follows the whole flow: Couchside → provider → Couchside, and
// ends up signed in as the profile the provider named.
func TestOIDCSignIn(t *testing.T) {
	_, ts, admin := passwordlessServer(t)
	p := newProvider(t)
	if code := admin.do("PUT", "/api/settings/oidc", map[string]any{"issuer": p.srv.URL + "/", "clientId": "couchside", "clientSecret": "shh", "claim": "", "label": "Sign in with Auth"}, nil); code != 204 {
		t.Fatalf("settings = %d", code)
	}
	var got struct {
		Config struct {
			Issuer, ClientID, ClientSecret, Claim, Label string
			HasSecret                                    bool
		}
		RedirectURI string
	}
	if code := admin.do("GET", "/api/settings/oidc", nil, &got); code != 200 || got.Config.ClientSecret != "" || !got.Config.HasSecret ||
		got.Config.Claim != "preferred_username" || got.RedirectURI != ts.URL+"/api/auth/oidc/callback" {
		t.Fatalf("settings back = %d %+v", code, got)
	}
	var info struct{ OIDC string }
	anon := newClient(t, ts.URL)
	if code := anon.do("GET", "/api/auth", nil, &info); code != 200 || info.OIDC != "Sign in with Auth" {
		t.Fatalf("auth info = %d %+v", code, info)
	}
	if code := admin.do("POST", "/api/accounts", map[string]any{"name": "Kid"}, nil); code != 201 {
		t.Fatalf("create = %d", code)
	}

	signIn := func(c *testClient) string { // returns where the browser ends up
		t.Helper()
		res, err := c.http.Get(ts.URL + "/api/auth/oidc/start")
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.Request.URL.RequestURI()
	}
	// The provider says this is "kid" (any case): signed in as Kid.
	p.claims["preferred_username"] = "kid"
	kid := newClient(t, ts.URL)
	if at := signIn(kid); at != "/" {
		t.Fatalf("after signing in the browser is at %s", at)
	}
	var me struct{ User struct{ Name string } }
	if code := kid.do("GET", "/api/auth", nil, &me); code != 200 || me.User.Name != "Kid" {
		t.Fatalf("signed in as %+v (%d)", me, code)
	}
	if p.lastForm.Get("client_secret") != "shh" || p.lastForm.Get("code_verifier") == "" || p.lastForm.Get("code") != "the-code" {
		t.Fatalf("token request: %v", p.lastForm)
	}

	// Someone with no profile here, and profiles aren't made automatically.
	p.claims["preferred_username"] = "stranger"
	stranger := newClient(t, ts.URL)
	if at := signIn(stranger); !strings.HasPrefix(at, "/?signin_error=") {
		t.Fatalf("a stranger ended up at %s", at)
	}
	if code := stranger.do("GET", "/api/home", nil, nil); code != 401 {
		t.Fatalf("a stranger's /api/home = %d", code)
	}
	// With "create" on, they get a profile; in the admin group, an admin one.
	if code := admin.do("PUT", "/api/settings/oidc", map[string]any{"issuer": p.srv.URL, "clientId": "couchside", "create": true, "adminGroup": "media-admins"}, nil); code != 204 {
		t.Fatalf("settings = %d", code)
	}
	p.claims["preferred_username"], p.claims["groups"] = "Newcomer", []string{"users"}
	if at := signIn(stranger); at != "/" {
		t.Fatalf("a newcomer ended up at %s", at)
	}
	var who struct {
		User struct {
			Name, Role     string
			PasswordLocked bool
		}
	}
	stranger.do("GET", "/api/auth", nil, &who)
	if who.User.Name != "Newcomer" || who.User.Role != "user" || !who.User.PasswordLocked {
		t.Fatalf("the new profile: %+v", who)
	}
	if p.lastForm.Get("client_secret") != "shh" {
		t.Fatal("saving the settings without a secret forgot the stored one")
	}
	p.claims["preferred_username"], p.claims["groups"] = "Boss", []string{"users", "media-admins"}
	boss := newClient(t, ts.URL)
	signIn(boss)
	boss.do("GET", "/api/auth", nil, &who)
	if who.User.Name != "Boss" || who.User.Role != "admin" {
		t.Fatalf("someone in the admin group: %+v", who)
	}

	// A callback nobody started, and a token for another sign-in, are refused.
	res, _ := newClient(t, ts.URL).http.Get(ts.URL + "/api/auth/oidc/callback?code=x&state=made-up")
	res.Body.Close()
	if !strings.HasPrefix(res.Request.URL.RequestURI(), "/?signin_error=") {
		t.Fatalf("a made-up state ended at %s", res.Request.URL.RequestURI())
	}
	p.claims["nonce"] = "someone-else's"
	replay := newClient(t, ts.URL)
	if at := signIn(replay); !strings.HasPrefix(at, "/?signin_error=") {
		t.Fatalf("a token with the wrong nonce ended at %s", at)
	}
	delete(p.claims, "nonce")

	// Turned off: the button and the routes go away.
	if code := admin.do("PUT", "/api/settings/oidc", map[string]any{"issuer": ""}, nil); code != 204 {
		t.Fatalf("turn off = %d", code)
	}
	anon.do("GET", "/api/auth", nil, &info)
	if code := anon.do("GET", "/api/auth/oidc/start", nil, nil); code != 404 {
		t.Fatalf("start with single sign-on off = %d", code)
	}
}

// A TV shows a code; someone signed in enters it; the TV is signed in as them.
func TestDeviceCodeSignIn(t *testing.T) {
	_, ts, admin := passwordlessServer(t)
	tv := newClient(t, ts.URL)
	var start struct {
		DeviceCode, UserCode, VerifyPath string
		ExpiresIn, Interval              int
	}
	if code := tv.do("POST", "/api/auth/device", map[string]string{"device": "Living room Roku"}, &start); code != 200 || len(start.UserCode) != 9 || start.DeviceCode == "" {
		t.Fatalf("start = %d %+v", code, start)
	}
	var pending map[string]string
	if code := tv.do("POST", "/api/auth/device/token", map[string]string{"deviceCode": start.DeviceCode}, &pending); code != 428 || pending["code"] != "authorization_pending" {
		t.Fatalf("before approval = %d %v", code, pending)
	}
	// Approving needs a session, and the right code.
	if code := newClient(t, ts.URL).do("POST", "/api/auth/device/approve", map[string]string{"userCode": start.UserCode}, nil); code != 401 {
		t.Fatalf("approve without signing in = %d", code)
	}
	if code := admin.do("POST", "/api/auth/device/approve", map[string]string{"userCode": "AAAA-AAAA"}, nil); code != 404 {
		t.Fatalf("a wrong code = %d", code)
	}
	typed := strings.ToLower(strings.ReplaceAll(start.UserCode, "-", " "))
	var ok map[string]string
	if code := admin.do("POST", "/api/auth/device/approve", map[string]string{"userCode": typed}, &ok); code != 200 || ok["device"] != "Living room Roku" {
		t.Fatalf("approve = %d %v", code, ok)
	}
	var tk tokens
	if code := tv.do("POST", "/api/auth/device/token", map[string]string{"deviceCode": start.DeviceCode}, &tk); code != 200 || tk.AccessToken == "" || tk.User.ID != 1 {
		t.Fatalf("after approval = %d %+v", code, tk)
	}
	tv.bearer = tk.AccessToken
	if code := tv.do("GET", "/api/home", nil, nil); code != 200 {
		t.Fatalf("the TV's session = %d", code)
	}
	// The code is spent.
	if code := tv.do("POST", "/api/auth/device/token", map[string]string{"deviceCode": start.DeviceCode}, nil); code != 404 {
		t.Fatalf("a second session from one code = %d", code)
	}
}

func TestOnHomeNetwork(t *testing.T) {
	for host, want := range map[string]bool{
		"127.0.0.1": true, "192.168.1.5": true, "10.0.0.2": true, "localhost": true, "authentik": true,
		"auth.home.lan": true, "sso.home.arpa": true, "idp.local": true,
		"auth.example.com": false, "203.0.113.4": false, "accounts.google.com": false,
	} {
		if got := onHomeNetwork(host); got != want {
			t.Errorf("onHomeNetwork(%q) = %v, want %v", host, got, want)
		}
	}
}
