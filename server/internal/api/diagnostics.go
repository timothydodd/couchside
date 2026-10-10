package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/timothydodd/couchside/internal/diskfree"
	"github.com/timothydodd/couchside/internal/sysstat"
)

// processStart is when this process started, for diagnostics.
var processStart = time.Now()

// diagnostics answers a zip an admin attaches to a bug report: versions,
// settings (keys masked), encoder detection, counts, migrations, database
// health, free disk, failed jobs and the recent log. It never holds
// auth.key, server.id, sessions, accounts or cached provider answers. Each
// part is made on its own, so one that fails becomes <name>.error.txt and
// the rest still arrive.
func (s *Server) diagnostics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name string, make func() (any, error)) {
		v, err := make()
		var b []byte
		if err == nil {
			switch t := v.(type) {
			case string:
				b = []byte(t)
			default:
				b, err = json.MarshalIndent(v, "", "  ")
			}
		}
		if err != nil {
			name = strings.TrimSuffix(strings.TrimSuffix(name, ".json"), ".txt") + ".error.txt"
			b = []byte(err.Error())
		}
		if f, err := zw.Create(name); err == nil {
			f.Write(b)
		}
	}
	add("README.txt", func() (any, error) {
		return "Couchside diagnostics, made " + time.Now().Format(time.RFC3339) + ".\n\n" +
			"These files name your folders and recent log lines (which can include file\n" +
			"names). API keys are masked and there are no passwords, sessions or keys in\n" +
			"here. Look it over before attaching it to a public bug report.\n", nil
	})
	add("server.json", func() (any, error) {
		return map[string]any{
			"version": s.version, "apiVersion": APIVersion, "features": s.features(ctx),
			"goos": runtime.GOOS, "goarch": runtime.GOARCH, "go": runtime.Version(), "cpus": runtime.NumCPU(),
			"started": processStart.Format(time.RFC3339), "inContainer": sysstat.InContainer(),
		}, nil
	})
	add("config.json", func() (any, error) {
		c := s.cfg
		c.TMDBKey = c.TMDBKeySource()
		if c.OMDbKey != "" {
			c.OMDbKey = "set"
		}
		return c, nil
	})
	add("transcode.json", func() (any, error) {
		if s.tc == nil {
			return map[string]any{"enabled": false}, nil
		}
		return map[string]any{"encoder": s.tc.Encoder(), "maxSessions": s.tc.Max(), "active": len(s.tc.Sessions())}, nil
	})
	add("tools.txt", func() (any, error) {
		var b strings.Builder
		for _, tool := range []string{s.cfg.FFmpeg, s.cfg.FFprobe} {
			fmt.Fprintf(&b, "$ %s -version\n%s\n", tool, firstLines(runVersion(ctx, tool), 3))
		}
		fmt.Fprintf(&b, "comskip available: %v\n", s.worker != nil && s.worker.CommercialsAvailable())
		return b.String(), nil
	})
	add("counts.json", func() (any, error) {
		counts, err := s.db.Counts(ctx)
		if err != nil {
			return nil, err
		}
		jobs, err := s.db.JobCounts(ctx)
		if err != nil {
			return nil, err
		}
		libs, err := s.db.Libraries(ctx)
		if err != nil {
			return nil, err
		}
		out := map[string]any{"counts": counts, "jobs": jobs, "libraries": libs}
		if s.tv != nil {
			out["livetv"] = s.tv.Summary()
		}
		return out, nil
	})
	add("migrations.txt", func() (any, error) {
		embedded, applied, err := s.db.MigrationReport(ctx)
		if err != nil {
			return nil, err
		}
		return "This build has:\n  " + strings.Join(embedded, "\n  ") + "\n\nThe database has had applied:\n  " + strings.Join(applied, "\n  ") + "\n", nil
	})
	add("database.txt", func() (any, error) {
		check, size, err := s.db.Health(ctx)
		if err != nil {
			return nil, err
		}
		out := fmt.Sprintf("quick_check: %s\nsize: %d bytes\n", check, size)
		if st, err := os.Stat(s.cfg.DataDir + string(os.PathSeparator) + "couchside.db-wal"); err == nil {
			out += fmt.Sprintf("wal: %d bytes\n", st.Size())
		}
		return out, nil
	})
	add("disk.json", func() (any, error) {
		dirs := map[string]string{"data": s.cfg.DataDir, "cache": s.cfg.CacheDir}
		if s.tv != nil {
			dirs["recordings"] = s.tv.RecordingsDir(ctx)
		}
		if libs, err := s.db.Libraries(ctx); err == nil {
			for _, l := range libs {
				dirs["library: "+l.Name] = l.Path
			}
		}
		out := map[string]any{}
		for name, dir := range dirs {
			if dir == "" {
				continue
			}
			if sp, err := diskfree.Of(dir); err != nil {
				out[name] = map[string]string{"path": dir, "error": err.Error()}
			} else {
				out[name] = map[string]any{"path": dir, "free": sp.Free, "total": sp.Total}
			}
		}
		return out, nil
	})
	add("jobs-failed.json", func() (any, error) {
		jobs, err := s.db.RecentJobs(ctx, 500)
		if err != nil {
			return nil, err
		}
		failed := jobs[:0]
		for _, j := range jobs {
			if j.Status == "failed" && len(failed) < 50 {
				failed = append(failed, j)
			}
		}
		return failed, nil
	})
	add("log.txt", func() (any, error) {
		if s.logs == nil {
			return "no log buffer\n", nil
		}
		entries, _, _ := s.logs.Since(0)
		var b strings.Builder
		for _, e := range entries {
			fmt.Fprintf(&b, "%s %-5s %s %s\n", time.UnixMilli(e.Time).Format("2006-01-02T15:04:05.000"), e.Level, e.Msg, e.Attrs)
		}
		return b.String(), nil
	})
	if err := zw.Close(); err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="couchside-diagnostics-%s.zip"`, time.Now().Format("20060102-150405")))
	w.Write(buf.Bytes())
}

// runVersion asks a program for its version, giving up after 5 seconds.
func runVersion(ctx context.Context, tool string) string {
	if tool == "" {
		return "(not set)"
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, tool, "-version").CombinedOutput()
	if err != nil {
		return "error: " + err.Error()
	}
	return string(out)
}

func firstLines(s string, n int) string {
	lines := strings.SplitN(strings.TrimSpace(s), "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}
