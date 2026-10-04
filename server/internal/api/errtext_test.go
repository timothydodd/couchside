package api

import (
	"errors"
	"io"
	"io/fs"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/timothydodd/couchside/internal/config"
	"github.com/timothydodd/couchside/internal/db"
	"github.com/timothydodd/couchside/internal/metadata"
	"github.com/timothydodd/couchside/internal/transcode"
	"github.com/timothydodd/couchside/internal/usererr"
	"github.com/timothydodd/couchside/internal/worker"
)

// Only messages written for users reach a response; anything else is logged
// and answered "internal error".
func TestUserFault(t *testing.T) {
	answer := func(err error) (int, string) {
		rec := httptest.NewRecorder()
		writeErr(rec, userFault(err))
		b, _ := io.ReadAll(rec.Body)
		return rec.Code, string(b)
	}
	if code, body := answer(usererr.New("cancel the recording first")); code != 400 || !strings.Contains(body, "cancel the recording first") {
		t.Fatalf("user message = %d %s", code, body)
	}
	pathErr := &fs.PathError{Op: "remove", Path: "/media/tv/Show/ep.ts", Err: errors.New("permission denied")}
	if code, body := answer(pathErr); code != 500 || strings.Contains(body, "/media") || strings.Contains(body, "permission") {
		t.Fatalf("path error = %d %s", code, body)
	}
	if code, _ := answer(db.ErrNotFound); code != 404 {
		t.Fatalf("not found = %d", code)
	}
}

// The media folder is a server path: only admins get it from /api/status.
func TestStatusHidesMediaRootFromUsers(t *testing.T) {
	s, ts, admin := passwordlessServer(t)
	s.cfg.MediaRoot = "/srv/media"
	s.worker = worker.New(s.db, config.Config{}, nil, transcode.Encoder{})
	s.providers = &metadata.Chain{}
	tc, err := transcode.NewManager(transcode.Encoder{}, "ffprobe", t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	s.tc = tc
	if code := admin.do("POST", "/api/accounts", map[string]any{"name": "Kid"}, nil); code != 201 {
		t.Fatalf("create = %d", code)
	}
	kid := newClient(t, ts.URL)
	if code := kid.do("POST", "/api/auth/pick", map[string]any{"profileId": 2}, nil); code != 200 {
		t.Fatalf("pick = %d", code)
	}
	var a, k struct{ MediaRoot string }
	if code := admin.do("GET", "/api/status", nil, &a); code != 200 || a.MediaRoot != "/srv/media" {
		t.Fatalf("admin status = %d %+v", code, a)
	}
	if code := kid.do("GET", "/api/status", nil, &k); code != 200 || k.MediaRoot != "" {
		t.Fatalf("user status = %d %+v", code, k)
	}
}

// Backups are an admin's: they hold password hashes and the session key.
func TestBackupRoutes(t *testing.T) {
	_, ts, admin := passwordlessServer(t)
	if code := admin.do("POST", "/api/accounts", map[string]any{"name": "Kid"}, nil); code != 201 {
		t.Fatalf("create = %d", code)
	}
	kid := newClient(t, ts.URL)
	if code := kid.do("POST", "/api/auth/pick", map[string]any{"profileId": 2}, nil); code != 200 {
		t.Fatalf("pick = %d", code)
	}
	var made struct{ Name, Kind string }
	if code := admin.do("POST", "/api/system/backups", nil, &made); code != 201 || made.Kind != "manual" {
		t.Fatalf("create backup = %d %+v", code, made)
	}
	var list struct {
		Backups []struct{ Name string }
		Daily   bool
		Keep    int
	}
	if code := admin.do("GET", "/api/system/backups", nil, &list); code != 200 || len(list.Backups) != 1 || !list.Daily || list.Keep != 7 {
		t.Fatalf("list = %d %+v", code, list)
	}
	for _, path := range []string{"/api/system/backups", "/api/system/backups/" + made.Name} {
		if code := kid.do("GET", path, nil, nil); code != 403 {
			t.Fatalf("a user's GET %s = %d, want 403", path, code)
		}
	}
	if code := admin.do("GET", "/api/system/backups/"+made.Name, nil, nil); code != 200 {
		t.Fatalf("download = %d", code)
	}
	if code := admin.do("GET", "/api/system/backups/couchside.db", nil, nil); code != 404 {
		t.Fatalf("download of a file that isn't a backup = %d, want 404", code)
	}
	if code := admin.do("PUT", "/api/settings/backup", map[string]any{"daily": false, "keep": 3}, nil); code != 204 {
		t.Fatalf("settings = %d", code)
	}
	if code := admin.do("GET", "/api/system/backups", nil, &list); code != 200 || list.Daily || list.Keep != 3 {
		t.Fatalf("after settings: %+v", list)
	}
	if code := admin.do("DELETE", "/api/system/backups/"+made.Name, nil, nil); code != 204 {
		t.Fatalf("delete = %d", code)
	}
}
