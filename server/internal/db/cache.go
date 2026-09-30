package db

import (
	"context"
	"time"
)

// CacheGet returns a cached provider response younger than maxAge.
func (d *DB) CacheGet(ctx context.Context, provider, key string, maxAge time.Duration) ([]byte, bool) {
	var body []byte
	err := d.sql.QueryRowContext(ctx, `SELECT body FROM provider_cache WHERE provider = ? AND key = ? AND fetched_at > ?`,
		provider, key, time.Now().Add(-maxAge).Unix()).Scan(&body)
	if err != nil {
		return nil, false
	}
	return body, true
}

func (d *DB) CachePut(ctx context.Context, provider, key string, body []byte) error {
	_, err := d.sql.ExecContext(ctx, `INSERT INTO provider_cache (provider, key, body, fetched_at) VALUES (?, ?, ?, unixepoch())
		ON CONFLICT (provider, key) DO UPDATE SET body = excluded.body, fetched_at = excluded.fetched_at`, provider, key, body)
	return err
}
