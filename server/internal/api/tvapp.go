package api

import (
	"net/http"
	"strings"
)

// Packaged TV apps (LG webOS) run from file://, so every request they make
// is cross-origin with Origin "null". tvAppCORS lets those read answers.
// Nothing here opens the API wider than it already is: no credentials are
// allowed, so cookies never ride along and the web's sessions can't be used;
// a request still needs a bearer token past the open routes, the same ones
// the Roku and any LAN client can already call.
func tvAppCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		o := r.Header.Get("Origin")
		if !strings.HasPrefix(r.URL.Path, "/api/") || !tvAppOrigin(o) {
			next.ServeHTTP(w, r)
			return
		}
		h := w.Header()
		h.Set("Access-Control-Allow-Origin", o)
		h.Add("Vary", "Origin")
		h.Set("Access-Control-Expose-Headers", "Content-Length, Content-Range, Retry-After")
		if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
			h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE")
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Range")
			h.Set("Access-Control-Max-Age", "600")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// tvAppOrigin is an app loaded from the device itself: Chromium sends "null"
// for file:// pages.
func tvAppOrigin(o string) bool {
	return o == "null" || strings.HasPrefix(o, "file://")
}

// queryToken lets a media element authenticate: a <video> or <track> can't
// send an Authorization header, so a TV web app puts its access token in
// ?access_token= on the stream and subtitle URLs. It's only read on these
// routes and only when no header is sent, and request logs carry the path
// alone, so the token isn't written anywhere.
func queryToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if tok := q.Get("access_token"); tok != "" && r.Header.Get("Authorization") == "" {
			r = r.Clone(r.Context())
			r.Header.Set("Authorization", "Bearer "+tok)
			q.Del("access_token")
			r.URL.RawQuery = q.Encode()
		}
		next.ServeHTTP(w, r)
	})
}
