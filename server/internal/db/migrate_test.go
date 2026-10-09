package db

import (
	"database/sql"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// rawExec runs SQL on a closed database file, around Open.
func rawExec(t *testing.T, path, query string) {
	t.Helper()
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if _, err := raw.Exec(query); err != nil {
		t.Fatal(err)
	}
}

func newDBFile(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "t.db")
	d, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	d.Close()
	return p
}

// An older build must not open a database a newer one migrated: it would
// misread the schema. It says which migration, and where the backup is.
func TestOpenRefusesNewerDatabase(t *testing.T) {
	p := newDBFile(t)
	rawExec(t, p, `INSERT INTO schema_migrations (version) VALUES ('9999_from_the_future.sql')`)
	_, err := Open(p)
	var e *ErrNewerDatabase
	if !errors.As(err, &e) {
		t.Fatalf("Open = %v, want ErrNewerDatabase", err)
	}
	if e.Newest != "9999_from_the_future.sql" || !strings.Contains(err.Error(), "couchside restore") ||
		!strings.Contains(err.Error(), filepath.Join(filepath.Dir(p), BackupDir)) {
		t.Fatalf("message = %v", err)
	}
}

// A gap below the newest applied migration means the table was edited.
func TestOpenRefusesOutOfOrderMigrations(t *testing.T) {
	p := newDBFile(t)
	rawExec(t, p, `DELETE FROM schema_migrations WHERE version = '0035_media_locations.sql'`)
	_, err := Open(p)
	if err == nil || !strings.Contains(err.Error(), "0035_media_locations.sql") {
		t.Fatalf("Open = %v, want an error naming 0035", err)
	}
}

// withExtraMigration adds a migration after the real ones for this test.
func withExtraMigration(t *testing.T, name, body string) {
	t.Helper()
	m := fstest.MapFS{}
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		b, err := migrationFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			t.Fatal(err)
		}
		m["migrations/"+e.Name()] = &fstest.MapFile{Data: b}
	}
	m["migrations/"+name] = &fstest.MapFile{Data: []byte(body)}
	migrations = m
	t.Cleanup(func() { migrations = migrationFS })
}

// A migration newer than every applied one is an ordinary upgrade.
func TestOpenAppliesANewMigration(t *testing.T) {
	p := newDBFile(t)
	withExtraMigration(t, "9998_next_release.sql", "CREATE TABLE next_release (x INTEGER);")
	d, err := Open(p)
	if err != nil {
		t.Fatalf("Open refused an ordinary upgrade: %v", err)
	}
	defer d.Close()
	if _, err := d.sql.Exec(`INSERT INTO next_release (x) VALUES (1)`); err != nil {
		t.Fatalf("new migration wasn't applied: %v", err)
	}
}
