package main

import (
	"net/http"
	"net/http/httptest"
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
