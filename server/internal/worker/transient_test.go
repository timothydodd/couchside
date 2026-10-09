package worker

import (
	"context"
	"errors"
	"fmt"
	"net"
	"syscall"
	"testing"
	"time"

	"github.com/timothydodd/couchside/internal/metadata"
	"github.com/timothydodd/couchside/internal/usererr"
)

func TestTransient(t *testing.T) {
	for _, c := range []struct {
		err  error
		want bool
	}{
		{&net.OpError{Op: "dial", Err: errors.New("no route")}, true},
		{fmt.Errorf("probe: %w", syscall.EIO), true},
		{fmt.Errorf("match: %w", metadata.ErrRateLimited), true},
		{context.DeadlineExceeded, true},
		{errProbeTimeout, true},
		{metadata.ErrBadKey, false},
		{usererr.New("this file is damaged"), false},
		{errors.New("ffmpeg: Invalid data found when processing input"), false},
	} {
		if got := transient(c.err); got != c.want {
			t.Errorf("transient(%v) = %v, want %v", c.err, got, c.want)
		}
	}
	if retryDelay(1) != time.Minute || retryDelay(3) != 4*time.Minute || retryDelay(20) != time.Hour {
		t.Errorf("delays: %v %v %v", retryDelay(1), retryDelay(3), retryDelay(20))
	}
}
