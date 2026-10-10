package main

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/timothydodd/couchside/internal/backup"
	"github.com/timothydodd/couchside/internal/config"
)

// restore puts a backup back:
//
//	couchside restore /data/backups/couchside-daily-20261003-031500.zip
//
// The server must be stopped first (scale the Deployment to 0, or stop the
// container, and run this in a one-off container with the data volume). A
// name without a folder is looked for in $DATA/backups. The database being
// replaced is kept as couchside.db.before-restore.
func restore(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: couchside restore <backup .zip or .db>")
	}
	cfg := config.Load()
	if serverRunning(cfg.Addr) {
		return errors.New("Couchside is running on " + cfg.Addr + "; stop it first, or the restored database would be overwritten")
	}
	file := args[0]
	if !strings.ContainsAny(file, `/\`) {
		if p, err := backup.Path(cfg.DataDir, file); err == nil {
			file = p
		}
	}
	if err := backup.Restore(cfg.DataDir, file); err != nil {
		return err
	}
	fmt.Println("Restored", filepath.Base(file), "into", cfg.DataDir)
	fmt.Println("The database it replaced is couchside.db.before-restore. Start Couchside again.")
	return nil
}

// serverRunning reports whether a Couchside server answers on addr. Any
// HTTP answer counts: a server whose database is busy (503 from /healthz)
// or an older one without /livez (404) is still using the database.
func serverRunning(addr string) bool {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	c := http.Client{Timeout: 2 * time.Second}
	res, err := c.Get("http://" + net.JoinHostPort(host, port) + "/livez")
	if err != nil {
		return false
	}
	res.Body.Close()
	return true
}
