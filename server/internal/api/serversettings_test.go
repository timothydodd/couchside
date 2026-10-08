package api

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/timothydodd/couchside/internal/netshare"
)

// adminWeb sets the server up and returns the first admin's browser.
func adminWeb(t *testing.T, s *Server, base string) *testClient {
	t.Helper()
	web := newClient(t, base)
	web.do("GET", "/api/auth", nil, nil) // makes the setup code
	s.auth.mu.Lock()
	code := s.auth.setupCode
	s.auth.mu.Unlock()
	if c := web.do("POST", "/api/auth/setup", map[string]string{"code": code, "name": "Me", "password": "long enough"}, nil); c != 200 {
		t.Fatalf("setup = %d", c)
	}
	return web
}

type settingsReply struct {
	Settings []serverSetting `json:"settings"`
	Pending  bool            `json:"pending"`
}

func (r settingsReply) get(key string) serverSetting {
	for _, s := range r.Settings {
		if s.Key == key {
			return s
		}
	}
	return serverSetting{}
}

func TestServerSettings(t *testing.T) {
	s, ts := authServer(t)
	web := adminWeb(t, s, ts.URL)
	t.Setenv("COUCHSIDE_MAX_TRANSCODES", "3")
	t.Setenv("OMDB_API_KEY", "from-env")

	var got settingsReply
	if c := web.do("GET", "/api/settings/server", nil, &got); c != 200 {
		t.Fatalf("get = %d", c)
	}
	if m := got.get("COUCHSIDE_MAX_TRANSCODES"); m.Env != "3" || m.Value != nil || got.Pending {
		t.Fatalf("before saving: %+v pending=%v", m, got.Pending)
	}
	if k := got.get("OMDB_API_KEY"); k.Env != "" || !k.EnvSet {
		t.Fatalf("a secret's value left the server: %+v", k)
	}
	if got.get("COUCHSIDE_DATA_DIR").Key != "" || got.get("COUCHSIDE_AUTH").Key != "" {
		t.Fatal("the data folder and the password lock must stay in the environment")
	}

	// A bad value saves nothing, not even the good ones beside it.
	if c := web.do("PUT", "/api/settings/server", map[string]any{"COUCHSIDE_WORKERS": "4", "COUCHSIDE_HWACCEL": "cuda"}, nil); c != 400 {
		t.Fatalf("bad encoder = %d", c)
	}
	if c := web.do("PUT", "/api/settings/server", map[string]any{"COUCHSIDE_AUTH": "false"}, nil); c != 400 {
		t.Fatalf("COUCHSIDE_AUTH from the web = %d", c)
	}
	if c := web.do("PUT", "/api/settings/server", map[string]any{"COUCHSIDE_MEDIA_ROOT": filepath.Join(t.TempDir(), "missing")}, nil); c != 400 {
		t.Fatalf("missing media folder = %d", c)
	}

	media := t.TempDir()
	if c := web.do("PUT", "/api/settings/server", map[string]any{"COUCHSIDE_MAX_TRANSCODES": " 5 ", "COUCHSIDE_MEDIA_ROOT": media, "OMDB_API_KEY": "mine"}, &got); c != 200 {
		t.Fatalf("save = %d", c)
	}
	if m := got.get("COUCHSIDE_MAX_TRANSCODES"); m.Value == nil || *m.Value != "5" || !m.Pending || !got.Pending {
		t.Fatalf("after saving: %+v pending=%v", m, got.Pending)
	}
	if k := got.get("OMDB_API_KEY"); k.Value != nil || !k.Saved {
		t.Fatalf("saved secret: %+v", k)
	}
	if got.get("COUCHSIDE_WORKERS").Saved {
		t.Fatal("a value from the refused request was saved")
	}

	// What the next start-up sees.
	saved, err := s.db.EnvOverrides(t.Context())
	if err != nil || saved["COUCHSIDE_MAX_TRANSCODES"] != "5" || saved["COUCHSIDE_MEDIA_ROOT"] != media || saved["OMDB_API_KEY"] != "mine" {
		t.Fatalf("overrides = %v, %v", saved, err)
	}

	// Started with them: nothing pending. Then null goes back to the environment.
	s.UseRestart(saved, "", nil)
	web.do("GET", "/api/settings/server", nil, &got)
	if got.Pending {
		t.Fatal("pending right after starting with the saved values")
	}
	if c := web.do("PUT", "/api/settings/server", map[string]any{"COUCHSIDE_MAX_TRANSCODES": nil}, &got); c != 200 {
		t.Fatalf("reset = %d", c)
	}
	if m := got.get("COUCHSIDE_MAX_TRANSCODES"); m.Saved || !m.Pending {
		t.Fatalf("after reset: %+v", m)
	}
	if c := web.do("POST", "/api/server/restart", nil, nil); c != 501 {
		t.Fatalf("restart without a way to = %d", c)
	}
}

func TestBrowse(t *testing.T) {
	s, ts := authServer(t)
	web := adminWeb(t, s, ts.URL)
	root := t.TempDir()
	for _, d := range []string{"Movies", "TV", ".hidden", filepath.Join("TV", "Show")} {
		if err := os.Mkdir(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(root, "file.mkv"), nil, 0o644)

	type reply struct {
		Path   string   `json:"path"`
		Parent *string  `json:"parent"`
		Dirs   []folder `json:"dirs"`
	}
	browse := func(q url.Values) (reply, int) {
		var r reply
		c := web.do("GET", "/api/fs?"+q.Encode(), nil, &r)
		return r, c
	}

	// No media folder: browsing starts at the top and goes anywhere.
	r, c := browse(url.Values{})
	if c != 200 || r.Path != "" || r.Parent != nil || len(r.Dirs) == 0 {
		t.Fatalf("start = %d %+v", c, r)
	}
	r, _ = browse(url.Values{"path": {root}})
	if len(r.Dirs) != 2 || r.Dirs[0].Name != "Movies" || r.Dirs[1].Path != filepath.Join(root, "TV") || r.Parent == nil {
		t.Fatalf("root = %+v", r)
	}

	// With one, it starts there and can't leave it, unless choosing the media folder itself.
	s.cfg.MediaRoot = root
	r, _ = browse(url.Values{})
	if r.Path != root || r.Parent != nil {
		t.Fatalf("media folder start = %+v", r)
	}
	r, _ = browse(url.Values{"path": {filepath.Join(root, "TV", "Show")}})
	if r.Parent == nil || *r.Parent != filepath.Join(root, "TV") {
		t.Fatalf("inside = %+v", r)
	}
	if _, c := browse(url.Values{"path": {filepath.Dir(root)}}); c != 400 {
		t.Fatalf("outside the media folder = %d", c)
	}
	if r, c := browse(url.Values{"path": {filepath.Dir(root)}, "all": {"1"}}); c != 200 || r.Path != filepath.Dir(root) {
		t.Fatalf("all=1 outside = %d %+v", c, r)
	}
}

// Off Windows the system mounts shares: the card is hidden and saving is refused.
func TestSharesUnsupportedHere(t *testing.T) {
	if netshare.Supported {
		t.Skip("Windows signs in to shares")
	}
	s, ts := authServer(t)
	web := adminWeb(t, s, ts.URL)
	var got struct {
		Supported bool       `json:"supported"`
		Shares    []shareRow `json:"shares"`
	}
	if c := web.do("GET", "/api/settings/shares", nil, &got); c != 200 || got.Supported || got.Shares == nil {
		t.Fatalf("shares = %d %+v", c, got)
	}
	if c := web.do("PUT", "/api/settings/shares", []map[string]string{{"path": `\\nas\media`, "user": "me", "password": "pw"}}, nil); c != 400 {
		t.Fatalf("save on a platform without shares = %d", c)
	}
}
