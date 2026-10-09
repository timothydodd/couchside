package api

import (
	"net/http/httptest"
	"path/filepath"
	"slices"
	"testing"

	"github.com/timothydodd/couchside/internal/config"
	"github.com/timothydodd/couchside/internal/db"
)

// /api/server is open, says which API version the server speaks, and names
// features from real state.
func TestServerInfo(t *testing.T) {
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	s, err := New(d, config.Config{DataDir: dir, ServerName: "Den"}, nil, nil, nil, nil, "0.19.0")
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	var info struct {
		App, Name, Version, SignIn string
		APIVersion                 int `json:"apiVersion"`
		Features                   []string
	}
	if c := newClient(t, ts.URL).do("GET", "/api/server", nil, &info); c != 200 {
		t.Fatalf("GET /api/server signed out = %d", c)
	}
	if info.App != "couchside" || info.Name != "Den" || info.Version != "0.19.0" || info.APIVersion != APIVersion || info.SignIn != "passwordless" {
		t.Fatalf("info = %+v", info)
	}
	for _, want := range []string{"deviceCode", "hls", "passwordless", "trickplay"} {
		if !slices.Contains(info.Features, want) {
			t.Errorf("features %v lack %q", info.Features, want)
		}
	}
	for _, not := range []string{"livetv", "tuner", "commercials", "metadata", "oidc"} {
		if slices.Contains(info.Features, not) {
			t.Errorf("features %v claim %q, which this server lacks", info.Features, not)
		}
	}
	if !slices.IsSorted(info.Features) {
		t.Errorf("features not sorted: %v", info.Features)
	}
}
