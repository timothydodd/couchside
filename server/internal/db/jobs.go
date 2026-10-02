package db

import (
	"context"
	"database/sql"
)

type Job struct {
	ID         int64    `json:"id"`
	Kind       string   `json:"kind"`
	RefID      int64    `json:"refId"`
	Label      string   `json:"label"`
	Status     string   `json:"status"`
	Attempts   int      `json:"attempts"`
	Error      string   `json:"error"`
	CreatedAt  int64    `json:"createdAt"`
	StartedAt  *int64   `json:"startedAt"`
	FinishedAt *int64   `json:"finishedAt"`
	Progress   *float64 `json:"progress"`
	Result     string   `json:"result"` // what a finished job did (scans: counts and skipped files)
}

const jobCols = `id, kind, ref_id, label, status, attempts, error, created_at, started_at, finished_at, progress, result`

func scanJob(r interface{ Scan(...any) error }) (Job, error) {
	var j Job
	err := r.Scan(&j.ID, &j.Kind, &j.RefID, &j.Label, &j.Status, &j.Attempts, &j.Error, &j.CreatedAt, &j.StartedAt, &j.FinishedAt, &j.Progress, &j.Result)
	return j, err
}

// Enqueue adds a job unless the same kind+ref is already queued or running.
func (d *DB) Enqueue(ctx context.Context, kind string, refID int64, label string) error {
	_, err := d.sql.ExecContext(ctx, `INSERT OR IGNORE INTO jobs (kind, ref_id, label) VALUES (?, ?, ?)`, kind, refID, label)
	return err
}

// encodeKinds are the job kinds handled by the separate encode pool, so
// hour-long encodes and commercial detection never block scans and metadata.
const encodeKinds = "'optimize', 'commercials'"

// ClaimJob atomically moves the oldest queued job to running. Scans go first
// so new files are discovered before we spend time on thumbnails, and quick
// commercial detection goes ahead of long encodes. encode
// selects between the encode pool and the general pool.
func (d *DB) ClaimJob(ctx context.Context, encode bool) (*Job, error) {
	cond := "kind NOT IN (" + encodeKinds + ")"
	if encode {
		cond = "kind IN (" + encodeKinds + ")"
	}
	j, err := scanJob(d.sql.QueryRowContext(ctx, `UPDATE jobs SET status = 'running', attempts = attempts + 1,
		started_at = unixepoch(), error = '', progress = NULL, result = ''
		WHERE id = (SELECT id FROM jobs WHERE status = 'queued' AND `+cond+`
		  ORDER BY CASE kind WHEN 'scan' THEN 0 WHEN 'match' THEN 1 WHEN 'artwork' THEN 2 WHEN 'commercials' THEN 3 ELSE 4 END, id LIMIT 1)
		RETURNING `+jobCols))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &j, nil
}

func (d *DB) FinishJob(ctx context.Context, id int64, jobErr error) error {
	status, msg := "done", ""
	if jobErr != nil {
		status, msg = "failed", jobErr.Error()
	}
	_, err := d.sql.ExecContext(ctx, `UPDATE jobs SET status = ?, error = ?, finished_at = unixepoch() WHERE id = ?`, status, msg, id)
	return err
}

// SetJobResult records what a job did, shown in Activity.
func (d *DB) SetJobResult(ctx context.Context, id int64, result string) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE jobs SET result = ? WHERE id = ?`, result, id)
	return err
}

func (d *DB) DeleteJob(ctx context.Context, id int64) error {
	_, err := d.sql.ExecContext(ctx, `DELETE FROM jobs WHERE id = ?`, id)
	return err
}

func (d *DB) SetJobProgress(ctx context.Context, id int64, p float64) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE jobs SET progress = ? WHERE id = ?`, p, id)
	return err
}

// CancelQueuedJob drops a queued job. A cancel is a choice, not a failure,
// so it leaves no row behind. Running jobs are cancelled through the worker.
func (d *DB) CancelQueuedJob(ctx context.Context, id int64) (bool, error) {
	res, err := d.sql.ExecContext(ctx, `DELETE FROM jobs WHERE id = ? AND status = 'queued'`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (d *DB) Job(ctx context.Context, id int64) (Job, error) {
	j, err := scanJob(d.sql.QueryRowContext(ctx, `SELECT `+jobCols+` FROM jobs WHERE id = ?`, id))
	return j, notFound(err)
}

// maxAttempts is how many times a job may start. One still running at a
// restart after that many has probably taken the server down with it.
const maxAttempts = 3

// RequeueRunning resets jobs left running by a crash or restart, failing
// those that have already started maxAttempts times.
func (d *DB) RequeueRunning(ctx context.Context) error {
	if _, err := d.sql.ExecContext(ctx, `UPDATE jobs SET status = 'failed', finished_at = unixepoch(),
		error = 'interrupted by a server restart ' || attempts || ' times; retry it from Activity'
		WHERE status = 'running' AND attempts >= ?`, maxAttempts); err != nil {
		return err
	}
	_, err := d.sql.ExecContext(ctx, `UPDATE jobs SET status = 'queued' WHERE status = 'running'`)
	return err
}

// PendingMatches lists a library's items whose match never finished: the
// provider couldn't be reached (network, rate limit, server error). Items it
// answered "not found" are 'unmatched' and aren't listed.
func (d *DB) PendingMatches(ctx context.Context, libraryID int64) ([]ItemRef, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT m.id, m.parsed_title FROM media_items m WHERE m.library_id = ? AND m.match_status = 'pending'
		AND NOT EXISTS (SELECT 1 FROM jobs j WHERE j.kind = 'match' AND j.ref_id = m.id AND j.status IN ('queued', 'running'))`, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ItemRef
	for rows.Next() {
		var r ItemRef
		if err := rows.Scan(&r.ID, &r.Title); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) RetryJob(ctx context.Context, id int64) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE jobs SET status = 'queued', error = '', finished_at = NULL
		WHERE id = ? AND status = 'failed'
		AND NOT EXISTS (SELECT 1 FROM jobs j2 WHERE j2.kind = jobs.kind AND j2.ref_id = jobs.ref_id AND j2.status IN ('queued', 'running'))`, id)
	return err
}

// PruneJobs drops finished and failed jobs older than cutoff (unix seconds).
func (d *DB) PruneJobs(ctx context.Context, cutoff int64) (int64, error) {
	res, err := d.sql.ExecContext(ctx, `DELETE FROM jobs WHERE status IN ('done', 'failed') AND COALESCE(finished_at, created_at) < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (d *DB) ClearFinishedJobs(ctx context.Context) error {
	_, err := d.sql.ExecContext(ctx, `DELETE FROM jobs WHERE status = 'done'`)
	return err
}

// Recent jobs for the Activity page: active and failed first, then history.
func (d *DB) RecentJobs(ctx context.Context, limit int) ([]Job, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT `+jobCols+` FROM jobs
		ORDER BY CASE status WHEN 'running' THEN 0 WHEN 'failed' THEN 1 WHEN 'queued' THEN 2 ELSE 3 END,
		  CASE WHEN status = 'queued' THEN id ELSE -id END LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Job{}
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

type JobCounts struct {
	Queued  int    `json:"queued"`
	Running int    `json:"running"`
	Failed  int    `json:"failed"`
	Current string `json:"current"` // label of the oldest running job
}

func (d *DB) JobCounts(ctx context.Context) (JobCounts, error) {
	var c JobCounts
	err := d.sql.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM jobs WHERE status = 'queued'),
		(SELECT COUNT(*) FROM jobs WHERE status = 'running'),
		(SELECT COUNT(*) FROM jobs WHERE status = 'failed'),
		COALESCE((SELECT label FROM jobs WHERE status = 'running' ORDER BY CASE kind WHEN 'scan' THEN 0 ELSE 1 END, id LIMIT 1), '')`).
		Scan(&c.Queued, &c.Running, &c.Failed, &c.Current)
	return c, err
}
