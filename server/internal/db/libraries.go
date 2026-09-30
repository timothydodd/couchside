package db

import "context"

type Library struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Path       string `json:"path"`
	Kind       string `json:"kind"` // movies | tv
	LastScanAt *int64 `json:"lastScanAt"`
	CreatedAt  int64  `json:"createdAt"`
	ItemCount  int    `json:"itemCount"`
	FileCount  int    `json:"fileCount"`
}

const libraryCols = `l.id, l.name, l.path, l.kind, l.last_scan_at, l.created_at,
	(SELECT COUNT(*) FROM media_items m WHERE m.library_id = l.id),
	(SELECT COUNT(*) FROM files f WHERE f.library_id = l.id)`

func scanLibrary(r interface{ Scan(...any) error }) (Library, error) {
	var l Library
	err := r.Scan(&l.ID, &l.Name, &l.Path, &l.Kind, &l.LastScanAt, &l.CreatedAt, &l.ItemCount, &l.FileCount)
	return l, err
}

func (d *DB) Libraries(ctx context.Context) ([]Library, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT `+libraryCols+` FROM libraries l ORDER BY l.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Library{}
	for rows.Next() {
		l, err := scanLibrary(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (d *DB) Library(ctx context.Context, id int64) (Library, error) {
	l, err := scanLibrary(d.sql.QueryRowContext(ctx, `SELECT `+libraryCols+` FROM libraries l WHERE l.id = ?`, id))
	return l, notFound(err)
}

func (d *DB) CreateLibrary(ctx context.Context, name, path, kind string) (int64, error) {
	res, err := d.sql.ExecContext(ctx, `INSERT INTO libraries (name, path, kind) VALUES (?, ?, ?)`, name, path, kind)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (d *DB) DeleteLibrary(ctx context.Context, id int64) error {
	_, err := d.sql.ExecContext(ctx, `DELETE FROM libraries WHERE id = ?`, id)
	return err
}

func (d *DB) MarkLibraryScanned(ctx context.Context, id, at int64) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE libraries SET last_scan_at = ? WHERE id = ?`, at, id)
	return err
}
