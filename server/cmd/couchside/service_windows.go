//go:build windows

package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows/svc"

	"github.com/timothydodd/couchside/internal/config"
)

// serviceName is the name the Windows installer registers the service under.
const serviceName = "Couchside"

func isService() bool {
	ok, err := svc.IsWindowsService()
	return err == nil && ok
}

// serviceMain runs the server as a Windows service. Its working folder is
// System32, so the data folder defaults to %ProgramData%\Couchside\data when
// the settings file doesn't name one, and the log goes to a file there.
func serviceMain() {
	config.Load() // reads %ProgramData%\Couchside\couchside.env into the environment
	if os.Getenv("COUCHSIDE_DATA_DIR") == "" {
		os.Setenv("COUCHSIDE_DATA_DIR", filepath.Join(os.Getenv("ProgramData"), "Couchside", "data"))
	}
	var w io.Writer = io.Discard
	if f, err := openLogFile(filepath.Join(os.Getenv("COUCHSIDE_DATA_DIR"), "logs", "couchside.log")); err == nil {
		w = f
	}
	setLogger(w)
	if err := svc.Run(serviceName, service{}); err != nil {
		slog.Error("windows service", "err", err)
		os.Exit(1)
	}
}

type service struct{}

func (service) Execute(_ []string, req <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- run(ctx) }()
	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case err := <-done:
			if err != nil {
				// A non-zero exit lets the service's recovery settings restart it.
				slog.Error("fatal", "err", err)
				return true, 1
			}
			return false, 0
		case c := <-req:
			switch c.Cmd {
			case svc.Interrogate:
				status <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending, WaitHint: 30000}
				cancel()
				select {
				case <-done:
				case <-time.After(25 * time.Second):
					slog.Warn("shutdown took too long; stopping anyway")
				}
				return false, 0
			}
		}
	}
}
