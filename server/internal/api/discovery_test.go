package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/timothydodd/couchside/internal/config"
	"github.com/timothydodd/couchside/internal/db"
)

func TestDiscoveryEndpoint(t *testing.T) {
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	s, err := New(d, config.Config{DataDir: dir, ServerID: "abc", ServerName: "Den"}, nil, nil, nil, nil, "v1")
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/api/discovery", nil)) // no session needed
	var got map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if rec.Code != 200 || got["id"] != "abc" || got["name"] != "Den" || got["signIn"] != "passwordless" || got["app"] != "couchside" {
		t.Fatalf("discovery = %d %v", rec.Code, got)
	}
	if s.SignInMode(context.Background()) != "passwordless" {
		t.Fatal("a server with no passwords should be passwordless")
	}
}
