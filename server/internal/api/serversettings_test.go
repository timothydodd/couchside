package api

import (
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/timothydodd/couchside/internal/config"
	"github.com/timothydodd/couchside/internal/transcode"
	"github.com/timothydodd/couchside/internal/worker"
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
	if c := web.do("PUT", "/api/settings/server", map[string]any{"COUCHSIDE_MEDIA_ROOT": t.TempDir()}, nil); c != 400 {
		t.Fatalf("media folder as a setting = %d (media locations have their own list)", c)
	}

	if c := web.do("PUT", "/api/settings/server", map[string]any{"COUCHSIDE_MAX_TRANSCODES": " 5 ", "OMDB_API_KEY": "mine"}, &got); c != 200 {
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
	if err != nil || saved["COUCHSIDE_MAX_TRANSCODES"] != "5" || saved["OMDB_API_KEY"] != "mine" {
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

	// No locations yet: nothing to browse but the whole server, for adding one.
	if r, c := browse(url.Values{}); c != 200 || r.Path != "" || len(r.Dirs) != 0 {
		t.Fatalf("start with no locations = %d %+v", c, r)
	}
	if _, c := browse(url.Values{"path": {root}}); c != 400 {
		t.Fatalf("outside every location = %d", c)
	}
	r, _ := browse(url.Values{"all": {"1"}})
	if len(r.Dirs) == 0 {
		t.Fatalf("all=1 start = %+v", r)
	}
	r, _ = browse(url.Values{"path": {root}, "all": {"1"}})
	if len(r.Dirs) != 2 || r.Dirs[0].Name != "Movies" || r.Dirs[1].Path != filepath.Join(root, "TV") || r.Parent == nil {
		t.Fatalf("all=1 root = %+v", r)
	}

	// With a location: it's the starting point, and browsing stays inside it.
	if c := web.do("POST", "/api/media/locations", map[string]string{"path": root}, nil); c != 200 {
		t.Fatalf("add location = %d", c)
	}
	r, _ = browse(url.Values{})
	if len(r.Dirs) != 1 || r.Dirs[0].Path != root {
		t.Fatalf("start = %+v", r)
	}
	r, _ = browse(url.Values{"path": {root}})
	if r.Parent == nil || *r.Parent != "" || len(r.Dirs) != 2 {
		t.Fatalf("a location's own folder = %+v (up goes back to the list)", r)
	}
	r, _ = browse(url.Values{"path": {filepath.Join(root, "TV", "Show")}})
	if r.Parent == nil || *r.Parent != filepath.Join(root, "TV") {
		t.Fatalf("inside = %+v", r)
	}
	if _, c := browse(url.Values{"path": {filepath.Dir(root)}}); c != 400 {
		t.Fatalf("outside the location = %d", c)
	}
}

type locationsReply struct {
	Locations []locationRow `json:"locations"`
	Shares    bool          `json:"shares"`
}

func TestMediaLocations(t *testing.T) {
	s, ts := authServer(t)
	web := adminWeb(t, s, ts.URL)
	media, other := t.TempDir(), t.TempDir()
	os.Mkdir(filepath.Join(media, "Movies"), 0o755)
	s.cfg.MediaRoots = []string{other}                                     // from COUCHSIDE_MEDIA_ROOT
	s.worker = worker.New(s.db, config.Config{}, nil, transcode.Encoder{}) // adding a library queues its scan

	var got locationsReply
	for _, bad := range []map[string]string{
		{"path": "relative"},
		{"path": filepath.Join(media, "missing")},
		{"path": other},                                 // the environment's already
		{"path": media, "user": "me", "password": "pw"}, // a sign-in is for a Windows share
	} {
		if c := web.do("POST", "/api/media/locations", bad, nil); c != 400 {
			t.Errorf("add %v = %d", bad, c)
		}
	}
	if c := web.do("POST", "/api/media/locations", map[string]string{"path": media + string(filepath.Separator)}, &got); c != 200 {
		t.Fatalf("add = %d", c)
	}
	if len(got.Locations) != 2 || !got.Locations[0].FromEnv || got.Locations[1].Path != media || !got.Locations[1].Found || got.Locations[1].ID == 0 {
		t.Fatalf("locations = %+v", got.Locations)
	}
	if c := web.do("POST", "/api/media/locations", map[string]string{"path": filepath.Join(media, "Movies")}, nil); c != 400 {
		t.Fatalf("a folder inside a location = %d", c)
	}

	// Libraries must be in a location; a location with one in it can't go.
	if c := web.do("POST", "/api/libraries", map[string]string{"name": "Elsewhere", "kind": "movies", "path": t.TempDir()}, nil); c != 400 {
		t.Fatalf("library outside the locations = %d", c)
	}
	if c := web.do("POST", "/api/libraries", map[string]string{"name": "Movies", "kind": "movies", "path": filepath.Join(media, "Movies")}, nil); c != 201 && c != 200 {
		t.Fatalf("library in a location = %d", c)
	}
	id := strconv.FormatInt(got.Locations[1].ID, 10)
	if c := web.do("DELETE", "/api/media/locations/"+id, nil, nil); c != 400 {
		t.Fatalf("removing a location with a library in it = %d", c)
	}
}

// Upgrading: the installer's media folder and existing libraries' folders
// become locations, once, so nothing that worked before is refused.
func TestPrepareLocations(t *testing.T) {
	s, _ := authServer(t)
	ctx := t.Context()
	a, b := t.TempDir(), t.TempDir()
	for _, p := range []string{a, filepath.Join(a, "Kids"), b} {
		os.MkdirAll(p, 0o755)
		if _, err := s.db.CreateLibrary(ctx, filepath.Base(p), p, "movies"); err != nil {
			t.Fatal(err)
		}
	}
	PrepareLocations(ctx, s.db, config.Config{})
	locs, _ := s.db.MediaLocations(ctx)
	if len(locs) != 2 {
		t.Fatalf("locations from libraries = %+v (Kids is inside the first)", locs)
	}
	// Once only: removing one later sticks, and nothing more is carried over.
	s.db.DeleteMediaLocation(ctx, locs[0].ID)
	PrepareLocations(ctx, s.db, config.Config{MediaRootsFromFile: []string{"/from/file"}})
	if locs, _ = s.db.MediaLocations(ctx); len(locs) != 1 {
		t.Fatalf("second start = %+v", locs)
	}
}

// Browsing the server's folders and its settings are for admins only: the
// path checks in browse and Setting.Check rely on it.
func TestServerPagesAreAdminOnly(t *testing.T) {
	s, ts := authServer(t)
	admin := adminWeb(t, s, ts.URL)
	if c := admin.do("POST", "/api/accounts", map[string]any{"name": "Kid", "password": "a long password", "role": "user"}, nil); c != 201 {
		t.Fatalf("create user = %d", c)
	}
	kid := newClient(t, ts.URL)
	if c := kid.do("POST", "/api/auth/login", map[string]string{"name": "Kid", "password": "a long password"}, nil); c != 200 {
		t.Fatalf("user sign-in = %d", c)
	}
	if c := kid.do("POST", "/api/auth/password", map[string]string{"current": "a long password", "password": "another long password"}, nil); c != 200 && c != 204 {
		t.Fatalf("change temporary password = %d", c)
	}
	if c := kid.do("GET", "/api/home", nil, nil); c != 200 {
		t.Fatalf("user's home = %d: the 403s below must come from the admin check", c)
	}
	for _, r := range []struct{ method, path string }{
		{"GET", "/api/fs"},
		{"GET", "/api/fs?path=/&all=1"},
		{"GET", "/api/settings/server"},
		{"PUT", "/api/settings/server"},
		{"POST", "/api/server/restart"},
		{"GET", "/api/media/locations"},
		{"POST", "/api/media/locations"},
		{"PUT", "/api/media/locations/1"},
		{"DELETE", "/api/media/locations/1"},
		{"POST", "/api/setup/complete"},
	} {
		if c := kid.do(r.method, r.path, map[string]any{}, nil); c != 403 {
			t.Errorf("%s %s as a user = %d, want 403", r.method, r.path, c)
		}
	}
	if c := newClient(t, ts.URL).do("GET", "/api/fs", nil, nil); c != 401 {
		t.Errorf("GET /api/fs signed out = %d, want 401", c)
	}
}
