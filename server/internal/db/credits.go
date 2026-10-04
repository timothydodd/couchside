package db

import (
	"context"
	"strings"
)

// Credit is one person's part in a title, as the provider gave it.
type Credit struct {
	PersonID    int64
	Name        string
	ProfilePath string
	Kind        string // cast | crew
	Role        string // character or job
	Order       int
}

// SetCredits replaces an item's cast and crew.
func (d *DB) SetCredits(ctx context.Context, itemID int64, credits []Credit) error {
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM item_credits WHERE item_id = ?`, itemID); err != nil {
		return err
	}
	for _, c := range credits {
		if c.PersonID == 0 || c.Name == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO people (id, name, profile_path) VALUES (?, ?, ?)
			ON CONFLICT (id) DO UPDATE SET name = excluded.name,
			  profile_path = CASE WHEN excluded.profile_path <> '' THEN excluded.profile_path ELSE people.profile_path END,
			  updated_at = unixepoch()`,
			c.PersonID, c.Name, c.ProfilePath); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO item_credits (item_id, person_id, kind, role, ord) VALUES (?, ?, ?, ?, ?)`,
			itemID, c.PersonID, c.Kind, c.Role, c.Order); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// PrunePeople drops people no title credits any more (their titles were
// removed or re-matched) and returns their ids, so the caller can remove
// their cached photos.
func (d *DB) PrunePeople(ctx context.Context) ([]int64, error) {
	rows, err := d.sql.QueryContext(ctx, `DELETE FROM people WHERE id NOT IN (SELECT person_id FROM item_credits) RETURNING id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// CreditRow is a person as a title's page lists them.
type CreditRow struct {
	PersonID int64  `json:"personId"`
	Name     string `json:"name"`
	Role     string `json:"role"` // character or job; several jobs are joined ("Director, Screenplay")
	HasPhoto bool   `json:"hasPhoto"`
}

// ItemCredits returns a title's cast in billing order and its crew, one row
// per person (a writer-director is listed once).
func (d *DB) ItemCredits(ctx context.Context, itemID int64) (cast, crew []CreditRow, err error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT c.kind, p.id, p.name, p.profile_path <> '',
		group_concat(c.role, ', ') FROM item_credits c JOIN people p ON p.id = c.person_id
		WHERE c.item_id = ? GROUP BY c.kind, p.id ORDER BY c.kind, MIN(c.ord), p.name`, itemID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	cast, crew = []CreditRow{}, []CreditRow{}
	for rows.Next() {
		var kind string
		var r CreditRow
		var roles *string
		if err := rows.Scan(&kind, &r.PersonID, &r.Name, &r.HasPhoto, &roles); err != nil {
			return nil, nil, err
		}
		if roles != nil {
			r.Role = *roles
		}
		if kind == "cast" {
			cast = append(cast, r)
		} else {
			crew = append(crew, r)
		}
	}
	return cast, crew, rows.Err()
}

// Person is someone from a title's credits.
type Person struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	HasPhoto    bool   `json:"hasPhoto"`
	ProfilePath string `json:"-"`
}

func (d *DB) Person(ctx context.Context, id int64) (Person, error) {
	var p Person
	err := d.sql.QueryRowContext(ctx, `SELECT id, name, profile_path <> '', profile_path FROM people WHERE id = ?`, id).
		Scan(&p.ID, &p.Name, &p.HasPhoto, &p.ProfilePath)
	return p, notFound(err)
}

// PersonItem is one title in the library a person is in, with what they did.
type PersonItem struct {
	ItemSummary
	Roles []string `json:"roles"` // characters played and jobs done
}

// PersonItems lists the library's titles a person is credited on, newest first.
func (d *DB) PersonItems(ctx context.Context, personID int64) ([]PersonItem, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT `+summaryCols(ctx)+`,
		(SELECT group_concat(CASE WHEN c.kind = 'cast' AND c.role = '' THEN 'Cast' ELSE c.role END, '|')
		 FROM item_credits c WHERE c.item_id = m.id AND c.person_id = ?)
		FROM media_items m WHERE m.id IN (SELECT item_id FROM item_credits WHERE person_id = ?)
		ORDER BY COALESCE(m.year, 0) DESC, m.sort_title`, personID, personID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PersonItem{}
	for rows.Next() {
		var it PersonItem
		var roles *string
		if err := rows.Scan(scanSummary(&it.ItemSummary, &roles)...); err != nil {
			return nil, err
		}
		it.Roles = []string{}
		if roles != nil {
			for _, r := range strings.Split(*roles, "|") {
				if r != "" {
					it.Roles = append(it.Roles, r)
				}
			}
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// ItemIDsInLibrary lists a library's items, for re-matching them all.
func (d *DB) ItemIDsInLibrary(ctx context.Context, libraryID int64) ([]ItemRef, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT id, parsed_title FROM media_items WHERE library_id = ? ORDER BY id`, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ItemRef{}
	for rows.Next() {
		var r ItemRef
		if err := rows.Scan(&r.ID, &r.Title); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type ItemRef struct {
	ID    int64
	Title string
}
