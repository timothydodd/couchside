package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// AccessTTL is how long a web access token lasts; the browser renews it in
// the background. TVAccessTTL is longer because a TV's video player keeps the
// headers it started with, and a film or a live channel plays for hours.
// Either is refused at once when its session ends (every request checks the
// session), so the lifetime only bounds a token copied out of a live session.
const (
	AccessTTL   = 15 * time.Minute
	TVAccessTTL = 12 * time.Hour
)

var ErrBadToken = errors.New("invalid or expired token")

// Claims is what an access token carries. The session id is checked against
// live sessions on every request, so signing someone out takes effect at once.
type Claims struct {
	Session string `json:"sid"`
	Profile int64  `json:"pid"`
	Expires int64  `json:"exp"`
}

// Signer makes and checks HMAC-SHA256 signed access tokens:
// "v1." + base64url(claims JSON) + "." + base64url(mac).
type Signer struct{ key []byte }

func NewSigner(key []byte) *Signer { return &Signer{key: key} }

func (s *Signer) Sign(c Claims) string {
	body, _ := json.Marshal(c)
	payload := "v1." + base64.RawURLEncoding.EncodeToString(body)
	return payload + "." + base64.RawURLEncoding.EncodeToString(s.mac(payload))
}

func (s *Signer) mac(payload string) []byte {
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte(payload))
	return m.Sum(nil)
}

// Verify checks the signature and expiry.
func (s *Signer) Verify(tok string, now time.Time) (Claims, error) {
	var c Claims
	i := strings.LastIndexByte(tok, '.')
	if i < 0 || !strings.HasPrefix(tok, "v1.") || len(tok) > 1024 {
		return c, ErrBadToken
	}
	sig, err := base64.RawURLEncoding.DecodeString(tok[i+1:])
	if err != nil || !hmac.Equal(sig, s.mac(tok[:i])) {
		return c, ErrBadToken
	}
	body, err := base64.RawURLEncoding.DecodeString(tok[len("v1."):i])
	if err != nil || json.Unmarshal(body, &c) != nil || c.Session == "" || c.Expires <= now.Unix() {
		return Claims{}, ErrBadToken
	}
	return c, nil
}

// LoadKey reads the signing key from path, creating a random one (readable
// only by us) on first start. Deleting the file signs every access token out;
// refresh tokens keep working.
func LoadKey(path string) ([]byte, error) {
	if b, err := os.ReadFile(path); err == nil && len(b) >= 32 {
		return b, nil
	} else if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, key, 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

// NewToken returns 256 random bits, URL-safe: refresh tokens and session ids.
func NewToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand doesn't fail on supported platforms
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// HashToken is how refresh tokens are stored: a stolen database holds
// nothing that can be used to sign in.
func HashToken(t string) string {
	h := sha256.Sum256([]byte(t))
	return hex.EncodeToString(h[:])
}

// SetupCode is the one-time code printed to the log for creating the first
// admin: 12 characters with no look-alikes, in groups of four.
func SetupCode() string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	var sb strings.Builder
	for i, c := range b {
		if i > 0 && i%4 == 0 {
			sb.WriteByte('-')
		}
		sb.WriteByte(alphabet[int(c)%len(alphabet)])
	}
	return sb.String()
}

// SameCode compares setup codes in constant time, ignoring case and dashes.
func SameCode(a, b string) bool {
	norm := func(s string) []byte {
		return []byte(strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(s)))
	}
	return hmac.Equal(norm(a), norm(b))
}
