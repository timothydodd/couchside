package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"testing"
)

// restore must not overwrite the database under a running server, even one
// whose database is busy or one too old to have /livez.
func TestServerRunning(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusServiceUnavailable, http.StatusNotFound} {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
		addr := strings.TrimPrefix(ts.URL, "http://")
		if !serverRunning(addr) {
			t.Errorf("a server answering %d wasn't seen as running", status)
		}
		ts.Close()
		if serverRunning(addr) {
			t.Errorf("a stopped server (was %d) is still seen as running", status)
		}
	}
}

// A data folder made by an older version (0755) loses group and other access.
func TestTightenDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no POSIX modes")
	}
	d := t.TempDir()
	if err := os.Chmod(d, 0o755); err != nil {
		t.Skip("chmod unsupported here")
	}
	if st, _ := os.Stat(d); st.Mode().Perm() != 0o755 {
		t.Skip("this file system ignores chmod")
	}
	tightenDir(d)
	if st, _ := os.Stat(d); st.Mode().Perm() != 0o700 {
		t.Errorf("mode = %o, want 700", st.Mode().Perm())
	}
}
