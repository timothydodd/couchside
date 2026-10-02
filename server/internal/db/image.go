package db

import (
	"encoding/json"
	"net/url"
	"strings"
)

// RemotePath is the server's image cache: images from guide data and
// metadata providers are fetched once through it (api/remote.go) and served
// from the cache after that, so clients never fetch them from the internet.
const RemotePath = "/api/artwork/remote?u="

// ImageURL is a remote image's address. It's stored as is, but goes out in
// JSON as a path to the server's image cache.
type ImageURL string

func (u ImageURL) MarshalJSON() ([]byte, error) {
	return json.Marshal(CachedImage(string(u)))
}

// CachedImage turns a remote image URL into its cache path ("" stays "").
func CachedImage(u string) string {
	if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
		return RemotePath + url.QueryEscape(u)
	}
	return u
}
