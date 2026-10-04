package db

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func openTest(t *testing.T) *DB {
	t.Helper()
	d, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestSetupAdminAndLastAdminGuard(t *testing.T) {
	d := openTest(t)
	bg := context.Background()
	if ok, _ := d.HasAdmin(bg); ok {
		t.Fatal("fresh database has an admin")
	}
	// Setting up as the existing "Me" keeps that profile (and its history).
	me, err := d.SetupAdmin(bg, "me", "hash")
	if err != nil || me.ID != 1 || me.Role != "admin" || !me.CanRecord || !me.HasPassword {
		t.Fatalf("setup = %+v, %v", me, err)
	}
	if ok, _ := d.HasAdmin(bg); !ok {
		t.Fatal("no admin after setup")
	}
	kid, err := d.CreateAccount(bg, "Kid", "pink", "user", false, "h2", false)
	if err != nil || !kid.MustChangePassword || kid.Role != "user" {
		t.Fatalf("create = %+v, %v", kid, err)
	}
	if _, _, err := d.ProfileForLogin(bg, " KID "); err != nil {
		t.Fatalf("login lookup is case-insensitive: %v", err)
	}
	if _, err := d.SetAccess(bg, me.ID, "user", true, false); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("demote last admin err = %v", err)
	}
	if _, err := d.SetAccess(bg, me.ID, "admin", true, true); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("disable last admin err = %v", err)
	}
	if err := d.DeleteProfile(bg, me.ID); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("delete last admin err = %v", err)
	}
	if _, err := d.SetAccess(bg, kid.ID, "admin", false, false); err != nil {
		t.Fatal(err)
	}
	if _, err := d.SetAccess(bg, me.ID, "user", true, false); err != nil {
		t.Fatalf("demote with another admin: %v", err)
	}
}

func TestRefreshRotation(t *testing.T) {
	d := openTest(t)
	bg := context.Background()
	p, _ := d.SetupAdmin(bg, "Me", "hash")
	if err := d.CreateSession(bg, Session{ID: "s1", ProfileID: p.ID, Client: "web", ExpiresAt: 1000}, "r1"); err != nil {
		t.Fatal(err)
	}
	if u, err := d.SessionUser(bg, "s1", 100); err != nil || u.Profile.ID != p.ID {
		t.Fatalf("session user = %+v, %v", u, err)
	}
	if _, err := d.SessionUser(bg, "s1", 1000); !errors.Is(err, ErrNotFound) {
		t.Fatal("expired session accepted")
	}

	idle := func(client string) int64 { return 1900 }
	s, res, err := d.RotateRefresh(bg, "r1", "r2", 100, idle, "1.2.3.4")
	if err != nil || res != RefreshOK || s.ID != "s1" || s.ExpiresAt != 2000 {
		t.Fatalf("rotate = %+v, %v, %v", s, res, err)
	}
	// r1 again within a minute: another tab racing, not a theft.
	if _, res, _ := d.RotateRefresh(bg, "r1", "r3", 130, idle, ""); res != RefreshStale {
		t.Fatalf("quick reuse = %v, want stale", res)
	}
	if _, res, _ := d.RotateRefresh(bg, "nope", "r3", 130, idle, ""); res != RefreshUnknown {
		t.Fatalf("unknown = %v", res)
	}
	// r1 much later: replayed, so the whole session ends, r2 included.
	if _, res, _ := d.RotateRefresh(bg, "r1", "r3", 500, idle, ""); res != RefreshReused {
		t.Fatalf("late reuse = %v, want reused", res)
	}
	if _, res, _ := d.RotateRefresh(bg, "r2", "r3", 501, idle, ""); res != RefreshUnknown {
		t.Fatalf("session should be gone, got %v", res)
	}

	// Disabled profiles can't refresh or use their sessions.
	if err := d.CreateSession(bg, Session{ID: "s2", ProfileID: p.ID, Client: "web", ExpiresAt: 1000}, "q1"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.CreateAccount(bg, "Other", "accent", "admin", true, "h", false); err != nil { // so Me can be disabled
		t.Fatal(err)
	}
	if _, err := d.SetAccess(bg, p.ID, "admin", true, true); err != nil {
		t.Fatal(err)
	}
	if _, res, _ := d.RotateRefresh(bg, "q1", "q2", 100, idle, ""); res != RefreshUnknown {
		t.Fatalf("disabled refresh = %v", res)
	}
	if _, err := d.SessionUser(bg, "s2", 100); !errors.Is(err, ErrNotFound) {
		t.Fatal("disabled profile's session accepted")
	}
}

// A TV app that never got the answer to a refresh asks again with the token
// it still has, and keeps its session. The token it never received is
// retired, and a token from further back is still treated as stolen.
func TestRefreshRetryForTV(t *testing.T) {
	d := openTest(t)
	bg := context.Background()
	p, _ := d.SetupAdmin(bg, "Me", "hash")
	if err := d.CreateSession(bg, Session{ID: "s1", ProfileID: p.ID, Client: "tv", ExpiresAt: 1000}, "r1"); err != nil {
		t.Fatal(err)
	}
	idle := func(string) int64 { return 100000 }
	rotate := func(old, next string, now int64) RefreshResult {
		t.Helper()
		_, res, err := d.RotateRefresh(bg, old, next, now, idle, "")
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	if res := rotate("r1", "r2", 100); res != RefreshOK {
		t.Fatalf("first rotate = %v", res)
	}
	// The answer with r2 was lost. Minutes later the app tries r1 again.
	if res := rotate("r1", "r3", 400); res != RefreshOK {
		t.Fatalf("retry with the previous token = %v, want ok", res)
	}
	// And again: still its newest token as far as it knows.
	if res := rotate("r1", "r4", 410); res != RefreshOK {
		t.Fatalf("second retry = %v, want ok", res)
	}
	// It got r4 and uses it: r1 is now two tokens back.
	if res := rotate("r4", "r5", 500); res != RefreshOK {
		t.Fatalf("rotate with the received token = %v", res)
	}
	if res := rotate("r4", "r6", 510); res != RefreshOK {
		t.Fatalf("retry of the latest refresh = %v, want ok", res)
	}
	// r2 never reached the app; someone presenting it later isn't the app.
	if res := rotate("r2", "x", 2000); res != RefreshReused {
		t.Fatalf("a token that was never delivered = %v, want reused", res)
	}
	if res := rotate("r6", "x", 2001); res != RefreshUnknown {
		t.Fatalf("session should be gone, got %v", res)
	}
}

// People nobody credits any more are dropped; the rest stay.
func TestPrunePeople(t *testing.T) {
	d := openTest(t)
	bg := context.Background()
	lib, _ := d.CreateLibrary(bg, "Films", "/films", "movies")
	item, _, _ := d.EnsureItem(bg, lib, "movie", "Heat", 1995)
	cast := []Credit{{PersonID: 1, Name: "Al Pacino", Kind: "cast"}, {PersonID: 2, Name: "Robert De Niro", Kind: "cast"}}
	if err := d.SetCredits(bg, item, cast); err != nil {
		t.Fatal(err)
	}
	if gone, err := d.PrunePeople(bg); err != nil || len(gone) != 0 {
		t.Fatalf("pruned credited people: %v %v", gone, err)
	}
	if err := d.SetCredits(bg, item, cast[:1]); err != nil { // a re-match dropped one
		t.Fatal(err)
	}
	gone, err := d.PrunePeople(bg)
	if err != nil || len(gone) != 1 || gone[0] != 2 {
		t.Fatalf("pruned = %v %v, want person 2", gone, err)
	}
}
