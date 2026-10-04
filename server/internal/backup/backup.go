// Package backup copies Couchside's database, with the two small files that
// belong with it, into $DATA/backups, and puts one back.
//
// A backup is a zip of couchside.db (a consistent copy taken with VACUUM
// INTO while the server runs), auth.key (which signs sessions) and server.id
// (how TV apps recognise this server). The copy the database makes of itself
// before an upgrade changes its tables is a bare .db file in the same folder.
package backup

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/timothydodd/couchside/internal/db"
)

// Kinds of backup. Only scheduled ones are pruned by Prune's keep.
const (
	Scheduled = "daily"
	Manual    = "manual"
	Upgrade   = db.UpgradeBackup // a .db written before migrations, by db.Open
)

const keepUpgrades = 3

// extras are the files beside the database that a backup includes.
var extras = []string{"auth.key", "server.id"}

var reName = regexp.MustCompile(`^couchside-(daily|manual|upgrade)-(\d{8}-\d{6})\.(zip|db)$`)

// Info describes one backup file.
type Info struct {
	Name string `json:"name"`
	Kind string `json:"kind"` // daily | manual | upgrade
	Size int64  `json:"size"`
	At   int64  `json:"at"` // unix seconds
}

// Dir is where backups live.
func Dir(dataDir string) string { return filepath.Join(dataDir, db.BackupDir) }

// Create writes a backup of kind now and returns it.
func Create(ctx context.Context, d *db.DB, dataDir, kind string) (Info, error) {
	dir := Dir(dataDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Info{}, err
	}
	name := fmt.Sprintf("couchside-%s-%s.zip", kind, time.Now().Format("20060102-150405"))
	copyDB := filepath.Join(dir, "."+name+".db")
	defer os.Remove(copyDB)
	if err := d.VacuumInto(ctx, copyDB); err != nil {
		return Info{}, fmt.Errorf("copy the database: %w", err)
	}
	tmp := filepath.Join(dir, "."+name+".tmp")
	defer os.Remove(tmp)
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600) // holds the session key
	if err != nil {
		return Info{}, err
	}
	zw := zip.NewWriter(out)
	add := func(src, as string) error {
		in, err := os.Open(src)
		if err != nil {
			return err
		}
		defer in.Close()
		w, err := zw.Create(as)
		if err != nil {
			return err
		}
		_, err = io.Copy(w, in)
		return err
	}
	err = add(copyDB, "couchside.db")
	for _, f := range extras {
		if err != nil {
			break
		}
		if e := add(filepath.Join(dataDir, f), f); e != nil && !errors.Is(e, os.ErrNotExist) {
			err = e
		}
	}
	if cerr := zw.Close(); err == nil {
		err = cerr
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return Info{}, err
	}
	if err := os.Rename(tmp, filepath.Join(dir, name)); err != nil {
		return Info{}, err
	}
	return stat(dir, name)
}

func stat(dir, name string) (Info, error) {
	m := reName.FindStringSubmatch(name)
	if m == nil {
		return Info{}, os.ErrNotExist
	}
	st, err := os.Stat(filepath.Join(dir, name))
	if err != nil {
		return Info{}, err
	}
	at, err := time.ParseInLocation("20060102-150405", m[2], time.Local)
	if err != nil {
		at = st.ModTime()
	}
	return Info{Name: name, Kind: m[1], Size: st.Size(), At: at.Unix()}, nil
}

// List returns the backups, newest first.
func List(dataDir string) ([]Info, error) {
	dir := Dir(dataDir)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []Info{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []Info{}
	for _, e := range entries {
		if i, err := stat(dir, e.Name()); err == nil && !e.IsDir() {
			out = append(out, i)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].At > out[j].At || (out[i].At == out[j].At && out[i].Name > out[j].Name)
	})
	return out, nil
}

// Path is a backup's file, for a name List returned. Anything else is refused.
func Path(dataDir, name string) (string, error) {
	if !reName.MatchString(name) {
		return "", os.ErrNotExist
	}
	p := filepath.Join(Dir(dataDir), name)
	if _, err := os.Stat(p); err != nil {
		return "", err
	}
	return p, nil
}

// Delete removes one backup.
func Delete(dataDir, name string) error {
	p, err := Path(dataDir, name)
	if err != nil {
		return err
	}
	return os.Remove(p)
}

// Prune keeps the newest keep scheduled backups and the last few made before
// upgrades. Manual ones stay until they're deleted.
func Prune(dataDir string, keep int) error {
	all, err := List(dataDir)
	if err != nil {
		return err
	}
	seen := map[string]int{}
	for _, b := range all {
		seen[b.Kind]++
		if (b.Kind == Scheduled && seen[b.Kind] > keep) || (b.Kind == Upgrade && seen[b.Kind] > keepUpgrades) {
			if err := os.Remove(filepath.Join(Dir(dataDir), b.Name)); err != nil {
				return err
			}
		}
	}
	return nil
}

// Restore replaces the database in dataDir with the one in file: a backup
// zip (its auth.key and server.id come back too) or a bare .db. The server
// must be stopped. The database being replaced is kept beside it as
// couchside.db.before-restore, in case the wrong file was picked.
func Restore(dataDir, file string) error {
	target := filepath.Join(dataDir, "couchside.db")
	staged := target + ".restoring"
	defer os.Remove(staged)
	restored := map[string]string{} // staged file → where it goes
	if strings.EqualFold(filepath.Ext(file), ".zip") {
		zr, err := zip.OpenReader(file)
		if err != nil {
			return fmt.Errorf("not a backup zip: %w", err)
		}
		defer zr.Close()
		found := false
		for _, f := range zr.File {
			dst := ""
			switch f.Name {
			case "couchside.db":
				dst, found = staged, true
			case "auth.key", "server.id":
				dst = filepath.Join(dataDir, f.Name+".restoring")
				defer os.Remove(dst)
				restored[dst] = filepath.Join(dataDir, f.Name)
			default:
				continue
			}
			if err := extract(f, dst); err != nil {
				return err
			}
		}
		if !found {
			return errors.New("that zip has no couchside.db in it")
		}
	} else if err := copyFile(file, staged); err != nil {
		return err
	}
	if err := db.Check(staged); err != nil {
		return fmt.Errorf("the backup's database can't be read: %w", err)
	}
	if _, err := os.Stat(target); err == nil {
		if err := os.Rename(target, target+".before-restore"); err != nil {
			return err
		}
	}
	// The old database's journal belongs to the old file.
	_ = os.Remove(target + "-wal")
	_ = os.Remove(target + "-shm")
	if err := os.Rename(staged, target); err != nil {
		return err
	}
	for from, to := range restored {
		if err := os.Rename(from, to); err != nil {
			return err
		}
	}
	return nil
}

func extract(f *zip.File, dst string) error {
	in, err := f.Open()
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}
