// Package auth holds Couchside's account security: password hashing, signed
// access tokens, refresh tokens and login throttling. It knows nothing about
// HTTP or the database.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters (OWASP's 19 MiB, 2 passes): strong, and cheap enough
// for a small home server. Hashes record their parameters, so raising these
// later only means old hashes get rehashed at their next sign-in.
type params struct {
	memory  uint32 // KiB
	time    uint32
	threads uint8
}

var current = params{memory: 19 * 1024, time: 2, threads: 1}

const (
	saltLen = 16
	keyLen  = 32

	MinPassword = 8
	// MaxPassword caps the input so hashing can't be used to burn CPU and memory.
	MaxPassword = 256
)

var (
	ErrPasswordShort = fmt.Errorf("a password needs at least %d characters", MinPassword)
	ErrPasswordLong  = fmt.Errorf("a password can't be longer than %d bytes", MaxPassword)
	errBadHash       = errors.New("unrecognised password hash")
)

// hashSlots limits concurrent hashing: each one holds 19 MiB, and a burst of
// logins shouldn't be able to run the pod out of memory.
var hashSlots = make(chan struct{}, 2)

// CheckPassword says whether a new password is acceptable.
func CheckPassword(pw string) error {
	if len(pw) > MaxPassword {
		return ErrPasswordLong
	}
	if utf8.RuneCountInString(strings.TrimSpace(pw)) < MinPassword {
		return ErrPasswordShort
	}
	return nil
}

// HashPassword returns a self-describing Argon2id hash:
// $argon2id$v=19$m=19456,t=2,p=1$<salt>$<key>
func HashPassword(pw string) (string, error) {
	if len(pw) > MaxPassword {
		return "", ErrPasswordLong
	}
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := derive(pw, salt, current)
	enc := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, current.memory, current.time, current.threads,
		enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

func derive(pw string, salt []byte, p params) []byte {
	hashSlots <- struct{}{}
	defer func() { <-hashSlots }()
	return argon2.IDKey([]byte(pw), salt, p.time, p.memory, p.threads, keyLen)
}

// VerifyPassword checks pw against a stored hash in constant time. rehash is
// true when the hash uses older parameters and should be replaced.
func VerifyPassword(hash, pw string) (ok, rehash bool) {
	if len(pw) > MaxPassword {
		return false, false
	}
	p, salt, key, err := parseHash(hash)
	if err != nil {
		// Still do the work, so a broken or empty hash takes as long as a real one.
		DummyVerify(pw)
		return false, false
	}
	got := derive(pw, salt, p)
	return subtle.ConstantTimeCompare(got, key) == 1, p != current
}

var dummySalt = make([]byte, saltLen)

// DummyVerify spends the same time as a real check. Use it when the account
// doesn't exist, so response times don't reveal which names are real.
func DummyVerify(pw string) {
	if len(pw) > MaxPassword {
		pw = pw[:MaxPassword]
	}
	derive(pw, dummySalt, current)
}

func parseHash(h string) (params, []byte, []byte, error) {
	var p params
	parts := strings.Split(h, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return p, nil, nil, errBadHash
	}
	var v int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &v); err != nil || v != argon2.Version {
		return p, nil, nil, errBadHash
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.memory, &p.time, &p.threads); err != nil {
		return p, nil, nil, errBadHash
	}
	// Refuse parameters far beyond ours: a tampered row shouldn't be able to
	// make one login allocate gigabytes.
	if p.memory == 0 || p.memory > 256*1024 || p.time == 0 || p.time > 10 || p.threads == 0 {
		return p, nil, nil, errBadHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) < 8 {
		return p, nil, nil, errBadHash
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(key) != keyLen {
		return p, nil, nil, errBadHash
	}
	return p, salt, key, nil
}
