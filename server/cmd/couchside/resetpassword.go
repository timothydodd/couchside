package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"

	"github.com/timothydodd/couchside/internal/auth"
	"github.com/timothydodd/couchside/internal/config"
	"github.com/timothydodd/couchside/internal/db"
)

// resetPassword is the way back in when the admin password is lost:
//
//	kubectl exec -it deploy/couchside -- couchside reset-password [-admin] <name>
//
// It sets the profile's password (asked for on the terminal, or read from
// stdin), re-enables it, and signs it out everywhere. -admin also makes it an
// admin. A running server notices within 30 seconds.
func resetPassword(args []string) error {
	fs := flag.NewFlagSet("reset-password", flag.ContinueOnError)
	admin := fs.Bool("admin", false, "also make the profile an admin")
	if err := fs.Parse(args); err != nil {
		return err
	}
	name := strings.Join(fs.Args(), " ")
	if name == "" {
		return errors.New("usage: couchside reset-password [-admin] <profile name>")
	}
	cfg := config.Load()
	d, err := db.Open(filepath.Join(cfg.DataDir, "couchside.db"))
	if err != nil {
		return err
	}
	defer d.Close()
	ctx := context.Background()
	p, _, err := d.ProfileForLogin(ctx, name)
	if errors.Is(err, db.ErrNotFound) {
		return fmt.Errorf("there's no profile called %q", name)
	} else if err != nil {
		return err
	}

	pw, err := readPassword("New password for " + p.Name + ": ")
	if err != nil {
		return err
	}
	if term.IsTerminal(int(os.Stdin.Fd())) {
		again, err := readPassword("Again: ")
		if err != nil {
			return err
		}
		if again != pw {
			return errors.New("the passwords don't match")
		}
	}
	if err := auth.CheckPassword(pw); err != nil {
		return err
	}
	hash, err := auth.HashPassword(pw)
	if err != nil {
		return err
	}
	if err := d.SetPassword(ctx, p.ID, hash, false); err != nil {
		return err
	}
	// The way back in mustn't need an authenticator that may be lost too.
	if err := d.DisableTOTP(ctx, p.ID); err != nil {
		return err
	}
	role := p.Role
	if *admin {
		role = "admin"
	}
	if _, err := d.SetAccess(ctx, p.ID, role, p.CanRecord || role == "admin", false); err != nil {
		return err
	}
	if err := d.DeleteSessions(ctx, p.ID, ""); err != nil {
		return err
	}
	fmt.Printf("Password set for %s (%s); signed out everywhere.\n", p.Name, role)
	return nil
}

func readPassword(prompt string) (string, error) {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprint(os.Stderr, prompt)
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		return string(b), err
	}
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return "", errors.New("no password on stdin")
	}
	return strings.TrimRight(line, "\r\n"), nil
}
