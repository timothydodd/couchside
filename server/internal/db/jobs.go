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
}

const jobCols = `id, kind, ref_id, label, status, attempts, error, created_at, started_at, finished_at, progress`

func scanJob(r interface{ Scan(...any) error }) (Job, error) {
	var j Job
	err := r.Scan(&j.ID, &j.Kind, &j.RefID, &j.Label, &j.Status, &j.Attempts, &j.Error, &j.CreatedAt, &j.StartedAt, &j.FinishedAt, &j.Progress)
	return j, err
}

// Enqueue adds a job unless the same kind+ref is already queued or running.
func (d *DB) Enqueue(ctx context.Context, kind string, refID int64, label string) error {
	_, err := d.sql.ExecContext(ctx, `INSERT OR IGNORE INTO jobs (kind, ref_id, label) VALUES (?, ?, ?)`, kind, refID, label)
	return err
}

// EncodeKind is the job kind handled by the separate encode pool, so hour-long
// encodes never block scans and metadata.
const EncodeKind = "optimize"

// ClaimJob atomically moves the oldest queued job to running. Scans go first
// so new files are discovered before we spend time on thumbnails. encode
// selects between the encode pool and the general pool.
func (d *DB) ClaimJob(ctx context.Context, encode bool) (*Job, error) {
	cond := "kind != '" + EncodeKind + "'"
	if encode {
		cond = "kind = '" + EncodeKind + "'"
	}
	j, err := scanJob(d.sql.QueryRowContext(ctx, `UPDATE jobs SET status = 'running', attempts = attempts + 1,
		started_at = unixepoch(), error = '', progress = NULL
		WHERE id = (SELECT id FROM jobs WHERE status = 'queued' AND `+cond+`
		  ORDER BY CASE kind WHEN 'scan' THEN 0 WHEN 'match' THEN 1 WHEN 'artwork' THEN 2 ELSE 3 END, id LIMIT 1)
		RETURNING `+jobCols))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &j, err
}

func (d *DB) FinishJob(ctx context.Context, id int64, jobErr error) error {
	status, msg := "done", ""
	if jobErr != nil {
		status, msg = "failed", jobErr.Error()
	}
	_, err := d.sql.ExecContext(ctx, `UPDATE jobs SET status = ?, error = ?, finished_at = unixepoch() WHERE id = ?`, status, msg, id)
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

// RequeueRunning resets jobs left running by a crash or restart.
func (d *DB) RequeueRunning(ctx context.Context) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE jobs SET status = 'queued' WHERE status = 'running'`)
	return err
}

func (d *DB) RetryJob(ctx context.Context, id int64) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE jobs SET status = 'queued', error = '', finished_at = NULL
		WHERE id = ? AND status = 'failed'
		AND NOT EXISTS (SELECT 1 FROM jobs j2 WHERE j2.kind = jobs.kind AND j2.ref_id = jobs.ref_id AND j2.status IN ('queued', 'running'))`, id)
	return err
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
