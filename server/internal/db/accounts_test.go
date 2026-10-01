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
	kid, err := d.CreateAccount(bg, "Kid", "pink", "user", false, "h2")
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
	if err := d.CreateSession(bg, Session{ID: "s1", ProfileID: p.ID, Client: "tv", ExpiresAt: 1000}, "r1"); err != nil {
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
	if _, err := d.CreateAccount(bg, "Other", "accent", "admin", true, "h"); err != nil { // so Me can be disabled
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
