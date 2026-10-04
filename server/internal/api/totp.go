package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"log/slog"

	"github.com/timothydodd/couchside/internal/auth"
	"github.com/timothydodd/couchside/internal/db"
)

// Two-step sign-in (TOTP): after the password, a six-digit code from an
// authenticator app, or one of the recovery codes given at enrolment. It
// applies to signing in with a password; a profile picked without one, or
// signed in through a provider, isn't asked (the provider has its own).
// A TV can be signed in with a code from /link instead of typing either.

const recoveryCodes = 8

// secondStep checks a sign-in's code for a profile with two-step on: the
// authenticator's current code (each usable once), or a recovery code.
func (s *Server) secondStep(ctx context.Context, p db.Profile, code string) bool {
	code = strings.TrimSpace(code)
	if code == "" {
		return false
	}
	secret, on, err := s.db.TOTP(ctx, p.ID)
	if err != nil || !on {
		return false
	}
	if step, ok := auth.VerifyTOTP(secret, code, time.Now()); ok {
		fresh, err := s.db.UseTOTPStep(ctx, p.ID, step)
		return err == nil && fresh
	}
	used, err := s.db.UseRecoveryCode(ctx, p.ID, auth.HashToken(normalizeUserCode(code)))
	if used {
		slog.Warn("a recovery code was used to sign in", "profile", p.Name)
	}
	return err == nil && used
}

// totpStatus tells the account page where two-step sign-in stands.
func (s *Server) totpStatus(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r.Context())
	_, on, err := s.db.TOTP(r.Context(), u.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	left, err := s.db.RecoveryCodesLeft(r.Context(), u.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": on, "recoveryCodesLeft": left})
}

// totpSetup starts enrolment: a new secret for the user's authenticator app.
// Nothing changes at sign-in until totpEnable proves the app has it.
func (s *Server) totpSetup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p, err := s.db.Profile(ctx, currentUser(ctx).ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if !p.HasPassword {
		writeErr(w, badRequest("set a password first: the code is asked for after it"))
		return
	}
	if p.TwoStep {
		writeErr(w, badRequest("two-step sign-in is already on; turn it off to set it up again"))
		return
	}
	secret := auth.NewTOTPSecret()
	if err := s.db.StartTOTP(ctx, p.ID, secret); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"secret": secret, "uri": auth.TOTPURI(secret, p.Name)})
}

// totpEnable switches two-step sign-in on once a code from the app checks
// out, and answers with the recovery codes (shown this once).
func (s *Server) totpEnable(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := currentUser(ctx)
	var in struct {
		Code string `json:"code"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	secret, on, err := s.db.TOTP(ctx, u.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if on || secret == "" {
		writeErr(w, badRequest("start setting it up first"))
		return
	}
	step, ok := auth.VerifyTOTP(secret, in.Code, time.Now())
	if !ok {
		writeErr(w, badRequest("that code isn't right; check the app shows Couchside and its clock is correct"))
		return
	}
	codes := make([]string, recoveryCodes)
	hashes := make([]string, recoveryCodes)
	for i := range codes {
		codes[i] = auth.SetupCode()[:9]
		hashes[i] = auth.HashToken(codes[i])
	}
	if err := s.db.EnableTOTP(ctx, u.ID, hashes); err != nil {
		writeErr(w, err)
		return
	}
	_, _ = s.db.UseTOTPStep(ctx, u.ID, step)
	s.auth.sessions.forgetAll() // cached profiles carry twoStep
	slog.Info("two-step sign-in turned on", "profile", u.ID)
	writeJSON(w, http.StatusOK, map[string]any{"recoveryCodes": codes})
}

// totpDisable switches it off, given a current code or a recovery code.
func (s *Server) totpDisable(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := currentUser(ctx)
	var in struct {
		Code string `json:"code"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	p, err := s.db.Profile(ctx, u.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if !p.TwoStep {
		// Abandoning an enrolment that was never finished needs no code.
		if err := s.db.DisableTOTP(ctx, u.ID); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	ip := clientIP(r)
	done, ok := s.attempt(w, ip, strings.ToLower(p.Name))
	if !ok {
		return
	}
	good := s.secondStep(ctx, p, in.Code)
	done(!good)
	if !good {
		writeErr(w, badRequest("that code isn't right"))
		return
	}
	if err := s.db.DisableTOTP(ctx, u.ID); err != nil {
		writeErr(w, err)
		return
	}
	s.auth.sessions.forgetAll()
	slog.Info("two-step sign-in turned off", "profile", p.Name)
	w.WriteHeader(http.StatusNoContent)
}

// resetTOTP is an admin switching it off for someone who has lost their
// authenticator and their recovery codes.
func (s *Server) resetTOTP(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.db.DisableTOTP(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	s.auth.sessions.forgetAll()
	slog.Info("two-step sign-in reset by an admin", "profile", id, "by", currentUser(r.Context()).ID)
	w.WriteHeader(http.StatusNoContent)
}
